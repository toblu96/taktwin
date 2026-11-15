package opcua

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"

	"github.com/toblu96/taktwin/internal/link"
)

// maxBrowse is the most references Browse lists below one node.
const maxBrowse = 2000

// Browse implements link.Browser: the objects and variables below a node, the Objects folder
// for "", each written with its namespace's URI.
func (Driver) Browse(ctx context.Context, settings json.RawMessage, under string) ([]link.Node, error) {
	s, err := parseSettings(settings)
	if err != nil {
		return nil, err
	}
	c, err := connect(ctx, s.Endpoint)
	if err != nil {
		return nil, err
	}
	defer (&session{c: c}).Close()
	call, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	namespaces, err := c.NamespaceArray(call)
	if err != nil {
		return nil, fmt.Errorf("reading the server's namespaces: %w", err)
	}
	from := ua.NewNumericNodeID(0, id.ObjectsFolder)
	if under != "" {
		if from, err = parseNodeID(under, namespaces); err != nil {
			return nil, err
		}
	}
	refs, err := children(call, c, from)
	if err != nil {
		return nil, err
	}
	nodes := make([]link.Node, 0, len(refs))
	var variables []int
	for _, r := range refs {
		n := link.Node{ID: writeNodeID(r.NodeID.NodeID, namespaces), Name: r.DisplayName.Text}
		if n.Name == "" && r.BrowseName != nil {
			n.Name = r.BrowseName.Name
		}
		if r.NodeClass == ua.NodeClassObject {
			n.Below, n.Under = true, n.ID
		}
		if r.NodeClass == ua.NodeClassVariable {
			variables = append(variables, len(nodes))
		}
		nodes = append(nodes, n)
	}
	if len(variables) == 0 {
		return nodes, nil
	}
	// What each variable holds and allows, in one request.
	req := &ua.ReadRequest{TimestampsToReturn: ua.TimestampsToReturnNeither}
	for _, i := range variables {
		node := refs[i].NodeID.NodeID
		for _, a := range []ua.AttributeID{ua.AttributeIDDataType, ua.AttributeIDAccessLevel, ua.AttributeIDUserAccessLevel} {
			req.NodesToRead = append(req.NodesToRead, &ua.ReadValueID{NodeID: node, AttributeID: a})
		}
	}
	res, err := c.Read(call, req)
	if err != nil || len(res.Results) != 3*len(variables) {
		return nodes, nil
	}
	for k, i := range variables {
		n := &nodes[i]
		dataType := res.Results[3*k]
		if dataType.Status == ua.StatusOK {
			if t, _ := dataType.Value.Value().(*ua.NodeID); t != nil {
				n.Type = typeName(t)
			}
		}
		level := accessLevel(res.Results[3*k+1])
		if user, ok := res.Results[3*k+2].Value.Value().(byte); ok && res.Results[3*k+2].Status == ua.StatusOK {
			level &= user
		}
		n.Readable = level&byte(ua.AccessLevelTypeCurrentRead) != 0
		n.Writable = level&byte(ua.AccessLevelTypeCurrentWrite) != 0
		n.Item, _ = json.Marshal(Item{NodeID: n.ID})
		// A variable can have variables below it, as a structure its fields.
		n.Below, n.Under = true, n.ID
	}
	return nodes, nil
}

// children lists the hierarchical references of a node to objects and variables, following
// continuation points up to maxBrowse.
func children(ctx context.Context, c *opcua.Client, node *ua.NodeID) ([]*ua.ReferenceDescription, error) {
	res, err := c.Browse(ctx, &ua.BrowseRequest{
		View:                          &ua.ViewDescription{ViewID: ua.NewTwoByteNodeID(0)},
		RequestedMaxReferencesPerNode: 500,
		NodesToBrowse: []*ua.BrowseDescription{{
			NodeID:          node,
			BrowseDirection: ua.BrowseDirectionForward,
			ReferenceTypeID: ua.NewNumericNodeID(0, id.HierarchicalReferences),
			IncludeSubtypes: true,
			NodeClassMask:   uint32(ua.NodeClassObject | ua.NodeClassVariable),
			ResultMask:      uint32(ua.BrowseResultMaskAll),
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("browsing %s: %w", node, err)
	}
	if len(res.Results) != 1 {
		return nil, fmt.Errorf("browsing %s: no answer", node)
	}
	r := res.Results[0]
	if r.StatusCode != ua.StatusOK {
		return nil, fmt.Errorf("browsing %s: %s", node, statusText(r.StatusCode))
	}
	refs := r.References
	for point := r.ContinuationPoint; len(point) > 0 && len(refs) < maxBrowse; {
		next, err := c.BrowseNext(ctx, &ua.BrowseNextRequest{ContinuationPoints: [][]byte{point}})
		if err != nil || len(next.Results) != 1 {
			break
		}
		refs = append(refs, next.Results[0].References...)
		point = next.Results[0].ContinuationPoint
	}
	return refs, nil
}

// writeNodeID writes a NodeId with its namespace's URI, as it holds whatever index the
// server gives the namespace, or as it is for namespace 0.
func writeNodeID(n *ua.NodeID, namespaces []string) string {
	s := n.String()
	ns := int(n.Namespace())
	if ns == 0 || ns >= len(namespaces) {
		return s
	}
	_, rest, ok := strings.Cut(s, ";")
	if !ok {
		return s
	}
	return "nsu=" + namespaces[ns] + ";" + rest
}

// typeName names the built-in data types, and writes the others as their NodeId.
func typeName(t *ua.NodeID) string {
	if t.Namespace() == 0 {
		if name, ok := map[uint32]string{
			id.Boolean: "Boolean", id.SByte: "SByte", id.Byte: "Byte", id.Int16: "Int16", id.UInt16: "UInt16",
			id.Int32: "Int32", id.UInt32: "UInt32", id.Int64: "Int64", id.UInt64: "UInt64",
			id.Float: "Float", id.Double: "Double", id.String: "String", id.DateTime: "DateTime",
		}[t.IntID()]; ok {
			return name
		}
	}
	return t.String()
}

// codesysVar is how CODESYS names a variable of a list on its server: |var|, the PLC name, the
// application, and the list, each after a dot.
var codesysVar = regexp.MustCompile(`^\|var\|(.+)\.([^.]+)\.([^.]+)$`)

// CODESYSList is where a CODESYS PLC has a variable list on its server.
type CODESYSList struct {
	// Namespace is that of its NodeIds, as nsu=<URI>.
	Namespace   string `json:"namespace"`
	Runtime     string `json:"runtime"`
	Application string `json:"application"`
}

// FindCODESYSList looks for a variable list by name on a CODESYS PLC's server, the way
// people would in a browser, so that the CODESYS connection's form can be filled in. It
// follows at most limit nodes.
func FindCODESYSList(ctx context.Context, endpoint, list string, limit int) (CODESYSList, error) {
	c, err := connect(ctx, endpoint)
	if err != nil {
		return CODESYSList{}, err
	}
	defer (&session{c: c}).Close()
	call, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	namespaces, err := c.NamespaceArray(call)
	if err != nil {
		return CODESYSList{}, err
	}
	queue := []*ua.NodeID{ua.NewNumericNodeID(0, id.ObjectsFolder)}
	seen := map[string]bool{}
	for visited := 0; len(queue) > 0 && visited < limit; visited++ {
		node := queue[0]
		queue = queue[1:]
		refs, err := children(call, c, node)
		if err != nil {
			continue
		}
		for _, r := range refs {
			n := r.NodeID.NodeID
			key := n.String()
			if seen[key] {
				continue
			}
			seen[key] = true
			if m := codesysVar.FindStringSubmatch(n.StringID()); m != nil && m[3] == list && int(n.Namespace()) < len(namespaces) {
				return CODESYSList{Namespace: "nsu=" + namespaces[n.Namespace()], Runtime: m[1], Application: m[2]}, nil
			}
			if r.NodeClass == ua.NodeClassObject || strings.HasPrefix(n.StringID(), "|var|") {
				queue = append(queue, n)
			}
		}
	}
	return CODESYSList{}, fmt.Errorf("the server at %s has no variable list %s: add it to the symbol configuration of the CODESYS project and download it", endpoint, list)
}
