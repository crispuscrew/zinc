//go:build e2e

package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"testing"
)

func fileDigest(check *testing.T, path string) string {
	check.Helper()
	file, err := os.Open(path)
	if err != nil {
		check.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		check.Fatal(err)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}
