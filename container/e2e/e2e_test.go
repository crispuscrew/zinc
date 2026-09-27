//go:build e2e

// Package e2e drives real binaries against rootless Podman.
package e2e

import "testing"

func TestE2E(t *testing.T) {
	environment := setup(t)
	t.Run("authoring", environment.authoring)
	t.Run("lifecycle", environment.lifecycle)
	t.Run("containment", environment.containment)
	t.Run("scratch_volumes", environment.scratch)
	t.Run("dependencies", environment.dependencies)
	t.Run("attached_reopen", environment.attached)
	t.Run("network_requires_provisioning", environment.missingNetwork)
	t.Run("network_enforcement_and_counters", environment.network)
	t.Run("dbus", environment.bus)
}
