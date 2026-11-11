// Package opcsim is a small OPC UA server inside the app: a stand-in for a soft PLC. It
// runs a PLC program and offers its variable list the same way CODESYS would, named as the
// PLC module says, so everything over OPC UA can be used without installing anything.
package opcsim

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/server"
	"github.com/gopcua/opcua/ua"

	"github.com/toblu96/taktwin/internal/plcio"
)

// DefaultPort is next to CODESYS's 4840, so both can run on one machine.
const DefaultPort = 4841

// Server serves one PLC program.
type Server struct {
	srv  *server.Server
	ns   *server.MapNamespace
	stop context.CancelFunc
	done chan struct{}
	port int

	mu     sync.Mutex
	ctl    plcio.Controller
	naming plcio.Naming
}

// Start begins serving on localhost and running the program. The port 0 is not supported,
// as the server has to know its own address for its endpoint list.
func Start(ctx context.Context, port int, ctl plcio.Controller, naming plcio.Naming) (*Server, error) {
	if err := naming.Check(); err != nil {
		return nil, err
	}
	srv := server.New(
		server.EndPoint("127.0.0.1", port),
		server.EnableSecurity("None", ua.MessageSecurityModeNone),
		server.EnableAuthMode(ua.UserTokenTypeAnonymous),
		server.ProductName("Taktwin simulated PLC"),
		server.ManufacturerName("Taktwin"),
	)
	if err := srv.Start(ctx); err != nil {
		return nil, fmt.Errorf("starting the simulated OPC UA server on port %d: %w", port, err)
	}
	s := &Server{srv: srv, port: port, ctl: ctl, naming: naming, done: make(chan struct{})}
	s.ns = server.NewMapNamespace(srv, "Taktwin")
	srv.AddNamespace(lockedSpace{s.ns})
	root, err := srv.Namespace(0)
	if err != nil {
		_ = srv.Close()
		return nil, err
	}
	root.Objects().AddRef(s.ns.Objects(), id.HasComponent, true)
	for b := range uint8(plcio.ImageBytes) {
		for bit := range uint8(8) {
			a := plcio.Addr{Byte: b, Bit: bit}
			s.ns.SetValue(s.inputKey(a), false)
			s.ns.SetValue(s.outputKey(a), false)
		}
	}
	// REAL in volts, the simplest way to declare them in a program.
	for channel := range plcio.AnalogInputs {
		if naming.Analog != "" {
			s.ns.SetValue(s.analogKey(channel), float32(0))
		}
	}

	scan, stop := context.WithCancel(ctx)
	s.stop = stop
	go s.run(scan)
	return s, nil
}

// StartOnFreePort is Start on the first free port from DefaultPort, as another instance
// of the app or another station may already have a server.
func StartOnFreePort(ctx context.Context, ctl plcio.Controller, naming plcio.Naming) (*Server, error) {
	var err error
	for port := DefaultPort; port < DefaultPort+20; port++ {
		var s *Server
		if s, err = Start(ctx, port, ctl, naming); err == nil {
			return s, nil
		}
	}
	return nil, err
}

// Endpoint is the address to connect to.
func (s *Server) Endpoint() string { return fmt.Sprintf("opc.tcp://127.0.0.1:%d", s.port) }

func (s *Server) run(ctx context.Context) {
	defer close(s.done)
	s.mu.Lock()
	cycle := s.ctl.Cycle()
	s.mu.Unlock()
	ticker := time.NewTicker(cycle)
	defer ticker.Stop()
	var last plcio.Image
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		var in plcio.Image
		for b := range uint8(plcio.ImageBytes) {
			for bit := range uint8(8) {
				a := plcio.Addr{Byte: b, Bit: bit}
				if on, _ := s.ns.GetValue(s.inputKey(a)).(bool); on {
					in.Set(a, true)
				}
			}
		}
		s.mu.Lock()
		out := s.ctl.Scan(in)
		s.mu.Unlock()
		for b := range uint8(plcio.ImageBytes) {
			for bit := range uint8(8) {
				a := plcio.Addr{Byte: b, Bit: bit}
				if out.Get(a) != last.Get(a) {
					s.ns.SetValue(s.outputKey(a), out.Get(a))
				}
			}
		}
		last = out
	}
}

// lockedSpace reads values under the namespace's lock. The library's own Attribute does
// not, and the program updates values while clients read them, which Go can answer by
// stopping the whole app. Registered after the plain namespace, it takes over its id.
type lockedSpace struct{ *server.MapNamespace }

// Attribute implements server.NameSpace.
func (l lockedSpace) Attribute(n *ua.NodeID, a ua.AttributeID) *ua.DataValue {
	l.Mu.RLock()
	defer l.Mu.RUnlock()
	return l.MapNamespace.Attribute(n, a)
}

func (s *Server) analogKey(channel int) string {
	return s.naming.List + "." + s.naming.AnalogInput(channel)
}

// Analog returns what the PLC reads on its analogue inputs, in volts.
func (s *Server) Analog() plcio.Analog {
	var in plcio.Analog
	for channel := range in {
		if s.naming.Analog == "" {
			continue
		}
		v, _ := s.ns.GetValue(s.analogKey(channel)).(float32)
		in[channel] = float64(v)
	}
	return in
}

func (s *Server) inputKey(a plcio.Addr) string  { return s.naming.List + "." + s.naming.Input(a) }
func (s *Server) outputKey(a plcio.Addr) string { return s.naming.List + "." + s.naming.Output(a) }

// Step reports what the program is doing, if it can.
func (s *Server) Step() (plcio.StepInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.ctl.(plcio.StepReporter); ok {
		return r.Step(), true
	}
	return plcio.StepInfo{}, false
}

// Reset restarts the program, as a power cycle of the PLC would.
func (s *Server) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctl.Reset()
}

// Close stops the program and the server.
func (s *Server) Close() {
	s.stop()
	<-s.done
	_ = s.srv.Close()
}
