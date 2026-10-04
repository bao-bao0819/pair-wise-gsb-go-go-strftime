package strftime

import (
	"strings"
	"time"
)

// Verdict summarizes whether a strftime format is suitable for
// writing a time out and reading it back with Parse.
type Verdict int

const (
	// Reversible formats carry enough information to losslessly
	// round-trip any time.Time through Format and Parse.
	Reversible Verdict = iota
	// Conditional formats round-trip only under external conditions:
	// a pivot window for two-digit years, a zone database for zone
	// names, or assumptions about components missing from the output.
	Conditional
	// Unparseable formats contain directives that Parse
	// cannot read back at all.
	Unparseable
)

func (v Verdict) String() string {
	switch v {
	case Reversible:
		return "reversible"
	case Conditional:
		return "conditional"
	case Unparseable:
		return "unparseable"
	}
	return "invalid"
}

// IssueKind classifies the problems found by Analyze.
type IssueKind int

const (
	// IssueUnsupported marks directives that cannot be parsed back.
	IssueUnsupported IssueKind = iota
	// IssueConflict marks mutually contradictory directives.
	IssueConflict
	// IssueMissing marks information missing from the output.
	IssueMissing
	// IssueLossy marks output that needs external conditions
	// to be read back exactly as written.
	IssueLossy
)

func (k IssueKind) String() string {
	switch k {
	case IssueUnsupported:
		return "unsupported"
	case IssueConflict:
		return "conflict"
	case IssueMissing:
		return "missing"
	case IssueLossy:
		return "lossy"
	}
	return "invalid"
}

// Issue describes a single problem found by Analyze.
type Issue struct {
	Kind      IssueKind
	Directive string // the directive responsible, or "" for the whole format
	Reason    string
}

func (i Issue) String() string {
	if i.Directive != "" {
		return i.Kind.String() + ": " + i.Directive + ": " + i.Reason
	}
	return i.Kind.String() + ": " + i.Reason
}

// Analysis reports what the output of a strftime format carries.
type Analysis struct {
	Date      bool          // a full calendar date is carried
	Time      bool          // a wall clock time is carried (hour and minute at least)
	Precision time.Duration // finest clock precision carried, zero if no time is carried
	Zone      bool          // a UTC offset is carried
	Verdict   Verdict       // overall suitability for round-tripping
	Issues    []Issue       // every problem found, in directive order

	format string
}

// RoundTrip reports the result of formatting a time with an analyzed
// format and parsing the output back.
type RoundTrip struct {
	Formatted string        // the output of Format
	Parsed    time.Time     // the result of Parse, if it succeeded
	Err       error         // the parse error, if any
	Delta     time.Duration // Parsed minus the original time
	Exact     bool          // whether the original instant was recovered
}

// Analyze statically inspects a strftime format and reports whether
// its output can be parsed back into the time that produced it.
//
// Composite directives like %c or %F are expanded before analysis;
// conclusions are drawn from the directives themselves,
// never from formatting a sample time.
func Analyze(format string) Analysis {
	a := Analysis{format: format}
	an := analyzer{comps: map[component]string{}}
	an.parse(format, 0)
	an.derive(&a)
	a.Issues = an.issues
	return a
}

// RoundTrip formats t with the analyzed format and parses the output
// back, reporting the difference. A successful round trip only proves
// the format works for t: it does not weaken the static Verdict,
// which accounts for every possible time.
func (a Analysis) RoundTrip(t time.Time) RoundTrip {
	r := RoundTrip{Formatted: Format(a.format, t)}
	parsed, err := Parse(a.format, r.Formatted)
	if err != nil {
		r.Err = err
		return r
	}
	r.Parsed = parsed
	r.Delta = parsed.Sub(t)
	r.Exact = parsed.Equal(t)
	return r
}

type component int

const (
	compYear component = iota
	compYear2
	compCentury
	compMonth
	compDay
	compYearDay
	compHour24
	compHour12
	compMeridian
	compMinute
	compSecond
	compMilli
	compMicro
	compNano
	compOffset
	compZoneName
	compISOYear
	compISOYear2
	compISOWeek
	compWeekday
	compWeekSunday
	compWeekMonday
	compUnix
	compUnixMilli
)

// composites expands combination directives, as documented for Format.
var composites = map[byte]string{
	'c': "%a %b %e %T %Y",
	'+': "%a %b %e %H:%M:%S %Z %Y",
	'D': "%m/%d/%y",
	'x': "%m/%d/%y",
	'F': "%Y-%m-%d",
	'v': "%e-%b-%Y",
	'r': "%I:%M:%S %p",
	'R': "%H:%M",
	'T': "%H:%M:%S",
	'X': "%H:%M:%S",
}

type analyzer struct {
	issues []Issue
	comps  map[component]string
	last   byte // last literal byte seen
}

func (an *analyzer) parse(format string, depth int) {
	if depth > 8 {
		return
	}
	var p parser
	p.literal = func(b byte) error {
		an.last = b
		return nil
	}
	p.format = func(spec, flag byte) error {
		an.directive(spec, depth)
		return nil
	}
	p.parse(format) // the callbacks never fail
}

func (an *analyzer) directive(spec byte, depth int) {
	last := an.last
	an.last = 0
	if exp, ok := composites[spec]; ok {
		an.parse(exp, depth+1)
		return
	}
	switch spec {
	case 'Y':
		an.add(compYear, "%Y")
	case 'y':
		an.add(compYear2, "%y")
	case 'C':
		an.add(compCentury, "%C")
		an.unsupported("%C")
	case 'm', 'b', 'B', 'h':
		an.add(compMonth, "%"+string(spec))
	case 'd', 'e':
		an.add(compDay, "%"+string(spec))
	case 'j':
		an.add(compYearDay, "%j")
	case 'H':
		an.add(compHour24, "%H")
	case 'k':
		an.add(compHour24, "%k")
		an.unsupported("%k")
	case 'I':
		an.add(compHour12, "%I")
	case 'l':
		an.add(compHour12, "%l")
		an.unsupported("%l")
	case 'p', 'P':
		an.add(compMeridian, "%"+string(spec))
	case 'M':
		an.add(compMinute, "%M")
	case 'S':
		an.add(compSecond, "%S")
	case 'L':
		an.add(compMilli, "%L")
		an.subsec("%L", last)
	case 'f':
		an.add(compMicro, "%f")
		an.subsec("%f", last)
	case 'N':
		an.add(compNano, "%N")
		an.subsec("%N", last)
	case 'z':
		an.add(compOffset, "%z")
	case 'Z':
		an.add(compZoneName, "%Z")
	case 'G':
		an.add(compISOYear, "%G")
		an.unsupported("%G")
	case 'g':
		an.add(compISOYear2, "%g")
		an.unsupported("%g")
	case 'V':
		an.add(compISOWeek, "%V")
		an.unsupported("%V")
	case 'U':
		an.add(compWeekSunday, "%U")
		an.unsupported("%U")
	case 'W':
		an.add(compWeekMonday, "%W")
		an.unsupported("%W")
	case 'a', 'A':
		an.add(compWeekday, "%"+string(spec))
	case 'u':
		an.add(compWeekday, "%u")
		an.unsupported("%u")
	case 'w':
		an.add(compWeekday, "%w")
		an.unsupported("%w")
	case 's':
		an.add(compUnix, "%s")
		an.unsupported("%s")
	case 'Q':
		an.add(compUnixMilli, "%Q")
		an.unsupported("%Q")
	case '%':
		an.last = '%'
	case 't':
		an.last = '\t'
	case 'n':
		an.last = '\n'
	default:
		an.issues = append(an.issues, Issue{IssueUnsupported, "%" + string(spec), "unknown directive"})
	}
}

func (an *analyzer) add(c component, directive string) {
	if _, ok := an.comps[c]; !ok {
		an.comps[c] = directive
	}
}

func (an *analyzer) unsupported(directive string) {
	an.issues = append(an.issues, Issue{IssueUnsupported, directive, "not supported for parsing"})
}

func (an *analyzer) subsec(directive string, last byte) {
	if last != '.' && last != ',' {
		an.issues = append(an.issues, Issue{IssueUnsupported, directive, "must follow '.' or ',' to be parseable"})
	}
}

func (an *analyzer) derive(a *Analysis) {
	has := func(c component) bool { _, ok := an.comps[c]; return ok }
	dir := func(cs ...component) string {
		for _, c := range cs {
			if d := an.comps[c]; d != "" {
				return d
			}
		}
		return ""
	}
	add := func(kind IssueKind, directive, reason string) {
		an.issues = append(an.issues, Issue{kind, directive, reason})
	}

	unix := has(compUnix) || has(compUnixMilli)
	isoYear := has(compISOYear) || has(compISOYear2)
	iso := isoYear || has(compISOWeek)
	year := has(compYear) || has(compYear2) || has(compCentury)
	hour := has(compHour24) || has(compHour12)
	clock := hour || has(compMinute) || has(compSecond) ||
		has(compMilli) || has(compMicro) || has(compNano)
	calendar := year || has(compMonth) || has(compDay) || has(compYearDay)

	// Conflicts between directives.
	if unix && (calendar || iso || clock) {
		add(IssueConflict, dir(compUnix, compUnixMilli),
			"epoch directives are self-sufficient, and cannot be mixed with calendar or clock directives")
	}
	if has(compHour24) && has(compHour12) {
		add(IssueConflict, dir(compHour12), "12-hour and 24-hour clocks are mixed")
	}
	if has(compYear) && has(compYear2) {
		add(IssueConflict, dir(compYear2), "full and two-digit years are mixed")
	}
	if has(compYearDay) && (has(compMonth) || has(compDay)) {
		add(IssueConflict, dir(compYearDay), "day of the year is mixed with month and day")
	}
	if iso && calendar {
		add(IssueConflict, dir(compISOYear, compISOYear2, compISOWeek),
			"ISO week date is mixed with a calendar date")
	}
	subsec := 0
	for _, c := range []component{compMilli, compMicro, compNano} {
		if has(c) {
			subsec++
		}
	}
	if subsec > 1 {
		add(IssueConflict, dir(compMilli, compMicro, compNano), "multiple sub-second directives")
	}

	// Missing or ambiguous companions.
	if has(compHour12) && !has(compMeridian) {
		add(IssueMissing, dir(compHour12), "12-hour clock without %p: AM/PM is ambiguous")
	}
	if has(compMeridian) && !has(compHour12) {
		add(IssueMissing, dir(compMeridian), "meridian indicator without a 12-hour clock (%I)")
	}
	if has(compCentury) && !has(compYear2) {
		add(IssueMissing, dir(compCentury), "century without %y")
	}
	if has(compISOWeek) && !isoYear {
		add(IssueMissing, dir(compISOWeek), "ISO week number without a week-based year (%G)")
	}
	if isoYear && !has(compISOWeek) {
		add(IssueMissing, dir(compISOYear, compISOYear2), "ISO week-based year without a week number (%V)")
	}
	if iso && !has(compWeekday) {
		add(IssueMissing, dir(compISOWeek, compISOYear, compISOYear2), "ISO week date without a weekday (%u)")
	}

	// Lossy components.
	if has(compYear2) && !has(compYear) && !has(compCentury) {
		add(IssueLossy, dir(compYear2), "two-digit year needs an external pivot window (Parse assumes 1969-2068)")
	}

	// Date completeness.
	fullYear := has(compYear) || has(compYear2) || (has(compCentury) && has(compYear2))
	calDate := fullYear && ((has(compMonth) && has(compDay)) || has(compYearDay))
	isoDate := isoYear && has(compISOWeek) && has(compWeekday)
	a.Date = unix || calDate || isoDate
	if !a.Date {
		if calendar || iso {
			var missing []string
			if !fullYear {
				missing = append(missing, "year")
			}
			if !has(compYearDay) {
				if !has(compMonth) {
					missing = append(missing, "month")
				}
				if !has(compDay) {
					missing = append(missing, "day")
				}
			}
			add(IssueMissing, "", "incomplete date: missing "+strings.Join(missing, ", "))
		} else {
			add(IssueMissing, "", "output carries no date")
		}
	}

	// Time completeness.
	a.Time = unix || (hour && has(compMinute))
	if !a.Time {
		if clock {
			var missing []string
			if !hour {
				missing = append(missing, "hour")
			}
			if !has(compMinute) {
				missing = append(missing, "minute")
			}
			add(IssueMissing, "", "incomplete time of day: missing "+strings.Join(missing, ", "))
		} else {
			add(IssueMissing, "", "output carries no time of day")
		}
	}

	// Precision.
	switch {
	case has(compNano):
		a.Precision = time.Nanosecond
	case has(compMicro):
		a.Precision = time.Microsecond
	case has(compMilli), has(compUnixMilli):
		a.Precision = time.Millisecond
	case has(compSecond), has(compUnix):
		a.Precision = time.Second
	case has(compMinute):
		a.Precision = time.Minute
	case hour:
		a.Precision = time.Hour
	}
	if a.Time && a.Precision > time.Nanosecond {
		add(IssueLossy, "", "clock is truncated to "+precision(a.Precision)+" precision")
	}

	// Time zone.
	a.Zone = unix || has(compOffset)
	if !a.Zone && (a.Date || a.Time) {
		if has(compZoneName) {
			add(IssueLossy, dir(compZoneName), "a zone name cannot be mapped back to a unique UTC offset")
		} else {
			add(IssueLossy, "", "no zone offset: Parse assumes UTC")
		}
	}

	// Backstop: anything else that makes the format unparseable.
	if _, err := layout(a.format, true); err != nil {
		unsupported := false
		for _, i := range an.issues {
			unsupported = unsupported || i.Kind == IssueUnsupported
		}
		if !unsupported {
			add(IssueUnsupported, "", err.Error())
		}
	}

	// Verdict.
	a.Verdict = Reversible
	for _, i := range an.issues {
		if i.Kind == IssueUnsupported {
			a.Verdict = Unparseable
		}
	}
	if a.Verdict == Reversible && len(an.issues) > 0 {
		a.Verdict = Conditional
	}
}

func precision(d time.Duration) string {
	switch d {
	case time.Hour:
		return "hour"
	case time.Minute:
		return "minute"
	case time.Second:
		return "second"
	case time.Millisecond:
		return "millisecond"
	case time.Microsecond:
		return "microsecond"
	case time.Nanosecond:
		return "nanosecond"
	}
	return d.String()
}
