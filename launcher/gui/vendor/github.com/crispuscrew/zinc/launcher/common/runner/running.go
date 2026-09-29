package runner

import (
	"bufio"
	"errors"
	"strconv"
	"strings"
)

// Running combines both runtimes' running sets. One unavailable runtime does not
// hide the other's apps; only failure of both makes this best-effort lookup fail.
func Running() (map[string]bool, error) {
	running := map[string]bool{}
	var failures []error
	for _, binary := range []string{Binary, VMBinary} {
		output, err := capture(binary, "ps")
		if err != nil {
			failures = append(failures, err)
			continue
		}
		names, err := runningNames(binary, output)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for name := range names {
			running[name] = true
		}
	}
	if len(failures) == 2 {
		return nil, errors.Join(failures...)
	}
	return running, nil
}

func runningNames(binary, output string) (map[string]bool, error) {
	running := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if binary == VMBinary {
			// zvr ps prints APP/PID/STATE columns, or "no guests running".
			fields := strings.Fields(name)
			if len(fields) < 3 {
				continue
			}
			processID, err := strconv.Atoi(fields[1])
			if err != nil || processID <= 0 {
				continue
			}
			name = fields[0]
		}
		if name != "" {
			running[name] = true
		}
	}
	return running, scanner.Err()
}
