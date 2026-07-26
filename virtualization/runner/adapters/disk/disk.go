// Package disk prepares what a guest boots from: a copy-on-write overlay over the pinned
// base image, and the cloud-init seed that gives a fresh guest its identity. Both are
// built by shelling out to the tools that own those formats (qemu-img, xorriso) rather
// than by writing qcow2 and ISO9660 by hand.
package disk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// EnsureOverlay makes sure the app has a writable disk backed by its pinned base, and
// that the base is still the one that was authorised. The overlay is created once and
// then left alone: it holds everything the guest has ever written.
func EnsureOverlay(base, digest, overlay string, sizeGiB int64) error {
	if err := VerifyBase(base, digest); err != nil {
		return err
	}
	if _, err := os.Stat(overlay); err == nil {
		return nil // the guest's disk already exists; never re-create it, that is its data
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", overlay, err)
	}

	// -F qcow2 states the backing file's format explicitly. Without it qemu-img probes,
	// and a probed backing format is a known way to confuse the format detection of
	// whatever opens the overlay next.
	args := []string{"create", "-f", "qcow2", "-F", "qcow2", "-b", base, overlay}
	if sizeGiB > 0 {
		args = append(args, strconv.FormatInt(sizeGiB, 10)+"G")
	}
	if output, err := exec.Command("qemu-img", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("create the app's disk overlay: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// VerifyBase checks the base image still hashes to the digest the config pins. A base is
// hashed in full the first time it is used, and after that only when the file looks like
// it changed - hashing a multi-gigabyte image on every launch would add seconds, or for a
// large base tens of seconds, to every start.
//
// "Looks like it changed" is deliberately broad: the identity recorded is the filesystem's
// (device, inode), the size, and BOTH timestamps. mtime alone is too weak twice over - it
// has coarse granularity on some filesystems, so a same-size replacement in the same tick
// would slip through, and it can be set to anything with utimes. ctime cannot: it moves
// whenever the inode does, including when someone rewinds mtime.
//
// What this does and does not buy, plainly: it reliably catches a base that was replaced,
// rebuilt, moved or restored, which is how a pin actually goes stale. It is not a defence
// against someone who can already write to the image directory, because they can rewrite
// this sidecar too. The pin's real strength is that the config names one exact image and
// zvr refuses to boot anything else.
func VerifyBase(base, digest string) error {
	info, err := os.Stat(base)
	if err != nil {
		return fmt.Errorf("base image %s: %w", base, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("base image %s: not a regular file", base)
	}

	current := identify(info)
	if cached, ok := readSidecar(base); ok && cached.Digest == digest && cached.Identity == current {
		return nil
	}

	sum, err := fileDigest(base)
	if err != nil {
		return err
	}
	if sum != digest {
		return fmt.Errorf("base image %s does not match the pinned digest\n  authorised: %s\n  on disk:    %s\nthe image was replaced or rebuilt; re-pin it in the app config if that was intended",
			base, digest, sum)
	}
	writeSidecar(base, sidecar{Identity: current, Digest: sum})
	return nil
}

// identify fingerprints a file's identity and both timestamps, at nanosecond resolution
// where the filesystem keeps it. Any ordinary write, replacement or restore moves at least
// one of these fields.
func identify(info os.FileInfo) string {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		// No stat details available: return a value that can never match a stored one, so
		// the image is simply re-hashed rather than trusted on weaker evidence.
		return ""
	}
	return fmt.Sprintf("%d:%d:%d:%d.%09d:%d.%09d",
		stat.Dev, stat.Ino, info.Size(),
		stat.Mtim.Sec, stat.Mtim.Nsec,
		stat.Ctim.Sec, stat.Ctim.Nsec)
}

// sidecar remembers a verified image so an unchanged one is not re-hashed on every boot.
type sidecar struct {
	Identity string `json:"identity"`
	Digest   string `json:"digest"`
}

func sidecarPath(base string) string {
	return filepath.Join(filepath.Dir(base), "."+filepath.Base(base)+".zinc-digest")
}

func readSidecar(base string) (sidecar, bool) {
	data, err := os.ReadFile(sidecarPath(base))
	if err != nil {
		return sidecar{}, false
	}
	var cached sidecar
	if err := json.Unmarshal(data, &cached); err != nil {
		return sidecar{}, false
	}
	return cached, true
}

// writeSidecar is best-effort: a read-only image directory means re-hashing next time,
// which is slower but never wrong, so a failure here is not worth failing a launch over.
func writeSidecar(base string, entry sidecar) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = os.WriteFile(sidecarPath(base), data, 0o644)
}

// fileDigest returns the file's content hash as "sha256:<hex>".
func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

// Digest is the public form of fileDigest, so `zvr pin` can tell an author what to write
// into a config.
func Digest(path string) (string, error) { return fileDigest(path) }

// WriteSeed builds the cloud-init seed ISO for an app: a tiny read-only filesystem
// labelled cidata that the guest's cloud-init finds on first boot and provisions itself
// from. Rebuilt on every launch, so editing the config's identity fields takes effect
// without touching the guest's disk.
func WriteSeed(path string, cfg schema.AppConfig) error {
	userData, err := userData(cfg)
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp("", "zinc-seed-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	metaData, err := metaData(cfg)
	if err != nil {
		return err
	}
	for name, content := range map[string]string{"user-data": userData, "meta-data": metaData} {
		if err := os.WriteFile(filepath.Join(stage, name), []byte(content), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	// -volid cidata is the whole contract: cloud-init's NoCloud source looks for a volume
	// with exactly that label. Joliet and Rock Ridge keep the names readable to any guest.
	args := []string{
		"-as", "mkisofs", "-output", path, "-volid", "cidata", "-joliet", "-rock",
		filepath.Join(stage, "user-data"), filepath.Join(stage, "meta-data"),
	}
	if output, err := exec.Command("xorriso", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("build the cloud-init seed image: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// metaData renders the seed's meta-data document as JSON. cloud-init documents this file
// as YAML, and JSON is valid YAML, so emitting JSON satisfies both the full implementation
// and the cut-down ones: cirros parses meta-data strictly as JSON and rejects a plain
// YAML mapping outright, which was found by booting one.
//
// instance-id is what cloud-init uses to decide whether it has already provisioned this
// guest, so keeping it stable per app means a rebuilt seed does not re-run first-boot
// steps on a disk that already has them. public-keys is the classic EC2-style field, read
// by implementations that never look at user-data's users list.
func metaData(cfg schema.AppConfig) (string, error) {
	document := map[string]any{
		"instance-id":    "zinc-" + cfg.AppNameID,
		"local-hostname": cfg.AppNameID,
	}
	init := cfg.VirtualizationMeta.CloudInit
	if init.SSHKeyPath != "" {
		key, err := publicKey(init.SSHKeyPath)
		if err != nil {
			return "", err
		}
		document["public-keys"] = []string{key}
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded) + "\n", nil
}

// publicKey reads an authorised key, refusing a private one. The config check screens the
// path; this screens the bytes, because the file behind a .pub name is not guaranteed to
// be what the name says - and the seed is handed to the guest.
func publicKey(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read the public key %s: %w", path, err)
	}
	key := strings.TrimSpace(string(data))
	if strings.Contains(key, "PRIVATE KEY") {
		return "", fmt.Errorf("%s contains a PRIVATE key; VirtualizationMeta.CloudInit.SSHKeyPath must be a public key", path)
	}
	return key, nil
}

// userData renders the cloud-config document. The app's Install steps become runcmd
// lines, which is the VM reading of the same field a container turns into its derived
// image's RUN layer: what to add on top of the pinned base.
func userData(cfg schema.AppConfig) (string, error) {
	init := cfg.VirtualizationMeta.CloudInit
	var doc strings.Builder
	doc.WriteString("#cloud-config\n")
	doc.WriteString("hostname: " + cfg.AppNameID + "\n")

	if init.UserName != "" {
		key := ""
		if init.SSHKeyPath != "" {
			authorised, err := publicKey(init.SSHKeyPath)
			if err != nil {
				return "", err
			}
			key = authorised
		}
		doc.WriteString("users:\n")
		doc.WriteString("  - name: " + init.UserName + "\n")
		doc.WriteString("    sudo: ALL=(ALL) NOPASSWD:ALL\n")
		doc.WriteString("    shell: /bin/bash\n")
		if key != "" {
			doc.WriteString("    ssh_authorized_keys:\n")
			doc.WriteString("      - " + yamlScalar(key) + "\n")
		}
	}

	if len(cfg.ImageMeta.Install) > 0 {
		doc.WriteString("runcmd:\n")
		for _, step := range cfg.ImageMeta.Install {
			doc.WriteString("  - " + yamlScalar(step) + "\n")
		}
	}
	return doc.String(), nil
}

// yamlScalar quotes a value so it cannot be read as YAML structure. Validation already
// rejects control characters in these fields, so single-quoting (with the doubling YAML
// requires) is enough to keep a value a value.
func yamlScalar(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
