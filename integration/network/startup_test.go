package network_test

import (
	"testing"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
)

func TestLiveMalformedRulesRollbackToDeny(test *testing.T) {
	lab := newLab(test, false)
	startServer(test, lab.server, "tcp", lab.server4, 8080)
	requireControl(test, lab.client, "tcp", lab.client4, lab.server4)
	_, manifest := lab.fixture(test, nil, nil)
	if _, err := command(lab.policyNS(), "this is not nft syntax\n", "sh", "-c", provision.ApplyScript(manifest)); err == nil {
		test.Fatal("invalid ruleset did not fail initialization")
	}
	result := runProbe(test, lab.client, "tcp", lab.client4, lab.server4, nextPort(), 8080)
	verifyProbe(test, result, false)
	values := counters(test, lab)
	if packets(values, "default") == 0 {
		test.Fatalf("rollback did not install default-deny: %+v", values)
	}
	test.Logf("real failed nft load left TCP blocked; default drop packets=%d", packets(values, "default"))
}
