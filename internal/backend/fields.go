package backend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/andrey-losikhin/zer0-waypass/internal/protocol"
)

const (
	FieldProtocolVersion = 2
	FieldFormat          = "zer0-waypass/fields-v1"
	ReservedFieldPrefix  = ".zer0-waypass/"
	maxManifestBytes     = 64 << 10
	maxPublicValueBytes  = 256 << 10
	maxPublicFieldsBytes = 1 << 20
)

var ErrInvalidManifest = errors.New("invalid field manifest")

type Field struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Visibility string `json:"visibility"`
	Multiline  bool   `json:"multiline"`
	Value      string `json:"value,omitempty"`
}

type FieldSet struct {
	Revision string  `json:"revision"`
	Fields   []Field `json:"fields"`
}

type manifest struct {
	Format   string  `json:"format"`
	BundleID string  `json:"bundle_id"`
	Revision string  `json:"revision"`
	Fields   []Field `json:"fields"`
	digest   string
}

type fieldPolicy struct {
	visibility string
	multiline  bool
}

var standardFields = map[string]fieldPolicy{
	"password": {"secret", false}, "username": {"public", false}, "url": {"public", false},
	"email": {"public", false}, "notes": {"public", true}, "host": {"public", false},
	"port": {"public", false}, "database": {"public", false}, "engine": {"public", false},
	"dsn": {"public", false}, "client_id": {"public", false}, "jump_host": {"public", false},
	"api_key": {"secret", false}, "token": {"secret", false}, "client_secret": {"secret", false},
	"private_key": {"secret", true}, "passphrase": {"secret", false},
	"sudo_password": {"secret", false}, "totp_secret": {"secret", false},
	"recovery_codes": {"secret", true},
}

func (g *Gopass) Fields(ctx context.Context, entryPath string) (FieldSet, error) {
	m, err := g.loadManifest(ctx, entryPath)
	if err != nil {
		if errors.Is(err, ErrEntryNotFound) {
			return g.legacyFields(ctx, entryPath)
		}
		return FieldSet{}, err
	}
	result := FieldSet{Revision: m.digest, Fields: make([]Field, len(m.Fields))}
	totalPublicBytes := 0
	for i, field := range m.Fields {
		result.Fields[i] = field
		valuePath := fieldValuePath(m, field.ID)
		if err := g.requireMember(ctx, valuePath); err != nil {
			return FieldSet{}, ErrInvalidManifest
		}
		if field.Visibility == "public" {
			value, err := g.outputBounded(ctx, maxPublicValueBytes, "show", "--noparsing", "--", valuePath)
			// gopass stores inserted values with one terminating newline.
			value = bytes.TrimSuffix(value, []byte("\n"))
			if err != nil || !validFieldValue(value, field.Multiline) {
				return FieldSet{}, ErrInvalidManifest
			}
			totalPublicBytes += len(value)
			if totalPublicBytes > maxPublicFieldsBytes {
				return FieldSet{}, ErrInvalidManifest
			}
			result.Fields[i].Value = string(value)
		}
	}
	return result, nil
}

func (g *Gopass) legacyFields(ctx context.Context, entryPath string) (FieldSet, error) {
	if err := g.requireMember(ctx, entryPath); err != nil {
		return FieldSet{}, err
	}
	// Legacy gopass records have no field manifest. Read the record only for
	// this explicit card request, derive its non-empty named fields, and keep
	// unknown values secret. This compatibility path never persists the record.
	raw, err := g.outputBounded(ctx, maxPublicFieldsBytes, "show", "--noparsing", "--", entryPath)
	if err != nil {
		return FieldSet{}, err
	}
	return FieldSet{Revision: "", Fields: parseLegacyFields(raw)}, nil
}

func parseLegacyFields(raw []byte) []Field {
	lines := strings.Split(string(raw), "\n")
	if len(lines) == 0 {
		return nil
	}
	fields := make([]Field, 0, len(lines))
	if lines[0] != "" {
		fields = append(fields, Field{ID: "legacy-password", Name: "Password", Kind: "password", Visibility: "secret"})
	}
	seen := map[string]bool{"Password": true}
	for _, line := range lines[1:] {
		name, value, ok := strings.Cut(line, ":")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !ok || value == "" || !validDisplayName(name) || seen[name] {
			continue
		}
		seen[name] = true
		kind := strings.ToLower(strings.ReplaceAll(name, " ", "_"))
		policy, standard := standardFields[kind]
		field := Field{ID: "legacy-" + legacyFieldID(name), Name: name, Kind: "custom", Visibility: "secret"}
		if standard {
			field.Kind, field.Visibility, field.Multiline = kind, policy.visibility, policy.multiline
		}
		if field.Visibility == "public" && validFieldValue([]byte(value), field.Multiline) {
			field.Value = value
		}
		fields = append(fields, field)
	}
	return fields
}

func legacyFieldID(name string) string {
	sum := sha256.Sum256([]byte(name))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

func (g *Gopass) ResolveField(ctx context.Context, entryPath, revision, fieldID string) (string, error) {
	if !validDigest(revision) || !validOpaqueID(fieldID) {
		return "", ErrInvalidManifest
	}
	m, err := g.loadManifest(ctx, entryPath)
	if err != nil {
		return "", err
	}
	if m.digest != revision {
		return "", ErrInvalidManifest
	}
	for _, field := range m.Fields {
		if field.ID == fieldID {
			valuePath := fieldValuePath(m, field.ID)
			if err := g.requireMember(ctx, valuePath); err != nil {
				return "", ErrInvalidManifest
			}
			return valuePath, nil
		}
	}
	return "", ErrInvalidManifest
}

func (g *Gopass) loadManifest(ctx context.Context, entryPath string) (manifest, error) {
	if strings.HasPrefix(entryPath, ReservedFieldPrefix) {
		return manifest{}, ErrInvalidManifest
	}
	if err := g.requireMember(ctx, entryPath); err != nil {
		return manifest{}, err
	}
	manifestPath, err := fieldManifestPath(entryPath)
	if err != nil {
		return manifest{}, ErrInvalidManifest
	}
	if err := g.requireMember(ctx, manifestPath); err != nil {
		return manifest{}, err
	}
	raw, err := g.outputBounded(ctx, maxManifestBytes, "show", "--noparsing", "--", manifestPath)
	if err != nil || len(raw) == 0 || len(raw) > maxManifestBytes || !utf8.Valid(raw) {
		return manifest{}, ErrInvalidManifest
	}
	if err := rejectDuplicateKeys(raw); err != nil {
		return manifest{}, ErrInvalidManifest
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var m manifest
	if err := decoder.Decode(&m); err != nil {
		return manifest{}, ErrInvalidManifest
	}
	if err := ensureJSONEOF(decoder); err != nil || !validManifest(m) {
		return manifest{}, ErrInvalidManifest
	}
	sum := sha256.Sum256(raw)
	m.digest = base64.RawURLEncoding.EncodeToString(sum[:])
	return m, nil
}

func fieldManifestPath(entryPath string) (string, error) {
	id, err := protocol.EncodeCanonicalPath(entryPath)
	if err != nil {
		return "", err
	}
	return ReservedFieldPrefix + "v1/manifests/" + string(id), nil
}

func (g *Gopass) outputBounded(ctx context.Context, limit int, args ...string) ([]byte, error) {
	if runner, ok := g.runner.(interface {
		OutputLimit(context.Context, int, string, ...string) ([]byte, error)
	}); ok {
		return runner.OutputLimit(ctx, limit, gopassExecutable, args...)
	}
	out, err := g.runner.Output(ctx, gopassExecutable, args...)
	if err != nil {
		return nil, err
	}
	if len(out) > limit {
		return nil, ErrOutputTooLarge
	}
	return out, nil
}

func validDigest(value string) bool {
	if len(value) != 43 {
		return false
	}
	b, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(b) == sha256.Size && base64.RawURLEncoding.EncodeToString(b) == value
}

func validManifest(m manifest) bool {
	if m.Format != FieldFormat || !validOpaqueID(m.BundleID) || !validOpaqueID(m.Revision) || len(m.Fields) < 1 || len(m.Fields) > 64 {
		return false
	}
	ids, names, kinds := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, f := range m.Fields {
		if !validOpaqueID(f.ID) || !validDisplayName(f.Name) || ids[f.ID] || names[f.Name] || f.Value != "" {
			return false
		}
		ids[f.ID], names[f.Name] = true, true
		if f.Visibility != "public" && f.Visibility != "secret" {
			return false
		}
		if f.Kind == "custom" {
			continue
		}
		p, ok := standardFields[f.Kind]
		if !ok || kinds[f.Kind] || p.visibility != f.Visibility || p.multiline != f.Multiline {
			return false
		}
		kinds[f.Kind] = true
	}
	return true
}

func validOpaqueID(value string) bool {
	if len(value) != 22 {
		return false
	}
	b, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(b) == 16 && base64.RawURLEncoding.EncodeToString(b) == value
}

func validDisplayName(value string) bool {
	if len(value) < 1 || len(value) > 64 || !utf8.ValidString(value) {
		return false
	}
	if strings.ContainsAny(value, ";&|$`'\"<>(){}[]*?!\r\n\t") {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return false
		}
	}
	return true
}

func validFieldValue(value []byte, multiline bool) bool {
	if len(value) < 1 || len(value) > maxPublicValueBytes || !utf8.Valid(value) || bytes.IndexByte(value, 0) >= 0 {
		return false
	}
	for _, r := range string(value) {
		if r == '\r' || (!multiline && r == '\n') || (unicode.IsControl(r) && r != '\n' && r != '\t') {
			return false
		}
	}
	return true
}

func fieldValuePath(m manifest, fieldID string) string {
	return fmt.Sprintf("%sv1/%s/%s/%s", ReservedFieldPrefix, m.BundleID, m.Revision, fieldID)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ErrInvalidManifest
	}
	return nil
}

func rejectDuplicateKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		tok, err := decoder.Token()
		if err != nil {
			return err
		}
		switch d := tok.(type) {
		case json.Delim:
			switch d {
			case '{':
				seen := map[string]bool{}
				for decoder.More() {
					k, err := decoder.Token()
					if err != nil {
						return err
					}
					key, ok := k.(string)
					if !ok || seen[key] {
						return ErrInvalidManifest
					}
					seen[key] = true
					if err := walk(); err != nil {
						return err
					}
				}
				_, err = decoder.Token()
				return err
			case '[':
				for decoder.More() {
					if err := walk(); err != nil {
						return err
					}
				}
				_, err = decoder.Token()
				return err
			}
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if decoder.More() {
		return ErrInvalidManifest
	}
	return nil
}
