package link

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/toblu96/taktwin/internal/plcio"
)

// Driver speaks one protocol. Drivers are part of the app and register themselves; a
// connection file can only name one.
type Driver interface {
	// Kind is the name connections use for it, such as "opcua".
	Kind() string
	// Open starts a session with the controller, with the connection's settings and
	// bindings. It may connect right away or in Resolve.
	Open(ctx context.Context, settings json.RawMessage, bindings []Binding) (Session, error)
}

// Session is one connection to a controller, until it fails or is closed.
type Session interface {
	// Endpoint is where the controller is, to show.
	Endpoint() string
	// Resolve connects if need be and checks every binding once: whether its item exists,
	// can be read or written as its signal needs, and holds a value it can convert. It
	// reports the problems of single bindings, which are then left out, and fails for
	// what makes the whole connection useless.
	Resolve(ctx context.Context) ([]Problem, error)
	// Run exchanges values until ctx ends or the connection fails.
	Run(ctx context.Context, x Exchange) error
	Close() error
}

// Problem is something wrong with one binding.
type Problem struct {
	Binding int
	Text    string
}

// Exchange is what a session exchanges values with: one value per binding, in the order
// of the bindings, as Binding.Outward and Binding.Inward make them.
type Exchange interface {
	// Inputs gives the station's value of each binding: 0 or 1 for a bit, the value on the
	// wire, such as volts, for an analogue input. Those of output bindings are 0.
	Inputs() []float64
	// SetOutputs takes the value the controller gives each output binding; the others are
	// ignored. It also tells the link that one exchange is done.
	SetOutputs(values []float64)
}

var (
	registryMu sync.Mutex
	registry   = map[string]Driver{}
)

// Register adds a driver. Drivers register themselves in an init function.
func Register(d Driver) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[d.Kind()] = d
}

// Lookup finds a driver by kind.
func Lookup(kind string) (Driver, error) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if d, ok := registry[kind]; ok {
		return d, nil
	}
	known := make([]string, 0, len(registry))
	for k := range registry {
		known = append(known, k)
	}
	sort.Strings(known)
	return nil, fmt.Errorf("there is no driver %q, only %s", kind, strings.Join(known, ", "))
}

// Program is what a link can ask of a PLC that runs inside this app.
type Program interface {
	Step() (plcio.StepInfo, bool)
	Reset()
	Close()
}

// Options configure a link.
type Options struct {
	// Name is shown as the PLC source.
	Name string
	// Connection is what to connect to.
	Connection Resolved
	// Program is set for a PLC running inside this app, which can then show its step and
	// be restarted along with the station.
	Program Program
}

const (
	cycle      = 10 * time.Millisecond
	retryAfter = time.Second
	// watchdog is how long an exchange may take before the link counts as lost.
	watchdog = 3 * time.Second
)

// Link is a plcio.Controller that exchanges a PLC's image with a controller through a
// driver. Its Scan never waits for the controller: the session runs on its own, so a slow
// or lost controller cannot stall the simulation. As on a real fieldbus, the station's
// outputs drop to off when the connection is lost.
type Link struct {
	opts   Options
	driver Driver
	stop   context.CancelFunc
	done   chan struct{}

	mu        sync.Mutex
	in        plcio.Image
	out       plcio.Image
	info      plcio.LinkInfo
	last      time.Time // the last exchange began
	connected bool
}

// Start begins connecting in the background. A driver that does not exist fails at once.
func Start(ctx context.Context, opts Options) (*Link, error) {
	d, err := Lookup(opts.Connection.Driver)
	if err != nil {
		return nil, err
	}
	ctx, stop := context.WithCancel(ctx)
	l := &Link{opts: opts, driver: d, stop: stop, done: make(chan struct{}), info: plcio.LinkInfo{State: plcio.LinkConnecting}}
	for _, s := range opts.Connection.Unbound {
		l.info.Unbound = append(l.info.Unbound, s.Label())
	}
	go l.run(ctx)
	return l, nil
}

// Name implements plcio.Controller.
func (l *Link) Name() string { return l.opts.Name }

// Cycle implements plcio.Controller.
func (l *Link) Cycle() time.Duration { return cycle }

// Scan implements plcio.Controller.
func (l *Link) Scan(in plcio.Image) plcio.Image {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.in = in
	if l.connected && !l.last.IsZero() && time.Since(l.last) > watchdog {
		// The session hangs: better off than stuck.
		return plcio.Image{}
	}
	return l.out
}

// Reset implements plcio.Controller.
func (l *Link) Reset() {
	if l.opts.Program != nil {
		l.opts.Program.Reset()
	}
}

// Link implements plcio.LinkReporter.
func (l *Link) Link() plcio.LinkInfo {
	l.mu.Lock()
	defer l.mu.Unlock()
	info := l.info
	info.Problems = slices.Clone(info.Problems)
	info.Unbound = slices.Clone(info.Unbound)
	return info
}

// Step implements plcio.StepReporter for a program this app runs. Another controller has
// nothing to report, which it says by step 0.
func (l *Link) Step() plcio.StepInfo {
	if l.opts.Program != nil {
		if step, ok := l.opts.Program.Step(); ok {
			return step
		}
	}
	return plcio.StepInfo{}
}

// Close implements plcio.Closer.
func (l *Link) Close() {
	l.stop()
	<-l.done
	if l.opts.Program != nil {
		l.opts.Program.Close()
	}
}

func (l *Link) set(state plcio.LinkState, detail string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.info.State, l.info.Detail = state, detail
	l.connected = state == plcio.LinkConnected
	if !l.connected {
		l.out, l.info.RoundTrip, l.last = plcio.Image{}, 0, time.Time{}
	}
}

func (l *Link) run(ctx context.Context) {
	defer close(l.done)
	for ctx.Err() == nil {
		if err := l.session(ctx); err != nil && ctx.Err() == nil {
			l.set(plcio.LinkFailed, err.Error())
		}
		select {
		case <-ctx.Done():
		case <-time.After(retryAfter):
		}
	}
}

// session opens a session and exchanges values until something goes wrong. A driver that
// panics fails the link, not the app.
func (l *Link) session(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the %s driver failed: %v", l.driver.Kind(), r)
		}
	}()
	bindings := l.opts.Connection.Bindings
	s, err := l.driver.Open(ctx, l.opts.Connection.Settings, bindings)
	if err != nil {
		return err
	}
	defer s.Close()
	l.mu.Lock()
	l.info.Endpoint = s.Endpoint()
	l.mu.Unlock()
	problems, err := s.Resolve(ctx)
	if err != nil {
		return err
	}
	bad := map[int]bool{}
	var texts, quiet []string
	for _, p := range problems {
		bad[p.Binding] = true
		switch {
		case p.Binding >= 0 && p.Binding < len(bindings) && bindings[p.Binding].Generated && bindings[p.Binding].Signal.Symbol == "":
			// A template's binding for a signal that nothing of the station uses, such as a
			// byte the controller has no port for, is left out without a word.
			quiet = append(quiet, bindings[p.Binding].Signal.Label())
		case p.Binding >= 0 && p.Binding < len(bindings):
			texts = append(texts, bindings[p.Binding].Signal.Label()+": "+p.Text)
		default:
			texts = append(texts, p.Text)
		}
	}
	l.mu.Lock()
	l.info.Problems, l.info.Variables = texts, len(bindings)-len(bad)
	l.info.Unbound = append(l.info.Unbound[:len(l.opts.Connection.Unbound):len(l.opts.Connection.Unbound)], quiet...)
	l.mu.Unlock()
	l.set(plcio.LinkConnected, "")
	err = s.Run(ctx, &exchange{l: l, bindings: bindings})
	if ctx.Err() != nil {
		return nil
	}
	if err == nil {
		err = fmt.Errorf("the connection ended")
	}
	return fmt.Errorf("connection lost: %w", err)
}

// exchange is the Exchange of one session.
type exchange struct {
	l        *Link
	bindings []Binding
	began    time.Time
}

func (x *exchange) Inputs() []float64 {
	l := x.l
	l.mu.Lock()
	in := l.in
	l.last = time.Now()
	l.mu.Unlock()
	x.began = time.Now()
	values := make([]float64, len(x.bindings))
	for i, b := range x.bindings {
		a := b.Signal.Address
		if !a.Input {
			continue
		}
		if s := b.Signal.Scaling; s != nil {
			values[i] = s.Read(in, a)
		} else if bit, ok := a.InImage(); ok && in.Get(bit) {
			values[i] = 1
		}
	}
	return values
}

func (x *exchange) SetOutputs(values []float64) {
	var out plcio.Image
	for i, b := range x.bindings {
		if b.Signal.Address.Input || i >= len(values) {
			continue
		}
		if s := b.Signal.Scaling; s != nil {
			s.Write(&out, b.Signal.Address, values[i])
		} else if bit, ok := b.Signal.Address.InImage(); ok {
			out.Set(bit, values[i] != 0)
		}
	}
	l := x.l
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out = out
	if x.began.IsZero() {
		return
	}
	// Smoothed, or the number would flicker too fast to read.
	took := time.Since(x.began).Seconds() * 1000
	if l.info.RoundTrip == 0 {
		l.info.RoundTrip = took
	} else {
		l.info.RoundTrip += (took - l.info.RoundTrip) * 0.05
	}
}
