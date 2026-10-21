// Package update updates the server build of Kero (cmd/kero-server) in
// place: it downloads the archive of a version from Kero's releases,
// checks it against the key the app's own updates are signed with, and
// puts it where the running one is, which a restart of kerod then runs.
package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// PublicKey checks Kero's releases: mygo.json's updates.publicKey, whose
// private key signs the app's updates and the server build's archives.
const PublicKey = "WzJsOGNIuf6mcEqo5ff8jub+NoQQOEk4JXreLPYjgyQ="

// Repo is the GitHub repository of Kero's releases.
const Repo = "egoist/kero"

// maxArchive bounds an archive's download.
const maxArchive = 256 << 20

// Asset is the name of the server build's archive of a version for a
// platform; its signature is Asset+".sig", its SHA-256 Asset+".sha256".
func Asset(version, goos, goarch string) string {
	return fmt.Sprintf("kero-server-%s-%s-%s.tar.gz", version, goos, goarch)
}

// releases is where the releases are: $KERO_RELEASES, for tests and
// mirrors, else GitHub's.
func releases() string {
	if r := os.Getenv("KERO_RELEASES"); r != "" {
		return strings.TrimSuffix(r, "/")
	}
	return "https://github.com/" + Repo + "/releases/download"
}

// URL is where a file of a version's release is.
func URL(version, name string) string {
	return releases() + "/v" + strings.TrimPrefix(version, "v") + "/" + name
}

// InstallScript is the URL of the script that installs a version of the
// server build: `curl -fsSL <it> | sh`.
func InstallScript(version string) string {
	if version == "" {
		return "https://github.com/" + Repo + "/releases/latest/download/install-server.sh"
	}
	return URL(version, "install-server.sh")
}

var httpClient = &http.Client{Timeout: 5 * time.Minute}

func get(ctx context.Context, url string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("%s: too large", url)
	}
	return b, nil
}

// Latest is the version of Kero's latest release.
func Latest(ctx context.Context) (string, error) {
	b, err := get(ctx, "https://api.github.com/repos/"+Repo+"/releases/latest", 1<<20)
	if err != nil {
		return "", err
	}
	var r struct {
		Tag string `json:"tag_name"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return "", err
	}
	if r.Tag == "" {
		return "", errors.New("no release of Kero is published")
	}
	return strings.TrimPrefix(r.Tag, "v"), nil
}

// Verify checks an archive against its signature: the Ed25519 signature
// of its SHA-256 digest, base64, as mygo's updates have.
func Verify(archive []byte, sig string) error {
	key, err := base64.StdEncoding.DecodeString(PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errors.New("update: bad public key")
	}
	s, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sig))
	digest := sha256.Sum256(archive)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(key), digest[:], s) {
		return errors.New("update: the archive's signature is not Kero's")
	}
	return nil
}

// Sign signs an archive as Verify checks it, with a private key as mygo
// keeps it (MYGO_UPDATER_PRIVATE_KEY): base64 of the Ed25519 key.
func Sign(archive []byte, privateKey string) (string, error) {
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privateKey))
	if err != nil || len(k) != ed25519.PrivateKeySize {
		return "", errors.New("update: bad private key")
	}
	digest := sha256.Sum256(archive)
	return base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(k), digest[:])), nil
}

// Install puts the server build of version, for this platform, in dir,
// in place of what it holds: the directory of the running kero, which
// keeps running the old one until it restarts.
func Install(ctx context.Context, version, dir string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("the server build of Kero is for Linux, not %s", runtime.GOOS)
	}
	version = strings.TrimPrefix(version, "v")
	if version == "" {
		v, err := Latest(ctx)
		if err != nil {
			return err
		}
		version = v
	}
	name := Asset(version, runtime.GOOS, runtime.GOARCH)
	archive, err := get(ctx, URL(version, name), maxArchive)
	if err != nil {
		return fmt.Errorf("downloading Kero %s: %w", version, err)
	}
	sig, err := get(ctx, URL(version, name+".sig"), 4096)
	if err != nil {
		return fmt.Errorf("downloading the signature of Kero %s: %w", version, err)
	}
	if err := Verify(archive, string(sig)); err != nil {
		return err
	}
	return Replace(archive, dir)
}

// Replace puts what the archive holds in place of dir, as a whole, so
// that no file of the old version stays: into a directory beside it
// first, then by renames.
func Replace(archive []byte, dir string) error {
	dir = filepath.Clean(dir)
	staging := dir + ".new"
	old := dir + ".old"
	os.RemoveAll(staging)
	os.RemoveAll(old)
	if err := extract(archive, staging); err != nil {
		os.RemoveAll(staging)
		return err
	}
	if st, err := os.Stat(filepath.Join(staging, "kero")); err != nil || runtime.GOOS != "windows" && st.Mode()&0o111 == 0 {
		os.RemoveAll(staging)
		return errors.New("update: the archive holds no kero")
	}
	if err := os.Rename(dir, old); err != nil {
		os.RemoveAll(staging)
		return err
	}
	if err := os.Rename(staging, dir); err != nil {
		os.Rename(old, dir)
		return err
	}
	os.RemoveAll(old)
	return nil
}

// extract unpacks a .tar.gz into dir: its regular files and directories,
// nothing outside dir.
func extract(archive []byte, dir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	tr := tar.NewReader(gz)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("update: %w", err)
		}
		name := filepath.Clean(filepath.FromSlash(h.Name))
		if name == "." {
			continue
		}
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("update: %s is outside the archive", h.Name)
		}
		path := filepath.Join(dir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode)&0o755)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, io.LimitReader(tr, maxArchive))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		}
	}
}
