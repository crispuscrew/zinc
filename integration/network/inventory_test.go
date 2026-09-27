package network_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
)

// This entry point is read-only and may run outside the test container. Runtime
// MAC randomization is represented by permanent identity when the kernel gives
// one. The shell retains and prints raw link differences independently.
func hostInventory() int {
	result := map[string][]map[string]any{}
	for _, entry := range []struct {
		name         string
		argv, fields []string
	}{
		{"links", []string{"-j", "link", "show"}, []string{"ifindex", "ifname", "flags", "mtu", "qdisc", "operstate", "master", "link_type", "address", "permaddr", "altnames"}},
		{"ipv4", []string{"-j", "-4", "route", "show", "table", "all"}, routeFields()},
		{"ipv6", []string{"-j", "-6", "route", "show", "table", "all"}, routeFields()},
	} {
		output, err := exec.Command("ip", entry.argv...).Output()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		var values []map[string]any
		if err := json.Unmarshal(output, &values); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		filtered := make([]map[string]any, 0, len(values))
		for _, value := range values {
			copy := map[string]any{}
			for _, field := range entry.fields {
				if found, exists := value[field]; exists {
					copy[field] = found
				}
			}
			if permanent, exists := copy["permaddr"]; entry.name == "links" && exists {
				copy["address"] = permanent
			}
			filtered = append(filtered, copy)
		}
		sort.Slice(filtered, func(first, second int) bool {
			left, _ := json.Marshal(filtered[first])
			right, _ := json.Marshal(filtered[second])
			return string(left) < string(right)
		})
		result[entry.name] = filtered
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

func routeFields() []string {
	return []string{"dst", "gateway", "dev", "protocol", "scope", "prefsrc", "metric", "table", "type", "flags", "pref", "nexthops"}
}
