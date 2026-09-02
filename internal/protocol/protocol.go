// Package protocol defines the metadata exchanged by the helper.
package protocol

import "encoding/json"

// ProtocolVersion is the current metadata protocol version.
const ProtocolVersion = 1
const FieldProtocolVersion = 2

// Item is the complete metadata representation of a password-store entry.
type Item struct {
	ID    EntryID `json:"id"`
	Label string  `json:"label"`
}

// ListEnvelope is the versioned response containing entry metadata.
type ListEnvelope struct {
	Protocol int    `json:"protocol"`
	Items    []Item `json:"items"`
}

// StatusEnvelope is the complete status response. BackendReady is "ready"
// only after a successful metadata-only backend probe.
type StatusEnvelope struct {
	Protocol int    `json:"protocol"`
	Backend  string `json:"backend"`
}

type FieldItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Visibility string `json:"visibility"`
	Multiline  bool   `json:"multiline"`
	Value      string `json:"value,omitempty"`
}

type FieldsEnvelope struct {
	Protocol int         `json:"protocol"`
	Revision string      `json:"revision"`
	Fields   []FieldItem `json:"fields"`
}

// MarshalJSON keeps an empty item list represented as [] instead of null.
func (e ListEnvelope) MarshalJSON() ([]byte, error) {
	items := e.Items
	if items == nil {
		items = []Item{}
	}

	type envelope struct {
		Protocol int    `json:"protocol"`
		Items    []Item `json:"items"`
	}

	return json.Marshal(envelope{
		Protocol: e.Protocol,
		Items:    items,
	})
}
