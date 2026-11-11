package plcio

// LinkState is how far a remote PLC link has come.
type LinkState string

const (
	LinkConnecting LinkState = "connecting"
	LinkConnected  LinkState = "connected"
	LinkFailed     LinkState = "failed"
)

// LinkInfo describes the connection to a remote PLC.
type LinkInfo struct {
	State    LinkState `json:"state"`
	Endpoint string    `json:"endpoint"`
	Detail   string    `json:"detail,omitempty"`
	// RoundTrip is how long one exchange of the image took, in milliseconds.
	RoundTrip float64 `json:"roundTrip"`
	// Variables is how many of the image's variables the server has.
	Variables int `json:"variables"`
}

// LinkReporter is implemented by controllers that reach a PLC over a connection. Such a
// PLC runs on the wall clock, so the simulation has to run at real speed with it.
type LinkReporter interface {
	Link() LinkInfo
}

// Closer is implemented by controllers with something to shut down.
type Closer interface {
	Close()
}

// Binding connects a bit of the simulation to a variable of the PLC.
type Binding struct {
	Local    Addr
	Variable string
	Invert   bool
}

// AddressMap lets a PLC program use other addresses than the station. Any variable it
// does not mention uses the same address on both sides.
type AddressMap struct {
	Inputs  []Binding
	Outputs []Binding
}

// RemoteInputs turns the station's inputs into values for the PLC's input variables.
func (m AddressMap) RemoteInputs(n Naming, local Image) map[string]bool {
	out := make(map[string]bool, ImageBytes*8)
	for b := range uint8(ImageBytes) {
		for bit := range uint8(8) {
			a := Addr{Byte: b, Bit: bit}
			out[n.Input(a)] = local.Get(a)
		}
	}
	for _, x := range m.Inputs {
		out[x.Variable] = local.Get(x.Local) != x.Invert
	}
	return out
}

// LocalOutputs turns the PLC's output variables into the station's outputs.
func (m AddressMap) LocalOutputs(n Naming, remote map[string]bool) Image {
	var im Image
	for b := range uint8(ImageBytes) {
		for bit := range uint8(8) {
			a := Addr{Byte: b, Bit: bit}
			im.Set(a, remote[n.Output(a)])
		}
	}
	for _, x := range m.Outputs {
		im.Set(x.Local, remote[x.Variable] != x.Invert)
	}
	return im
}
