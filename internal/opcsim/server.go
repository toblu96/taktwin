// Package opcsim is a small OPC UA server inside the app: the soft PLC's. It runs a PLC
// program and offers its image the way a PLC's OPC UA server does, so the simulation reaches
// it as it reaches any other controller, and any OPC UA client can watch it. Each signal of
// the image is a node named by its address, as nsu=urn:taktwin:softplc;s=cabinet.%IX0.3: a
// Boolean for a bit, an Int16 for a word such as an analogue channel's INT at %IW8, and a
// Float for a double word, an analogue channel's REAL.
package opcsim

import (
	"context"
	"fmt"
	"math"
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
	// Addresses are those the server has a node for. Without them, it has those of the image
	// a PLC module has without saying: each bit of 8 bytes of inputs and of outputs, and the 4
	// words of each direction from byte 8 on.
	Addresses []plcio.Address
}

// addresses are those the server has a node for.
func (sp Space) addresses() []plcio.Address {
	if len(sp.Addresses) > 0 {
		return sp.Addresses
	}
	var out []plcio.Address
	for _, input := range []bool{true, false} {
		for b := range uint8(8) {
			for bit := range uint8(8) {
				out = append(out, plcio.BitAddress(input, plcio.Addr{Byte: b, Bit: bit}))
			}
		}
		for ch := range 4 {
			out = append(out, plcio.WordAddress(input, 8+2*ch))
		}
	}
	return out
}

// value is the value of a node for a raw value of the image.
func value(a plcio.Address, raw uint32) any {
	switch a.Size {
	case plcio.SizeByte:
		return uint8(raw)
	case plcio.SizeWord:
		return int16(uint16(raw))
	case plcio.SizeDouble:
		return math.Float32frombits(raw)
	}
	return raw != 0
}

// raw is the raw value of the image for the value of a node.
func raw(v any) uint32 {
	switch t := v.(type) {
	case bool:
		if t {
			return 1
		}
	case uint8:
		return uint32(t)
	case int16:
		return uint32(uint16(t))
	case float32:
		return math.Float32bits(t)
	}
	return 0
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
	for _, a := range space.addresses() {
		s.ns.SetValue(space.Key(a), value(a, 0))
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
	addresses := s.space.addresses()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		in := s.Inputs()
		s.mu.Lock()
		out := s.ctl.Scan(in)
		s.mu.Unlock()
		for _, a := range addresses {
			if !a.Input && out.Raw(a) != last.Raw(a) {
				s.ns.SetValue(s.space.Key(a), value(a, out.Raw(a)))
			}
		}
		last = out
	}
}

// Inputs is the input image as the server's nodes have it, which the program reads.
func (s *Server) Inputs() plcio.Image {
	var in plcio.Image
	for _, a := range s.space.addresses() {
		if a.Input {
			in.SetRaw(a, raw(s.ns.GetValue(s.space.Key(a))))
		}
	}
	return in
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
	if a == ua.AttributeIDDataType && dv.Status == ua.StatusOK {
		// The library knows the types of only some values.
		switch l.Data[n.StringID()].(type) {
		case int16:
			dv.Value = ua.MustVariant(ua.NewNumericNodeID(0, id.Int16))
		case uint8:
			dv.Value = ua.MustVariant(ua.NewNumericNodeID(0, id.Byte))
		}
	}
	if a == ua.AttributeIDDisplayName && dv.Status == ua.StatusOK {
		if label, ok := l.labels[n.StringID()]; ok {
			dv.Value = ua.MustVariant(&ua.LocalizedText{EncodingMask: ua.LocalizedTextText, Text: label})
		}
	}
	return dv
}

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
