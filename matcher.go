package strftime

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// matcherEntry maps a name to its value (month, weekday, or meridiem).
type matcherEntry struct {
	name  string
	value int
}

// nameMatcher matches localized names against input.
//
// Names are matched by exact comparison first, then using simple
// case folding ([strings.EqualFold]). When several names are prefixes
// of the input, the longest name is preferred.
//
// Matching is rune-safe: name lengths are measured in runes, never
// truncated to three bytes.
type nameMatcher struct {
	kind         string
	entries      []string
	abbrevOffset int
	exact        map[string]int
	fold         map[string]int
	sorted       []matcherEntry
}

func newNameMatcher(kind string, full, abbrev []string) (nameMatcher, error) {
	m := nameMatcher{
		kind:         kind,
		entries:      make([]string, len(full)+len(abbrev)),
		abbrevOffset: len(full),
		exact:        map[string]int{},
		fold:         map[string]int{},
	}
	copy(m.entries, full)
	copy(m.entries[len(full):], abbrev)

	for i, name := range full {
		if name == "" {
			return m, errors.New("strftime: empty " + kind + " name at index " + strconv.Itoa(i))
		}
		if err := m.add(name, i); err != nil {
			return m, err
		}
	}
	for i, name := range abbrev {
		if name == "" {
			return m, errors.New("strftime: empty " + kind + " abbreviation at index " + strconv.Itoa(i))
		}
		if err := m.add(name, i); err != nil {
			return m, err
		}
	}

	m.finalize()
	return m, nil
}

// finalize sorts the registered names for longest-prefix matching.
func (m *nameMatcher) finalize() {
	m.sorted = make([]matcherEntry, 0, len(m.exact))
	for name, value := range m.exact {
		m.sorted = append(m.sorted, matcherEntry{name, value})
	}
	sort.SliceStable(m.sorted, func(i, j int) bool {
		ri, rj := utf8.RuneCountInString(m.sorted[i].name), utf8.RuneCountInString(m.sorted[j].name)
		if ri != rj {
			return ri > rj
		}
		return m.sorted[i].name < m.sorted[j].name
	})
}

// add registers a name for value, rejecting names that already map,
// exactly or under case folding, to a different value.
func (m *nameMatcher) add(name string, value int) error {
	if existing, ok := m.exact[name]; ok && existing != value {
		return errors.New("strftime: duplicate " + m.kind + " name: " + name)
	}
	folded := strings.ToLower(name)
	if existing, ok := m.fold[folded]; ok && existing != value {
		return errors.New("strftime: ambiguous " + m.kind + " name: " + name)
	}
	m.exact[name] = value
	m.fold[folded] = value
	return nil
}

// matchAt reports the value of the longest name that is a prefix of
// input at offset, and how many bytes of input it consumes.
// Exact names take precedence over case-folded ones.
func (m *nameMatcher) matchAt(input string, offset int) (value, size int, ok bool) {
	rest := input[offset:]

	exactValue, exactLen, exactOK := m.prefix(rest, true)
	if exactOK {
		return exactValue, exactLen, true
	}
	foldValue, foldLen, foldOK := m.prefix(rest, false)
	if foldOK {
		return foldValue, foldLen, true
	}
	return 0, 0, false
}

// prefix returns the value and byte length of the longest listed
// name that prefixes rest, using exact or case-folded comparison.
func (m *nameMatcher) prefix(rest string, exact bool) (value, size int, ok bool) {
	for _, e := range m.sorted {
		if len(rest) >= len(e.name) && hasPrefixFold(rest[:len(e.name)], e.name, exact) {
			return e.value, len(e.name), true
		}
	}
	return 0, 0, false
}

// hasPrefixFold reports whether s equals name, exactly (when exact is
// true) or under simple case folding. Both strings must have the same
// byte length, which keeps the comparison rune-safe without slicing
// names to a fixed number of bytes.
func hasPrefixFold(s, name string, exact bool) bool {
	if exact {
		return s == name
	}
	return strings.EqualFold(s, name)
}
