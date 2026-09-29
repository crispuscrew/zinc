package app

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

// Resolve DNS and compile policy before preparing disks, TPM or audio helpers.
// Rebuilding the command with actual private audio endpoints then uses exactly
// the same DNS snapshot, with no additional network requests or policy drift.
func (svc Service) freezeNetwork(plan *launchPlan) error {
	lookup := plan.Lookup
	cache := map[string][]netip.Addr{}
	frozen := false
	plan.Lookup = func(meta schema.DNSMeta, name string) ([]netip.Addr, error) {
		body, err := json.Marshal(meta)
		if err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%x/%s", sha256.Sum256(body), name)
		if result, found := cache[key]; found {
			return slices.Clone(result), nil
		}
		if frozen {
			return nil, fmt.Errorf("domain lookup absent from approved launch snapshot")
		}
		if lookup == nil {
			return nil, fmt.Errorf("domain policy requires an explicitly configured DNS lookup")
		}
		result, err := lookup(meta, name)
		if err != nil {
			return nil, err
		}
		cache[key] = slices.Clone(result)
		return slices.Clone(result), nil
	}
	_, _, err := svc.networkCommand(*plan, qemu.Args(plan.RuntimeConfig, plan.Layout))
	frozen = true
	return err
}
