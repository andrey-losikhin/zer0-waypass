package protocol

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestEntryIDRoundTrip(t *testing.T) {
	paths := []string{
		"synthetic/alice",
		"unicode/пароль/鍵",
		"folder/entry with spaces",
		".private/.entry",
	}
	for _, canonicalPath := range paths {
		entryID, err := EncodeCanonicalPath(canonicalPath)
		if err != nil {
			t.Fatalf("encode valid path: %v", err)
		}
		decoded, err := DecodeEntryID(entryID)
		if err != nil {
			t.Fatalf("decode valid ID: %v", err)
		}
		if decoded != canonicalPath {
			t.Fatal("decoded path differs from original")
		}
	}
}

func TestEncodeCanonicalPathDeterministicVector(t *testing.T) {
	const want EntryID = "AXN5bnRoZXRpYy9hbGljZQ"
	got, err := EncodeCanonicalPath("synthetic/alice")
	if err != nil {
		t.Fatalf("EncodeCanonicalPath: %v", err)
	}
	if got != want {
		t.Fatal("EncodeCanonicalPath produced an unexpected vector")
	}
	if strings.Contains(string(got), "synthetic/alice") {
		t.Fatal("EntryID exposes canonical path as plaintext")
	}
}

func TestEncodeCanonicalPathUsesRawURLAlphabet(t *testing.T) {
	entryID, err := EncodeCanonicalPath("boundary/ÿþ")
	if err != nil {
		t.Fatalf("EncodeCanonicalPath: %v", err)
	}
	for _, character := range entryID {
		if !((character >= 'A' && character <= 'Z') ||
			(character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-') {
			t.Fatal("EntryID contains a character outside the raw URL-safe alphabet")
		}
	}
}

func TestDecodeEntryIDRejectsMalformedEncoding(t *testing.T) {
	wrongVersion := rawEntryID([]byte{0x02, 'a'})
	invalidUTF8 := rawEntryID([]byte{entryIDFormatVersion, 0xff})
	cases := map[string]EntryID{
		"empty encoded ID":   "",
		"malformed base64":   "%",
		"padding":            "AWE=",
		"standard alphabet":  "AWE+",
		"noncanonical input": "AWE\n",
		"trailing bits":      "AR",
		"wrong version":      wrongVersion,
		"missing version":    rawEntryID([]byte("entry")),
		"missing payload":    "AQ",
		"invalid UTF-8":      invalidUTF8,
	}
	for name, entryID := range cases {
		t.Run(name, func(t *testing.T) {
			assertInvalidDecode(t, entryID)
		})
	}
}

func TestEntryIDRejectsInvalidPaths(t *testing.T) {
	cases := map[string]string{
		"empty":             "",
		"absolute":          "/entry",
		"traversal":         "folder/../entry",
		"dot segment":       "folder/./entry",
		"double slash":      "folder//entry",
		"trailing slash":    "folder/",
		"leading dash":      "-entry",
		"backslash":         `folder\entry`,
		"NUL":               "folder/\x00entry",
		"control":           "folder/\nentry",
		"DEL":               "folder/\x7fentry",
		"Unicode control":   "folder/\u0085entry",
		"invalid UTF-8":     string([]byte{'a', 0xff}),
		"noncanonical root": ".",
	}
	for name, canonicalPath := range cases {
		t.Run(name, func(t *testing.T) {
			assertInvalidPath(t, canonicalPath)
		})
	}
}

func TestEntryIDRejectsShellMetacharacters(t *testing.T) {
	for _, character := range []rune{'\\', ';', '&', '|', '$', '`', '\'', '"', '<', '>', '(', ')', '{', '}', '[', ']', '*', '?', '!'} {
		canonicalPath := "folder/entry" + string(character)
		t.Run(base64.RawURLEncoding.EncodeToString([]byte(string(character))), func(t *testing.T) {
			assertInvalidPath(t, canonicalPath)
		})
	}
}

func TestEntryIDPathSizeBoundary(t *testing.T) {
	validPath := strings.Repeat("a", maxCanonicalPathBytes)
	entryID, err := EncodeCanonicalPath(validPath)
	if err != nil {
		t.Fatalf("encode 4096-byte path: %v", err)
	}
	if len(entryID) != maxEntryIDLength {
		t.Fatalf("encoded boundary length = %d, want %d", len(entryID), maxEntryIDLength)
	}
	decoded, err := DecodeEntryID(entryID)
	if err != nil || decoded != validPath {
		t.Fatalf("decode 4096-byte path failed: %v", err)
	}

	assertInvalidPath(t, strings.Repeat("a", maxCanonicalPathBytes+1))
	assertInvalidDecode(t, EntryID(strings.Repeat("A", maxEntryIDLength+1)))
}

func TestInvalidEntryIDErrorsAreSingleRedactedSentinel(t *testing.T) {
	const marker = "SYNTHETIC_PRIVATE_PATH_MARKER"
	_, encodeErr := EncodeCanonicalPath("../" + marker)
	encodedMarker := rawEntryID(append([]byte{entryIDFormatVersion}, []byte("../"+marker)...))
	_, decodeErr := DecodeEntryID(encodedMarker)
	for _, err := range []error{encodeErr, decodeErr} {
		if !errors.Is(err, ErrInvalidEntryID) || err != ErrInvalidEntryID {
			t.Fatalf("error is not the exported sentinel: %v", err)
		}
		if strings.Contains(err.Error(), marker) || err.Error() != "invalid entry ID" {
			t.Fatal("validation error exposed rejected input")
		}
	}
}

func assertInvalidPath(t *testing.T, canonicalPath string) {
	t.Helper()
	entryID, err := EncodeCanonicalPath(canonicalPath)
	if entryID != "" || !errors.Is(err, ErrInvalidEntryID) {
		t.Fatal("invalid path was accepted or returned a non-generic error")
	}

	encoded := rawEntryID(append([]byte{entryIDFormatVersion}, []byte(canonicalPath)...))
	assertInvalidDecode(t, encoded)
}

func assertInvalidDecode(t *testing.T, entryID EntryID) {
	t.Helper()
	decoded, err := DecodeEntryID(entryID)
	if decoded != "" || !errors.Is(err, ErrInvalidEntryID) {
		t.Fatal("invalid ID was accepted or returned a non-generic error")
	}
}

func rawEntryID(payload []byte) EntryID {
	return EntryID(base64.RawURLEncoding.EncodeToString(payload))
}
