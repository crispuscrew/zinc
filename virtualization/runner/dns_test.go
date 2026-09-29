package main

import (
	"strings"
	"testing"
)

func TestDNSProxyDispatchDoesNotLoadAppStore(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/unavailable/configuration")
	err := run([]string{"dns-proxy"})
	if err == nil || !strings.Contains(err.Error(), "usage: dns-proxy") {
		t.Fatalf("DNS command was not dispatched: %v", err)
	}
}
