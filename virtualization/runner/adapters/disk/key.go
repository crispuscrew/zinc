package disk

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

func publicKey(path string) (string, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s: public key must be a regular file", path)
	}
	const maximum = 64 << 10
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || len(data) > maximum {
		return "", fmt.Errorf("read public SSH key %s: invalid or oversized input", path)
	}
	key := strings.TrimSpace(string(data))
	if strings.Contains(key, "PRIVATE KEY") {
		return "", fmt.Errorf("%s contains a PRIVATE key; ImageMeta.PublicSSHKeyPath requires a public key", path)
	}
	fields := strings.Fields(key)
	if len(fields) < 2 || strings.ContainsAny(key, "\n\r\x00") {
		return "", fmt.Errorf("%s: require one OpenSSH public key", path)
	}
	switch fields[0] {
	case "ssh-ed25519", "ssh-rsa", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521":
	default:
		return "", fmt.Errorf("%s: unsupported SSH public-key algorithm", path)
	}
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil || len(blob) < 4 {
		return "", fmt.Errorf("%s: invalid SSH public-key encoding", path)
	}
	length := int(binary.BigEndian.Uint32(blob[:4]))
	if length > len(blob)-4 || string(blob[4:4+length]) != fields[0] || len(blob) <= 8+length {
		return "", fmt.Errorf("%s: invalid SSH public-key payload", path)
	}
	return key, nil
}
