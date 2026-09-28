package backend

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	gpgExecutable             = "gpg"
	gpgConnectAgentExecutable = "gpg-connect-agent"
	maxGPGIDBytes             = 64 << 10
	maxRecipients             = 64
)

// KeyCached reports whether gpg-agent can decrypt for the root store without
// asking for a passphrase. It reads only key metadata and never decrypts, so a
// caller can avoid starting pinentry under a layer-shell surface. Recipients
// of nested .gpg-id files and mounts are not considered.
func (g *Gopass) KeyCached(ctx context.Context) (bool, error) {
	recipients, err := g.rootRecipients(ctx)
	if err != nil {
		return false, err
	}
	args := append([]string{"--batch", "--with-colons", "--with-keygrip", "--list-secret-keys", "--"}, recipients...)
	keys, err := g.runner.Output(ctx, gpgExecutable, args...)
	if err != nil {
		// No secret key for any recipient: decryption cannot succeed silently.
		if errors.Is(classifyError(ctx, err), ErrBackend) {
			return false, nil
		}
		return false, classifyError(ctx, err)
	}
	grips := encryptionKeygrips(keys)
	if len(grips) == 0 {
		return false, nil
	}
	// --no-autostart: an agent that is not running holds no cached key.
	agent, err := g.runner.Output(ctx, gpgConnectAgentExecutable, "--no-autostart", "KEYINFO --list", "/bye")
	if err != nil {
		if errors.Is(classifyError(ctx, err), ErrBackend) {
			return false, nil
		}
		return false, classifyError(ctx, err)
	}
	return anyKeyUsable(agent, grips), nil
}

func (g *Gopass) storeRoot(ctx context.Context) (string, error) {
	out, err := g.runner.Output(ctx, gopassExecutable, "config", "mounts.path")
	if err != nil {
		return "", classifyError(ctx, err)
	}
	root := strings.TrimSpace(string(out))
	if !filepath.IsAbs(root) || strings.ContainsAny(root, "\x00\n") {
		return "", ErrBackend
	}
	return filepath.Clean(root), nil
}

// requireSidecar confirms that a canonical reserved path is a regular
// encrypted file inside the root store; os.Root refuses symlink escapes.
func (g *Gopass) requireSidecar(ctx context.Context, entryPath string) error {
	root, err := g.storeRoot(ctx)
	if err != nil {
		return err
	}
	store, err := os.OpenRoot(root)
	if err != nil {
		return ErrBackend
	}
	defer store.Close()
	info, err := store.Lstat(filepath.FromSlash(entryPath) + ".gpg")
	switch {
	case errors.Is(err, os.ErrNotExist):
		return ErrEntryNotFound
	case err != nil || !info.Mode().IsRegular():
		return ErrBackend
	}
	return nil
}

func (g *Gopass) rootRecipients(ctx context.Context) ([]string, error) {
	root, err := g.storeRoot(ctx)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(filepath.Join(root, ".gpg-id"))
	if err != nil {
		return nil, ErrBackend
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxGPGIDBytes+1))
	if err != nil || len(raw) > maxGPGIDBytes {
		return nil, ErrBackend
	}
	return parseRecipients(raw)
}

func parseRecipients(raw []byte) ([]string, error) {
	var recipients []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "-") || strings.IndexFunc(line, unicode.IsControl) >= 0 {
			return nil, ErrBackend
		}
		recipients = append(recipients, line)
	}
	if len(recipients) == 0 || len(recipients) > maxRecipients {
		return nil, ErrBackend
	}
	return recipients, nil
}

// encryptionKeygrips returns keygrips of keys whose own capability field has
// lowercase "e"; uppercase flags on a primary key describe the whole key.
func encryptionKeygrips(colons []byte) map[string]bool {
	grips := map[string]bool{}
	encrypt := false
	for _, line := range strings.Split(string(colons), "\n") {
		f := strings.Split(strings.TrimSuffix(line, "\r"), ":")
		switch f[0] {
		case "sec", "ssb":
			encrypt = len(f) > 11 && strings.Contains(f[11], "e")
		case "grp":
			if encrypt && len(f) > 9 && f[9] != "" {
				grips[f[9]] = true
			}
			encrypt = false
		}
	}
	return grips
}

// anyKeyUsable parses "S KEYINFO <grip> <type> <serial> <idstr> <cached>
// <protection> ..." lines: cached "1" or protection "C" (no passphrase).
func anyKeyUsable(keyinfo []byte, grips map[string]bool) bool {
	for _, line := range strings.Split(string(keyinfo), "\n") {
		f := strings.Fields(line)
		if len(f) < 8 || f[0] != "S" || f[1] != "KEYINFO" || !grips[f[2]] {
			continue
		}
		if f[6] == "1" || f[7] == "C" {
			return true
		}
	}
	return false
}
