package link

import (
	"context"
	"encoding/json"
)

// Browser is implemented by drivers that can show what a controller offers, so that people
// can pick the items of bindings instead of typing them.
type Browser interface {
	// Browse lists what is below a node, under as a Node gives it, or the top for "".
	Browse(ctx context.Context, settings json.RawMessage, under string) ([]Node, error)
}

// Node is one thing a controller offers, as a browser shows it.
type Node struct {
	// ID is how people write it, such as nsu=urn:x;s=B1.state.
	ID   string `json:"id"`
	Name string `json:"name"`
	// Item is the item of a binding to it, for a node that holds a value.
	Item json.RawMessage `json:"item,omitempty"`
	// Type is the type of its value, such as Boolean, for a node that holds one.
	Type string `json:"type,omitempty"`
	// Readable and Writable say what a binding can do with its value.
	Readable bool `json:"readable"`
	Writable bool `json:"writable"`
	// Below is set when it has nodes below it, which Browse lists given Under.
	Below bool   `json:"below"`
	Under string `json:"under,omitempty"`
}
