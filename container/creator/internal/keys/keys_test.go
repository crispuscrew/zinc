package keys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDefault(t *testing.T) {
	cases := []struct {
		ctx  Context
		key  string
		want Action
	}{
		{CtxList, "q", Quit},
		{CtxList, "k", Up},
		{CtxList, "?", Keys},
		{CtxForm, "ctrl+s", Save},
		{CtxForm, "esc", Cancel},
		{CtxLogs, "q", Back},
		{CtxConfirm, "y", Yes},
	}
	for _, tc := range cases {
		if got, ok := Default.Resolve(tc.ctx, tc.key); !ok || got != tc.want {
			t.Errorf("Default.Resolve(%v, %q) = %q,%v; want %q", tc.ctx, tc.key, got, ok, tc.want)
		}
	}
}

// A nil scheme must behave exactly as Default, so callers that never load a
// scheme keep today's bindings.
func TestNilSchemeFallsBackToDefault(t *testing.T) {
	var s Scheme
	if got, ok := s.Resolve(CtxList, "q"); !ok || got != Quit {
		t.Fatalf("nil scheme should resolve like default, got %q,%v", got, ok)
	}
	if !s.Is(CtxForm, Save, "ctrl+s") {
		t.Fatal("nil scheme should know default form bindings")
	}
}

// The hand-authored built-ins must pass validation (catches accidental key
// collisions in list/logs/confirm).
func TestBuiltinSchemesValid(t *testing.T) {
	for _, name := range BuiltinNames() {
		sc, ok := SchemeFor(name)
		if !ok {
			t.Fatalf("SchemeFor(%q) missing", name)
		}
		if err := Validate(sc); err != nil {
			t.Fatalf("built-in %q failed validation: %v", name, err)
		}
	}
}

func TestVimDiffersFromDefault(t *testing.T) {
	// "g" refreshes under default but is freed under vim.
	if _, ok := Default.Resolve(CtxList, "g"); !ok {
		t.Fatal("default should bind 'g' in the list")
	}
	if _, ok := Vim.Resolve(CtxList, "g"); ok {
		t.Fatal("vim should leave 'g' unbound in the list")
	}
	// vim adds ctrl+n / ctrl+p for form field navigation.
	if !Vim.Is(CtxForm, NextField, "ctrl+n") {
		t.Fatal("vim should bind ctrl+n to next field")
	}
	if Default.Is(CtxForm, NextField, "ctrl+n") {
		t.Fatal("default should not bind ctrl+n")
	}
}

func TestValidateDetectsCollision(t *testing.T) {
	bad := Scheme{CtxList: {Quit: {"x"}, New: {"x"}}}
	if err := Validate(bad); err == nil {
		t.Fatal("a key bound to two list actions should fail validation")
	}
}

func TestValidateAllowsFormKeyReuse(t *testing.T) {
	// space drives both enum-next and toggle in the form - legal, kind-gated.
	ok := Scheme{CtxForm: {EnumNext: {" "}, Toggle: {" "}}}
	if err := Validate(ok); err != nil {
		t.Fatalf("form key reuse should be allowed, got %v", err)
	}
}

func TestValidateRejectsEmptyAndUnknown(t *testing.T) {
	if err := Validate(Scheme{CtxList: {Quit: {}}}); err == nil {
		t.Fatal("an action with no keys should fail")
	}
	if err := Validate(Scheme{CtxList: {Action("frobnicate"): {"z"}}}); err == nil {
		t.Fatal("an unknown action should fail")
	}
}

func TestHintRendersSpace(t *testing.T) {
	if got := Default.Hint(CtxForm, EnumNext); !strings.Contains(got, "space") {
		t.Fatalf("space key should render as 'space', got %q", got)
	}
}

// HintPrimary shows only the first bound key (compact footers), and still renders
// the space key as "space".
func TestHintPrimary(t *testing.T) {
	if got := Default.HintPrimary(CtxList, Edit); got != "e" { // Edit = {"e","enter"}
		t.Fatalf("HintPrimary should show only the first key, got %q", got)
	}
	if got := Default.HintPrimary(CtxForm, Toggle); got != "space" { // Toggle starts with " "
		t.Fatalf("primary space key should render as 'space', got %q", got)
	}
	if got := Default.HintPrimary(CtxList, Action("nope")); got != "" {
		t.Fatalf("unbound action should hint empty, got %q", got)
	}
}

// The built-ins bind the new Build action (b) without colliding with anything in
// the list (Validate catches collisions; this nails the specific key).
func TestBuildBound(t *testing.T) {
	for _, name := range BuiltinNames() {
		scheme, _ := SchemeFor(name)
		if got, ok := scheme.Resolve(CtxList, "b"); !ok || got != Build {
			t.Fatalf("%s: 'b' should trigger build in the list, got %q,%v", name, got, ok)
		}
	}
}

// --- store (I/O) ---

func TestStoreLoadDefaultWhenMissing(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	active, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if active.Name != "default" {
		t.Fatalf("missing keys.toml should mean default, got %q", active.Name)
	}
	if _, ok := active.Scheme.Resolve(CtxList, "q"); !ok {
		t.Fatal("loaded default scheme should resolve keys")
	}
}

func TestStoreSetActiveRoundtrip(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	if err := st.SetActive("vim"); err != nil {
		t.Fatal(err)
	}
	active, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if active.Name != "vim" {
		t.Fatalf("active scheme should be vim, got %q", active.Name)
	}
	if _, ok := active.Scheme.Resolve(CtxList, "g"); ok {
		t.Fatal("vim must not bind 'g' in the list")
	}
}

func TestStoreSetActiveRejectsUnknown(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	if err := st.SetActive("nope"); err == nil {
		t.Fatal("activating an unknown scheme should fail")
	}
}

func TestStoreCustomMergeOnBase(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	writeScheme(t, st, "mine", `base = "vim"
[bindings.list]
quit = ["Q"]
`)
	sc, err := st.Resolve("mine")
	if err != nil {
		t.Fatalf("custom scheme should resolve: %v", err)
	}
	if got, _ := sc.Resolve(CtxList, "Q"); got != Quit {
		t.Fatalf("override should bind Q→quit, got %q", got)
	}
	if _, ok := sc.Resolve(CtxList, "g"); ok {
		t.Fatal("custom should inherit vim's freed 'g'")
	}
	// the un-overridden vim binding survives the merge
	if got, _ := sc.Resolve(CtxList, "k"); got != Up {
		t.Fatalf("inherited binding lost, k→%q", got)
	}
}

func TestStoreRejectsUnknownAction(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	writeScheme(t, st, "bad", `base = "default"
[bindings.list]
frobnicate = ["z"]
`)
	if err := st.Validate("bad"); err == nil {
		t.Fatal("an unknown action in a custom scheme should fail")
	}
}

func TestStoreRejectsCircularBase(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	writeScheme(t, st, "a", "base = \"b\"\n")
	writeScheme(t, st, "b", "base = \"a\"\n")
	if err := st.Validate("a"); err == nil {
		t.Fatal("a circular base chain should fail")
	}
}

func TestStoreListAndEnsureEditable(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	writeScheme(t, st, "mine", "base = \"default\"\n")

	names, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 3 || names[0] != "default" || names[1] != "vim" {
		t.Fatalf("list should be built-ins first then customs, got %v", names)
	}
	if !contains(names, "mine") {
		t.Fatalf("custom scheme missing from list: %v", names)
	}

	// Editing a built-in scaffolds a non-shadowing custom copy.
	name, path, err := st.EnsureEditable("vim")
	if err != nil {
		t.Fatal(err)
	}
	if name != "vim-custom" {
		t.Fatalf("editing a built-in should scaffold <name>-custom, got %q", name)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("scaffolded file should exist: %v", err)
	}
}

func writeScheme(t *testing.T, st *Store, name, body string) {
	t.Helper()
	dir := filepath.Join(st.Dir, "schemes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
