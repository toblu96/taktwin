// Package link couples a PLC of a station to a controller outside the simulation. A
// connection says which driver talks to the controller, how it is set up, and which item of
// the controller each signal of the station is exchanged with. The package knows no
// protocol: drivers, such as the one in link/opcua, do.
package link

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/toblu96/taktwin/internal/plcio"
)

// Connection is how one PLC of a station is coupled to a controller: the content of a
// connection file, or a preset the app makes.
type Connection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Station limits the connection to one station, by id; without it, it fits any station
	// whose PLC is PLC.
	Station string `json:"station,omitempty"`
	// PLC is the instance of the station's PLC module it is for; without it, the first PLC.
	PLC string `json:"plc,omitempty"`
	// Driver names the driver, such as "opcua".
	Driver string `json:"driver"`
	// Vars is a JSON Schema of an object, one property per value someone may change, such as
	// the computer the controller runs on. The form to connect is made from it.
	Vars json.RawMessage `json:"vars,omitempty"`
	// Settings are the driver's, such as its endpoint. Their texts may use {var}.
	Settings json.RawMessage `json:"settings,omitempty"`
	// Namespaces gives names to texts that items use often, such as an OPC UA namespace URI
	// for nsu={fs}.
	Namespaces map[string]string `json:"namespaces,omitempty"`
	Bindings   []BindingDef      `json:"bindings,omitempty"`
	Generate   []GenerateDef     `json:"generate,omitempty"`
}

// BindingDef exchanges one signal of the station, named by its symbol or its address, with
// an item of the controller.
type BindingDef struct {
	// Signal is the symbol of the signal, such as B1; it follows the signal when it moves to
	// another pin.
	Signal string `json:"signal,omitempty"`
	// Address is the address of the signal, such as %IX0.3; whatever is wired there is
	// exchanged.
	Address string `json:"address,omitempty"`
	// Item is what the driver exchanges it with, such as {"nodeId": "ns=2;s=B1"}. Its texts
	// may use {var}.
	Item json.RawMessage `json:"item"`
	// Invert exchanges a bit the other way round.
	Invert bool `json:"invert,omitempty"`
	// Scale converts an analogue value linearly, from the station's unit to the item's.
	Scale *Scale `json:"scale,omitempty"`
}

// Scale maps From, a range in the station's unit such as [0, 10] volts, to To, the same
// range in the item's, such as [0, 27648].
type Scale struct {
	From [2]float64 `json:"from"`
	To   [2]float64 `json:"to"`
}

// GenerateDef makes a binding for each signal of a kind, from a template of its item. Its
// texts may use {address} (%IX0.3), {shown} (as the PLC shows it), {symbol}, {byte}, {bit},
// {bit:2} (two digits), {channel} (of an analogue input), and the vars.
type GenerateDef struct {
	// Signals is inputs, outputs, analog (inputs) or all.
	Signals string          `json:"signals"`
	Item    json.RawMessage `json:"item"`
}

// Signal is one signal of a PLC as a station has it: a bit of its inputs or outputs, or an
// analogue input.
type Signal struct {
	Address plcio.Address
	// Symbol is its name, if it has one, such as B1.
	Symbol string
	// Shown is its address as the PLC shows it, such as IB0Bit03.
	Shown string
}

// Label is how a report names the signal.
func (s Signal) Label() string {
	shown := cmp.Or(s.Shown, s.Address.String())
	if s.Symbol == "" {
		return shown
	}
	return s.Symbol + " (" + shown + ")"
}

// Binding is a resolved binding: a signal of the station and the driver's item for it.
type Binding struct {
	Signal Signal
	Invert bool
	Scale  *Scale
	// Item is the driver's, with the vars filled in. The link itself never reads it.
	Item json.RawMessage
}

// Analog says whether the binding carries an analogue value.
func (b Binding) Analog() bool {
	_, ok := b.Signal.Address.AnalogInput()
	return ok
}

// Kind is what kind of value an item holds, as far as a binding cares.
type Kind int

const (
	KindBool Kind = iota
	KindInt
	KindFloat
)

// Outward turns a value of the station into one for an item of kind k: a bit as 0 or 1, and
// an analogue value scaled, or else in volts for a number and millivolts for an integer.
func (b Binding) Outward(v float64, k Kind) float64 {
	if !b.Analog() {
		if (v != 0) != b.Invert {
			return 1
		}
		return 0
	}
	switch {
	case b.Scale != nil:
		return b.Scale.apply(v)
	case k == KindInt:
		return v * 1000
	}
	return v
}

// Inward turns the value of an item into the station's: a bit is on for anything but 0.
func (b Binding) Inward(v float64) float64 {
	if (v != 0) != b.Invert {
		return 1
	}
	return 0
}

func (s *Scale) apply(v float64) float64 {
	span := s.From[1] - s.From[0]
	if span == 0 {
		return s.To[0]
	}
	return s.To[0] + (v-s.From[0])*(s.To[1]-s.To[0])/span
}

// Parse reads a connection file.
func Parse(data []byte) (Connection, error) {
	var c Connection
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, err
	}
	switch {
	case c.ID == "":
		return c, fmt.Errorf("the connection has no id")
	case c.Driver == "":
		return c, fmt.Errorf("connection %s names no driver", c.ID)
	}
	if _, err := c.schema(); err != nil {
		return c, fmt.Errorf("connection %s: %w", c.ID, err)
	}
	return c, nil
}

// schema compiles the connection's vars, or gives nil without them.
func (c Connection) schema() (*jsonschema.Schema, error) {
	if len(c.Vars) == 0 {
		return nil, nil
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(c.Vars))
	if err != nil {
		return nil, fmt.Errorf("vars: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	// Only the schema itself: a reference to anywhere else is refused, not fetched.
	compiler.UseLoader(noLoader{})
	if err := compiler.AddResource("vars.json", doc); err != nil {
		return nil, fmt.Errorf("vars: %w", err)
	}
	s, err := compiler.Compile("vars.json")
	if err != nil {
		return nil, fmt.Errorf("vars: %w", err)
	}
	return s, nil
}

type noLoader struct{}

func (noLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("vars may not refer to %s", url)
}

// Values are the vars as someone filled them in, with the defaults of the vars for the
// others, checked against the vars.
func (c Connection) Values(given map[string]any) (map[string]any, error) {
	out := map[string]any{}
	var props struct {
		Properties map[string]struct {
			Default any `json:"default"`
		} `json:"properties"`
	}
	if len(c.Vars) > 0 {
		if err := json.Unmarshal(c.Vars, &props); err != nil {
			return nil, fmt.Errorf("vars: %w", err)
		}
	}
	for name, p := range props.Properties {
		if p.Default != nil {
			out[name] = p.Default
		}
	}
	for name, v := range given {
		if _, known := props.Properties[name]; known {
			out[name] = v
		}
	}
	s, err := c.schema()
	if err != nil || s == nil {
		return out, err
	}
	// The validator wants numbers as JSON has them.
	data, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if err := s.Validate(doc); err != nil {
		return nil, fmt.Errorf("the values do not fit: %s", strings.TrimSpace(validationText(err)))
	}
	return out, nil
}

// validationText is the first line of each problem of a validation, without the schema
// locations that mean nothing to someone filling in a form.
func validationText(err error) string {
	var v *jsonschema.ValidationError
	if ok := asValidation(err, &v); !ok {
		return err.Error()
	}
	p := message.NewPrinter(language.English)
	var lines []string
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			where := "/" + strings.Join(e.InstanceLocation, "/")
			lines = append(lines, where+": "+e.ErrorKind.LocalizedString(p))
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(v)
	return strings.Join(lines, "; ")
}

func asValidation(err error, v **jsonschema.ValidationError) bool {
	e, ok := err.(*jsonschema.ValidationError)
	*v = e
	return ok
}

var placeholder = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*(?::2)?)\}`)

// fill puts values into the {name}s of a text. A name without a value is an error.
func fill(s string, values map[string]string) (string, error) {
	var missing []string
	out := placeholder.ReplaceAllStringFunc(s, func(m string) string {
		name := m[1 : len(m)-1]
		v, ok := values[name]
		if !ok {
			missing = append(missing, m)
			return m
		}
		return v
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("%s has no value for %s", s, strings.Join(missing, ", "))
	}
	return out, nil
}

// fillJSON fills every text in a JSON value.
func fillJSON(raw json.RawMessage, values map[string]string) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	var err error
	var walk func(any) any
	walk = func(x any) any {
		switch t := x.(type) {
		case string:
			s, e := fill(t, values)
			if e != nil && err == nil {
				err = e
			}
			return s
		case []any:
			for i := range t {
				t[i] = walk(t[i])
			}
		case map[string]any:
			for k := range t {
				t[k] = walk(t[k])
			}
		}
		return x
	}
	v = walk(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func text(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

// Resolved is a connection made concrete for a station: its driver's settings and bindings,
// with every var filled in.
type Resolved struct {
	Driver   string
	Settings json.RawMessage
	Bindings []Binding
	// Unbound are the signals the connection does not exchange.
	Unbound []Signal
}

// Resolve makes a connection concrete for the signals of a PLC, with the values someone
// gave its vars. Explicit bindings take the place of generated ones that write the same:
// for an input, the same item; for an output, the same signal.
func (c Connection) Resolve(signals []Signal, given map[string]any) (Resolved, error) {
	values, err := c.Values(given)
	if err != nil {
		return Resolved{}, err
	}
	vars := map[string]string{}
	for name, v := range values {
		vars[name] = text(v)
	}
	for name, uri := range c.Namespaces {
		if _, taken := vars[name]; taken {
			return Resolved{}, fmt.Errorf("%s is both a var and a namespace", name)
		}
		vars[name] = uri
	}
	r := Resolved{Driver: c.Driver}
	if r.Settings, err = fillJSON(c.Settings, vars); err != nil {
		return r, fmt.Errorf("settings: %w", err)
	}

	var explicit, generated []Binding
	for i, g := range c.Generate {
		for _, s := range signals {
			if !g.matches(s) {
				continue
			}
			item, err := fillJSON(g.Item, signalValues(s, vars))
			if err != nil {
				return r, fmt.Errorf("generate[%d]: %w", i, err)
			}
			generated = append(generated, Binding{Signal: s, Item: item})
		}
	}
	for i, d := range c.Bindings {
		s, err := find(signals, d)
		if err != nil {
			return r, fmt.Errorf("bindings[%d]: %w", i, err)
		}
		item, err := fillJSON(d.Item, signalValues(s, vars))
		if err != nil {
			return r, fmt.Errorf("bindings[%d]: %w", i, err)
		}
		if len(item) == 0 || string(item) == "null" {
			return r, fmt.Errorf("bindings[%d] has no item", i)
		}
		explicit = append(explicit, Binding{Signal: s, Invert: d.Invert, Scale: d.Scale, Item: item})
	}
	replaced := func(g Binding) bool {
		return slices.ContainsFunc(explicit, func(e Binding) bool {
			if g.Signal.Address.Input {
				return bytes.Equal(canonical(g.Item), canonical(e.Item))
			}
			return g.Signal.Address == e.Signal.Address
		})
	}
	for _, g := range generated {
		if !replaced(g) {
			r.Bindings = append(r.Bindings, g)
		}
	}
	r.Bindings = append(r.Bindings, explicit...)

	// The PLC writes an output: two items for it would fight over it.
	outputs := map[plcio.Address]bool{}
	for _, b := range r.Bindings {
		if b.Signal.Address.Input {
			continue
		}
		if outputs[b.Signal.Address] {
			return r, fmt.Errorf("the output %s is bound twice", b.Signal.Label())
		}
		outputs[b.Signal.Address] = true
	}
	for _, s := range signals {
		if !slices.ContainsFunc(r.Bindings, func(b Binding) bool { return b.Signal.Address == s.Address }) {
			r.Unbound = append(r.Unbound, s)
		}
	}
	return r, nil
}

// canonical writes JSON the same way whatever order or spacing it came in.
func canonical(raw json.RawMessage) []byte {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return raw
	}
	out, _ := json.Marshal(v)
	return out
}

func (g GenerateDef) matches(s Signal) bool {
	_, analog := s.Address.AnalogInput()
	switch g.Signals {
	case "inputs":
		return s.Address.Input && !analog
	case "outputs":
		return !s.Address.Input
	case "analog":
		return analog
	case "all":
		return true
	}
	return false
}

// signalValues are the vars and what a template can say of a signal.
func signalValues(s Signal, vars map[string]string) map[string]string {
	out := make(map[string]string, len(vars)+7)
	for k, v := range vars {
		out[k] = v
	}
	out["address"], out["shown"], out["symbol"] = s.Address.String(), s.Shown, s.Symbol
	out["byte"] = strconv.Itoa(s.Address.Byte)
	out["bit"] = strconv.Itoa(int(s.Address.Bit))
	out["bit:2"] = fmt.Sprintf("%02d", s.Address.Bit)
	if ch, ok := s.Address.AnalogInput(); ok {
		out["channel"] = strconv.Itoa(ch)
	}
	return out
}

func find(signals []Signal, d BindingDef) (Signal, error) {
	switch {
	case d.Signal != "" && d.Address != "":
		return Signal{}, fmt.Errorf("a binding names its signal by symbol or by address, not both")
	case d.Address != "":
		a, err := plcio.ParseAddress(d.Address)
		if err != nil {
			return Signal{}, err
		}
		if i := slices.IndexFunc(signals, func(s Signal) bool { return s.Address == a }); i >= 0 {
			return signals[i], nil
		}
		return Signal{}, fmt.Errorf("the station has no signal at %s", d.Address)
	case d.Signal != "":
		var found []Signal
		for _, s := range signals {
			if s.Symbol == d.Signal {
				found = append(found, s)
			}
		}
		switch len(found) {
		case 0:
			return Signal{}, fmt.Errorf("the station has no signal %s", d.Signal)
		case 1:
			return found[0], nil
		}
		return Signal{}, fmt.Errorf("the station has more than one signal %s", d.Signal)
	}
	return Signal{}, fmt.Errorf("a binding needs a signal or an address")
}
