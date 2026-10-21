package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The key Kero checks is the one mygo.json signs the app's updates with.
func TestPublicKeyIsTheApps(t *testing.T) {
	b, err := os.ReadFile("../../mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Updates struct {
			PublicKey string `json:"publicKey"`
		} `json:"updates"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Updates.PublicKey != PublicKey {
		t.Errorf("update.PublicKey is %q, mygo.json's %q", PublicKey, cfg.Updates.PublicKey)
	}
}

func TestVerify(t *testing.T) {
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	archive := []byte("an archive")
	sig, err := Sign(archive, base64.StdEncoding.EncodeToString(other))
	if err != nil {
		t.Fatal(err)
	}
	// Signed, but not with Kero's key.
	if err := Verify(archive, sig); err == nil {
		t.Error("an archive another key signed passes")
	}
	if err := Verify(archive, "not base64!"); err == nil {
		t.Error("a bad signature passes")
	}
	if _, err := Sign(archive, "short"); err == nil {
		t.Error("signed with a bad key")
	}
}

func tarball(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestReplace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kero")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "kero"), []byte("old"), 0o755)
	os.WriteFile(filepath.Join(dir, "stale.so"), []byte("old"), 0o644)
	if err := Replace(tarball(t, map[string]string{"kero": "new", "libghostty-vt.so": "lib"}), dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "kero")); string(b) != "new" {
		t.Errorf("kero is %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "stale.so")); err == nil {
		t.Error("a file of the old version stayed")
	}
	for _, leftover := range []string{dir + ".new", dir + ".old"} {
		if _, err := os.Stat(leftover); err == nil {
			t.Errorf("%s stayed", leftover)
		}
	}
	// An archive without kero, or with a path out of it, changes nothing.
	for _, bad := range []map[string]string{{"other": "x"}, {"kero": "x", "../escape": "x"}} {
		if err := Replace(tarball(t, bad), dir); err == nil {
			t.Errorf("replaced with %v", bad)
		}
		if b, _ := os.ReadFile(filepath.Join(dir, "kero")); string(b) != "new" {
			t.Errorf("a bad archive changed kero to %q", b)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape")); err == nil {
		t.Error("a file got out of the archive")
	}
}
