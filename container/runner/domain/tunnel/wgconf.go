// Package tunnel reads a wg-quick-format WireGuard config into the parts the runner needs
// to build the interface itself. Pure: it parses text and performs no I/O.
//
// It exists because `wg setconf` and wg-quick do not accept the same file. wg-quick is a
// shell script that reads Address, DNS, MTU and the script hooks, strips them, and hands the
// rest to `wg setconf`. Zinc does the stripping here instead, in Go, where what is accepted
// and what is refused can be stated and tested - rather than shipping wg-quick, which wants
// to be root, rewrite resolv.conf, and run whatever the file tells it to.
package tunnel

import (
	"fmt"
	"strconv"
	"strings"
)

// Config is a WireGuard config split into what each step of the setup needs.
type Config struct {
	// SetConf is the body handed to `wg setconf` on stdin: the keys that tool accepts, and
	// nothing else. The private key is in here, which is why it travels on stdin.
	SetConf string
	// Addresses are the [Interface] Address entries, assigned to the link.
	Addresses []string
	// Routes are the peers' AllowedIPs: what the tunnel carries, and so what is routed into
	// it.
	Routes []string
	// Endpoints are the peer addresses, without ports. The route to an endpoint must NOT go
	// through the tunnel, or the encrypted packets would be sent into the thing they are
	// carrying; the caller pins them to the pre-existing route first.
	Endpoints []string
	// MTU is the [Interface] MTU, or 0 when the file does not set one.
	MTU int
}

// interfaceKeys are the [Interface] settings `wg setconf` understands. Everything else in
// that section is wg-quick's own and is handled here or refused.
var interfaceKeys = map[string]bool{
	"privatekey": true,
	"listenport": true,
	"fwmark":     true,
}

// peerKeys are the [Peer] settings `wg setconf` understands. AllowedIPs and Endpoint are
// passed through AND read out, because the interface needs them and so does the routing.
var peerKeys = map[string]bool{
	"publickey":           true,
	"presharedkey":        true,
	"allowedips":          true,
	"endpoint":            true,
	"persistentkeepalive": true,
}

// refused are the wg-quick directives Zinc will not honour, with the reason each one gets.
// They are refused rather than ignored: a config that silently did less than it says would
// be the exact failure this project keeps designing against.
var refused = map[string]string{
	"postup":     "runs arbitrary shell, and the helper that would run it holds NET_ADMIN in the app's network namespace - a config file is not a place to accept code from",
	"preup":      "runs arbitrary shell, and the helper that would run it holds NET_ADMIN in the app's network namespace - a config file is not a place to accept code from",
	"postdown":   "runs arbitrary shell; Zinc tears the namespace down with the pod instead",
	"predown":    "runs arbitrary shell; Zinc tears the namespace down with the pod instead",
	"saveconfig": "would have Zinc write back to your config file, which it never does",
	"table":      "chooses a routing table; Zinc installs the peers' AllowedIPs itself and does not offer a second scheme",
	"dns":        "Zinc already has one place to name resolvers, and two would disagree - use NetworkMeta.DNSServers",
}

// Parse reads a wg-quick config. Unknown keys are an error rather than a shrug: this file
// decides what an app can reach, and a typo in it must not quietly widen or narrow that.
func Parse(text string) (Config, error) {
	var cfg Config
	var setConf strings.Builder
	section := ""
	peers := 0

	for number, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			switch section {
			case "interface":
				setConf.WriteString("[Interface]\n")
			case "peer":
				peers++
				setConf.WriteString("[Peer]\n")
			default:
				return Config{}, fmt.Errorf("line %d: unknown section [%s]", number+1, section)
			}
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			return Config{}, fmt.Errorf("line %d: want key = value", number+1)
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		if section == "" {
			return Config{}, fmt.Errorf("line %d: %q comes before any [Interface] or [Peer] section", number+1, key)
		}
		if why, no := refused[key]; no {
			return Config{}, fmt.Errorf("line %d: %s is not supported - %s", number+1, key, why)
		}
		if err := cfg.absorb(&setConf, section, key, value, number+1); err != nil {
			return Config{}, err
		}
	}
	if peers == 0 {
		return Config{}, fmt.Errorf("no [Peer] section: a tunnel with no peer carries nothing")
	}
	if len(cfg.Addresses) == 0 {
		return Config{}, fmt.Errorf("no Address in [Interface]: the tunnel interface needs one to be reachable")
	}
	cfg.SetConf = setConf.String()
	return cfg, nil
}

// absorb files one setting: into the setconf body, into the fields the caller needs, or
// both.
func (cfg *Config) absorb(setConf *strings.Builder, section, key, value string, line int) error {
	switch section {
	case "interface":
		switch key {
		case "address":
			cfg.Addresses = append(cfg.Addresses, splitList(value)...)
			return nil
		case "mtu":
			mtu, err := strconv.Atoi(value)
			if err != nil || mtu < 576 || mtu > 65535 {
				return fmt.Errorf("line %d: MTU %q: want a number between 576 and 65535", line, value)
			}
			cfg.MTU = mtu
			return nil
		}
		if !interfaceKeys[key] {
			return fmt.Errorf("line %d: unknown [Interface] setting %q", line, key)
		}
	case "peer":
		switch key {
		case "allowedips":
			cfg.Routes = append(cfg.Routes, splitList(value)...)
		case "endpoint":
			host, err := endpointHost(value)
			if err != nil {
				return fmt.Errorf("line %d: Endpoint %q: %w", line, value, err)
			}
			cfg.Endpoints = append(cfg.Endpoints, host)
		}
		if !peerKeys[key] {
			return fmt.Errorf("line %d: unknown [Peer] setting %q", line, key)
		}
	}
	fmt.Fprintf(setConf, "%s = %s\n", key, value)
	return nil
}

// endpointHost takes the address off an Endpoint, dropping the port. A name rather than an
// address is refused: it would have to be resolved to be pinned to a route, and the pinning
// has to happen before the netns is closed - so a name that resolved to something else a
// moment later would send the encrypted packets into the tunnel they belong to.
func endpointHost(value string) (string, error) {
	host := value
	if strings.HasPrefix(value, "[") { // [v6]:port
		end := strings.Index(value, "]")
		if end < 0 {
			return "", fmt.Errorf("unclosed [")
		}
		host = value[1:end]
	} else if index := strings.LastIndex(value, ":"); index >= 0 && strings.Count(value, ":") == 1 {
		host = value[:index]
	}
	if host == "" {
		return "", fmt.Errorf("no host")
	}
	if !isAddress(host) {
		return "", fmt.Errorf("must be an IP address, not a name - it has to be pinned to a route before the namespace closes, and a name could resolve elsewhere by then")
	}
	return host, nil
}

// isAddress reports whether text is a bare IPv4 or IPv6 address. Deliberately not
// net.ParseIP-strict about zones; a zoned address is not a usable endpoint anyway.
func isAddress(text string) bool {
	if strings.Contains(text, ":") {
		return !strings.ContainsAny(text, "/ ") && strings.Count(text, ":") >= 2
	}
	parts := strings.Split(text, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 || number > 255 || (len(part) > 1 && part[0] == '0') {
			return false
		}
	}
	return true
}

// splitList reads a comma-separated value into its entries.
func splitList(value string) []string {
	var out []string
	for _, entry := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// stripComment drops a trailing `#` comment.
func stripComment(line string) string {
	if index := strings.Index(line, "#"); index >= 0 {
		return line[:index]
	}
	return line
}
