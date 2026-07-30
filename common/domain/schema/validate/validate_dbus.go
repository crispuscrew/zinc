package validate

import (
	"regexp"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// busNameRE is one D-Bus well-known name: two or more dot-separated elements, each starting
// with a letter, underscore or hyphen and continuing in [A-Za-z0-9_-]. Anchored, and the
// charset is deliberately narrow: these names are passed to xdg-dbus-proxy as --talk/--own
// arguments, so a name carrying a space or its own "--" would splice an extra option into
// the very filter the app is confined by (the section 5.5 argument-shifting concern, applied
// to the bus).
var busNameRE = regexp.MustCompile(`^[A-Za-z_-][A-Za-z0-9_-]*(\.[A-Za-z_-][A-Za-z0-9_-]*)+$`)

// maxBusName is the bus-name length cap from the D-Bus specification.
const maxBusName = 255

// checkDBus screens DBusMeta. An empty block is the fail-closed default (no bus at all) and
// has nothing to check, so the rules below only apply once an app has asked for bus access.
//
// The KeepUserID requirement is the one non-obvious rule. A filtered bus is an agreement
// about uid: xdg-dbus-proxy serves the socket as the invoking host user, and an app in a
// different user namespace cannot connect to it - the failure surfaces inside the app as a
// bare "connection refused", with nothing pointing at the namespace as the cause. Zinc could
// silently switch the app to keep-id to make it work, but that would change who the app runs
// as on the strength of an unrelated field, so the config is refused instead and says which
// key to set.
func checkDBus(cfg schema.AppConfig, add addFunc) {
	bus := cfg.DBusMeta
	if bus.IsZero() {
		return
	}
	if !cfg.InternalUserMeta.KeepUserID {
		add("DBusMeta: a filtered bus needs InternalUserMeta.KeepUserID: true - the proxy serves the socket as the host user and an app in its own user namespace cannot connect to it; set KeepUserID rather than have Zinc change who the app runs as on the strength of a bus grant")
	}
	for index, name := range bus.Talk {
		checkBusName("Talk", index, name, true, add)
	}
	for index, name := range bus.Own {
		checkBusName("Own", index, name, false, add)
	}
}

// checkBusName screens one Talk/Own entry. allowWildcard separates the two fields: a
// trailing ".*" is a meaningful (if broad) thing to be allowed to CALL, and meaningless as
// something to own, since a process claims one concrete name or none.
func checkBusName(field string, index int, name string, allowWildcard bool, add addFunc) {
	trimmed := strings.TrimSpace(name)
	base := trimmed
	wildcard := strings.HasSuffix(trimmed, ".*")
	if wildcard {
		base = strings.TrimSuffix(trimmed, ".*")
	}
	switch {
	case trimmed == "":
		add("DBusMeta.%s[%d]: must not be empty", field, index)
	case trimmed != name:
		add("DBusMeta.%s[%d]: %q has leading or trailing whitespace", field, index, name)
	case len(trimmed) > maxBusName:
		add("DBusMeta.%s[%d]: %q is longer than the %d-character bus-name limit", field, index, name, maxBusName)
	case wildcard && !allowWildcard:
		add("DBusMeta.%s[%d]: %q - a subtree wildcard cannot be owned, since a process claims one concrete name or none", field, index, name)
	case !busNameRE.MatchString(base):
		add("DBusMeta.%s[%d]: %q is not a well-known bus name - two or more dot-separated elements of [A-Za-z0-9_-], no element starting with a digit", field, index, name)
	}
}

// checkSourceTag screens ImageMeta.SourceTag. It is provenance rather than something a launch
// acts on, but it is handed to a registry client when someone asks whether the pin is stale,
// so it gets the same treatment as any other reference: no whitespace or control characters
// that could shift an argument, and a digest is refused because a tag that is already a digest
// records nothing - re-resolving it would return itself and report "never stale" forever.
func checkSourceTag(tag string, add addFunc) {
	trimmed := strings.TrimSpace(tag)
	switch {
	case trimmed == "":
		return // absent is fine: a hand-pinned digest has no known origin
	case trimmed != tag || hasUnsafe(tag):
		add("ImageMeta.SourceTag: %q must not contain whitespace or control characters", tag)
	case digestRE.MatchString(tag):
		add("ImageMeta.SourceTag: %q is a digest, not a tag - re-resolving it would return itself and report the pin as never stale; record the tag it came from, or leave this empty", tag)
	}
}
