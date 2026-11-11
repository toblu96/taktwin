// Package opclink connects a station to a PLC over OPC UA. The station is the client and
// the PLC the server, which is how CODESYS works: it exposes the variables of its symbol
// configuration. All it needs from the PLC is the Bits list, found by name.
package opclink

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"

	"github.com/toblu96/taktwin/internal/plcio"
)

const (
	cycle      = 10 * time.Millisecond
	timeout    = 3 * time.Second
	retryAfter = time.Second
	maxBrowse  = 5000
	maxDepth   = 10
)

// Options configures a link.
type Options struct {
	Endpoint string
	// Name is shown to the user as the PLC source.
	Name string
	// Naming is how the PLC's program names its variables, and the list they are in.
	Naming plcio.Naming
	Map    plcio.AddressMap
	// Analog names the analogue inputs the station uses, to say so if the PLC lacks them.
	Analog []string
	// Bytes is how many bytes of the image the station uses, to say so if the PLC lacks
	// variables of them; all of them without it.
	Bytes int
	// Program is set for a PLC running inside this app, which can then show its step
	// and be restarted along with the station.
	Program Program
}

// Program is what a link can ask of a PLC it knows more about.
type Program interface {
	Step() (plcio.StepInfo, bool)
	Reset()
	Close()
}

// Link is a plcio.Controller. Its Scan never waits for the network: a separate loop
// keeps exchanging the image, so a slow or lost PLC cannot stall the simulation. As on a
// real fieldbus, the station's outputs drop to off when the connection is lost.
type Link struct {
	opts Options
	stop context.CancelFunc
	done chan struct{}

	mu   sync.Mutex
	in   plcio.Image
	out  plcio.Image
	ai   plcio.Analog
	info plcio.LinkInfo
}

// SetAnalog implements plcio.AnalogReceiver.
func (l *Link) SetAnalog(in plcio.Analog) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ai = in
}

// Start begins connecting in the background.
func Start(ctx context.Context, opts Options) *Link {

	ctx, stop := context.WithCancel(ctx)
	l := &Link{opts: opts, stop: stop, done: make(chan struct{}), info: plcio.LinkInfo{State: plcio.LinkConnecting, Endpoint: opts.Endpoint}}
	go l.run(ctx)
	return l
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
	return l.info
}

// Step implements plcio.StepReporter for a program this app runs. An external PLC has
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

func (l *Link) set(state plcio.LinkState, detail string, variables int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.info.State, l.info.Detail, l.info.Variables = state, detail, variables
	if state != plcio.LinkConnected {
		l.out, l.info.RoundTrip = plcio.Image{}, 0
	}
}

func (l *Link) run(ctx context.Context) {
	defer close(l.done)
	for ctx.Err() == nil {
		if err := l.session(ctx); err != nil && ctx.Err() == nil {
			l.set(plcio.LinkFailed, err.Error(), 0)
		}
		select {
		case <-ctx.Done():
		case <-time.After(retryAfter):
		}
	}
}

// session connects and exchanges the image until something goes wrong.
func (l *Link) session(ctx context.Context) error {
	c, err := connect(ctx, l.opts.Endpoint)
	if err != nil {
		return err
	}
	defer func() {
		closing, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = c.Close(closing)
	}()
	nodes, analogNodes, err := findList(ctx, c, l.opts.Naming)
	if err != nil {
		return err
	}
	analog, unusable, err := analogInputs(ctx, c, analogNodes, l.opts.Naming)
	if err != nil {
		return err
	}
	l.set(plcio.LinkConnected, missing(l.opts.Naming, nodes, analogNodes, unusable, l.opts.Analog, l.opts.Bytes), len(nodes))

	written := map[string]bool{}
	var inputs, outputs []string
	for name := range nodes {
		if _, input, _ := l.opts.Naming.Parse(name); input {
			inputs = append(inputs, name)
		} else {
			outputs = append(outputs, name)
		}
	}
	slices.Sort(inputs)
	slices.Sort(outputs)
	read := &ua.ReadRequest{TimestampsToReturn: ua.TimestampsToReturnNeither}
	for _, name := range outputs {
		read.NodesToRead = append(read.NodesToRead, &ua.ReadValueID{NodeID: nodes[name], AttributeID: ua.AttributeIDValue})
	}

	ticker := time.NewTicker(cycle)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		began := time.Now()
		l.mu.Lock()
		local, volts := l.in, l.ai
		l.mu.Unlock()

		call, cancel := context.WithTimeout(ctx, timeout)
		err := writeInputs(call, c, nodes, inputs, l.opts.Map.RemoteInputs(l.opts.Naming, local), written)
		if err == nil {
			err = writeAnalog(call, c, analog, volts)
		}
		var remote map[string]bool
		if err == nil {
			remote, err = readOutputs(call, c, read, outputs)
		}
		cancel()
		if err != nil {
			return fmt.Errorf("connection lost: %w", err)
		}
		out := l.opts.Map.LocalOutputs(l.opts.Naming, remote)
		took := time.Since(began).Seconds() * 1000
		l.mu.Lock()
		l.out = out
		// Smoothed, or the number would flicker too fast to read.
		if l.info.RoundTrip == 0 {
			l.info.RoundTrip = took
		} else {
			l.info.RoundTrip += (took - l.info.RoundTrip) * 0.05
		}
		l.mu.Unlock()
	}
}

// connect opens an unsecured, anonymous session, the way a training setup on one machine
// is usually configured. A server that insists on more says so in the error.
func connect(ctx context.Context, endpoint string) (*opcua.Client, error) {
	call, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	endpoints, err := opcua.GetEndpoints(call, endpoint, opcua.DialTimeout(timeout), opcua.RequestTimeout(timeout))
	if err != nil {
		return nil, fmt.Errorf("no OPC UA server at %s: %w", endpoint, err)
	}
	ep, err := opcua.SelectEndpoint(endpoints, "None", ua.MessageSecurityModeNone)
	if err != nil {
		return nil, fmt.Errorf("the server at %s does not allow an unsecured connection: enable it in the server's security settings", endpoint)
	}
	c, err := opcua.NewClient(endpoint,
		opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeAnonymous),
		opcua.AuthAnonymous(),
		opcua.AutoReconnect(false),
		opcua.DialTimeout(timeout),
		opcua.RequestTimeout(timeout),
	)
	if err != nil {
		return nil, err
	}
	if err := c.Connect(call); err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", endpoint, err)
	}
	return c, nil
}

// findList walks the server's address space for the list's variables, as where CODESYS
// puts them depends on the device name and version. It accepts a variable under a node
// named like the list, or named with the list in front, as in Bits.IB0Bit00.
func findList(ctx context.Context, c *opcua.Client, naming plcio.Naming) (map[string]*ua.NodeID, map[string]*ua.NodeID, error) {
	list := naming.List
	type item struct {
		id     *ua.NodeID
		name   string
		parent string
		depth  int
	}
	found, analog := map[string]*ua.NodeID{}, map[string]*ua.NodeID{}
	seen := map[string]bool{}
	queue := []item{{id: ua.NewNumericNodeID(0, id.ObjectsFolder)}}
	for len(queue) > 0 && len(seen) < maxBrowse {
		at := queue[0]
		queue = queue[1:]
		call, cancel := context.WithTimeout(ctx, timeout)
		refs, err := c.Node(at.id).References(call, id.HierarchicalReferences, ua.BrowseDirectionForward, ua.NodeClassAll, true)
		cancel()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, nil, err
			}
			continue // some nodes cannot be browsed, which does not matter here
		}
		for _, ref := range refs {
			key := ref.NodeID.String()
			if seen[key] {
				continue
			}
			seen[key] = true
			name := ""
			if ref.BrowseName != nil {
				name = ref.BrowseName.Name
			}
			// The server's own objects are many and never hold the list.
			if ref.NodeID.NodeID.Namespace() == 0 && name != "" {
				continue
			}
			short, qualified := strings.CutPrefix(name, list+".")
			if _, _, err := naming.Parse(short); err == nil && (qualified || at.name == list) {
				found[short] = ref.NodeID.NodeID
				continue
			}
			if _, ok := naming.ParseAnalog(short); ok && (qualified || at.name == list) {
				analog[short] = ref.NodeID.NodeID
				continue
			}
			if at.depth < maxDepth {
				queue = append(queue, item{id: ref.NodeID.NodeID, name: name, parent: at.name, depth: at.depth + 1})
			}
		}
	}
	if len(found) == 0 {
		return nil, nil, fmt.Errorf("connected, but the server has no %q variables: add the list to the symbol configuration with OPC UA enabled, and download the program", list)
	}
	return found, analog, nil
}

// missing says which variables of the bytes the station uses the server does not have, and
// which of the station's analogue inputs it cannot take.
func missing(naming plcio.Naming, nodes, analog map[string]*ua.NodeID, unusable, wanted []string, bytes int) string {
	var notes []string
	if bytes <= 0 || bytes > plcio.ImageBytes {
		bytes = plcio.ImageBytes
	}
	found := 0
	for b := range uint8(bytes) {
		for bit := range uint8(8) {
			a := plcio.Addr{Byte: b, Bit: bit}
			for _, name := range []string{naming.Input(a), naming.Output(a)} {
				if _, ok := nodes[name]; ok {
					found++
				}
			}
		}
	}
	if want := bytes * 8 * 2; found != want {
		notes = append(notes, fmt.Sprintf("%d of %d variables found", found, want))
	}
	for _, name := range wanted {
		if _, ok := analog[name]; !ok {
			notes = append(notes, name+" is not on the server, so the PLC does not get its value")
		} else if slices.Contains(unusable, name) {
			notes = append(notes, name+" has a type that cannot hold a value, use REAL or INT")
		}
	}
	return strings.Join(notes, ". ")
}

// analogInput is an analogue input on the server, in the type the PLC program declared it.
type analogInput struct {
	channel int
	name    string
	node    *ua.NodeID
	kind    ua.TypeID
	sent    float64
	written bool
}

// analogInputs reads what type each analogue input has. It returns those that cannot hold
// a value separately, as they are no reason to lose the rest of the link.
func analogInputs(ctx context.Context, c *opcua.Client, nodes map[string]*ua.NodeID, naming plcio.Naming) ([]*analogInput, []string, error) {
	var inputs []*analogInput
	var unusable []string
	for name, node := range nodes {
		channel, _ := naming.ParseAnalog(name)
		call, cancel := context.WithTimeout(ctx, timeout)
		res, err := c.Read(call, &ua.ReadRequest{
			TimestampsToReturn: ua.TimestampsToReturnNeither,
			NodesToRead:        []*ua.ReadValueID{{NodeID: node, AttributeID: ua.AttributeIDValue}},
		})
		cancel()
		if err != nil {
			return nil, nil, err
		}
		kind := ua.TypeIDFloat
		if len(res.Results) == 1 && res.Results[0].Value != nil {
			kind = res.Results[0].Value.Type()
		}
		if _, err := analogVariant(0, kind); err != nil {
			unusable = append(unusable, name)
			continue
		}
		inputs = append(inputs, &analogInput{channel: channel, name: name, node: node, kind: kind})
	}
	return inputs, unusable, nil
}

// analogVariant is a value in the type of the variable: volts for a floating point number,
// millivolts for an integer.
func analogVariant(volts float64, kind ua.TypeID) (*ua.Variant, error) {
	mv := math.Round(volts * 1000)
	switch kind {
	case ua.TypeIDFloat:
		return ua.NewVariant(float32(volts))
	case ua.TypeIDDouble:
		return ua.NewVariant(volts)
	case ua.TypeIDInt16:
		return ua.NewVariant(int16(mv))
	case ua.TypeIDUint16:
		return ua.NewVariant(uint16(mv))
	case ua.TypeIDInt32:
		return ua.NewVariant(int32(mv))
	case ua.TypeIDUint32:
		return ua.NewVariant(uint32(mv))
	}
	return nil, fmt.Errorf("type %v cannot hold an analogue value", kind)
}

// writeAnalog sends the analogue inputs that changed by more than a millivolt.
func writeAnalog(ctx context.Context, c *opcua.Client, inputs []*analogInput, volts plcio.Analog) error {
	req := &ua.WriteRequest{}
	var changed []*analogInput
	for _, in := range inputs {
		value := volts[in.channel]
		if in.written && math.Abs(in.sent-value) < 0.001 {
			continue
		}
		v, err := analogVariant(value, in.kind)
		if err != nil {
			return err
		}
		req.NodesToWrite = append(req.NodesToWrite, &ua.WriteValue{
			NodeID: in.node, AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{EncodingMask: ua.DataValueValue, Value: v},
		})
		changed = append(changed, in)
	}
	if len(changed) == 0 {
		return nil
	}
	res, err := c.Write(ctx, req)
	if err != nil {
		return err
	}
	for i, code := range res.Results {
		if code != ua.StatusOK {
			return fmt.Errorf("the server refused writing %s: %v", changed[i].name, code)
		}
		changed[i].sent, changed[i].written = volts[changed[i].channel], true
	}
	return nil
}

func writeInputs(ctx context.Context, c *opcua.Client, nodes map[string]*ua.NodeID, inputs []string, values, written map[string]bool) error {
	req := &ua.WriteRequest{}
	var changed []string
	for _, name := range inputs {
		if old, ok := written[name]; ok && old == values[name] {
			continue
		}
		v, err := ua.NewVariant(values[name])
		if err != nil {
			return err
		}
		req.NodesToWrite = append(req.NodesToWrite, &ua.WriteValue{
			NodeID: nodes[name], AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{EncodingMask: ua.DataValueValue, Value: v},
		})
		changed = append(changed, name)
	}
	if len(changed) == 0 {
		return nil
	}
	res, err := c.Write(ctx, req)
	if err != nil {
		return err
	}
	for i, code := range res.Results {
		if code != ua.StatusOK {
			return fmt.Errorf("the server refused writing %s: %v", changed[i], code)
		}
		written[changed[i]] = values[changed[i]]
	}
	return nil
}

func readOutputs(ctx context.Context, c *opcua.Client, read *ua.ReadRequest, outputs []string) (map[string]bool, error) {
	remote := make(map[string]bool, len(outputs))
	if len(outputs) == 0 {
		return remote, nil
	}
	res, err := c.Read(ctx, read)
	if err != nil {
		return nil, err
	}
	for i, r := range res.Results {
		if r.Status != ua.StatusOK {
			return nil, fmt.Errorf("the server refused reading %s: %v", outputs[i], r.Status)
		}
		if r.Value != nil {
			remote[outputs[i]], _ = r.Value.Value().(bool)
		}
	}
	return remote, nil
}
