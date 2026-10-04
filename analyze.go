package strftime

import (
	"sort"
	"strings"
	"time"
)

// Verdict is the static conclusion of a format analysis.
type Verdict int

const (
	// Reversible means the format carries every piece of information
	// needed to recover the exact time instant: a full date,
	// an unambiguous time of day, nanosecond precision
	// (or no fractional seconds at all), and a fixed UTC offset.
	Reversible Verdict = iota
	// Conditional means the format is parseable in principle,
	// but the written value cannot be pinned to a single instant
	// without an assumption supplied by the caller
	// (a two-digit-year window, a meridian, a time zone, and so on).
	Conditional
	// Unparseable means the format cannot reliably be read back at all
	// with this package, either because it contains directives that
	// have no parsing counterpart, an invalid layout,
	// or contradictory date/time/time-zone fields.
	Unparseable
)

// String returns "reversible", "conditional", or "unparseable".
func (v Verdict) String() string {
	switch v {
	case Reversible:
		return "reversible"
	case Conditional:
		return "conditional"
	default:
		return "unparseable"
	}
}

// Precision describes the finest fraction of a second a format carries.
type Precision int

const (
	// SecondPrecision: no fractional seconds.
	SecondPrecision Precision = iota
	// MillisecondPrecision: %L, three digits.
	MillisecondPrecision
	// MicrosecondPrecision: %f, six digits.
	MicrosecondPrecision
	// NanosecondPrecision: %N, nine digits.
	NanosecondPrecision
)

// String returns "second", "millisecond", "microsecond", or "nanosecond".
func (p Precision) String() string {
	switch p {
	case MillisecondPrecision:
		return "millisecond"
	case MicrosecondPrecision:
		return "microsecond"
	case NanosecondPrecision:
		return "nanosecond"
	default:
		return "second"
	}
}

// Severity classifies a Finding.
type Severity int

const (
	// Note points at information that is redundant or merely unusual.
	Note Severity = iota
	// Warning marks information that is missing or ambiguous;
	// the format needs an external condition to round-trip.
	Warning
	// Error marks something that makes the format unparseable or contradictory.
	Error
)

// String returns "note", "warning", or "error".
func (s Severity) String() string {
	switch s {
	case Warning:
		return "warning"
	case Error:
		return "error"
	default:
		return "note"
	}
}

// FindingCode identifies a particular static issue or observation.
type FindingCode string

const (
	// FindingUnknownDirective: directive is not recognized by this package.
	FindingUnknownDirective FindingCode = "unknown_directive"
	// FindingUnsupportedDirective: directive formats, but has no parsing counterpart.
	FindingUnsupportedDirective FindingCode = "unsupported_directive"
	// FindingInvalidFraction: %L/%f/%N must immediately follow '.' or ','.
	FindingInvalidFraction FindingCode = "invalid_fraction_placement"
	// FindingInvalidLiteral: layout literal breaks parsing (digits, Jan, Mon, MST, PM, pm).
	FindingInvalidLiteral FindingCode = "invalid_literal"

	// FindingTwoDigitYear: %y/%g needs a century window to recover the year.
	FindingTwoDigitYear FindingCode = "two_digit_year"
	// Finding12HourNoMeridian: %I/%l without %p/%P defaults to AM.
	Finding12HourNoMeridian FindingCode = "12_hour_no_meridian"

	// FindingMissingDate: no field identifies the date.
	FindingMissingDate FindingCode = "missing_date"
	// FindingMissingMonth: year present, month absent.
	FindingMissingMonth FindingCode = "missing_month"
	// FindingMissingDay: year and month present, day absent.
	FindingMissingDay FindingCode = "missing_day"
	// FindingMissingTime: no field identifies the time of day.
	FindingMissingTime FindingCode = "missing_time"
	// FindingMissingMinute: hour present, minute absent.
	FindingMissingMinute FindingCode = "missing_minute"
	// FindingMissingSecond: hour and minute present, second absent.
	FindingMissingSecond FindingCode = "missing_second"

	// FindingSubsecondTruncated: anything below the written precision is lost.
	FindingSubsecondTruncated FindingCode = "subsecond_truncated"

	// FindingZoneNameOnly: %Z resolves abbreviations ambiguously (unknown names get a zero offset).
	FindingZoneNameOnly FindingCode = "zone_name_only"
	// FindingMissingZone: the UTC offset is not recoverable.
	FindingMissingZone FindingCode = "missing_zone"
	// FindingZoneConflict: a UTC marker ('Z', "UTC", "GMT") coexists with another zone field.
	FindingZoneConflict FindingCode = "zone_conflict"

	// FindingISOWeekIncomplete: %G/%g/%V must appear together with %u/%w to name a date.
	FindingISOWeekIncomplete FindingCode = "iso_week_incomplete"
	// FindingWeekNumberIncomplete: %U/%W must be combined with a year and weekday to name a date.
	FindingWeekNumberIncomplete FindingCode = "week_number_incomplete"
	// FindingDateSystemConflict: two incompatible ways of naming a date coexist.
	FindingDateSystemConflict FindingCode = "date_system_conflict"
	// FindingTimeSystemConflict: 12-hour and 24-hour hour fields coexist.
	FindingTimeSystemConflict FindingCode = "time_system_conflict"
	// FindingMeridianConflict: both %p and %P coexist.
	FindingMeridianConflict FindingCode = "meridian_conflict"
	// FindingEpochConflict: %s/%Q already pin an instant and contradict other fields.
	FindingEpochConflict FindingCode = "epoch_conflict"

	// FindingRedundantField: a field repeats or is derivable from the others.
	FindingRedundantField FindingCode = "redundant_field"
)

// Finding is a single observation about an analyzed format.
type Finding struct {
	// Code identifies the issue programmatically.
	Code FindingCode
	// Severity is Error, Warning, or Note.
	Severity Severity
	// Directive is the surface directive the finding refers to
	// ("%y", "%F", "%Z"). Expanded composites point at the
	// atomic directive; Source names the composite it came from.
	Directive string
	// Source is the user-written composite directive
	// ("%c", "%F") that Directive was expanded from, if any.
	Source string
	// Position is the byte offset of Directive (or Source) in the format.
	Position int
	// Message is a human-readable explanation.
	Message string
}

// DateInfo describes the date fields carried by a format.
type DateInfo struct {
	// Present reports whether any date field is present.
	Present bool
	// Year reports the years carried by the format (zero otherwise).
	Years []string
	// Month reports the months carried by the format (zero otherwise).
	Months []string
	// Day reports the days carried by the format (zero otherwise).
	Days []string
	// Weekdays reports the weekday names/numbers carried by the format.
	Weekdays []string
	// DayOfYear is true when %j is present.
	DayOfYear bool
	// ISOWeekYear, ISOWeekNumber, ISOWeekday report the ISO week fields.
	ISOWeekYear   bool
	ISOWeekNumber bool
	ISOWeekday    bool
	// WeekSunday (%U) and WeekMonday (%W) report non-ISO week numbers.
	WeekSunday bool
	WeekMonday bool
}

// TimeInfo describes the time-of-day fields carried by a format.
type TimeInfo struct {
	// Present reports whether any time-of-day field is present.
	Present bool
	// Hour24 is true when an unambiguous 24-hour hour (%H/%k) is present.
	Hour24 bool
	// Hour12 is true when a 12-hour hour (%I/%l) is present.
	Hour12 bool
	// Hours, Minutes, Seconds list the directives carrying those fields.
	Hours   []string
	Minutes []string
	Seconds []string
	// MeridianUpper and MeridianLower report %p and %P.
	MeridianUpper bool
	MeridianLower bool
}

// ZoneInfo describes the time-zone fields carried by a format.
type ZoneInfo struct {
	// Present reports whether any time-zone information is present.
	Present bool
	// Offset reports the numeric-offset directives (%z, %:z).
	Offset []string
	// Name reports %Z.
	Name bool
	// UTC reports a literal UTC marker ("Z", "UTC", "GMT").
	UTC bool
}

// Report is the result of analyzing a strftime format.
//
// The analysis is purely static: compound directives such as %c, %F,
// %T, %r and %+ are expanded into their atomic directives, and the
// resulting fields are inspected. No time value is formatted, so a
// single lucky sample cannot mask an ambiguity.
type Report struct {
	// Format is the analyzed format string.
	Format string
	// Verdict is the static reversibility conclusion.
	Verdict Verdict
	// Findings lists the issues and observations, ordered by position
	// (errors first).
	Findings []Finding
	// Date describes the date fields the format carries.
	Date DateInfo
	// Time describes the time-of-day fields the format carries.
	Time TimeInfo
	// Precision is the finest sub-second precision carried.
	Precision Precision
	// Zone describes the time-zone fields the format carries.
	Zone ZoneInfo
	// Epoch is true when %s/%s pins an instant directly.
	Epoch bool

	atoms []atom
}

// Analyze statically analyzes fmt and reports whether a value written
// with it can be read back to the same time instant.
//
// Compound directives (%c, %+, %F, %D, %T, %R, %r, %v, %x, %X)
// are expanded before analysis, so %F is understood as %Y-%m-%d
// rather than treated as an opaque token. The conclusion never
// depends on formatting a sample time; use Report.RoundTrip to
// additionally check one concrete value.
func Analyze(fmt string) *Report {
	r := &Report{Format: fmt}
	r.atoms = scanAtoms(fmt, "", 0)
	r.classify()
	return r
}

type atom struct {
	spec    byte
	flag    byte
	literal string
	pos     int
	source  string

	// computed surface directive ("%y", "%F") for messages
	directive string
}

func (a atom) ref() string {
	if a.directive != "" {
		return a.directive
	}
	return "%" + string(a.spec)
}

// scanAtoms tokenizes fmt into atoms, expanding composite directives.
// parent is the user-written composite ("%F") that fmt was expanded
// from (empty at the top level); offset shifts positions accordingly.
func scanAtoms(fmt, parent string, offset int) []atom {
	var atoms []atom
	litStart := 0
	flush := func(end int) {
		if end > litStart {
			atoms = append(atoms, atom{
				literal: fmt[litStart:end],
				pos:     litStart + offset,
				source:  parent,
			})
		}
	}

	for i := 0; i < len(fmt); {
		if fmt[i] != '%' {
			i++
			continue
		}
		start := i
		i++
		var flag byte
		if i < len(fmt) && (fmt[i] == '-' || fmt[i] == ':') {
			flag = fmt[i]
			i++
		}
		var modifier byte
		if i < len(fmt) && (fmt[i] == 'E' || fmt[i] == 'O') {
			modifier = fmt[i]
			i++
		}
		if i >= len(fmt) {
			flush(start)
			litStart = start
			i = len(fmt)
			break
		}
		spec := fmt[i]
		i++

		if modifier != 0 && !okModifier(modifier, spec) {
			// The parser treats a bad modifier as a literal "%E" sequence.
			flush(start)
			litStart = start
			continue
		}

		flush(start)
		litStart = i

		if expansion, ok := expand(spec); ok {
			directive := fmt[start:i]
			children := scanAtoms(expansion, directive, start+offset)
			atoms = append(atoms, children...)
			continue
		}

		directive := fmt[start:i]
		atoms = append(atoms, atom{
			spec:      spec,
			flag:      flag,
			pos:       start + offset,
			source:    parent,
			directive: directive,
		})
	}
	flush(len(fmt))
	return atoms
}

func expand(spec byte) (string, bool) {
	switch spec {
	case 'c':
		return "%a %b %e %T %Y", true
	case '+':
		return "%a %b %e %H:%M:%S %Z %Y", true
	case 'F':
		return "%Y-%m-%d", true
	case 'D', 'x':
		return "%m/%d/%y", true
	case 'v':
		return "%e-%b-%Y", true
	case 'T', 'X':
		return "%H:%M:%S", true
	case 'R':
		return "%H:%M", true
	case 'r':
		return "%I:%M:%S %p", true
	}
	return "", false
}

type probe struct {
	years       []atom
	centuries   []atom
	months      []atom
	days        []atom
	weekdays    []atom
	dayOfYear   []atom
	isoYear     []atom
	isoWeek     []atom
	isoWeekday  []atom
	weekSunday  []atom
	weekMonday  []atom
	weekWeekday []atom
	hours24     []atom
	hours12     []atom
	minutes     []atom
	seconds     []atom
	meridians   []atom
	zonesOff    []atom
	zonesName   []atom
	epochs      []atom
	fractions   []atom
	utc         []atom
}

// classify collects the fields carried by the atoms and derives
// findings and the verdict.
func (r *Report) classify() {
	var p probe

	add := func(list *[]atom, a atom) { *list = append(*list, a) }

	for _, a := range r.atoms {
		if a.literal != "" {
			if hasInvalidLiteral(a.literal) {
				r.add(Finding{
					Code:      FindingInvalidLiteral,
					Severity:  Error,
					Position:  a.pos,
					Directive: literalToken(a.literal),
					Message:   "literal breaks the Go layout the parser relies on",
				})
			}
			if token := utcLiteral(a.literal); token != "" {
				add(&p.utc, atom{literal: token, pos: a.pos, source: a.source})
			}
			continue
		}

		switch a.spec {
		default:
			if !knownDirective(a.spec) {
				r.add(Finding{
					Code:      FindingUnknownDirective,
					Severity:  Error,
					Position:  a.pos,
					Directive: a.ref(),
					Source:    a.source,
					Message:   "not a recognized strftime directive",
				})
			}
		case 'Y':
			add(&p.years, a)
		case 'y':
			add(&p.years, a)
		case 'C':
			add(&p.centuries, a)
		case 'm', 'B', 'b', 'h':
			add(&p.months, a)
		case 'd', 'e':
			add(&p.days, a)
		case 'A', 'a':
			add(&p.weekdays, a)
		case 'u', 'w':
			add(&p.weekdays, a)
			if a.spec == 'u' {
				add(&p.isoWeekday, a)
			} else {
				add(&p.weekWeekday, a)
			}
		case 'j':
			add(&p.dayOfYear, a)
		case 'G', 'g':
			add(&p.isoYear, a)
		case 'V':
			add(&p.isoWeek, a)
		case 'U':
			add(&p.weekSunday, a)
		case 'W':
			add(&p.weekMonday, a)
		case 'H', 'k':
			add(&p.hours24, a)
		case 'I', 'l':
			add(&p.hours12, a)
		case 'M':
			add(&p.minutes, a)
		case 'S':
			add(&p.seconds, a)
		case 'p', 'P':
			add(&p.meridians, a)
		case 'z':
			add(&p.zonesOff, a)
		case 'Z':
			add(&p.zonesName, a)
		case 's', 'Q':
			add(&p.epochs, a)
		case 'L', 'f', 'N':
			add(&p.fractions, a)
			sep := byte(0)
			if a.pos > 0 {
				sep = r.Format[a.pos-1]
			}
			if sep != '.' && sep != ',' {
				r.add(Finding{
					Code:      FindingInvalidFraction,
					Severity:  Error,
					Position:  a.pos,
					Directive: a.ref(),
					Source:    a.source,
					Message:   "fractional-second directive must immediately follow '.' or ','",
				})
			}
		case '%', 't', 'n':
			// pure literals, no information
		}
		if a.literal == "" && a.spec != '%' && a.spec != 't' && a.spec != 'n' &&
			unsupportedForParse(a.spec) {
			r.add(Finding{
				Code:      FindingUnsupportedDirective,
				Severity:  Error,
				Position:  a.pos,
				Directive: a.ref(),
				Source:    a.source,
				Message:   "directive cannot be parsed back by this package",
			})
		}
	}

	r.exposeFields(&p)
	r.analyzeDate(&p)
	r.analyzeTime(&p)
	r.analyzeZone(&p)
	r.analyzePrecision(&p)

	r.sortFindings()
	for _, f := range r.Findings {
		if f.Severity == Error {
			r.Verdict = Unparseable
			return
		}
	}
	for _, f := range r.Findings {
		if f.Severity == Warning {
			r.Verdict = Conditional
			return
		}
	}
	r.Verdict = Reversible
}

func knownDirective(spec byte) bool {
	switch spec {
	case 'A', 'a', 'B', 'b', 'h', 'C', 'd', 'e',
		'G', 'g', 'H', 'k', 'I', 'l', 'j', 'L', 'f', 'N',
		'M', 'm', 'p', 'P', 'Q', 'R', 'r', 'S', 's',
		'U', 'u', 'V', 'v', 'W', 'w', 'X', 'x', 'Y', 'y', 'Z', 'z':
		return true
	}
	return false
}

func unsupportedForParse(spec byte) bool {
	switch spec {
	case 'g', 'k', 'l', 's', 'u', 'w', 'C', 'G', 'Q', 'U', 'V', 'W':
		return true
	}
	return false
}

func hasInvalidLiteral(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return true
		}
	}
	for _, token := range []string{"Jan", "Mon", "MST", "PM", "pm"} {
		if strings.Contains(s, token) {
			return true
		}
	}
	return false
}

func literalToken(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return string(s[i])
		}
	}
	for _, token := range []string{"Jan", "Mon", "MST", "PM", "pm"} {
		if i := strings.Index(s, token); i >= 0 {
			return token
		}
	}
	return s
}

// utcLiteral reports whether a literal run contains a UTC marker.
// Go treats the unquoted tokens Z, UTC and GMT as "the time is UTC".
func utcLiteral(s string) string {
	for _, token := range []string{"UTC", "GMT", "Z"} {
		if strings.Contains(s, token) {
			return token
		}
	}
	return ""
}

func (r *Report) add(f Finding) {
	if f.Directive == "" {
		f.Directive = r.Format
	}
	r.Findings = append(r.Findings, f)
}

func (r *Report) sortFindings() {
	severityRank := func(s Severity) int {
		switch s {
		case Error:
			return 0
		case Warning:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		return severityRank(a.Severity) < severityRank(b.Severity)
	})
}

func refs(list []atom) []string {
	if len(list) == 0 {
		return nil
	}
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.ref()
	}
	return out
}

func hasSpec(list []atom, spec byte) bool {
	for _, a := range list {
		if a.spec == spec {
			return true
		}
	}
	return false
}

func (r *Report) firstOf(lists ...[]atom) (atom, bool) {
	for _, l := range lists {
		if len(l) > 0 {
			return l[0], true
		}
	}
	return atom{}, false
}

func dateFieldCount(p *probe) int {
	return len(p.years) + len(p.centuries) + len(p.months) + len(p.days) +
		len(p.dayOfYear) + len(p.isoYear) + len(p.isoWeek) + len(p.isoWeekday) +
		len(p.weekSunday) + len(p.weekMonday) + len(p.weekWeekday)
}

func (r *Report) exposeFields(p *probe) {
	r.Date.Years = refs(p.years)
	r.Date.Months = refs(p.months)
	r.Date.Days = refs(p.days)
	r.Date.Weekdays = refs(p.weekdays)
	r.Date.DayOfYear = len(p.dayOfYear) > 0
	r.Date.ISOWeekYear = len(p.isoYear) > 0
	r.Date.ISOWeekNumber = len(p.isoWeek) > 0
	r.Date.ISOWeekday = len(p.isoWeekday) > 0
	r.Date.WeekSunday = len(p.weekSunday) > 0
	r.Date.WeekMonday = len(p.weekMonday) > 0
	r.Date.Present = len(p.years)+len(p.centuries)+len(p.months)+len(p.days) > 0 ||
		r.Date.DayOfYear || r.Date.ISOWeekYear || r.Date.ISOWeekNumber ||
		r.Date.WeekSunday || r.Date.WeekMonday || len(p.weekWeekday) > 0

	hours := make([]atom, 0, len(p.hours24)+len(p.hours12))
	hours = append(hours, p.hours24...)
	hours = append(hours, p.hours12...)
	r.Time.Hours = refs(hours)
	r.Time.Minutes = refs(p.minutes)
	r.Time.Seconds = refs(p.seconds)
	r.Time.Hour24 = len(p.hours24) > 0
	r.Time.Hour12 = len(p.hours12) > 0
	r.Time.MeridianUpper = hasSpec(p.meridians, 'p')
	r.Time.MeridianLower = hasSpec(p.meridians, 'P')
	r.Time.Present = len(hours) > 0

	r.Zone.Offset = refs(p.zonesOff)
	r.Zone.Name = len(p.zonesName) > 0
	r.Zone.UTC = len(p.utc) > 0
	r.Zone.Present = len(p.zonesOff)+len(p.zonesName) > 0 || r.Zone.UTC

	r.Epoch = len(p.epochs) > 0
}

func (r *Report) analyzeDate(p *probe) {
	if r.Epoch {
		if dateFieldCount(p) > 0 ||
			len(p.hours24)+len(p.hours12)+len(p.minutes)+len(p.seconds) > 0 ||
			len(p.zonesOff)+len(p.zonesName) > 0 || len(p.utc) > 0 {
			a := p.epochs[0]
			r.add(Finding{
				Code:      FindingEpochConflict,
				Severity:  Error,
				Position:  a.pos,
				Directive: a.ref(),
				Source:    a.source,
				Message:   "epoch seconds already determine the instant; extra date/time/zone fields conflict",
			})
		}
		return
	}

	hasCalendar := len(p.months)+len(p.days) > 0
	hasOrdinal := len(p.dayOfYear) > 0
	hasISO := len(p.isoYear)+len(p.isoWeek)+len(p.isoWeekday) > 0
	hasWeekNum := len(p.weekSunday)+len(p.weekMonday) > 0

	systems := 0
	for _, b := range []bool{hasCalendar, hasOrdinal, hasISO, hasWeekNum} {
		if b {
			systems++
		}
	}
	if len(p.weekSunday) > 0 && len(p.weekMonday) > 0 {
		systems++
	}
	if systems > 1 {
		if a, ok := r.firstOf(p.dayOfYear, p.isoYear, p.isoWeek, p.isoWeekday,
			p.weekSunday, p.weekMonday, p.months, p.days); ok {
			r.add(Finding{
				Code:      FindingDateSystemConflict,
				Severity:  Error,
				Position:  a.pos,
				Directive: a.ref(),
				Source:    a.source,
				Message:   "calendar, ordinal, ISO-week and week-number dates are mutually exclusive",
			})
		}
	}

	hasYear := len(p.years) > 0
	switch {
	case hasCalendar:
		r.analyzeCalendarDate(p)
	case hasOrdinal:
		r.analyzeOrdinalDate(p)
	case hasISO:
		r.analyzeISODate(p)
	case hasWeekNum:
		r.analyzeWeekNumberDate(p)
	default:
		switch {
		case hasYear:
			a := p.years[0]
			r.add(Finding{
				Code:      FindingMissingMonth,
				Severity:  Warning,
				Position:  a.pos,
				Directive: a.ref(),
				Source:    a.source,
				Message:   "only the year is present; month and day are absent",
			})
		case len(p.weekdays) > 0 && !hasYear:
			a := p.weekdays[0]
			r.add(Finding{
				Code:      FindingMissingDate,
				Severity:  Warning,
				Position:  a.pos,
				Directive: a.ref(),
				Source:    a.source,
				Message:   "weekday alone does not identify a date; year/month/day are absent",
			})
		case len(p.centuries) > 0:
			a := p.centuries[0]
			r.add(Finding{
				Code:      FindingMissingDate,
				Severity:  Warning,
				Position:  a.pos,
				Directive: a.ref(),
				Source:    a.source,
				Message:   "century alone cannot name a year or date",
			})
		}
	}

	determinedCalendar := hasYear && len(p.months) > 0 && len(p.days) > 0
	determinedOrdinal := hasYear && hasOrdinal
	if systems <= 1 && (determinedCalendar || determinedOrdinal) && len(p.weekdays) > 0 {
		a := p.weekdays[0]
		r.add(Finding{
			Code:      FindingRedundantField,
			Severity:  Note,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "weekday is derivable from the date and carries no extra information",
		})
	}
}

func (r *Report) analyzeCalendarDate(p *probe) {
	var fullYear, twoDigitYear bool
	for _, a := range p.years {
		switch a.spec {
		case 'y':
			twoDigitYear = true
			r.add(Finding{
				Code:      FindingTwoDigitYear,
				Severity:  Warning,
				Position:  a.pos,
				Directive: a.ref(),
				Source:    a.source,
				Message:   "two-digit year needs an external pivot/window to recover the century",
			})
		case 'Y':
			fullYear = true
		}
	}

	switch {
	case len(p.centuries) > 0 && !fullYear && !twoDigitYear:
		a := p.centuries[0]
		r.add(Finding{
			Code:      FindingMissingDate,
			Severity:  Warning,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "century without a two-digit year cannot name a year",
		})
	case !fullYear && !twoDigitYear && (len(p.months) > 0 || len(p.days) > 0):
		if a, ok := r.firstOf(p.months, p.days); ok {
			r.add(Finding{
				Code:      FindingMissingDate,
				Severity:  Warning,
				Position:  a.pos,
				Directive: a.ref(),
				Source:    a.source,
				Message:   "month/day without a year cannot name a date",
			})
		}
	}

	if (fullYear || twoDigitYear) && len(p.months) == 0 {
		a := p.years[0]
		r.add(Finding{
			Code:      FindingMissingMonth,
			Severity:  Warning,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "year present but month absent",
		})
	}
	if (fullYear || twoDigitYear) && len(p.months) > 0 && len(p.days) == 0 {
		a := p.months[0]
		r.add(Finding{
			Code:      FindingMissingDay,
			Severity:  Warning,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "year and month present but day absent",
		})
	}

	if len(p.centuries) > 0 && twoDigitYear && !fullYear {
		r.add(Finding{
			Code:      FindingRedundantField,
			Severity:  Note,
			Position:  p.centuries[0].pos,
			Directive: p.centuries[0].ref(),
			Source:    p.centuries[0].source,
			Message:   "%C duplicates the century already implied by %y and has no parsing counterpart",
		})
	}
	if len(p.years) > 1 || len(p.months) > 1 || len(p.days) > 1 {
		if a, ok := r.firstOf(p.years, p.months, p.days); ok {
			r.add(Finding{
				Code:      FindingRedundantField,
				Severity:  Note,
				Position:  a.pos,
				Directive: a.ref(),
				Source:    a.source,
				Message:   "date component written more than once",
			})
		}
	}
}

func (r *Report) analyzeOrdinalDate(p *probe) {
	for _, a := range p.years {
		if a.spec == 'y' {
			r.add(Finding{
				Code:      FindingTwoDigitYear,
				Severity:  Warning,
				Position:  a.pos,
				Directive: a.ref(),
				Source:    a.source,
				Message:   "two-digit year needs an external pivot/window to recover the century",
			})
		}
	}
	if len(p.years) == 0 {
		a := p.dayOfYear[0]
		r.add(Finding{
			Code:      FindingMissingDate,
			Severity:  Warning,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "day of year without a year cannot name a date",
		})
	}
}

func (r *Report) analyzeISODate(p *probe) {
	for _, a := range p.isoYear {
		if a.spec == 'g' {
			r.add(Finding{
				Code:      FindingTwoDigitYear,
				Severity:  Warning,
				Position:  a.pos,
				Directive: a.ref(),
				Source:    a.source,
				Message:   "two-digit ISO week-based year needs an external pivot/window",
			})
		}
	}
	if (len(p.isoYear) > 0 || len(p.isoWeek) > 0) &&
		(len(p.isoYear) == 0 || len(p.isoWeek) == 0 || len(p.isoWeekday) == 0) {
		var a atom
		var anchor []atom
		switch {
		case len(p.isoYear) > 0:
			anchor = p.isoYear
		case len(p.isoWeek) > 0:
			anchor = p.isoWeek
		default:
			anchor = p.isoWeekday
		}
		a = anchor[0]
		var missing []string
		if len(p.isoYear) == 0 {
			missing = append(missing, "%G/%g")
		}
		if len(p.isoWeek) == 0 {
			missing = append(missing, "%V")
		}
		if len(p.isoWeekday) == 0 {
			missing = append(missing, "%u/%w")
		}
		r.add(Finding{
			Code:      FindingISOWeekIncomplete,
			Severity:  Warning,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "ISO week date needs all of %G %V %u; missing " + strings.Join(missing, ", "),
		})
	}
}

func (r *Report) analyzeWeekNumberDate(p *probe) {
	var a atom
	if len(p.weekSunday) > 0 {
		a = p.weekSunday[0]
	} else {
		a = p.weekMonday[0]
	}
	var need []string
	if len(p.years) == 0 {
		need = append(need, "%Y")
	}
	if len(p.weekWeekday) == 0 {
		need = append(need, "%w/%u")
	}
	if len(need) > 0 {
		r.add(Finding{
			Code:      FindingWeekNumberIncomplete,
			Severity:  Warning,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "week number must be combined with a year and weekday; missing " + strings.Join(need, ", "),
		})
	}
}

func (r *Report) analyzeTime(p *probe) {
	if r.Epoch {
		return
	}

	hours12 := len(p.hours12) > 0
	hours24 := len(p.hours24) > 0

	if hours12 && hours24 {
		a := append(append([]atom{}, p.hours12...), p.hours24...)[0]
		r.add(Finding{
			Code:      FindingTimeSystemConflict,
			Severity:  Error,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "12-hour (%I/%l) and 24-hour (%H/%k) hour fields both present",
		})
	}

	if hasSpec(p.meridians, 'p') && hasSpec(p.meridians, 'P') {
		a := p.meridians[0]
		r.add(Finding{
			Code:      FindingMeridianConflict,
			Severity:  Error,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "both uppercase %p and lowercase %P meridian fields present",
		})
	}

	meridian := len(p.meridians) > 0
	if hours12 && !meridian {
		a := p.hours12[0]
		r.add(Finding{
			Code:      Finding12HourNoMeridian,
			Severity:  Warning,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "12-hour hour without %p/%P; hours 01..12 are read back as AM",
		})
	}
	if !hours12 && meridian {
		a := p.meridians[0]
		r.add(Finding{
			Code:      FindingRedundantField,
			Severity:  Note,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "meridian without a 12-hour hour is ignored on parsing",
		})
	}

	hasHour := hours12 || hours24
	hasCalendarDate := len(p.years)+len(p.months)+len(p.days)+len(p.centuries)+len(p.dayOfYear) > 0
	switch {
	case !hasHour:
		if hasCalendarDate {
			r.add(Finding{
				Code:      FindingMissingTime,
				Severity:  Warning,
				Position:  len(r.Format),
				Directive: r.Format,
				Message:   "no time of day present; midnight is assumed",
			})
		}
	case len(p.minutes) == 0:
		var a atom
		if hours24 {
			a = p.hours24[0]
		} else {
			a = p.hours12[0]
		}
		r.add(Finding{
			Code:      FindingMissingMinute,
			Severity:  Warning,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "hour present but minute absent; minute 00 is assumed",
		})
	case len(p.seconds) == 0:
		a := p.minutes[0]
		r.add(Finding{
			Code:      FindingMissingSecond,
			Severity:  Warning,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "hour and minute present but second absent; second 00 is assumed",
		})
	}

	if len(p.years)+len(p.months)+len(p.days)+len(p.centuries)+len(p.dayOfYear) == 0 &&
		len(p.isoYear)+len(p.isoWeek)+len(p.isoWeekday) == 0 &&
		len(p.weekSunday)+len(p.weekMonday)+len(p.weekWeekday) == 0 && hasHour {
		var a atom
		if hours24 {
			a = p.hours24[0]
		} else {
			a = p.hours12[0]
		}
		r.add(Finding{
			Code:      FindingMissingDate,
			Severity:  Warning,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "time of day without a date; year 1 is assumed",
		})
	}

	if len(p.hours12) > 1 || len(p.hours24) > 1 || len(p.minutes) > 1 || len(p.seconds) > 1 {
		all := append(append(append(append([]atom{}, p.hours12...),
			p.hours24...), p.minutes...), p.seconds...)
		a := all[0]
		r.add(Finding{
			Code:      FindingRedundantField,
			Severity:  Note,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "time component written more than once",
		})
	}
}

func (r *Report) analyzeZone(p *probe) {
	if r.Epoch {
		return
	}

	hasOffset := len(p.zonesOff) > 0
	hasName := len(p.zonesName) > 0
	hasUTC := len(p.utc) > 0

	if hasUTC && (hasOffset || hasName) {
		var a atom
		if len(p.utc) > 0 {
			a = p.utc[0]
		}
		r.add(Finding{
			Code:      FindingZoneConflict,
			Severity:  Error,
			Position:  a.pos,
			Directive: a.literal,
			Message:   "literal UTC marker conflicts with a written zone field",
		})
	}

	if !hasOffset && hasName {
		a := p.zonesName[0]
		r.add(Finding{
			Code:      FindingZoneNameOnly,
			Severity:  Warning,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "zone abbreviation does not uniquely determine the offset (unknown names parse as +00:00)",
		})
	}
	if hasName && hasOffset {
		a := p.zonesName[0]
		r.add(Finding{
			Code:      FindingRedundantField,
			Severity:  Note,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "zone name is redundant next to a numeric offset",
		})
	}
	if len(p.zonesName) > 1 {
		a := p.zonesName[1]
		r.add(Finding{
			Code:      FindingRedundantField,
			Severity:  Note,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "zone name written more than once",
		})
	}
	if len(p.zonesOff) > 1 {
		a := p.zonesOff[1]
		r.add(Finding{
			Code:      FindingRedundantField,
			Severity:  Note,
			Position:  a.pos,
			Directive: a.ref(),
			Source:    a.source,
			Message:   "numeric offset written more than once",
		})
	}

	hasCalendarDate := len(p.years)+len(p.months)+len(p.days)+len(p.centuries)+len(p.dayOfYear) > 0
	if !hasOffset && !hasName && !hasUTC {
		hasClock := len(p.hours24)+len(p.hours12)+len(p.minutes)+len(p.seconds) > 0
		fullISO := len(p.isoYear) > 0 && len(p.isoWeek) > 0 && len(p.isoWeekday) > 0
		if hasCalendarDate || fullISO || hasClock {
			r.add(Finding{
				Code:      FindingMissingZone,
				Severity:  Warning,
				Position:  len(r.Format),
				Directive: r.Format,
				Message:   "no time zone present; the value is read back in UTC",
			})
		}
	}
}

func (r *Report) analyzePrecision(p *probe) {
	best := SecondPrecision
	var bestAtom atom
	rank := func(spec byte) Precision {
		switch spec {
		case 'N':
			return NanosecondPrecision
		case 'f':
			return MicrosecondPrecision
		default:
			return MillisecondPrecision
		}
	}
	for _, a := range p.fractions {
		if pr := rank(a.spec); pr > best {
			best = pr
			bestAtom = a
		}
	}
	r.Precision = best

	if r.Epoch {
		// %Q carries milliseconds; %s carries seconds. Both truncate.
		epoch := p.epochs[0]
		var pr Precision
		if epoch.spec == 'Q' {
			pr = MillisecondPrecision
		}
		r.Precision = pr
		r.add(Finding{
			Code:      FindingSubsecondTruncated,
			Severity:  Warning,
			Position:  epoch.pos,
			Directive: epoch.ref(),
			Source:    epoch.source,
			Message:   "epoch precision discards sub-second detail",
		})
		return
	}

	hasAnyClock := len(p.hours24)+len(p.hours12)+len(p.minutes)+len(p.seconds) > 0
	if hasAnyClock && best < NanosecondPrecision {
		f := Finding{
			Code:      FindingSubsecondTruncated,
			Severity:  Warning,
			Position:  len(r.Format),
			Directive: r.Format,
			Message:   "anything below the second is discarded on write",
		}
		if best != SecondPrecision {
			f.Position = bestAtom.pos
			f.Directive = bestAtom.ref()
			f.Source = bestAtom.source
			f.Message = "anything finer than " + best.String() + " resolution is discarded on write"
		}
		r.add(f)
	}
}

// RoundTripReport compares a concrete time with the value obtained by
// writing it with the format and parsing the result back.
//
// It is deliberately secondary to the static Verdict: a clean round-trip
// on one sample cannot clear a static ambiguity. For instance, a 12-hour
// time written in the morning, or a two-digit year inside the current
// century, may round-trip exactly while remaining Conditional for other
// values.
type RoundTripReport struct {
	// Formatted is the written value.
	Formatted string
	// ParseError is non-nil when the written value could not be parsed.
	ParseError error
	// Parsed is the recovered value (zero when ParseError is set).
	Parsed time.Time

	// Exact is true when the recovered instant, date fields, time fields
	// and zone offset all match.
	Exact bool
	// InstantDifference is parsed.Sub(original) as an absolute duration
	// (0 when the instants agree).
	InstantDifference time.Duration
	// ZoneDifference is parsed offset minus original offset.
	ZoneDifference time.Duration
	// FractionLost is the sub-second portion discarded by the format,
	// when the larger fields match.
	FractionLost time.Duration

	// Differences maps FindingCode values from the static analysis
	// ("missing_date", "missing_zone", "two_digit_year", ...) to the
	// difference they actually produced for this sample.
	Differences map[FindingCode]time.Duration
}

// RoundTrip formats t with r.Format, parses the result back, and reports
// where the recovered value deviates. It never changes r.Verdict.
func (r *Report) RoundTrip(t time.Time) *RoundTripReport {
	out := &RoundTripReport{
		Formatted:   Format(r.Format, t),
		Differences: map[FindingCode]time.Duration{},
	}

	parsed, err := Parse(r.Format, out.Formatted)
	if err != nil {
		out.ParseError = err
		return out
	}
	out.Parsed = parsed

	origOffset := offsetDuration(t)
	parsedOffset := offsetDuration(parsed)
	out.ZoneDifference = parsedOffset - origOffset

	delta := parsed.Sub(t)
	if delta < 0 {
		out.InstantDifference = -delta
	} else {
		out.InstantDifference = delta
	}

	// Attribute the difference. Sub-second loss is only reported when the
	// larger date/time fields agree, so it is not confused with a zone
	// or date difference that happens to share a magnitude.
	py, pm, pd := parsed.Date()
	ty, tm, td := t.Date()
	sameLargeFields := py == ty && pm == tm && pd == td &&
		parsed.Hour() == t.Hour() &&
		parsed.Minute() == t.Minute() &&
		parsed.Second() == t.Second()

	if sameLargeFields {
		lost := t.Nanosecond() - parsed.Nanosecond()
		if lost != 0 {
			out.FractionLost = time.Duration(lost)
			out.Differences[FindingSubsecondTruncated] = out.FractionLost
		}
	}

	if out.ZoneDifference != 0 {
		out.Differences[FindingMissingZone] = absDuration(out.ZoneDifference)
		if r.Zone.Name && len(r.Zone.Offset) == 0 && !r.Zone.UTC {
			out.Differences[FindingZoneNameOnly] = absDuration(out.ZoneDifference)
		}
	}

	// 12-hour ambiguity: hour recovered as AM but the original was PM.
	if r.Time.Hour12 && !(r.Time.MeridianUpper || r.Time.MeridianLower) &&
		t.Hour() >= 12 && parsed.Hour() < 12 {
		out.Differences[Finding12HourNoMeridian] = 12 * time.Hour
	}

	// Two-digit year: report the year gap the pivot produced.
	if hasCode(r, FindingTwoDigitYear) && parsed.Year() != t.Year() {
		out.Differences[FindingTwoDigitYear] =
			time.Duration(parsed.Year()-t.Year()) * 365 * 24 * time.Hour
	}

	if !r.Date.Present {
		if d := dateOnlyDelta(t, parsed); d != 0 {
			out.Differences[FindingMissingDate] = d
		}
	}

	// Reduced time accuracy, when the date matches.
	if py == ty && pm == tm && pd == td {
		if hasCode(r, FindingMissingMinute) && parsed.Minute() != t.Minute() {
			out.Differences[FindingMissingMinute] =
				time.Duration(parsed.Hour()-t.Hour())*time.Hour +
					time.Duration(parsed.Minute()-t.Minute())*time.Minute
		}
		if hasCode(r, FindingMissingSecond) && parsed.Second() != t.Second() {
			out.Differences[FindingMissingSecond] =
				time.Duration(parsed.Minute()-t.Minute())*time.Minute +
					time.Duration(parsed.Second()-t.Second())*time.Second
		}
	}

	if out.InstantDifference == 0 && out.ZoneDifference == 0 &&
		out.FractionLost == 0 {
		out.Exact = true
	}
	return out
}

func offsetDuration(t time.Time) time.Duration {
	_, off := t.Zone()
	return time.Duration(off) * time.Second
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func hasCode(r *Report, code FindingCode) bool {
	for _, f := range r.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

// dateOnlyDelta measures the calendar gap between t and parsed,
// ignoring time-of-day and zone offsets.
func dateOnlyDelta(t, parsed time.Time) time.Duration {
	y1, m1, d1 := t.Date()
	y2, m2, d2 := parsed.Date()
	t1 := time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC)
	delta := t2.Sub(t1)
	return absDuration(delta)
}
