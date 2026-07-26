package keys

import (
	"errors"
	"fmt"
	"sort"
)

// Validate checks a fully merged scheme and accumulates every problem (the
// config.Validate style) rather than stopping at the first:
//   - an action that isn't valid for its context,
//   - a bound action with no keys or an empty key string,
//   - a key bound to two different actions within the same context.
//
// It is pure. The collision check is skipped for the form, which dispatches by
// the focused field's kind: there the same key intentionally drives different
// gestures (space cycles an enum field and toggles a bool field), and the
// kind-independent commands take precedence - so key reuse there is by design,
// not a conflict.
func Validate(s Scheme) error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	for _, ctx := range Contexts {
		cname := ContextName[ctx]

		// Unknown actions, sorted so messages are deterministic.
		var unknown []string
		for act := range s[ctx] {
			if !knownAction(ctx, act) {
				unknown = append(unknown, string(act))
			}
		}
		sort.Strings(unknown)
		for _, a := range unknown {
			add("[%s] unknown action %q", cname, a)
		}

		// Empty bindings (all contexts) and key collisions (key-resolved
		// contexts only), walked in declared action order.
		owner := map[string]Action{}
		for _, act := range ActionsByContext[ctx] {
			ks, ok := s[ctx][act]
			if !ok {
				continue
			}
			if len(ks) == 0 {
				add("[%s] %s: needs at least one key", cname, act)
			}
			for _, k := range ks {
				if k == "" { // note: " " is the valid space key, not empty
					add("[%s] %s: empty key binding", cname, act)
					continue
				}
				if ctx == CtxForm {
					continue
				}
				if prev, dup := owner[k]; dup && prev != act {
					add("[%s] key %q is bound to both %q and %q", cname, k, prev, act)
					continue
				}
				owner[k] = act
			}
		}
	}
	return errors.Join(errs...)
}
