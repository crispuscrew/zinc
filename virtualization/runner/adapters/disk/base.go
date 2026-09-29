package disk

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// VerifyBase's cache is only a performance optimization, not a trust database.
func VerifyBase(base, digest string) error {
	info, err := os.Stat(base)
	if err != nil {
		return fmt.Errorf("base image %s: %w", base, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("base image %s: not a regular file", base)
	}
	if err := checkSelfContained(base); err != nil {
		return err
	}
	current := identify(info)
	if cached, ok := readSidecar(base); current != "" && ok && cached.Digest == digest && cached.Identity == current {
		return nil
	}
	sum, err := fileDigest(base)
	if err != nil {
		return err
	}
	if sum != digest {
		return fmt.Errorf("base image %s does not match the pinned digest\n  authorised: %s\n  on disk: %s\nupdate runtime options explicitly if this change was intended", base, digest, sum)
	}
	after, err := os.Stat(base)
	if err != nil || identify(after) != current {
		return fmt.Errorf("base image changed while hashing; retry after its writer has finished")
	}
	writeSidecar(base, sidecar{Identity: current, Digest: sum})
	return nil
}

const (
	qcowMagicLen        = 4
	qcowBackingOffsetAt = 8
	qcowIncompatibleAt  = 72
	qcowHeaderProbe     = 80
	qcowExternalDataBit = 1 << 2
)

var qcowMagic = []byte{'Q', 'F', 'I', 0xfb}

func checkSelfContained(base string) error {
	file, err := os.Open(base)
	if err != nil {
		return err
	}
	defer file.Close()
	header := make([]byte, qcowHeaderProbe)
	read, err := io.ReadFull(file, header)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return err
	}
	if read < qcowHeaderProbe || !bytes.Equal(header[:qcowMagicLen], qcowMagic) {
		return nil
	}
	refuse := func(what string) error {
		return fmt.Errorf("base image %s declares %s; a digest covers only this file, so flatten it to a self-contained qcow2 and explicitly re-pin", base, what)
	}
	if binary.BigEndian.Uint64(header[qcowBackingOffsetAt:]) != 0 {
		return refuse("a backing file")
	}
	if binary.BigEndian.Uint32(header[qcowMagicLen:]) >= 3 && binary.BigEndian.Uint64(header[qcowIncompatibleAt:])&qcowExternalDataBit != 0 {
		return refuse("an external data file")
	}
	return nil
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s: not a regular file", path)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func Digest(path string) (string, error) { return fileDigest(path) }
