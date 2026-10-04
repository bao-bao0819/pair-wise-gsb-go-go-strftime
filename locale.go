package strftime

import (
	"errors"
	"strings"
	"time"
)

// LocaleData configures a [Locale].
//
// Months and Weekdays hold the full and abbreviated names,
// indexed like [time.Month] and [time.Weekday]:
// months from January (index 0), weekdays from Sunday (index 0).
// AM and PM hold the meridiem names (uppercase, as used by %p;
// %P renders the same names in lowercase).
// Formats maps combination specifiers (e.g. "c", "r") to strftime
// specifications, which may reference each other.
//
// Every slice must have the exact number of non-empty entries,
// and every map value must be non-empty; otherwise [NewLocale]
// returns an error. NewLocale copies the slices and map,
// so later changes to them do not affect the [Locale].
type LocaleData struct {
	Months        []string // full month names, indexed January=0, length 12
	MonthAbbrev   []string // abbreviated month names, indexed January=0, length 12
	Weekdays      []string // full weekday names, indexed Sunday=0, length 7
	WeekdayAbbrev []string // abbreviated weekday names, indexed Sunday=0, length 7
	AM            string   // meridiem name used for hours [0,12)
	PM            string   // meridiem name used for hours [12,24)
	Formats       map[string]string
}

// A Locale holds localized month, weekday, and meridiem names,
// and combination formats, for use with [Locale.Format] and
// [Locale.Parse].
//
// A Locale is safe for concurrent use by multiple goroutines.
// Locales never read the operating system language configuration.
type Locale struct {
	months   nameMatcher
	weekdays nameMatcher
	meridiem nameMatcher
	am       string
	pm       string
	formats  map[string]string
	english  bool
}

// NewLocale returns a [Locale] built from data.
//
// It rejects empty names, names that match exactly or under
// simple case folding to more than one entry of the same kind,
// and combination formats that reference each other in a cycle
// or contain unsupported directives.
func NewLocale(data LocaleData) (*Locale, error) {
	if len(data.Months) != 12 {
		return nil, errors.New("strftime: Months must have length 12")
	}
	if len(data.MonthAbbrev) != 12 {
		return nil, errors.New("strftime: MonthAbbrev must have length 12")
	}
	if len(data.Weekdays) != 7 {
		return nil, errors.New("strftime: Weekdays must have length 7")
	}
	if len(data.WeekdayAbbrev) != 7 {
		return nil, errors.New("strftime: WeekdayAbbrev must have length 7")
	}

	loc := &Locale{
		am:      data.AM,
		pm:      data.PM,
		formats: make(map[string]string, len(data.Formats)),
	}

	var err error
	if loc.months, err = newNameMatcher("month", data.Months, data.MonthAbbrev); err != nil {
		return nil, err
	}
	if loc.weekdays, err = newNameMatcher("weekday", data.Weekdays, data.WeekdayAbbrev); err != nil {
		return nil, err
	}

	loc.meridiem = nameMatcher{
		kind:  "meridiem",
		exact: map[string]int{},
		fold:  map[string]int{},
	}
	if err := loc.meridiem.add(loc.am, 0); err != nil {
		return nil, err
	}
	if err := loc.meridiem.add(loc.pm, 1); err != nil {
		return nil, err
	}
	if loc.am == "" || loc.pm == "" {
		return nil, errors.New("strftime: empty meridiem name")
	}
	loc.meridiem.finalize()

	for key, value := range data.Formats {
		if len(key) != 1 || !isFormatKey(key[0]) {
			return nil, errors.New("strftime: invalid format key: %" + key)
		}
		if strings.TrimSpace(value) == "" {
			return nil, errors.New("strftime: empty format for key: %" + key)
		}
		loc.formats[key] = value
	}

	for key, value := range loc.formats {
		// Validate syntax and expand with cycle detection.
		expanded, err := loc.expand(value, []byte{key[0]})
		if err != nil || strings.TrimSpace(expanded) == "" {
			if err == nil {
				err = errors.New("empty format")
			}
			return nil, errors.New("strftime: bad format for key %" + key + ": " + err.Error())
		}
		if err := validatePrimitives(expanded); err != nil {
			return nil, errors.New("strftime: bad format for key %" + key + ": " + err.Error())
		}
	}

	return loc, nil
}

// englishLocale is the [Locale] equivalent of the package-level functions.
var englishLocale = &Locale{english: true}

func isAlpha(b byte) bool {
	return 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
}

func isFormatKey(b byte) bool {
	return isAlpha(b) || b >= '0' && b <= '9'
}

// validatePrimitives checks that every directive in a fully expanded
// format (no combinations left) is a recognized primitive specifier.
func validatePrimitives(fmt string) error {
	var parser parser
	parser.literal = func(byte) error { return nil }
	parser.format = func(spec, flag byte) error {
		if goLayout(spec, flag, true) != "" {
			return nil
		}
		// Specifiers handled by the locale format/parse engines but
		// without a Go layout equivalent.
		switch spec {
		case 'A', 'a', 'B', 'b', 'h', 'p', 'P', 'C', 'g', 'G', 'U', 'V',
			'W', 'w', 'u', 's', 'Q', 'L', 'f', 'N', 'j', 'e', 'k', 'l':
			return nil
		}
		return formatError{}
	}
	return parser.parse(fmt)
}

// expand expands every combination directive in fmt into primitive
// directives and literals. Custom formats take precedence over the
// built-in combinations. stack detects reference cycles.
func (l *Locale) expand(fmt string, stack []byte) (string, error) {
	var dst strings.Builder
	dst.Grow(len(fmt) * 2)
	var parser parser

	parser.literal = func(b byte) error {
		dst.WriteByte(b)
		return nil
	}

	parser.format = func(spec, flag byte) error {
		value, custom := l.lookupFormat(spec)
		if value == "" || flag != 0 {
			dst.WriteByte('%')
			if flag != 0 {
				dst.WriteByte(flag)
			}
			dst.WriteByte(spec)
			return nil
		}
		if custom {
			for _, k := range stack {
				if k == spec {
					return errors.New("cycle in combination formats")
				}
			}
			stack = append(stack, spec)
		}
		expanded, err := l.expand(value, stack)
		if err != nil {
			return err
		}
		dst.WriteString(expanded)
		return nil
	}

	if err := parser.parse(fmt); err != nil {
		return "", err
	}
	return dst.String(), nil
}

// lookupFormat returns the format specification for a combination
// directive and whether it is a user-provided override.
func (l *Locale) lookupFormat(spec byte) (string, bool) {
	if l.formats != nil {
		if value, ok := l.formats[string(spec)]; ok {
			return value, true
		}
	}
	return builtinFormat(spec), false
}

func builtinFormat(spec byte) string {
	switch spec {
	case 'c':
		return "%a %b %e %T %Y"
	case '+':
		return "%a %b %e %H:%M:%S %Z %Y"
	case 'D':
		return "%m/%d/%y"
	case 'x':
		return "%m/%d/%y"
	case 'F':
		return "%Y-%m-%d"
	case 'v':
		return "%e-%b-%Y"
	case 'X':
		return "%T"
	case 'r':
		return "%I:%M:%S %p"
	case 'R':
		return "%H:%M"
	case 'T':
		return "%H:%M:%S"
	default:
		return ""
	}
}

// monthName returns the full or abbreviated name of month m.
func (l *Locale) monthName(m time.Month, abbrev bool) string {
	switch {
	case l.english:
		if abbrev {
			return m.String()[:3]
		}
		return m.String()
	default:
		idx := int(m) - 1
		if abbrev {
			return l.months.entries[l.months.abbrevOffset+idx]
		}
		return l.months.entries[idx]
	}
}

// weekdayName returns the full or abbreviated name of weekday w.
func (l *Locale) weekdayName(w time.Weekday, abbrev bool) string {
	switch {
	case l.english:
		if abbrev {
			return w.String()[:3]
		}
		return w.String()
	default:
		idx := int(w)
		if abbrev {
			return l.weekdays.entries[l.weekdays.abbrevOffset+idx]
		}
		return l.weekdays.entries[idx]
	}
}

func (l *Locale) meridian(hour int) string {
	if hour >= 12 {
		return l.pm
	}
	return l.am
}
