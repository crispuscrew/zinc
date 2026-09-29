package disk

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func qcowHeader(version uint32, backingOffset, incompatible uint64) []byte {
	header := make([]byte, 80)
	copy(header, "QFI\xfb")
	binary.BigEndian.PutUint32(header[4:], version)
	binary.BigEndian.PutUint64(header[8:], backingOffset)
	binary.BigEndian.PutUint64(header[72:], incompatible)
	return header
}

func writeBase(t *testing.T, data []byte) (string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "base.qcow2")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := Digest(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, digest
}

func TestVerifyBaseRefusesExternalData(t *testing.T) {
	for _, test := range []struct {
		name    string
		header  []byte
		refusal string
	}{
		{"v2 backing", qcowHeader(2, 512, 0), "a backing file"},
		{"v3 backing", qcowHeader(3, 512, 0), "a backing file"},
		{"external data", qcowHeader(3, 0, 0x04), "an external data file"},
		{"different feature", qcowHeader(3, 0, 0x02), ""},
		{"self contained", qcowHeader(3, 0, 0), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, digest := writeBase(t, test.header)
			err := VerifyBase(path, digest)
			if test.refusal == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.refusal) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestVerifyBaseDetectsSameSizeReplacementAndRestoredMtime(t *testing.T) {
	path, digest := writeBase(t, []byte("original"))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyBase(path, digest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sidecarPath(path)); err != nil {
		t.Fatal("cache missing:", err)
	}
	if err := os.WriteFile(path, []byte("replaced"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBase(path, digest); err == nil || !strings.Contains(err.Error(), "does not match the pinned digest") {
		t.Fatalf("got %v", err)
	}
}

func TestVerifyBaseCacheCannotReplaceAuthorization(t *testing.T) {
	path, digest := writeBase(t, []byte("stable image"))
	for range 3 {
		if err := VerifyBase(path, digest); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBase(path, digest); err != nil {
		t.Fatal(err)
	}
	if VerifyBase(path, "sha256:"+strings.Repeat("b", 64)) == nil {
		t.Fatal("cached hash replaced configured authorization")
	}
}
