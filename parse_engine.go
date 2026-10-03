package strftime

import (
	"strconv"
	"strings"
	"time"
)

// Parse converts a textual representation of time to the time value it
// represents according to the compiled format specification.
//
// Unlike parsing a Go time layout, literal text is matched verbatim:
// literals may freely contain digits and sequences such as "2006",
// "Jan", "Mon", "MST", "PM", as well as arbitrary UTF-8 text.
//
// Trailing input after the format is fully matched is an error.
func (c *Compiled) Parse(value string) (time.Time, error) {
	st := parseState{
		format: c.format,
		value:  value,
		tokens: c.tokens,
		month:  -1,
		day:    -1,
		yday:   -1,
		offset: -1,
	}

	for i := range c.tokens {
		tok := &c.tokens[i]
		if tok.kind == tokLiteral {
			if len(value) < len(tok.text) || value[:len(tok.text)] != tok.text {
				return time.Time{}, st.literalError(tok, len(st.value)-len(value))
			}
			value = value[len(tok.text):]
			continue
		}

		inputStart := len(st.value) - len(value)
		if tok.pop == nil {
			return time.Time{}, compileError(c.format, tok.offset, tok.text, "is not supported for parsing")
		}

		st.nextPop = c.nextPopSpec(i)
		st.curTok = tok
		rem, err := tok.pop(&st, value, tok.flag)
		if err != nil {
			off := inputStart + len(value) - len(rem)
			return time.Time{}, st.directiveError(tok, err.Error(), off)
		}
		value = rem
	}

	if value != "" {
		return time.Time{}, &ParseError{
			Format:    c.format,
			Value:     st.value,
			Directive: "",
			Offset:    len(st.value) - len(value),
			message:   "extra text: " + strconv.Quote(value),
		}
	}

	return st.build()
}

// ParseError describes a problem parsing a time value against a format.
type ParseError struct {
	Format    string // the format specification
	Value     string // the value being parsed
	Directive string // the directive that failed, or "" for literal/trailing errors
	Offset    int    // byte offset in Value where the failure was detected
	message   string
}

func (e *ParseError) Error() string {
	if e.Directive != "" {
		return "strftime: cannot parse " + strconv.Quote(e.Value) +
			" as " + strconv.Quote(e.Format) +
			": directive " + e.Directive +
			" at input offset " + strconv.Itoa(e.Offset) + ": " + e.message
	}
	return "strftime: cannot parse " + strconv.Quote(e.Value) +
		" as " + strconv.Quote(e.Format) +
		" at input offset " + strconv.Itoa(e.Offset) + ": " + e.message
}

type parseState struct {
	format  string
	value   string
	tokens  []token
	nextPop byte
	curTok  *token

	yearSet                   bool
	year                      int
	month, day                int
	yday                      int
	hour, min, sec, nsec      int
	hour12Set, meridianSet    bool
	hour12                    int
	meridianPM                bool
	nsecSet                   bool
	ydaySet                   bool
	monthSet, daySet          bool
	hourSet, minSet, secSet   bool
	zone                      *time.Location
	zoneNameSet               bool
	zoneName                  string
	zoneNumericName           bool
	offsetSet                 bool
	offset                    int
	weekdayShort, weekdayLong int
	weekdayShortSet           bool
	weekdayLongSet            bool
	weekdayShortTok           *token
	weekdayLongTok            *token
	ydayTok                   *token
}

func (s *parseState) directiveError(tok *token, message string, inputOff int) *ParseError {
	return &ParseError{
		Format:    s.format,
		Value:     s.value,
		Directive: tok.text,
		Offset:    inputOff,
		message:   message,
	}
}

func (s *parseState) literalError(tok *token, inputOff int) *ParseError {
	return &ParseError{
		Format:    s.format,
		Value:     s.value,
		Directive: "",
		Offset:    inputOff,
		message:   "expected literal " + strconv.Quote(tok.text),
	}
}

// parseOp parses the directive from the start of value, returning the
// remainder. On error the returned remainder indicates the input offset
// that failed: callers compute len(value)-len(rem).
type parseOp func(s *parseState, value string, flag byte) (string, error)

var parseOps = [256]parseOp{
	'Y': parseYear,
	'y': parseYear2,
	'm': parseMonth,
	'B': parseLongMonthName,
	'b': parseShortMonthName,
	'h': parseShortMonthName,
	'd': parseDay,
	'e': parseBlankDay,
	'j': parseYearDay,
	'H': parseHour24,
	'I': parseHour12,
	'M': parseMinute,
	'S': parseSecond,
	'p': parsePMUpper,
	'P': parsePMLower,
	'A': parseLongWeekday,
	'a': parseShortWeekday,
	'Z': parseZoneName,
	'z': parseZoneOffset,
	'L': parseFraction3,
	'f': parseFraction6,
	'N': parseFraction9,
}

func parseYear(s *parseState, value string, _ byte) (string, error) {
	n, rem, ok := readFixedDigits(value, 4)
	if !ok {
		return value, errBad
	}
	if !s.setYear(n) {
		return value, errConflict
	}
	return rem, nil
}

func parseYear2(s *parseState, value string, _ byte) (string, error) {
	n, rem, ok := readFixedDigits(value, 2)
	if !ok {
		return value, errBad
	}
	if n >= 69 {
		n += 1900
	} else {
		n += 2000
	}
	if !s.setYear(n) {
		return value, errConflict
	}
	return rem, nil
}

func parseMonth(s *parseState, value string, _ byte) (string, error) {
	n, rem, ok := readNum(value)
	if !ok || n < 1 || n > 12 {
		return value, errBad
	}
	if s.monthSet && s.month != n {
		return value, errConflict
	}
	s.month, s.monthSet = n, true
	return rem, nil
}

func parseLongMonthName(s *parseState, value string, _ byte) (string, error) {
	n, rem, ok := lookupName(longMonthNames, value)
	if !ok {
		return value, errBad
	}
	if s.monthSet && s.month != n {
		return value, errConflict
	}
	s.month, s.monthSet = n, true
	return rem, nil
}

func parseShortMonthName(s *parseState, value string, _ byte) (string, error) {
	n, rem, ok := lookupName(shortMonthNames, value)
	if !ok {
		return value, errBad
	}
	if s.monthSet && s.month != n {
		return value, errConflict
	}
	s.month, s.monthSet = n, true
	return rem, nil
}

func parseDay(s *parseState, value string, _ byte) (string, error) {
	n, rem, ok := readNum(value)
	if !ok {
		return value, errBad
	}
	if s.daySet && s.day != n {
		return value, errConflict
	}
	s.day, s.daySet = n, true
	return rem, nil
}

func parseBlankDay(s *parseState, value string, _ byte) (string, error) {
	if len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}
	n, rem, ok := readNum(value)
	if !ok {
		return value, errBad
	}
	if s.daySet && s.day != n {
		return value, errConflict
	}
	s.day, s.daySet = n, true
	return rem, nil
}

func parseYearDay(s *parseState, value string, _ byte) (string, error) {
	n, rem, ok := readNum3(value)
	if !ok {
		return value, errBad
	}
	if s.ydaySet && s.yday != n {
		return value, errConflict
	}
	s.yday, s.ydaySet = n, true
	s.ydayTok = s.curTok
	return rem, nil
}

func parseHour24(s *parseState, value string, _ byte) (string, error) {
	n, rem, ok := readNum(value)
	if !ok || n < 0 || n >= 24 {
		return value, errBad
	}
	if s.hourSet && s.hour != n {
		return value, errConflict
	}
	s.hour, s.hourSet = n, true
	return rem, nil
}

func parseHour12(s *parseState, value string, _ byte) (string, error) {
	n, rem, ok := readNum(value)
	if !ok || n < 1 || n > 12 {
		return value, errBad
	}
	if s.hour12Set && s.hour12 != n {
		return value, errConflict
	}
	s.hour12, s.hour12Set = n, true
	return rem, nil
}

func parseMinute(s *parseState, value string, _ byte) (string, error) {
	n, rem, ok := readNum(value)
	if !ok || n < 0 || n >= 60 {
		return value, errBad
	}
	if s.minSet && s.min != n {
		return value, errConflict
	}
	s.min, s.minSet = n, true
	return rem, nil
}

func parseSecond(s *parseState, value string, _ byte) (string, error) {
	n, rem, ok := readNum(value)
	if !ok || n < 0 || n >= 60 {
		return value, errBad
	}
	if s.secSet && s.sec != n {
		return value, errConflict
	}
	s.sec, s.secSet = n, true

	// Match time.Parse: consume an optional fractional second even when
	// the format does not include one, unless a fractional directive
	// (%L %f %N) follows in the format.
	if len(rem) >= 2 && (rem[0] == '.' || rem[0] == ',') && isASCIIDigit(rem[1]) &&
		(s.nextPop != 'L' && s.nextPop != 'f' && s.nextPop != 'N') {
		i := 1
		for i < len(rem) && isASCIIDigit(rem[i]) {
			i++
		}
		nsec := fractionToNanos(rem[1:i])
		s.nsec, s.nsecSet = nsec, true
		rem = rem[i:]
	}
	return rem, nil
}

func parsePMUpper(s *parseState, value string, _ byte) (string, error) {
	return parseMeridian(s, value, true)
}

func parsePMLower(s *parseState, value string, _ byte) (string, error) {
	return parseMeridian(s, value, false)
}

func parseMeridian(s *parseState, value string, upper bool) (string, error) {
	var pm, am string
	if upper {
		pm, am = "PM", "AM"
	} else {
		pm, am = "pm", "am"
	}
	var isPM bool
	switch {
	case strings.HasPrefix(value, pm):
		isPM = true
	case strings.HasPrefix(value, am):
	default:
		return value, errBad
	}
	if s.meridianSet && s.meridianPM != isPM {
		return value, errConflict
	}
	s.meridianPM, s.meridianSet = isPM, true
	return value[2:], nil
}

func parseLongWeekday(s *parseState, value string, _ byte) (string, error) {
	n, rem, ok := lookupName(longDayNames, value)
	if !ok {
		return value, errBad
	}
	if s.weekdayLongSet && s.weekdayLong != n {
		return value, errConflict
	}
	s.weekdayLong, s.weekdayLongSet = n, true
	s.weekdayLongTok = s.curTok
	return rem, nil
}

func parseShortWeekday(s *parseState, value string, _ byte) (string, error) {
	n, rem, ok := lookupName(shortDayNames, value)
	if !ok {
		return value, errBad
	}
	if s.weekdayShortSet && s.weekdayShort != n {
		return value, errConflict
	}
	s.weekdayShort, s.weekdayShortSet = n, true
	s.weekdayShortTok = s.curTok
	return rem, nil
}

func parseFraction3(s *parseState, value string, _ byte) (string, error) {
	return parseFraction(s, value, 3)
}

func parseFraction6(s *parseState, value string, _ byte) (string, error) {
	return parseFraction(s, value, 6)
}

func parseFraction9(s *parseState, value string, _ byte) (string, error) {
	return parseFraction(s, value, 9)
}

func parseFraction(s *parseState, value string, width int) (string, error) {
	nsec, rem, ok := readFractionWidth(value, width)
	if !ok {
		return value, errBad
	}
	if s.nsecSet && s.nsec != nsec {
		return value, errConflict
	}
	s.nsec, s.nsecSet = nsec, true
	return rem, nil
}

// setYear records a parsed year and reports whether it is consistent.
func (s *parseState) setYear(y int) bool {
	if s.yearSet && s.year != y {
		return false
	}
	s.year, s.yearSet = y, true
	return true
}
