package network_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMain(suite *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--host-inventory" {
		os.Exit(hostInventory())
	}
	if os.Getenv("ZINC_LIVE_NETWORK") != "1" {
		fmt.Fprintln(os.Stderr, "live network suite requires make -C integration/network test; refusing host execution")
		os.Exit(2)
	}
	current, err := os.Readlink("/proc/self/ns/net")
	if err != nil || os.Getenv("ZINC_HOST_NETNS") == "" || current == os.Getenv("ZINC_HOST_NETNS") {
		fmt.Fprintln(os.Stderr, "test network namespace is not isolated from host", err)
		os.Exit(2)
	}
	if len(os.Args) > 1 && os.Args[1] == "--agent" {
		os.Exit(agent(os.Args[2:]))
	}
	if err := pristine(); err != nil {
		fmt.Fprintln(os.Stderr, "test container must start disconnected:", err)
		os.Exit(2)
	}
	for _, tool := range []string{"ip", "nft", "nsenter", "ping"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mandatory tool unavailable:", err)
			os.Exit(2)
		}
		fmt.Printf("tool %s=%s\n", tool, path)
	}
	for _, argv := range [][]string{{"nft", "--version"}, {"ip", "-Version"}, {"uname", "-r"}} {
		output, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
		fmt.Printf("%s: %s", strings.Join(argv, " "), output)
		if err != nil {
			os.Exit(2)
		}
	}
	status := suite.Run()
	if sequence.Load() == 0 {
		fmt.Fprintln(os.Stderr, "no live case executed; check selection and capability skips")
		status = 1
	}
	if err := pristine(); err != nil {
		fmt.Fprintln(os.Stderr, "test resource leak:", err)
		status = 1
	} else {
		fmt.Println("CLEANUP: parent namespace contains only loopback, no routes, no nft tables, no named namespaces")
	}
	os.Exit(status)
}

func pristine() error {
	output, err := exec.Command("ip", "-j", "link", "show").Output()
	if err != nil {
		return err
	}
	var links []struct {
		Name string `json:"ifname"`
	}
	if err := json.Unmarshal(output, &links); err != nil {
		return err
	}
	if len(links) != 1 || links[0].Name != "lo" {
		return fmt.Errorf("unexpected links: %s", output)
	}
	for _, argv := range [][]string{{"ip", "route", "show"}, {"ip", "-6", "route", "show"}, {"ip", "netns", "list"}, {"nft", "list", "ruleset"}} {
		output, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
		if err != nil || len(strings.TrimSpace(string(output))) != 0 {
			return fmt.Errorf("%v: %s (%v)", argv, output, err)
		}
	}
	return nil
}
