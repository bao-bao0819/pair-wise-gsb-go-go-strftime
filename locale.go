package strftime

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// LocaleSpec describes the month, weekday and meridiem names of a locale,
// as well as its composite date and time formats.
//
// NewLocale copies all slices and maps,
// so the caller retains ownership of the spec.
type LocaleSpec struct {
	// Full month names, January to December.
	Months []string
	// Abbreviated month names, January to December.
	AbbreviatedMonths []string
	// Full weekday names, Sunday to Saturday.
	Weekdays []string
	// Abbreviated weekday names, Sunday to Saturday.
	AbbreviatedWeekdays []string
	// Meridiem designators, AM and PM.
	AM, PM string
	// Composite format overrides for the
	// %c %D %F %r %R %T %v %x %X %+ specifiers.
	Formats map[byte]string
	// CaseFold makes parsing match names case-insensitively,
	// using Unicode simple case folding.
	// Otherwise, names must match exactly.
	CaseFold bool
}

// Locale formats and parses times using localized
// month, weekday and meridiem names.
//
// A Locale is immutable and safe for concurrent use.
type Locale struct {
	months       [12]string
	monthsAbbr   [12]string
	weekdays     [7]string
	weekdaysAbbr [7]string
	ampm         [2]string
	formats      map[byte]string
	monthNames   nameTable
	weekdayNames nameTable
	ampmNames    nameTable
}

// NewLocale builds a Locale from an explicit spec,
// without consulting the operating system's locale configuration.
//
// It returns an error if the spec has missing or empty names,
// if a name is ambiguous (it matches two different months,
// weekdays or meridiems under the spec's matching strategy),
// or if the composite formats are unsupported, empty, or cyclic.
func NewLocale(spec LocaleSpec) (*Locale, error) {
	if len(spec.Months) != 12 || len(spec.AbbreviatedMonths) != 12 {
		return nil, localeError("need 12 full and abbreviated month names")
	}
	if len(spec.Weekdays) != 7 || len(spec.AbbreviatedWeekdays) != 7 {
		return nil, localeError("need 7 full and abbreviated weekday names")
	}

	loc := &Locale{}
	copy(loc.months[:], spec.Months)
	copy(loc.monthsAbbr[:], spec.AbbreviatedMonths)
	copy(loc.weekdays[:], spec.Weekdays)
	copy(loc.weekdaysAbbr[:], spec.AbbreviatedWeekdays)
	loc.ampm[0], loc.ampm[1] = spec.AM, spec.PM

	var err error
	if loc.monthNames, err = makeNameTable("month", spec.CaseFold, loc.months[:], loc.monthsAbbr[:]); err != nil {
		return nil, err
	}
	if loc.weekdayNames, err = makeNameTable("weekday", spec.CaseFold, loc.weekdays[:], loc.weekdaysAbbr[:]); err != nil {
		return nil, err
	}
	if loc.ampmNames, err = makeNameTable("meridiem", spec.CaseFold, loc.ampm[:]); err != nil {
		return nil, err
	}
	if err := loc.makeFormats(spec.Formats); err != nil {
		return nil, err
	}
	return loc, nil
}

// Format returns a textual representation of the time value
// formatted according to the strftime format specification,
// using the locale's month, weekday and meridiem names
// and composite formats.
func (l *Locale) Format(fmt string, t time.Time) string {
	buf := buffer(fmt)
	return string(l.AppendFormat(buf, fmt, t))
}

// AppendFormat is like Locale.Format, but appends the textual representation
// to dst and returns the extended buffer.
func (l *Locale) AppendFormat(dst []byte, fmt string, t time.Time) []byte {
	return appendFormat(dst, l.expand(fmt), t, l)
}

// Parse converts a textual representation of time to the time value it represents
// according to the strptime format specification,
// using the locale's month, weekday and meridiem names
// and composite formats.
//
// When names share common prefixes, the longest name
// that completes the current directive is matched.
// If the format specifies both a weekday and a day of the month,
// they are checked for consistency.
//
// The same specifiers and literal restrictions as Parse apply.
func (l *Locale) Parse(fmt, value string) (time.Time, error) {
	expanded := l.expand(fmt)
	pattern, err := layout(expanded, true)
	if err != nil {
		return time.Time{}, err
	}
	translated, weekday, hasDay, err := l.translate(expanded, value)
	if err != nil {
		return time.Time{}, err
	}
	t, err := time.Parse(pattern, translated)
	if err != nil {
		return time.Time{}, err
	}
	if weekday >= 0 && hasDay && t.Weekday() != time.Weekday(weekday) {
		return time.Time{}, parseError("weekday does not match date")
	}
	return t, nil
}

// expand replaces composite specifiers with their
// (possibly overridden) definitions.
func (l *Locale) expand(fmt string) string {
	if !strings.Contains(fmt, "%") {
		return fmt
	}
	dst := buffer(fmt)
	var parser parser

	parser.literal = func(b byte) error {
		dst = append(dst, b)
		return nil
	}

	parser.format = func(spec, flag byte) error {
		if format, ok := l.formats[spec]; ok {
			dst = append(dst, format...)
			return nil
		}
		dst = append(dst, '%')
		if flag != 0 {
			dst = append(dst, flag)
		}
		dst = append(dst, spec)
		return nil
	}

	parser.parse(fmt)
	return string(dst)
}

// translate replaces localized names in value with their English
// counterparts, so the result can be parsed by time.Parse.
// It walks fmt and value in lockstep, so literals are never
// consumed by name matching.
// It also reports the matched weekday, if any,
// and whether the format specifies a day of the month.
func (l *Locale) translate(fmt, value string) (string, int, bool, error) {
	dst := buffer(fmt)
	weekday := -1
	hasDay := false
	pos := 0

	var parser parser

	parser.literal = func(b byte) error {
		if pos < len(value) {
			// Copy the value byte, not the literal:
			// any mismatch is reported by time.Parse.
			dst = append(dst, value[pos])
			pos++
		}
		return nil
	}

	parser.format = func(spec, flag byte) error {
		rest := value[pos:]
		switch spec {
		case 'A', 'a':
			index, n, ok := l.weekdayNames.match(rest)
			if !ok {
				return parseError("cannot parse weekday name: " + rest)
			}
			name := time.Weekday(index).String()
			if spec == 'a' {
				name = name[:3]
			}
			dst = append(dst, name...)
			weekday = index
			pos += n

		case 'B', 'b', 'h':
			index, n, ok := l.monthNames.match(rest)
			if !ok {
				return parseError("cannot parse month name: " + rest)
			}
			name := time.Month(index + 1).String()
			if spec != 'B' {
				name = name[:3]
			}
			dst = append(dst, name...)
			pos += n

		case 'p', 'P':
			index, n, ok := l.ampmNames.match(rest)
			if !ok {
				return parseError("cannot parse meridiem: " + rest)
			}
			var name string
			switch {
			case index == 0 && spec == 'p':
				name = "AM"
			case index == 0:
				name = "am"
			case spec == 'p':
				name = "PM"
			default:
				name = "pm"
			}
			dst = append(dst, name...)
			pos += n

		default:
			if spec == 'd' || spec == 'e' || spec == 'j' {
				hasDay = true
			}
			n := skipValue(spec, flag, rest)
			dst = append(dst, rest[:n]...)
			pos += n
		}
		return nil
	}

	if err := parser.parse(fmt); err != nil {
		return "", 0, false, err
	}
	dst = append(dst, value[pos:]...)
	return string(dst), weekday, hasDay, nil
}

// defaultFormats defines the composite specifiers
// in terms of other specifiers.
var defaultFormats = map[byte]string{
	'+': "%a %b %e %T %Z %Y",
	'c': "%a %b %e %T %Y",
	'v': "%e-%b-%Y",
	'F': "%Y-%m-%d",
	'D': "%m/%d/%y",
	'x': "%m/%d/%y",
	'r': "%I:%M:%S %p",
	'R': "%H:%M",
	'T': "%H:%M:%S",
	'X': "%H:%M:%S",
}

func isComposite(spec byte) bool {
	switch spec {
	case '+', 'c', 'D', 'F', 'r', 'R', 'T', 'v', 'x', 'X':
		return true
	}
	return false
}

// makeFormats validates the composite format overrides
// and builds the fully expanded composite formats.
func (l *Locale) makeFormats(overrides map[byte]string) error {
	for spec, format := range overrides {
		if !isComposite(spec) {
			return localeError("unsupported composite format: %" + string(spec))
		}
		if format == "" {
			return localeError("empty composite format: %" + string(spec))
		}
	}

	l.formats = make(map[byte]string, len(defaultFormats))
	expanding := make(map[byte]bool)
	var expand func(spec byte) (string, error)
	expand = func(spec byte) (string, error) {
		if format, ok := l.formats[spec]; ok {
			return format, nil
		}
		if expanding[spec] {
			return "", localeError("composite format cycle: %" + string(spec))
		}
		expanding[spec] = true

		format, ok := overrides[spec]
		if !ok {
			format = defaultFormats[spec]
		}
		var expanded []byte
		var parser parser
		parser.literal = func(b byte) error {
			expanded = append(expanded, b)
			return nil
		}
		parser.format = func(spec, flag byte) error {
			if isComposite(spec) {
				format, err := expand(spec)
				if err != nil {
					return err
				}
				expanded = append(expanded, format...)
				return nil
			}
			expanded = append(expanded, '%')
			if flag != 0 {
				expanded = append(expanded, flag)
			}
			expanded = append(expanded, spec)
			return nil
		}

		err := parser.parse(format)
		delete(expanding, spec)
		if err != nil {
			return "", err
		}
		l.formats[spec] = string(expanded)
		return l.formats[spec], nil
	}

	for spec := range defaultFormats {
		if _, err := expand(spec); err != nil {
			return err
		}
	}
	return nil
}

// nameTable matches names against input,
// preferring the longest matching name.
type nameTable struct {
	names []string
	index []int
	fold  bool
}

func makeNameTable(what string, fold bool, sets ...[]string) (nameTable, error) {
	var table nameTable
	table.fold = fold
	for _, set := range sets {
		for i, name := range set {
			if name == "" {
				return table, localeError("empty " + what + " name")
			}
			table.names = append(table.names, name)
			table.index = append(table.index, i)
		}
	}
	for i, name := range table.names {
		for j := i + 1; j < len(table.names); j++ {
			if table.index[i] == table.index[j] {
				continue
			}
			if name == table.names[j] || fold && strings.EqualFold(name, table.names[j]) {
				return table, localeError("ambiguous " + what + " name: " + name)
			}
		}
	}
	return table, nil
}

// match returns the index and length of the longest name
// that is a prefix of s.
func (t *nameTable) match(s string) (index, length int, ok bool) {
	for i, name := range t.names {
		var n int
		var matched bool
		if t.fold {
			n, matched = foldPrefix(s, name)
		} else if strings.HasPrefix(s, name) {
			n, matched = len(name), true
		}
		if matched && n > length {
			index, length, ok = t.index[i], n, true
		}
	}
	return index, length, ok
}

// foldPrefix returns the length of the prefix of s
// that case-folds to name.
func foldPrefix(s, name string) (int, bool) {
	n := 0
	for _, r := range name {
		if n >= len(s) {
			return 0, false
		}
		r2, size := utf8.DecodeRuneInString(s[n:])
		if !foldEqual(r, r2) {
			return 0, false
		}
		n += size
	}
	return n, true
}

// foldEqual reports whether a and b are equal
// under Unicode simple case folding.
func foldEqual(a, b rune) bool {
	if a == b {
		return true
	}
	for f := unicode.SimpleFold(a); f != a; f = unicode.SimpleFold(f) {
		if f == b {
			return true
		}
	}
	return false
}

// skipValue returns the length of the prefix of s
// that a directive consumes, mirroring time.Parse.
func skipValue(spec, flag byte, s string) int {
	digits := func(s string, max int) int {
		n := 0
		for n < len(s) && n < max && '0' <= s[n] && s[n] <= '9' {
			n++
		}
		return n
	}
	switch spec {
	case 'm', 'd', 'H', 'I', 'M', 'S', 'y':
		return digits(s, 2)
	case 'Y':
		return digits(s, 4)
	case 'e':
		n := 0
		if n < len(s) && s[n] == ' ' {
			n++
		}
		return n + digits(s[n:], 2)
	case 'j':
		if flag == '-' {
			n := 0
			for n < len(s) && n < 2 && s[n] == ' ' {
				n++
			}
			return n + digits(s[n:], 3)
		}
		return digits(s, 3)
	case 'L':
		return digits(s, 3)
	case 'f':
		return digits(s, 6)
	case 'N':
		return digits(s, 9)
	case 'z':
		if len(s) > 0 && s[0] == 'Z' {
			return 1
		}
		n := 0
		if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
			n = 1 + digits(s[1:], 2)
			if flag == ':' {
				if n < len(s) && s[n] == ':' {
					n += 1 + digits(s[n+1:], 2)
				}
			} else {
				n += digits(s[n:], 2)
			}
		}
		return n
	case 'Z':
		return skipZone(s)
	case '%', 'n', 't':
		if len(s) > 0 {
			return 1
		}
	}
	return 0
}

// skipZone returns the length of the time zone
// at the beginning of s, mirroring time.Parse.
func skipZone(s string) int {
	if len(s) >= 3 {
		if s[:3] == "UTC" {
			return 3
		}
		if s[:3] == "GMT" {
			n := 3
			if n < len(s) && (s[n] == '+' || s[n] == '-') {
				n++
				for n < len(s) && ('0' <= s[n] && s[n] <= '9' || s[n] == ':') {
					n++
				}
			}
			return n
		}
	}
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		n := 1
		for n < len(s) && '0' <= s[n] && s[n] <= '9' {
			n++
		}
		return n
	}
	n := 0
	for n < len(s) && n < 5 && 'A' <= s[n] && s[n] <= 'Z' {
		n++
	}
	if n >= 3 {
		return n
	}
	return 0
}

type localeError string

func (e localeError) Error() string {
	return "strftime: invalid locale: " + string(e)
}

type parseError string

func (e parseError) Error() string {
	return "strftime: " + string(e)
}
