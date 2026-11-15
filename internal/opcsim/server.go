// Package opcsim is a small OPC UA server inside the app: the soft PLC's. It runs a PLC
// program and offers its image the way a PLC's OPC UA server does, so the simulation reaches
// it as it reaches any other controller, and tools such as UaExpert can watch it. Each bit
// of the image is a node named by its address, as nsu=urn:taktwin:softplc;s=cabinet.%IX0.3,
// and each analogue input a node of volts, as …;s=cabinet.%IW8.
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

// Namespace is the URI of the namespace of the image's nodes.
const Namespace = "urn:taktwin:softplc"

// Space is how the server names the nodes of the image.
type Space struct {
	// PLC is the instance of the PLC module, which comes before each address.
	PLC string
	// Labels name addresses for people, such as "IB0Bit03 B1" for %IX0.3, as the nodes'
	// display names.
	Labels map[string]string
}

// Key is the name of the node of an address in the namespace.
func (sp Space) Key(a plcio.Address) string {
	if sp.PLC == "" {
		return a.String()
	}
	return sp.PLC + "." + a.String()
}

// Server serves one PLC program.
type Server struct {
	srv  *server.Server
	ns   *server.MapNamespace
	stop context.CancelFunc
	done chan struct{}
	port int

	mu    sync.Mutex
	ctl   plcio.Controller
	space Space
}

// Start begins serving on localhost and running the program. The port 0 is not supported,
// as the server has to know its own address for its endpoint list.
func Start(ctx context.Context, port int, ctl plcio.Controller, space Space) (*Server, error) {
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
	s := &Server{srv: srv, port: port, ctl: ctl, space: space, done: make(chan struct{})}
	// The library registers a map namespace as it makes it, and lockedSpace again after it,
	// at the next index. Only lockedSpace has the namespace's URI, so that a client that
	// looks the namespace up by its URI finds that one.
	s.ns = server.NewMapNamespace(srv, Namespace+":unlocked")
	labels := map[string]string{}
	for address, label := range space.Labels {
		if a, err := plcio.ParseAddress(address); err == nil {
			labels[space.Key(a)] = label
		}
	}
	srv.AddNamespace(lockedSpace{s.ns, labels})
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
		s.ns.SetValue(s.analogKey(channel), float32(0))
	}

	scan, stop := context.WithCancel(ctx)
	s.stop = stop
	go s.run(scan)
	return s, nil
}

// StartOnFreePort is Start on the first free port from DefaultPort, as another instance
// of the app or another station may already have a server.
func StartOnFreePort(ctx context.Context, ctl plcio.Controller, space Space) (*Server, error) {
	var err error
	for port := DefaultPort; port < DefaultPort+20; port++ {
		var s *Server
		if s, err = Start(ctx, port, ctl, space); err == nil {
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
// stopping the whole app. Registered after the plain namespace, it takes over its id. It
// also gives the nodes their display names.
type lockedSpace struct {
	*server.MapNamespace
	labels map[string]string // by key
}

// Name implements server.NameSpace: the namespace URI.
func (l lockedSpace) Name() string { return Namespace }

// Attribute implements server.NameSpace.
func (l lockedSpace) Attribute(n *ua.NodeID, a ua.AttributeID) *ua.DataValue {
	l.Mu.RLock()
	defer l.Mu.RUnlock()
	dv := l.MapNamespace.Attribute(n, a)
	if a == ua.AttributeIDDisplayName && dv.Status == ua.StatusOK {
		if label, ok := l.labels[n.StringID()]; ok {
			dv.Value = ua.MustVariant(&ua.LocalizedText{EncodingMask: ua.LocalizedTextText, Text: label})
		}
	}
	return dv
}

func (s *Server) analogKey(channel int) string {
	return s.space.Key(plcio.AnalogInputAddress(channel))
}

// Analog returns what the PLC reads on its analogue inputs, in volts.
func (s *Server) Analog() plcio.Analog {
	var in plcio.Analog
	for channel := range in {
		v, _ := s.ns.GetValue(s.analogKey(channel)).(float32)
		in[channel] = float64(v)
	}
	return in
}

func (s *Server) inputKey(a plcio.Addr) string  { return s.space.Key(plcio.BitAddress(true, a)) }
func (s *Server) outputKey(a plcio.Addr) string { return s.space.Key(plcio.BitAddress(false, a)) }

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
