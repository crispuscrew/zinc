package pipewirectx

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func readStatus(input *os.File) (string, error) {
	if err := input.SetReadDeadline(time.Now().Add(readyTimeout)); err != nil {
		return "", err
	}
	reader := bufio.NewReaderSize(input, 4096)
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return "", fmt.Errorf("holder did not report readiness: %w", err)
	}
	return parseStatus(strings.TrimSuffix(string(line), "\n"))
}

func parseStatus(line string) (string, error) {
	verb, rest, _ := strings.Cut(line, " ")
	if verb != "ok" {
		return "", fmt.Errorf("holder refused audio: %s", line)
	}
	if !filepath.IsAbs(rest) || strings.ContainsAny(rest, "\x00\r\n") {
		return "", fmt.Errorf("holder returned invalid socket")
	}
	return rest, nil
}
