package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

const digestPin = "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func quiet(t *testing.T) {
	t.Helper()
	previous := os.Stdout
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = null
	t.Cleanup(func() { os.Stdout = previous; null.Close() })
}

func writeApp(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func captureStdout(t *testing.T, call func()) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	os.Stdout = writer
	defer func() { os.Stdout = previous; writer.Close() }()
	output := make(chan []byte, 1)
	go func() { data, _ := io.ReadAll(reader); output <- data }()
	call()
	writer.Close()
	return string(<-output)
}
