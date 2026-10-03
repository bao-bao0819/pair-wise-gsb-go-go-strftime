package strftime

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PivotYear controls how two-digit week-based years (%g) are expanded
// when parsing. A two-digit year is mapped to the 100-year window
// [PivotYear, PivotYear+100). The default, 1969, maps 69–99 to 1969–1999
// and 00–68 to 2000–2068, matching the POSIX rule for %y.
var PivotYear = 1969

// parseWeekDate parses value according to format,
// if format contains ISO 8601 week date directives
// (%G, %g, %V or %u). It reports ok as false
// if format contains no such directives.
func parseWeekDate(format, value string) (t time.Time, ok bool, err error) {
	var s isoScanner
	p := parser{format: s.format, literal: s.literal}
	if err := p.parse(format); err != nil {
		return t, false, err
	}
	if !s.hasISO() {
		return t, false, nil
	}
	ok = true

	if !(s.year4 || s.year2) || !s.week || !s.weekday {
		return t, ok, errors.New("strftime: ISO week date requires %G (or %g), %V and %u")
	}

	rx, err := regexp.Compile(`^(?:` + s.pattern.String() + `)$`)
	if err != nil {
		return t, ok, err
	}
	m := rx.FindStringSubmatchIndex(value)
	if m == nil {
		return t, ok, &time.ParseError{
			Layout: format, Value: value,
			Message: ": does not match format",
		}
	}

	var spans [][2]int
	group := func(name string) string {
		i := rx.SubexpIndex(name)
		if i < 0 {
			return ""
		}
		spans = append(spans, [2]int{m[2*i], m[2*i+1]})
		return value[m[2*i]:m[2*i+1]]
	}
	year4, year2 := group("isoy"), group("isoy2")
	week, weekday := group("isow"), group("isou")

	// Remove the ISO week date fields from the value,
	// so the rest can be parsed by time.Parse.
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	var cleaned strings.Builder
	prev := 0
	for _, span := range spans {
		cleaned.WriteString(value[prev:span[0]])
		prev = span[1]
	}
	cleaned.WriteString(value[prev:])

	var year int
	if year4 != "" {
		year, err = strconv.Atoi(year4)
	} else {
		var yy int
		yy, err = strconv.Atoi(year2)
		year = expandYear2(yy)
	}
	if err != nil {
		return t, ok, err
	}
	w, _ := strconv.Atoi(week)
	d, _ := strconv.Atoi(weekday)

	if d < 1 || d > 7 {
		return t, ok, fmt.Errorf("strftime: ISO weekday %d out of range [1,7]", d)
	}
	if w < 1 || w > 53 {
		return t, ok, fmt.Errorf("strftime: ISO week %d out of range [1,53]", w)
	}
	if w == 53 && isoWeeksInYear(year) != 53 {
		return t, ok, fmt.Errorf("strftime: ISO year %d has no week 53", year)
	}
	date := isoWeekDate(year, w, d)

	layout, err := layout(s.cleaned.String(), true)
	if err != nil {
		return t, ok, err
	}
	tm, err := time.Parse(layout, cleaned.String())
	if err != nil {
		return t, ok, err
	}

	// If calendar date directives are also present,
	// both representations must agree on the date.
	if s.hasYear && tm.Year() != date.Year() ||
		s.hasMonth && tm.Month() != date.Month() ||
		s.hasDay && tm.Day() != date.Day() ||
		s.hasYDay && tm.YearDay() != date.YearDay() {
		return t, ok, fmt.Errorf(
			"strftime: calendar date %s does not match ISO week date %s",
			tm.Format("2006-01-02"), isoWeekString(year, w, d))
	}

	return time.Date(date.Year(), date.Month(), date.Day(),
		tm.Hour(), tm.Minute(), tm.Second(), tm.Nanosecond(),
		tm.Location()), true, nil
}

// isoScanner scans a format specification, removing ISO week date
// directives (%G, %g, %V, %u), and building a regular expression
// that extracts them from a value.
type isoScanner struct {
	cleaned strings.Builder
	pattern strings.Builder

	year4, year2, week, weekday        bool
	hasYear, hasMonth, hasDay, hasYDay bool
}

func (s *isoScanner) hasISO() bool {
	return s.year4 || s.year2 || s.week || s.weekday
}

func (s *isoScanner) literal(b byte) error {
	s.cleaned.WriteByte(b)
	s.pattern.WriteString(regexp.QuoteMeta(string([]byte{b})))
	return nil
}

func (s *isoScanner) format(spec, flag byte) error {
	switch spec {
	case 'G':
		s.group(&s.year4, "isoy", `-?\d+`)
		return nil
	case 'g':
		s.group(&s.year2, "isoy2", `\d{2}`)
		return nil
	case 'V':
		s.group(&s.week, "isow", `\d{1,2}`)
		return nil
	case 'u':
		s.group(&s.weekday, "isou", `\d`)
		return nil
	}

	re, err := s.directive(spec, flag)
	if err != nil {
		return err
	}
	s.cleaned.WriteByte('%')
	if flag != 0 {
		s.cleaned.WriteByte(flag)
	}
	s.cleaned.WriteByte(spec)
	s.pattern.WriteString(re)
	return nil
}

func (s *isoScanner) group(seen *bool, name, re string) {
	if *seen {
		s.pattern.WriteString("(?:" + re + ")")
	} else {
		*seen = true
		s.pattern.WriteString("(?P<" + name + ">" + re + ")")
	}
}

func (s *isoScanner) directive(spec, flag byte) (string, error) {
	switch spec {
	default:
		return "", formatError{}

	case 'Y':
		s.hasYear = true
		return `-?\d+`, nil
	case 'y':
		s.hasYear = true
		return `\d{2}`, nil
	case 'm':
		s.hasMonth = true
		return `\d{1,2}`, nil
	case 'd':
		s.hasDay = true
		return `\d{1,2}`, nil
	case 'e':
		s.hasDay = true
		return `\s?\d{1,2}`, nil
	case 'j':
		s.hasYDay = true
		if flag == '-' {
			return `\s{0,2}\d{1,3}`, nil
		}
		return `\d{3}`, nil

	case 'B', 'b', 'h', 'A', 'a':
		return `\pL+`, nil
	case 'I', 'H', 'M', 'S':
		return `\d{1,2}`, nil
	case 'p', 'P':
		return `(?i:[ap]m)`, nil
	case 'Z':
		return `[A-Za-z]+`, nil
	case 'z':
		if flag == ':' {
			return `(?:Z|[+-]\d{2}:\d{2})`, nil
		}
		return `(?:Z|[+-]\d{4})`, nil
	case 'L':
		return `\d{3}`, nil
	case 'f':
		return `\d{6}`, nil
	case 'N':
		return `\d{9}`, nil

	case '%':
		return `%`, nil
	case 't':
		return `\t`, nil
	case 'n':
		return `\n`, nil

	case 'c':
		return s.combo("%a %b %e %T %Y")
	case '+':
		return s.combo("%a %b %e %T %Z %Y")
	case 'D', 'x':
		return s.combo("%m/%d/%y")
	case 'F':
		return s.combo("%Y-%m-%d")
	case 'v':
		return s.combo("%e-%b-%Y")
	case 'r':
		return s.combo("%I:%M:%S %p")
	case 'R':
		return s.combo("%H:%M")
	case 'T', 'X':
		return s.combo("%H:%M:%S")
	}
}

func (s *isoScanner) combo(format string) (string, error) {
	var b strings.Builder
	p := parser{
		literal: func(c byte) error {
			b.WriteString(regexp.QuoteMeta(string([]byte{c})))
			return nil
		},
		format: func(spec, flag byte) error {
			re, err := s.directive(spec, flag)
			if err != nil {
				return err
			}
			b.WriteString(re)
			return nil
		},
	}
	if err := p.parse(format); err != nil {
		return "", err
	}
	return b.String(), nil
}

// expandYear2 expands a two-digit year to the 100-year window
// [PivotYear, PivotYear+100).
func expandYear2(yy int) int {
	century := PivotYear / 100
	if PivotYear < 0 && PivotYear%100 != 0 {
		century--
	}
	year := century*100 + yy
	if year < PivotYear {
		year += 100
	}
	return year
}

// isoWeeksInYear returns the number of ISO 8601 weeks in a year.
// A year has 53 weeks if it starts on a Thursday,
// or if it is a leap year that starts on a Wednesday.
func isoWeeksInYear(year int) int {
	switch jan1 := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC); jan1.Weekday() {
	case time.Thursday:
		return 53
	case time.Wednesday:
		if isLeapYear(year) {
			return 53
		}
	}
	return 52
}

func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// isoWeekDate converts an ISO 8601 week date to a calendar date,
// using calendar arithmetic, never fixed-length days.
func isoWeekDate(year, week, weekday int) time.Time {
	// January 4th is always in week 1.
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	monday := jan4.AddDate(0, 0, -(int(jan4.Weekday())+6)%7)
	return monday.AddDate(0, 0, 7*(week-1)+weekday-1)
}

func isoWeekString(year, week, weekday int) string {
	return fmt.Sprintf("%04d-W%02d-%d", year, week, weekday)
}
