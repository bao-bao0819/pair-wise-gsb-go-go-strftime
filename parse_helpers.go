package strftime

import (
	"errors"
	"time"
)

var (
	errBad      = errors.New("bad value for field")
	errConflict = errors.New("value does not match an earlier directive")
)

var shortMonthNames = []string{
	"Jan", "Feb", "Mar", "Apr", "May", "Jun",
	"Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
}

var longMonthNames = []string{
	"January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December",
}

var shortDayNames = []string{
	"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat",
}

var longDayNames = []string{
	"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday",
}

func isASCIIDigit(b byte) bool { return '0' <= b && b <= '9' }

// readNum reads one or two decimal digits, mirroring time.getnum.
func readNum(value string) (int, string, bool) {
	if len(value) == 0 || !isASCIIDigit(value[0]) {
		return 0, value, false
	}
	if len(value) < 2 || !isASCIIDigit(value[1]) {
		return int(value[0] - '0'), value[1:], true
	}
	return int(value[0]-'0')*10 + int(value[1]-'0'), value[2:], true
}

// readNum3 reads one to three decimal digits, mirroring time.getnum3.
func readNum3(value string) (int, string, bool) {
	i := 0
	n := 0
	for i < len(value) && i < 3 && isASCIIDigit(value[i]) {
		n = n*10 + int(value[i]-'0')
		i++
	}
	if i == 0 {
		return 0, value, false
	}
	return n, value[i:], true
}

// readFixedDigits reads exactly width decimal digits.
func readFixedDigits(value string, width int) (int, string, bool) {
	if len(value) < width {
		return 0, value, false
	}
	n := 0
	for i := 0; i < width; i++ {
		if !isASCIIDigit(value[i]) {
			return 0, value, false
		}
		n = n*10 + int(value[i]-'0')
	}
	return n, value[width:], true
}

// lookupName matches one of the names case-sensitively at the start of value,
// mirroring time.lookup: the first table entry that is a prefix wins.
func lookupName(names []string, value string) (int, string, bool) {
	for i, name := range names {
		if len(value) >= len(name) && value[:len(name)] == name {
			return i + 1, value[len(name):], true
		}
	}
	return 0, value, false
}

// fractionToNanos scales a run of fractional digits to nanoseconds.
func fractionToNanos(digits string) int {
	if len(digits) > 9 {
		digits = digits[:9]
	}
	n := 0
	for i := 0; i < len(digits); i++ {
		n = n*10 + int(digits[i]-'0')
	}
	for i := len(digits); i < 9; i++ {
		n *= 10
	}
	return n
}

// readFractionWidth reads a fixed-width fractional second.
// A leading '.' or ',' is consumed when present (the usual Go layout
// convention), but plain digits are accepted as well.
func readFractionWidth(value string, width int) (int, string, bool) {
	start := 0
	if len(value) > 0 && (value[0] == '.' || value[0] == ',') {
		start = 1
	}
	if len(value) < start+width {
		return 0, value, false
	}
	n := 0
	for i := start; i < start+width; i++ {
		if !isASCIIDigit(value[i]) {
			return 0, value, false
		}
		n = n*10 + int(value[i]-'0')
	}
	for i := 0; i < 9-width; i++ {
		n *= 10
	}
	return n, value[start+width:], true
}

// build validates the parsed fields and constructs the resulting time.
func (s *parseState) build() (time.Time, error) {
	year := s.year
	month := time.January
	day := 1

	if s.ydaySet {
		if s.yday < 1 || s.yday > 366 || (s.yday == 366 && !isLeap(year)) {
			return time.Time{}, &ParseError{
				Format:  s.format,
				Value:   s.value,
				Offset:  len(s.value),
				message: "day-of-year out of range",
			}
		}
		m, d := ydayToMonthDay(year, s.yday)
		if s.monthSet && s.month != int(m) {
			return time.Time{}, s.conflictError(s.ydayTok, "day-of-year does not match month")
		}
		if s.daySet && s.day != d {
			return time.Time{}, s.conflictError(s.ydayTok, "day-of-year does not match day")
		}
		month, day = time.Month(m), d
	} else {
		if s.monthSet {
			month = time.Month(s.month)
		}
		if s.daySet {
			day = s.day
		}
	}

	if day < 1 || day > daysInMonth(month, year) {
		return time.Time{}, &ParseError{
			Format:  s.format,
			Value:   s.value,
			Offset:  len(s.value),
			message: "day out of range",
		}
	}

	hour := s.hour
	if s.hour12Set {
		hour = s.hour12
		if s.meridianSet {
			if s.meridianPM {
				if hour < 12 {
					hour += 12
				}
			} else if hour == 12 {
				hour = 0
			}
		}
		if hour >= 24 {
			return time.Time{}, s.consistencyError("hour out of range")
		}
	}

	if s.weekdayShortSet && s.weekdayLongSet && s.weekdayShort != s.weekdayLong {
		return time.Time{}, s.conflictError(s.weekdayLongTok, "weekday does not match %"+string(s.weekdayShortTok.spec))
	}

	var loc *time.Location
	switch {
	case s.zone != nil:
		loc = s.zone
	case s.offsetSet:
		if s.offset == 0 && !s.zoneNameSet {
			loc = time.UTC
			break
		}
		loc = time.FixedZone(s.zoneName, s.offset)
	case s.zoneNameSet:
		loc = resolveZoneName(s.zoneName)
	default:
		loc = time.UTC
	}
	if s.zoneNumericName {
		// A GMT±h zone name describes UTC fields with an offset location
		// attached: build in UTC first, then reinterpret in the location.
		t := time.Date(year, month, day, hour, s.min, s.sec, s.nsec, time.UTC)
		return t.In(loc), nil
	}

	return time.Date(year, month, day, hour, s.min, s.sec, s.nsec, loc), nil
}

func (s *parseState) consistencyError(message string) *ParseError {
	return &ParseError{
		Format:  s.format,
		Value:   s.value,
		Offset:  len(s.value),
		message: message,
	}
}

func (s *parseState) conflictError(tok *token, message string) *ParseError {
	if tok == nil {
		return s.consistencyError(message)
	}
	return &ParseError{
		Format:    s.format,
		Value:     s.value,
		Directive: tok.text,
		Offset:    tok.offset,
		message:   message,
	}
}

func ydayToMonthDay(year, yday int) (month int, day int) {
	t := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, yday-1)
	return int(t.Month()), t.Day()
}

func daysInMonth(m time.Month, year int) int {
	if m == time.February {
		if isLeap(year) {
			return 29
		}
		return 28
	}
	return 30 + int((int(m)+int(m)>>3)&1)
}

func isLeap(year int) bool {
	mask := 0xf
	if year%25 != 0 {
		mask = 3
	}
	return year&mask == 0
}
