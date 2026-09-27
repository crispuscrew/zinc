// Package nftrules renders ordered, default-deny policy for resolved endpoints.
package nftrules

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

const TableName = "zinc"

// DefaultDrop never depends on the number or polarity of rules.
func DefaultDrop(schema.AppConfig) bool { return true }

// Render is the fail-closed compatibility API. Identity cannot be inferred from
// config alone, so consumers needing connectivity must use RenderResolved.
func Render(cfg schema.AppConfig) string {
	if len(cfg.NetworkMeta.Interfaces) == 0 {
		return ""
	}
	return DenyAll()
}

func DenyAll() string {
	var result strings.Builder
	result.WriteString("table inet zinc\ndelete table inet zinc\ntable inet zinc {\n")
	for _, hook := range []string{"input", "output", "forward"} {
		fmt.Fprintf(&result, " chain %s { type filter hook %s priority 0; policy drop; counter drop comment \"default\"; }\n", hook, hook)
	}
	result.WriteString("}\n")
	return result.String()
}

func RenderResolved(resolved network.Resolved) (string, error) {
	if err := network.Validate(resolved); err != nil {
		return "", err
	}
	if len(resolved.Config.NetworkMeta.Interfaces) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(resolved)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	mark := binary.BigEndian.Uint32(digest[:4]) | 1
	var result strings.Builder
	result.WriteString("table inet zinc\ndelete table inet zinc\ntable inet zinc {\n")
	writeSets(&result, resolved)
	for _, hook := range []string{"input", "output", "forward"} {
		active := (resolved.Topology.Mode == network.Container && hook != "forward") || (resolved.Topology.Mode == network.VirtualMachine && hook == "forward")
		if active {
			writePolicy(&result, resolved, resolved.Config.AppNameID, resolved.Config.NetworkMeta, hook, "owner_"+hook)
			for index, peer := range resolved.Topology.Peers {
				writePolicy(&result, resolved, peer.AppNameID, peer.Policy, hook, fmt.Sprintf("peer_%d_%s", index, hook))
			}
		}
		writeChain(&result, resolved, hook, active, mark)
	}
	result.WriteString("}\n")
	if resolved.Topology.Mode == network.VirtualMachine {
		writeLinkGuards(&result, resolved)
	}
	return result.String(), nil
}

func counted(verdict, label string) string {
	return fmt.Sprintf("counter %s comment %q", verdict, label)
}
