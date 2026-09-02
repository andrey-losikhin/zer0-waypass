package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestProtocolVersion(t *testing.T) {
	if ProtocolVersion != 1 {
		t.Fatalf("ProtocolVersion = %d, want 1", ProtocolVersion)
	}
}

func TestListEnvelopeJSON(t *testing.T) {
	response := ListEnvelope{
		Protocol: ProtocolVersion,
		Items: []Item{
			{ID: EntryID("opaque-placeholder"), Label: "example.test"},
		},
	}

	got, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := `{"protocol":1,"items":[{"id":"opaque-placeholder","label":"example.test"}]}`
	if string(got) != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}

func TestListEnvelopeEmptyItemsJSON(t *testing.T) {
	response := ListEnvelope{Protocol: ProtocolVersion}

	got, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := `{"protocol":1,"items":[]}`
	if string(got) != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}

func TestStatusEnvelopeJSON(t *testing.T) {
	response := StatusEnvelope{Protocol: ProtocolVersion, Backend: "ready"}

	got, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := `{"protocol":1,"backend":"ready"}`
	if string(got) != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}

func TestProtocolStructFields(t *testing.T) {
	assertJSONFields(t, reflect.TypeOf(Item{}), map[string]string{
		"ID":    "id",
		"Label": "label",
	})
	assertJSONFields(t, reflect.TypeOf(ListEnvelope{}), map[string]string{
		"Protocol": "protocol",
		"Items":    "items",
	})
	assertJSONFields(t, reflect.TypeOf(StatusEnvelope{}), map[string]string{
		"Protocol": "protocol",
		"Backend":  "backend",
	})
}

func assertJSONFields(t *testing.T, typ reflect.Type, want map[string]string) {
	t.Helper()

	if typ.NumField() != len(want) {
		t.Fatalf("%s has %d fields, want exactly %d", typ.Name(), typ.NumField(), len(want))
	}

	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		wantTag, ok := want[field.Name]
		if !ok {
			t.Fatalf("%s contains unexpected field %q", typ.Name(), field.Name)
		}
		if got := field.Tag.Get("json"); got != wantTag {
			t.Fatalf("%s.%s JSON tag = %q, want %q", typ.Name(), field.Name, got, wantTag)
		}
	}
}
