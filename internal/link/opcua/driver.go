// Package opcua is the link driver for OPC UA: a plain client for any server. It reaches a
// value by its NodeId, written with the namespace URI (nsu=…) so that it holds whatever
// index the server gives the namespace, or by a browse path from a node. It checks every
// node once, then writes the station's inputs when they change and reads its outputs on
// every cycle.
package opcua

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"

	"github.com/toblu96/taktwin/internal/link"
)

const timeout = 3 * time.Second

func init() { link.Register(Driver{}) }

// Driver is the OPC UA driver.
type Driver struct{}

// Kind implements link.Driver.
func (Driver) Kind() string { return "opcua" }

// Settings are the driver's part of a connection.
type Settings struct {
	// Endpoint is the server, such as opc.tcp://127.0.0.1:4840.
	Endpoint string `json:"endpoint"`
	Security struct {
		// Policy is the security policy, such as None.
		Policy string `json:"policy"`
		// Mode is the message security mode: None, Sign or SignAndEncrypt.
		Mode string `json:"mode"`
	} `json:"security"`
	Auth struct {
		// Type is anonymous.
		Type string `json:"type"`
	} `json:"auth"`
	Outputs struct {
		// Exchange is how outputs are read: poll.
		Exchange string `json:"exchange"`
		// Interval is how often, in milliseconds.
		Interval int `json:"interval"`
	} `json:"outputs"`
}

// Item is what a binding is exchanged with: a node by NodeId, or by a browse path.
type Item struct {
	// NodeID is the node, as nsu=<namespace URI>;s=<name> or ns=<index>;s=<name>, or with
	// i=, g= or b= for other kinds of identifier.
	NodeID string `json:"nodeId,omitempty"`
	// Path is a browse path, such as /2:Projects/2:B1/2:state: browse names, each with the
	// index of its namespace, followed from From.
	Path string `json:"path,omitempty"`
	// From is the node a path starts from; the Objects folder without it.
	From string `json:"from,omitempty"`
}

func parseSettings(raw json.RawMessage) (Settings, error) {
	var s Settings
	if len(raw) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&s); err != nil {
			return s, fmt.Errorf("OPC UA settings: %w", err)
		}
	}
	if !strings.HasPrefix(s.Endpoint, "opc.tcp://") {
		return s, fmt.Errorf("%q is not an OPC UA address: it starts with opc.tcp://", s.Endpoint)
	}
	if p := s.Security.Policy; p != "" && p != "None" {
		return s, fmt.Errorf("the security policy %s is not supported yet: use None", p)
	}
	if m := s.Security.Mode; m != "" && m != "None" {
		return s, fmt.Errorf("the security mode %s is not supported yet: use None", m)
	}
	if a := s.Auth.Type; a != "" && a != "anonymous" {
		return s, fmt.Errorf("logging in as %s is not supported yet: use anonymous", a)
	}
	if e := s.Outputs.Exchange; e != "" && e != "poll" {
		return s, fmt.Errorf("outputs can only be polled so far, not exchanged by %s", e)
	}
	switch {
	case s.Outputs.Interval == 0:
		s.Outputs.Interval = 10
	case s.Outputs.Interval < 5 || s.Outputs.Interval > 1000:
		return s, fmt.Errorf("an interval of %d ms is outside 5 to 1000", s.Outputs.Interval)
	}
	return s, nil
}

// Open implements link.Driver.
func (Driver) Open(_ context.Context, settings json.RawMessage, bindings []link.Binding) (link.Session, error) {
	s, err := parseSettings(settings)
	if err != nil {
		return nil, err
	}
	sess := &session{settings: s, bindings: bindings, nodes: make([]*node, len(bindings))}
	return sess, nil
}

// node is a resolved item.
type node struct {
	id      *ua.NodeID
	typ     ua.TypeID
	kind    link.Kind
	sent    float64
	written bool
}

type session struct {
	settings Settings
	bindings []link.Binding
	c        *opcua.Client
	// nodes holds the node of each binding that can be exchanged, nil for the others.
	nodes []*node
}

func (s *session) Endpoint() string { return s.settings.Endpoint }

func (s *session) Close() error {
	if s.c == nil {
		return nil
	}
	closing, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return s.c.Close(closing)
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

// Resolve implements link.Session.
func (s *session) Resolve(ctx context.Context) ([]link.Problem, error) {
	c, err := connect(ctx, s.settings.Endpoint)
	if err != nil {
		return nil, err
	}
	s.c = c
	call, cancel := context.WithTimeout(ctx, timeout)
	namespaces, err := c.NamespaceArray(call)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("reading the server's namespaces: %w", err)
	}
	var problems []link.Problem
	report := func(i int, format string, args ...any) {
		problems = append(problems, link.Problem{Binding: i, Text: fmt.Sprintf(format, args...)})
		s.nodes[i] = nil
	}
	// Each item to a node.
	for i, b := range s.bindings {
		var item Item
		if err := json.Unmarshal(b.Item, &item); err != nil {
			report(i, "the item is not an OPC UA item: %v", err)
			continue
		}
		id, err := s.nodeID(ctx, item, namespaces)
		if err != nil {
			report(i, "%v", err)
			continue
		}
		s.nodes[i] = &node{id: id}
	}
	// What each node holds and allows, in one request.
	req := &ua.ReadRequest{TimestampsToReturn: ua.TimestampsToReturnNeither}
	var asked []int
	for i, n := range s.nodes {
		if n == nil {
			continue
		}
		for _, a := range []ua.AttributeID{ua.AttributeIDDataType, ua.AttributeIDAccessLevel, ua.AttributeIDUserAccessLevel} {
			req.NodesToRead = append(req.NodesToRead, &ua.ReadValueID{NodeID: n.id, AttributeID: a})
		}
		asked = append(asked, i)
	}
	if len(asked) > 0 {
		call, cancel := context.WithTimeout(ctx, timeout)
		res, err := c.Read(call, req)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("reading what the nodes hold: %w", err)
		}
		if len(res.Results) != 3*len(asked) {
			return nil, errors.New("the server answered for fewer nodes than asked")
		}
		for k, i := range asked {
			n, b := s.nodes[i], s.bindings[i]
			dataType, access, userAccess := res.Results[3*k], res.Results[3*k+1], res.Results[3*k+2]
			if dataType.Status != ua.StatusOK {
				report(i, "%s: %v", n.id, statusText(dataType.Status))
				continue
			}
			typeID, _ := dataType.Value.Value().(*ua.NodeID)
			typ, kind, ok := kindOf(typeID)
			if !ok {
				report(i, "%s holds %v, which cannot carry a signal", n.id, typeID)
				continue
			}
			n.typ, n.kind = typ, kind
			level := accessLevel(access)
			if user, ok := userAccess.Value.Value().(byte); ok && userAccess.Status == ua.StatusOK {
				level &= user
			}
			if b.Signal.Address.Input && level&byte(ua.AccessLevelTypeCurrentWrite) == 0 {
				report(i, "%s cannot be written", n.id)
				continue
			}
			if !b.Signal.Address.Input && level&byte(ua.AccessLevelTypeCurrentRead) == 0 {
				report(i, "%s cannot be read", n.id)
				continue
			}
		}
	}
	return problems, nil
}

func accessLevel(dv *ua.DataValue) byte {
	if dv.Status != ua.StatusOK || dv.Value == nil {
		// A server that does not say is taken at its word when reading and writing.
		return byte(ua.AccessLevelTypeCurrentRead | ua.AccessLevelTypeCurrentWrite)
	}
	level, _ := dv.Value.Value().(byte)
	return level
}

func statusText(code ua.StatusCode) string {
	if code == ua.StatusBadNodeIDUnknown {
		return "no such node"
	}
	return code.Error()
}

// nodeID finds the node of an item.
func (s *session) nodeID(ctx context.Context, item Item, namespaces []string) (*ua.NodeID, error) {
	switch {
	case item.NodeID != "" && item.Path != "":
		return nil, errors.New("an item has a nodeId or a path, not both")
	case item.NodeID != "":
		return parseNodeID(item.NodeID, namespaces)
	case item.Path != "":
		from := ua.NewNumericNodeID(0, id.ObjectsFolder)
		if item.From != "" {
			var err error
			if from, err = parseNodeID(item.From, namespaces); err != nil {
				return nil, err
			}
		}
		names, err := parsePath(item.Path)
		if err != nil {
			return nil, err
		}
		call, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		found, err := s.c.Node(from).TranslateBrowsePathsToNodeIDs(call, names)
		if err != nil {
			return nil, fmt.Errorf("no node at %s: %v", item.Path, err)
		}
		return found, nil
	}
	return nil, errors.New("an item needs a nodeId or a path")
}

// parseNodeID reads a NodeId, with its namespace by URI or by index.
func parseNodeID(s string, namespaces []string) (*ua.NodeID, error) {
	if strings.HasPrefix(s, "nsu=") {
		uri, _, _ := strings.Cut(strings.TrimPrefix(s, "nsu="), ";")
		found := false
		for _, n := range namespaces {
			found = found || n == uri
		}
		if !found {
			return nil, fmt.Errorf("the server has no namespace %s", uri)
		}
	}
	id, err := ua.ParseExpandedNodeID(s, namespaces)
	if err != nil {
		return nil, fmt.Errorf("%q is not a NodeId: %v", s, err)
	}
	return id.NodeID, nil
}

// parsePath reads a browse path such as /2:Projects/2:B1, each step a browse name with the
// index of its namespace, or of namespace 0 without one.
func parsePath(path string) ([]*ua.QualifiedName, error) {
	var names []*ua.QualifiedName
	for _, step := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if step == "" {
			return nil, fmt.Errorf("%q has an empty step", path)
		}
		ns, name, ok := strings.Cut(step, ":")
		if !ok {
			names = append(names, &ua.QualifiedName{Name: step})
			continue
		}
		n, err := strconv.ParseUint(ns, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("%q: %s has no namespace index", path, step)
		}
		names = append(names, &ua.QualifiedName{NamespaceIndex: uint16(n), Name: name})
	}
	return names, nil
}

// kindOf says whether a node's data type can carry a signal.
func kindOf(dataType *ua.NodeID) (ua.TypeID, link.Kind, bool) {
	if dataType == nil || dataType.Namespace() != 0 {
		return 0, 0, false
	}
	switch t := ua.TypeID(dataType.IntID()); t {
	case ua.TypeIDBoolean:
		return t, link.KindBool, true
	case ua.TypeIDSByte, ua.TypeIDByte, ua.TypeIDInt16, ua.TypeIDUint16, ua.TypeIDInt32, ua.TypeIDUint32, ua.TypeIDInt64, ua.TypeIDUint64:
		return t, link.KindInt, true
	case ua.TypeIDFloat, ua.TypeIDDouble:
		return t, link.KindFloat, true
	}
	return 0, 0, false
}

// variant is a value as the node's own type, which a server may insist on.
func variant(v float64, t ua.TypeID) (*ua.Variant, error) {
	n := math.Round(v)
	clamp := func(lo, hi float64) float64 { return math.Max(lo, math.Min(hi, n)) }
	switch t {
	case ua.TypeIDBoolean:
		return ua.NewVariant(v != 0)
	case ua.TypeIDSByte:
		return ua.NewVariant(int8(clamp(math.MinInt8, math.MaxInt8)))
	case ua.TypeIDByte:
		return ua.NewVariant(uint8(clamp(0, math.MaxUint8)))
	case ua.TypeIDInt16:
		return ua.NewVariant(int16(clamp(math.MinInt16, math.MaxInt16)))
	case ua.TypeIDUint16:
		return ua.NewVariant(uint16(clamp(0, math.MaxUint16)))
	case ua.TypeIDInt32:
		return ua.NewVariant(int32(clamp(math.MinInt32, math.MaxInt32)))
	case ua.TypeIDUint32:
		return ua.NewVariant(uint32(clamp(0, math.MaxUint32)))
	case ua.TypeIDInt64:
		return ua.NewVariant(int64(clamp(math.MinInt64, math.MaxInt64)))
	case ua.TypeIDUint64:
		return ua.NewVariant(uint64(clamp(0, math.MaxUint64)))
	case ua.TypeIDFloat:
		return ua.NewVariant(float32(v))
	case ua.TypeIDDouble:
		return ua.NewVariant(v)
	}
	return nil, fmt.Errorf("type %v cannot carry a signal", t)
}

// number reads a value as a number.
func number(v *ua.Variant) float64 {
	if v == nil {
		return 0
	}
	switch t := v.Value().(type) {
	case bool:
		if t {
			return 1
		}
	case int8:
		return float64(t)
	case uint8:
		return float64(t)
	case int16:
		return float64(t)
	case uint16:
		return float64(t)
	case int32:
		return float64(t)
	case uint32:
		return float64(t)
	case int64:
		return float64(t)
	case uint64:
		return float64(t)
	case float32:
		return float64(t)
	case float64:
		return t
	}
	return 0
}

// Run implements link.Session.
func (s *session) Run(ctx context.Context, x link.Exchange) error {
	read := &ua.ReadRequest{TimestampsToReturn: ua.TimestampsToReturnNeither}
	var outputs []int
	for i, n := range s.nodes {
		if n != nil && !s.bindings[i].Signal.Address.Input {
			read.NodesToRead = append(read.NodesToRead, &ua.ReadValueID{NodeID: n.id, AttributeID: ua.AttributeIDValue})
			outputs = append(outputs, i)
		}
	}
	ticker := time.NewTicker(time.Duration(s.settings.Outputs.Interval) * time.Millisecond)
	defer ticker.Stop()
	values := make([]float64, len(s.bindings))
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		inputs := x.Inputs()
		call, cancel := context.WithTimeout(ctx, timeout)
		err := s.write(call, inputs)
		if err == nil && len(outputs) > 0 {
			err = s.read(call, read, outputs, values)
		}
		cancel()
		if err != nil {
			return err
		}
		x.SetOutputs(values)
	}
}

// write sends the inputs that changed.
func (s *session) write(ctx context.Context, inputs []float64) error {
	req := &ua.WriteRequest{}
	var changed []int
	var sent []float64
	for i, n := range s.nodes {
		b := s.bindings[i]
		if n == nil || !b.Signal.Address.Input {
			continue
		}
		v := b.Outward(inputs[i], n.kind)
		if n.written && math.Abs(v-n.sent) < 1e-6 {
			continue
		}
		value, err := variant(v, n.typ)
		if err != nil {
			return err
		}
		req.NodesToWrite = append(req.NodesToWrite, &ua.WriteValue{
			NodeID: n.id, AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{EncodingMask: ua.DataValueValue, Value: value},
		})
		changed = append(changed, i)
		sent = append(sent, v)
	}
	if len(changed) == 0 {
		return nil
	}
	res, err := s.c.Write(ctx, req)
	if err != nil {
		return err
	}
	for k, code := range res.Results {
		i := changed[k]
		if code != ua.StatusOK {
			return fmt.Errorf("the server refused writing %s to %s: %v", s.bindings[i].Signal.Label(), s.nodes[i].id, code)
		}
		s.nodes[i].sent, s.nodes[i].written = sent[k], true
	}
	return nil
}

// read reads the outputs into values.
func (s *session) read(ctx context.Context, req *ua.ReadRequest, outputs []int, values []float64) error {
	res, err := s.c.Read(ctx, req)
	if err != nil {
		return err
	}
	if len(res.Results) != len(outputs) {
		return errors.New("the server answered for fewer nodes than asked")
	}
	for k, r := range res.Results {
		i := outputs[k]
		if r.Status != ua.StatusOK {
			return fmt.Errorf("the server refused reading %s from %s: %v", s.bindings[i].Signal.Label(), s.nodes[i].id, r.Status)
		}
		values[i] = s.bindings[i].Inward(number(r.Value), s.nodes[i].kind)
	}
	return nil
}
