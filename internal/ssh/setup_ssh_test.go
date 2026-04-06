package ssh

import (
	"os"
	"path/filepath"
	"testing"
)

// withFakeHome points os.UserHomeDir() at a temp directory for the test and
// returns that directory. On all supported platforms UserHomeDir consults
// HOME (unix) or USERPROFILE (windows), both of which t.Setenv covers.
func withFakeHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".ssh"), 0700); err != nil {
		t.Fatalf("mkdir .ssh: %v", err)
	}
	return dir
}

func writeKey(t *testing.T, home, name, contents string) {
	t.Helper()
	p := filepath.Join(home, ".ssh", name)
	if err := os.WriteFile(p, []byte(contents), 0600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestDefaultPublicKey_PrefersEd25519(t *testing.T) {
	home := withFakeHome(t)
	writeKey(t, home, "id_ed25519.pub", "ssh-ed25519 AAAAED25519 user@host\n")
	writeKey(t, home, "id_rsa.pub", "ssh-rsa AAAARSA user@host\n")
	writeKey(t, home, "id_ecdsa.pub", "ecdsa-sha2-nistp256 AAAAECDSA user@host\n")

	path, key, err := DefaultPublicKey()
	if err != nil {
		t.Fatalf("DefaultPublicKey: %v", err)
	}
	if filepath.Base(path) != "id_ed25519.pub" {
		t.Errorf("picked %q, want id_ed25519.pub", path)
	}
	if key != "ssh-ed25519 AAAAED25519 user@host" {
		t.Errorf("key = %q, want trimmed ed25519 key", key)
	}
}

func TestDefaultPublicKey_FallsBackToRSA(t *testing.T) {
	home := withFakeHome(t)
	writeKey(t, home, "id_rsa.pub", "ssh-rsa AAAARSA user@host\n")

	path, key, err := DefaultPublicKey()
	if err != nil {
		t.Fatalf("DefaultPublicKey: %v", err)
	}
	if filepath.Base(path) != "id_rsa.pub" {
		t.Errorf("picked %q, want id_rsa.pub", path)
	}
	if key != "ssh-rsa AAAARSA user@host" {
		t.Errorf("key = %q", key)
	}
}

func TestDefaultPublicKey_FallsBackToECDSA(t *testing.T) {
	home := withFakeHome(t)
	writeKey(t, home, "id_ecdsa.pub", "ecdsa-sha2-nistp256 AAAAECDSA user@host\n")

	path, _, err := DefaultPublicKey()
	if err != nil {
		t.Fatalf("DefaultPublicKey: %v", err)
	}
	if filepath.Base(path) != "id_ecdsa.pub" {
		t.Errorf("picked %q, want id_ecdsa.pub", path)
	}
}

func TestDefaultPublicKey_NoKeys(t *testing.T) {
	withFakeHome(t)
	_, _, err := DefaultPublicKey()
	if err == nil {
		t.Fatal("expected error when no keys exist, got nil")
	}
}

func TestDefaultPublicKey_TrimsWhitespace(t *testing.T) {
	home := withFakeHome(t)
	// Some editors append trailing newlines / CRLF; result must be trimmed
	// because the key is embedded into a PowerShell single-quoted string.
	writeKey(t, home, "id_ed25519.pub", "\nssh-ed25519 AAAAED25519 user@host  \r\n")

	_, key, err := DefaultPublicKey()
	if err != nil {
		t.Fatalf("DefaultPublicKey: %v", err)
	}
	want := "ssh-ed25519 AAAAED25519 user@host"
	if key != want {
		t.Errorf("key = %q, want %q", key, want)
	}
}
