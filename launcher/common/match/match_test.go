package match

import "testing"

func TestMatch_Subsequence(t *testing.T) {
	cases := []struct {
		query, target string
		want          bool
	}{
		{"fb", "foo-bar", true},        // scattered subsequence
		{"foobar", "foo-bar", true},    // subsequence skipping the separator
		{"bar", "foo-bar", true},       // tail
		{"", "anything", true},         // empty query matches everything
		{"FOO", "foobar", true},        // case-insensitive
		{"xyz", "foo", false},          // not present
		{"foobarbaz", "foobar", false}, // query longer than any subsequence
	}
	for _, tc := range cases {
		if _, ok := Match(tc.query, tc.target); ok != tc.want {
			t.Errorf("Match(%q, %q) ok = %v, want %v", tc.query, tc.target, ok, tc.want)
		}
	}
}

// A match at the start of the target beats one at a word boundary, which beats one in the
// middle of a word.
func TestMatch_StartBeatsBoundaryBeatsMiddle(t *testing.T) {
	start, _ := Match("cat", "cat-photos") // c at index 0
	boundary, _ := Match("cat", "my-cat")  // c after '-'
	middle, _ := Match("cat", "scatter")   // c inside "scatter"
	if !(start > boundary && boundary > middle) {
		t.Fatalf("want start > boundary > middle, got %d, %d, %d", start, boundary, middle)
	}
}

// Filter ranks best matches first, drops non-matches, and keeps input order for ties.
func TestFilter_RanksAndDropsNonMatches(t *testing.T) {
	targets := []string{"firefox", "fire-drill", "config-refire", "backup"}
	ranked := Filter("fire", targets)

	got := make([]int, len(ranked))
	for i, r := range ranked {
		got[i] = r.Index
	}
	// firefox(0) and fire-drill(1) both match "fire" at the start (tie -> input order),
	// config-refire(2) matches but weaker; backup(3) has no "fire" subsequence.
	want := []int{0, 1, 2}
	if len(got) != len(want) {
		t.Fatalf("Filter returned %v, want indices %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Filter order = %v, want %v", got, want)
		}
	}
}

// A non-ASCII query against an ASCII target is a clean no-match (no panic from byte
// indexing a multi-byte rune). App names are ASCII, so this is the correct answer.
func TestMatch_NonASCIIQuery(t *testing.T) {
	for _, query := range []string{"é", "café", "日本", "éè"} {
		if _, ok := Match(query, "firefox"); ok {
			t.Errorf("Match(%q, firefox) should be a no-match", query)
		}
	}
}

// An empty query returns every target in the original order.
func TestFilter_EmptyQueryKeepsOrder(t *testing.T) {
	targets := []string{"alpha", "beta", "gamma"}
	ranked := Filter("", targets)
	if len(ranked) != 3 {
		t.Fatalf("empty query should return all %d, got %d", len(targets), len(ranked))
	}
	for i, r := range ranked {
		if r.Index != i {
			t.Fatalf("empty query order = %d at %d, want %d", r.Index, i, i)
		}
	}
}
