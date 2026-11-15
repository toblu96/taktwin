package opcua

import (
	"encoding/json"

	"github.com/toblu96/taktwin/internal/link"
	"github.com/toblu96/taktwin/internal/opcsim"
)

// SoftPLC is how the simulation reaches the soft PLC of a PLC module instance on its server:
// every signal by its address.
func SoftPLC(endpoint, plc string) link.Connection {
	settings, _ := json.Marshal(map[string]string{"endpoint": endpoint})
	// As opcsim.Space.Key names the node of an address.
	key := "{address}"
	if plc != "" {
		key = plc + ".{address}"
	}
	item, _ := json.Marshal(map[string]string{"nodeId": "nsu=" + opcsim.Namespace + ";s=" + key})
	return link.Connection{
		ID: "soft", Name: "Soft PLC", Driver: Driver{}.Kind(), Settings: settings,
		Generate: []link.GenerateDef{{Signals: "all", Item: item}},
	}
}
