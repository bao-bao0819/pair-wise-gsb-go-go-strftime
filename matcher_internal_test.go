package strftime

import "testing"

func TestNameMatcher(t *testing.T) {
	m, err := newNameMatcher("month",
		[]string{"January", "February", "March"},
		[]string{"Jan", "Feb", "Mar"})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		input string
		value int
		size  int
		ok    bool
	}{
		{"January", 0, 7, true},    // exact full
		{"JANUARY", 0, 7, true},    // folded full
		{"Jan", 0, 3, true},        // exact abbrev
		{"jan", 0, 3, true},        // folded abbrev
		{"March rest", 2, 5, true}, // full name + trailing
		{"Mar", 2, 3, true},
		{"Januaryx", 0, 7, true}, // prefix match, leftover for caller
		{"Ma", 0, 0, false},      // no name prefixes
		{"Jun", 0, 0, false},     // not present
	}
	for _, c := range cases {
		v, n, ok := m.matchAt(c.input, 0)
		if ok != c.ok || (ok && (v != c.value || n != c.size)) {
			t.Errorf("matchAt(%q)=(%d,%d,%v), want (%d,%d,%v)",
				c.input, v, n, ok, c.value, c.size, c.ok)
		}
	}
}

func TestNameMatcherMultibyte(t *testing.T) {
	m, err := newNameMatcher("month",
		[]string{"十一月", "十二月"},
		[]string{"十一", "十二"})
	if err != nil {
		t.Fatal(err)
	}
	// Longest prefix must win across rune boundaries.
	for _, c := range []struct {
		in       string
		value    int
		runesLen int
	}{
		{"十一月", 0, 3},
		{"十二月", 1, 3},
		{"十二", 1, 2},
		{"十一", 0, 2},
	} {
		v, n, ok := m.matchAt(c.in, 0)
		if !ok || v != c.value || n != c.runesLen*3 {
			t.Errorf("matchAt(%q)=(%d,%d,%v)", c.in, v, n, ok)
		}
	}
}

func TestNameMatcherAmbiguity(t *testing.T) {
	// Case-folded duplicate pointing at different indices is rejected
	// ("JAN" folds onto "jan" = January, but claims February).
	if _, err := newNameMatcher("m",
		[]string{"jan", "JAN", "mar", "apr", "may", "jun",
			"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
		[]string{"a", "b", "c", "d", "e", "f",
			"g", "h", "i", "j", "k", "l"}); err == nil {
		t.Errorf("exact/fold ambiguity should be rejected")
	}

	// A full name equal to its own abbreviation maps to the same value
	// and is allowed.
	if _, err := newNameMatcher("m",
		[]string{"January", "February", "March", "April", "May", "June",
			"July", "August", "September", "October", "November", "December"},
		[]string{"January", "Feb", "Mar", "Apr", "May", "Jun",
			"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}); err != nil {
		t.Errorf("same-value duplicate allowed, got %v", err)
	}

	// Empty names rejected.
	if _, err := newNameMatcher("w",
		[]string{"A", "", "C", "D", "E", "F", "G"},
		[]string{"a", "b", "c", "d", "e", "f", "g"}); err == nil {
		t.Errorf("empty name should be rejected")
	}
}
