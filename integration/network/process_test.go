package network_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func startServer(test *testing.T, namespace, protocol, address string, port int) {
	test.Helper()
	process := exec.Command("nsenter", "--net=/run/netns/"+namespace, "--", executable(test), "--agent", "serve", protocol, address, strconv.Itoa(port))
	var diagnostic bytes.Buffer
	process.Stderr = &diagnostic
	output, err := process.StdoutPipe()
	if err != nil {
		test.Fatal(err)
	}
	if err := process.Start(); err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() {
		if process.ProcessState == nil {
			_ = process.Process.Kill()
			_ = process.Wait()
		}
	})
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		if scanner.Scan() {
			ready <- scanner.Text()
		} else {
			ready <- ""
		}
	}()
	select {
	case message := <-ready:
		if message != "READY" {
			_ = process.Process.Kill()
			_ = process.Wait()
			test.Fatalf("listener failed: %s", diagnostic.String())
		}
	case <-time.After(4 * time.Second):
		test.Fatal("listener readiness deadline exceeded")
	}
}

func runProbe(test *testing.T, namespace, protocol, source, destination string, sourcePort, destinationPort int) probeResult {
	test.Helper()
	output := requireCommand(test, namespace, "", executable(test), "--agent", "probe", protocol, destination,
		strconv.Itoa(destinationPort), source, strconv.Itoa(sourcePort))
	var result probeResult
	if err := json.Unmarshal(output, &result); err != nil {
		test.Fatalf("invalid probe output %q: %v", output, err)
	}
	return result
}

func verifyProbe(test *testing.T, result probeResult, allowed bool) {
	test.Helper()
	if allowed && !result.Success {
		test.Fatalf("allowed packet exchange failed: %+v", result)
	}
	if !allowed && (result.Success || !(result.Timeout || result.Rejected)) {
		test.Fatalf("denial must time out or return local permission rejection, plus nft drop evidence: %+v", result)
	}
}

func nextPort() int { return 40000 + int(sequence.Add(1)) }

func requireControl(test *testing.T, namespace, protocol, source, destination string) {
	test.Helper()
	result := runProbe(test, namespace, protocol, source, destination, nextPort(), 8080)
	if !result.Success {
		test.Fatalf("unfiltered positive control failed (%s): %s", protocol, fmt.Sprint(result))
	}
	test.Logf("positive control: %s %s -> %s echo succeeded", protocol, source, destination)
}
