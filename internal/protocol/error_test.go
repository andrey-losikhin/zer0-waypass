package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMarshalErrorExactClosedShape(t *testing.T) {
	codes := []ErrorCode{
		ErrorInvalidInvocation,
		ErrorBackendUnavailable,
		ErrorBackendTimeout,
		ErrorOperationCanceled,
		ErrorBackendInvalidData,
		ErrorBackendOutputTooLarge,
		ErrorBackend,
		ErrorOutput,
	}
	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			got, ok := MarshalError(code)
			want := `{"protocol":1,"error":{"code":"` + string(code) + `"}}` + "\n"
			if !ok || string(got) != want {
				t.Fatalf("MarshalError = %q/%v, want %q/true", got, ok, want)
			}

			var decoded map[string]any
			if err := json.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("decode error JSON: %v", err)
			}
			if !reflect.DeepEqual(sortedKeys(decoded), []string{"error", "protocol"}) {
				t.Fatalf("top-level keys = %v", sortedKeys(decoded))
			}
			errorObject, ok := decoded["error"].(map[string]any)
			if !ok || !reflect.DeepEqual(sortedKeys(errorObject), []string{"code"}) {
				t.Fatalf("error object = %#v, want only code", decoded["error"])
			}
		})
	}
}

func TestMarshalErrorRejectsUnknownCode(t *testing.T) {
	if encoded, ok := MarshalError(ErrorCode("raw/private/cause")); ok || encoded != nil {
		t.Fatalf("unknown code serialized as %q", encoded)
	}
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	// The expected maps have at most two keys; avoid adding a dependency.
	if len(keys) == 2 && keys[0] > keys[1] {
		keys[0], keys[1] = keys[1], keys[0]
	}
	return keys
}
