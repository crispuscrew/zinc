// Package validate holds the hard schema rules and create-time advisories for an
// app config: a pure sibling of schema (no I/O), used by zcc (save) and zcr (launch).
package validate

import (
	"net"
	"regexp"
	"strings"
)

// Low-level safety vocabulary: charset regexes and metacharacter/CIDR/path/image
// screens. Almost every field is interpolated into a podman arg, an image ref, or a
// ':'-delimited mount spec, where a stray space/comma/':' shifts fields (section 5.5).

// nameRE: podman object-name charset (lowercase [a-z0-9._-], starts alphanumeric).
var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// digestRE: canonical sha256 pin (@sha256: + 64 hex), anchored - a short/fake digest
// must not smuggle extra FROM-line directives (section 5.5).
var digestRE = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)

// ifaceRE: interface charset; no comma/space that would splice pasta options.
var ifaceRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// capRE: one capability - optional CAP_ then [A-Z_]. ALL is rejected separately.
var capRE = regexp.MustCompile(`^(CAP_)?[A-Z_]+$`)

// addFunc collects one error; Validate threads it through every check so all problems
// surface at once, not just the first.
type addFunc = func(format string, args ...any)

// hasUnsafe reports whitespace/control chars - metacharacters that shift a
// ':'-delimited podman field or inject a directive/flag.
func hasUnsafe(str string) bool {
	for _, run := range str {
		if run == ' ' || run == '\t' || run == '\r' || run == '\n' || run < 0x20 || run == 0x7f {
			return true
		}
	}
	return false
}

// hasDotDot reports a ".." segment (bundle escape); paths here are '/'-separated.
func hasDotDot(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// validCIDR reports a valid CIDR in the wanted family (wantV6), so an address can't
// sit under the wrong key (e.g. IPv6 in IPv4CIDR).
func validCIDR(cidr string, wantV6 bool) bool {
	addr, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	return (addr.To4() == nil) == wantV6
}

// LocalImage reports a localhost/ image - the only refs exempt from the section 5.5 digest
// pin. The boundary is the namespace, not a name: "localhost/" resolves to local
// storage only, never a short name that could pull something remote.
func LocalImage(image string) bool {
	return strings.HasPrefix(image, "localhost/")
}
