package dnsproxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// CheckReady authenticates the Unix peer and matches its live config and listeners.
// Readiness describes bound UDP+TCP listeners, not upstream reachability.
func CheckReady(controlSocket string, meta schema.DNSMeta, addresses []string) error {
	expected, err := listenerAddresses(addresses)
	if err != nil {
		return err
	}
	if err := validateControlPath(controlSocket); err != nil {
		return err
	}
	before, err := socketIdentity(controlSocket)
	if err != nil {
		return err
	}
	connection, err := net.DialTimeout("unix", controlSocket, attemptTimeout)
	if err != nil {
		return err
	}
	defer connection.Close()
	if err := checkPeer(connection.(*net.UnixConn)); err != nil {
		return err
	}
	after, err := socketIdentity(controlSocket)
	if err != nil || !os.SameFile(before, after) {
		return fmt.Errorf("DNS control socket changed during readiness check")
	}
	if err := connection.SetDeadline(time.Now().Add(attemptTimeout)); err != nil {
		return err
	}
	encoded, err := io.ReadAll(io.LimitReader(connection, 8193))
	if err != nil {
		return err
	}
	if len(encoded) > 8192 {
		return fmt.Errorf("DNS control response too large")
	}
	var actual status
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&actual); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing DNS control response")
	}
	slices.Sort(actual.Addresses)
	if actual.Version != 1 || !actual.Ready || actual.Digest != configDigest(meta) || !slices.Equal(actual.Addresses, expected) {
		return fmt.Errorf("DNS proxy is not ready with the required configuration and UDP/TCP listeners")
	}
	return nil
}
