package runner

import (
	"errors"
	"strconv"
	"strings"
)

func RunningAll() (map[string]bool, error) {
	containers, containerErr := Running()
	if containers == nil {
		containers = map[string]bool{}
	}
	output, _, vmErr := CaptureTo(VMBinary, "ps")
	if vmErr == nil {
		for _, line := range strings.Split(output, "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			pid, err := strconv.Atoi(fields[1])
			if err == nil && pid > 0 {
				containers[fields[0]] = true
			}
		}
	}
	if containerErr != nil && vmErr != nil {
		return containers, errors.Join(containerErr, vmErr)
	}
	return containers, nil
}
