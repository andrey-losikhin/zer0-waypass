package protocol

import (
	"encoding/base64"
	"errors"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// entryIDFormatVersion versions the payload encoded in an EntryID. It is
// independent of ProtocolVersion, which versions the JSON envelope.
const entryIDFormatVersion byte = 0x01

const (
	maxCanonicalPathBytes = 4096
	maxEntryIDBytes       = 1 + maxCanonicalPathBytes
	maxEntryIDLength      = (maxEntryIDBytes*8 + 5) / 6
)

// ErrInvalidEntryID is returned for every invalid entry path or encoded ID.
// It deliberately carries no rejected input or decoded bytes.
var ErrInvalidEntryID = errors.New("invalid entry ID")

// EntryID is the opaque transport representation of a password-store entry.
// It is reversible and is neither authentication nor authorization.
type EntryID string

// EncodeCanonicalPath validates and encodes a canonical relative entry path.
func EncodeCanonicalPath(canonicalPath string) (EntryID, error) {
	if err := validateCanonicalPath(canonicalPath); err != nil {
		return "", err
	}

	payload := make([]byte, 1+len(canonicalPath))
	payload[0] = entryIDFormatVersion
	copy(payload[1:], canonicalPath)

	return EntryID(base64.RawURLEncoding.EncodeToString(payload)), nil
}

// DecodeEntryID strictly decodes and validates an untrusted EntryID.
// The returned path is still not authorization; callers must confirm exact
// membership in a fresh backend listing before using it.
func DecodeEntryID(entryID EntryID) (string, error) {
	encoded := string(entryID)
	if len(encoded) == 0 || len(encoded) > maxEntryIDLength {
		return "", ErrInvalidEntryID
	}

	payload, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(payload) < 2 || payload[0] != entryIDFormatVersion {
		return "", ErrInvalidEntryID
	}
	if base64.RawURLEncoding.EncodeToString(payload) != encoded {
		return "", ErrInvalidEntryID
	}

	canonicalPath := string(payload[1:])
	if err := validateCanonicalPath(canonicalPath); err != nil {
		return "", err
	}
	return canonicalPath, nil
}

func validateCanonicalPath(canonicalPath string) error {
	if len(canonicalPath) == 0 || len(canonicalPath) > maxCanonicalPathBytes || !utf8.ValidString(canonicalPath) {
		return ErrInvalidEntryID
	}
	if canonicalPath[0] == '/' || canonicalPath[0] == '-' || canonicalPath[len(canonicalPath)-1] == '/' {
		return ErrInvalidEntryID
	}
	if strings.Contains(canonicalPath, "//") || path.Clean(canonicalPath) != canonicalPath {
		return ErrInvalidEntryID
	}

	for _, segment := range strings.Split(canonicalPath, "/") {
		if segment == "." || segment == ".." {
			return ErrInvalidEntryID
		}
	}
	for _, character := range canonicalPath {
		if unicode.IsControl(character) || strings.ContainsRune("\\;&|$`\"'<>()[]{}*?!", character) {
			return ErrInvalidEntryID
		}
	}

	return nil
}
