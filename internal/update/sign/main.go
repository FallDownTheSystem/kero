// Sign signs the server build's archives for a release, as the release
// workflow runs it: for each file, file.sig, which update.Verify checks,
// and file.sha256, which install-server.sh checks. The private key is
// $MYGO_UPDATER_PRIVATE_KEY, the one the app's updates are signed with.
//
//	go run ./internal/update/sign build/server/*.tar.gz
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"kero/internal/update"
)

func main() {
	key := os.Getenv("MYGO_UPDATER_PRIVATE_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "sign: MYGO_UPDATER_PRIVATE_KEY is not set")
		os.Exit(1)
	}
	for _, path := range os.Args[1:] {
		b, err := os.ReadFile(path)
		if err != nil {
			fail(err)
		}
		sig, err := update.Sign(b, key)
		if err != nil {
			fail(err)
		}
		if err := update.Verify(b, sig); err != nil {
			fail(fmt.Errorf("%s: the key is not the one Kero checks (update.PublicKey): %w", path, err))
		}
		sum := sha256.Sum256(b)
		if err := os.WriteFile(path+".sig", []byte(sig+"\n"), 0o644); err != nil {
			fail(err)
		}
		line := hex.EncodeToString(sum[:]) + "  " + filepath.Base(path) + "\n"
		if err := os.WriteFile(path+".sha256", []byte(line), 0o644); err != nil {
			fail(err)
		}
		fmt.Println("signed", path)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "sign:", err)
	os.Exit(1)
}
