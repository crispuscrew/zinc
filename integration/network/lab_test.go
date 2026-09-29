package network_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var sequence atomic.Uint32

type laboratory struct {
	client, server, router             string
	clientDevice, serverDevice         string
	client4, server4, client6, server6 string
	routed                             bool
}

func command(namespace string, input string, argv ...string) ([]byte, error) {
	if namespace != "" {
		argv = append([]string{"nsenter", "--net=/run/netns/" + namespace, "--"}, argv...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, argv[0], argv[1:]...)
	process.Stdin = strings.NewReader(input)
	output, err := process.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%v: %w: %s", argv, err, output)
	}
	return output, nil
}

func requireCommand(test *testing.T, namespace, input string, argv ...string) []byte {
	test.Helper()
	output, err := command(namespace, input, argv...)
	if err != nil {
		test.Fatal(err)
	}
	return output
}

func newLab(test *testing.T, routed bool) laboratory {
	test.Helper()
	identifier := sequence.Add(1)
	lab := laboratory{client: fmt.Sprintf("zc%d", identifier), server: fmt.Sprintf("zs%d", identifier),
		clientDevice: "client0", serverDevice: "server0", routed: routed,
		client4: "192.0.2.10", server4: "192.0.2.20", client6: "2001:db8:1::10", server6: "2001:db8:1::20"}
	names := []string{lab.client, lab.server}
	if routed {
		lab.router = fmt.Sprintf("zr%d", identifier)
		lab.server4, lab.server6 = "198.51.100.20", "2001:db8:2::20"
		names = append(names, lab.router)
	}
	for _, namespace := range names {
		if namespace == lab.router {
			requireCommand(test, "", "", "ip", "netns", "attach", namespace, fmt.Sprint(os.Getpid()))
		} else {
			requireCommand(test, "", "", "ip", "netns", "add", namespace)
		}
		test.Cleanup(func() {
			if output, err := command("", "", "ip", "netns", "del", namespace); err != nil {
				test.Errorf("namespace cleanup: %v %s", err, output)
			}
		})
		requireCommand(test, namespace, "", "ip", "link", "set", "lo", "up")
	}
	if routed {
		test.Cleanup(func() { requireCommand(test, "", "", "nft", "flush", "ruleset") })
	}
	if routed {
		lab.routedLinks(test)
	} else {
		lab.directLink(test)
	}
	return lab
}

func linkPair(test *testing.T, firstNS, firstDevice, secondNS, secondDevice string) {
	test.Helper()
	requireCommand(test, firstNS, "", "ip", "link", "add", firstDevice, "type", "veth", "peer", "name", secondDevice)
	test.Cleanup(func() {
		if output, err := command(firstNS, "", "ip", "link", "del", firstDevice); err != nil {
			test.Errorf("veth cleanup: %v %s", err, output)
		}
	})
	requireCommand(test, firstNS, "", "ip", "link", "set", secondDevice, "netns", secondNS)
}

func addressLink(test *testing.T, namespace, device, mac, address4, address6 string) {
	test.Helper()
	requireCommand(test, namespace, "", "ip", "link", "set", device, "address", mac)
	requireCommand(test, namespace, "", "ip", "addr", "add", address4+"/24", "dev", device)
	requireCommand(test, namespace, "", "ip", "-6", "addr", "add", address6+"/64", "dev", device, "nodad")
	requireCommand(test, namespace, "", "ip", "link", "set", device, "up")
}

func neighbor(test *testing.T, namespace, device, address, mac string) {
	test.Helper()
	requireCommand(test, namespace, "", "ip", "neigh", "replace", address, "lladdr", mac, "nud", "permanent", "dev", device)
}

func (lab laboratory) directLink(test *testing.T) {
	linkPair(test, lab.client, "client0", lab.server, "server0")
	addressLink(test, lab.client, "client0", "02:00:00:00:00:10", lab.client4, lab.client6)
	addressLink(test, lab.server, "server0", "02:00:00:00:00:20", lab.server4, lab.server6)
	for _, address := range []string{lab.server4, lab.server6} {
		neighbor(test, lab.client, "client0", address, "02:00:00:00:00:20")
	}
	for _, address := range []string{lab.client4, lab.client6} {
		neighbor(test, lab.server, "server0", address, "02:00:00:00:00:10")
	}
}

func (lab laboratory) policyNS() string {
	if lab.routed {
		return lab.router
	}
	return lab.client
}

func executable(test *testing.T) string {
	test.Helper()
	path, err := os.Executable()
	if err != nil {
		test.Fatal(err)
	}
	return path
}
