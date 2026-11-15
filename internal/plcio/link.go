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
	// Variables is how many signals are exchanged.
	Variables int `json:"variables"`
	// Problems are the bindings that cannot be exchanged, and why.
	Problems []string `json:"problems,omitempty"`
	// Unbound are the station's signals the connection does not exchange.
	Unbound []string `json:"unbound,omitempty"`
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
