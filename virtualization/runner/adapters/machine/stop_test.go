package machine

import (
	"encoding/json"
	"net"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/virtualization/runner/domain/paths"
)

func TestStopQuitsGuestAlreadyInShutdown(check *testing.T) {
	_, pid := fakeGuest(check, "kept", false)
	runtime := Runtime{Paths: paths.Paths{RunDir: check.TempDir()}}
	if err := os.WriteFile(runtime.Paths.PIDFile("kept"), []byte(strconv.Itoa(pid)), 0o600); err != nil {
		check.Fatal(err)
	}
	listener, err := net.Listen("unix", runtime.Paths.QMP("kept"))
	if err != nil {
		check.Fatal(err)
	}
	defer listener.Close()
	commands := make(chan string, 4)
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			serveShutdown(connection, pid, commands)
		}
	}()
	if err := runtime.Stop("kept", false, time.Second); err != nil {
		check.Fatal(err)
	}
	select {
	case command := <-commands:
		if command != "quit" {
			check.Fatalf("already shut-down guest received %s instead of quit", command)
		}
	case <-time.After(time.Second):
		check.Fatal("no shutdown command observed")
	}
}

func serveShutdown(connection net.Conn, pid int, commands chan<- string) {
	defer connection.Close()
	encoder, decoder := json.NewEncoder(connection), json.NewDecoder(connection)
	if encoder.Encode(map[string]any{"QMP": map[string]any{}}) != nil {
		return
	}
	for {
		var request struct {
			Execute string `json:"execute"`
		}
		if decoder.Decode(&request) != nil {
			return
		}
		response := map[string]any{}
		if request.Execute == "query-status" {
			response = map[string]any{"status": "shutdown", "running": false}
		}
		if encoder.Encode(map[string]any{"return": response}) != nil {
			return
		}
		if request.Execute == "quit" || request.Execute == "system_powerdown" {
			commands <- request.Execute
			_ = syscall.Kill(pid, syscall.SIGTERM)
			return
		}
	}
}
