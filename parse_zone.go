package strftime

import (
	"time"
)

// parseFraction reads a fixed-width fractional second (%L/%f/%N).
// The separator is a literal op and has already been matched.
func (p *valueParser) parseFraction(o *op) error {
	if p.digitRun(o.width) < o.width {
		return p.failf(o, "expected fractional second")
	}
	nsec := 0
	for k := 0; k < o.width; k++ {
		nsec = nsec*10 + int(p.value[p.pos+k]-'0')
	}
	p.pos += o.width
	for scale := 9 - o.width; scale > 0; scale-- {
		nsec *= 10
	}
	return p.setField(fieldNSecond, nsec, o)
}

// consumeImplicitFraction consumes a ".nnn" not represented by a directive,
// mirroring time.Parse: up to 9 digits, truncated/padded to nanoseconds.
func (p *valueParser) consumeImplicitFraction(o *op) error {
	p.pos++ // separator
	start := p.pos
	for p.pos < len(p.value) && p.value[p.pos] >= '0' && p.value[p.pos] <= '9' &&
		p.pos-start < 9 {
		p.pos++
	}
	digits := p.value[start:p.pos]
	nsec := 0
	for k := 0; k < len(digits); k++ {
		nsec = nsec*10 + int(digits[k]-'0')
	}
	for scale := 9 - len(digits); scale > 0; scale-- {
		nsec *= 10
	}
	return p.setField(fieldNSecond, nsec, o)
}

// parseZoneOffset parses %z (+hhmm) or %:z (+hh:mm); a leading Z means UTC.
func (p *valueParser) parseZoneOffset(o *op) error {
	if p.pos < len(p.value) && p.value[p.pos] == 'Z' {
		p.pos++
		p.zoneUTC = true
		return p.setField(fieldZoneOffset, 0, o)
	}

	colon := o.flag == ':'
	minLen := 5
	if colon {
		minLen = 6
	}
	if len(p.value)-p.pos < minLen {
		return p.failf(o, "expected zone offset")
	}
	sign := p.value[p.pos]
	if sign != '+' && sign != '-' {
		return p.failf(o, "expected zone offset")
	}
	h1, h2 := p.value[p.pos+1], p.value[p.pos+2]
	var m1, m2 byte
	if colon {
		if p.value[p.pos+3] != ':' {
			return p.failf(o, "expected zone offset")
		}
		m1, m2 = p.value[p.pos+4], p.value[p.pos+5]
	} else {
		m1, m2 = p.value[p.pos+3], p.value[p.pos+4]
	}
	for _, c := range []byte{h1, h2, m1, m2} {
		if c < '0' || c > '9' {
			return p.failf(o, "expected zone offset")
		}
	}
	hour := int(h1-'0')*10 + int(h2-'0')
	min := int(m1-'0')*10 + int(m2-'0')
	if hour > 24 || min > 60 {
		return p.failf(o, "time zone offset out of range")
	}
	offset := hour*3600 + min*60
	if sign == '-' {
		offset = -offset
	}
	p.pos += minLen
	p.zoneUTC = false
	return p.setField(fieldZoneOffset, offset, o)
}

// parseZoneName parses %Z. "UTC" is recognized specially and "GMT" may
// carry an hour offset; other names follow the time.Parse heuristic.
func (p *valueParser) parseZoneName(o *op) error {
	rest := p.value[p.pos:]
	if len(rest) >= 3 && rest[:3] == "UTC" {
		p.pos += 3
		p.zoneName = "UTC"
		p.zoneUTC = true
		return nil
	}

	length, ok := timeZoneNameLength(rest)
	if !ok {
		return p.failf(o, "expected time zone")
	}
	p.zoneName = rest[:length]
	p.pos += length
	p.zoneUTC = false
	return nil
}

// timeZoneNameLength mirrors the heuristic in time.parseTimeZone for
// recognizing an abbreviated zone name at the start of value.
func timeZoneNameLength(value string) (int, bool) {
	if len(value) < 3 {
		return 0, false
	}
	if len(value) >= 4 && (value[:4] == "ChST" || value[:4] == "MeST") {
		return 4, true
	}
	if value[:3] == "GMT" {
		return 3 + signedOffsetLength(value[3:]), true
	}
	if value[0] == '+' || value[0] == '-' {
		n := signedOffsetLength(value)
		return n, n > 0
	}
	n := 0
	for n < 6 && n < len(value) {
		c := value[n]
		if c < 'A' || c > 'Z' {
			break
		}
		n++
	}
	switch n {
	case 3:
		return 3, true
	case 4:
		if value[3] == 'T' || value[:4] == "WITA" {
			return 4, true
		}
	case 5:
		if value[4] == 'T' {
			return 5, true
		}
	}
	return 0, false
}

func signedOffsetLength(value string) int {
	if len(value) == 0 || (value[0] != '-' && value[0] != '+') {
		return 0
	}
	i := 1
	x := 0
	for i < len(value) && value[i] >= '0' && value[i] <= '9' {
		x = x*10 + int(value[i]-'0')
		i++
	}
	if i == 1 || x > 23 {
		return 0
	}
	return i
}

// time assembles the parsed fields into a time.Time.
func (p *valueParser) time() (time.Time, error) {
	if p.fields&fieldMeridian != 0 {
		switch {
		case p.pm && p.hour < 12:
			p.hour += 12
		case p.am && p.hour == 12:
			p.hour = 0
		}
	}

	year := p.year
	month := p.month
	day := p.day

	if p.fields&fieldYearDay != 0 {
		m, d, ok := dayFromYearDay(year, p.yday)
		if !ok {
			return time.Time{}, p.trailingError("day-of-year out of range")
		}
		if p.fields&fieldMonth != 0 && month != m {
			return time.Time{}, p.trailingError("day-of-year does not match month")
		}
		if p.fields&fieldDay != 0 && day != d {
			return time.Time{}, p.trailingError("day-of-year does not match day")
		}
		month, day = m, d
	}
	if month == 0 {
		month = int(time.January)
	}
	if day == 0 {
		day = 1
	}

	if day < 1 || day > daysInMonth(time.Month(month), year) {
		return time.Time{}, p.trailingError("day out of range")
	}

	loc := p.location(year, month, day)
	return time.Date(year, time.Month(month), day, p.hour, p.min, p.sec, p.nsec, loc), nil
}

func (p *valueParser) trailingError(message string) error {
	return &ParseError{
		Value:        p.value,
		FormatOffset: -1,
		ValueOffset:  len(p.value),
		Message:      message,
	}
}

// location resolves the location implied by %z/%Z, if any.
func (p *valueParser) location(year, month, day int) *time.Location {
	switch {
	case p.zoneUTC:
		return time.UTC
	case p.fields&fieldZoneOffset != 0:
		// Match time.Parse: if the local zone was in effect with this
		// offset at the parsed instant, use it; otherwise a fixed zone.
		t := time.Date(year, time.Month(month), day, p.hour, p.min, p.sec, p.nsec, time.UTC).
			Add(-time.Duration(p.zoneOffset) * time.Second)
		if local, err := time.LoadLocation("Local"); err == nil {
			if _, offset := t.In(local).Zone(); offset == p.zoneOffset {
				return local
			}
		}
		return time.FixedZone(p.zoneName, p.zoneOffset)
	case p.zoneName != "":
		offset := 0
		if len(p.zoneName) > 3 && p.zoneName[:3] == "GMT" {
			if n, err := parseInt(p.zoneName[3:]); err == nil {
				offset = n * 3600
			}
		}
		return time.FixedZone(p.zoneName, offset)
	}
	if p.loc != nil {
		return p.loc
	}
	return time.UTC
}

func parseInt(s string) (int, error) {
	neg := false
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	n := 0
	if s == "" {
		return 0, errBadNumber
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, errBadNumber
		}
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		n = -n
	}
	return n, nil
}

var errBadNumber = parseErr("bad number")

type parseErr string

func (e parseErr) Error() string { return string(e) }

func isLeap(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

func daysInMonth(m time.Month, year int) int {
	days := int(daysBefore[m] - daysBefore[m-1])
	if m == time.February && isLeap(year) {
		days++
	}
	return days
}

// daysBefore[m] counts the number of days in a non-leap year
// before month m begins. There is an entry for m=13, which counts
// the number of days in a year after all months are finished.
// It's the same as daysBefore[12] + 31.
var daysBefore = [...]int32{
	0,
	31,
	31 + 28,
	31 + 28 + 31,
	31 + 28 + 31 + 30,
	31 + 28 + 31 + 30 + 31,
	31 + 28 + 31 + 30 + 31 + 30,
	31 + 28 + 31 + 30 + 31 + 30 + 31,
	31 + 28 + 31 + 30 + 31 + 30 + 31 + 31,
	31 + 28 + 31 + 30 + 31 + 30 + 31 + 31 + 30,
	31 + 28 + 31 + 30 + 31 + 30 + 31 + 31 + 30 + 31,
	31 + 28 + 31 + 30 + 31 + 30 + 31 + 31 + 30 + 31 + 30,
	31 + 28 + 31 + 30 + 31 + 30 + 31 + 31 + 30 + 31 + 30 + 31,
}

// dayFromYearDay converts a 1-based year day to (month, 1-based day),
// following the rules in the time package.
func dayFromYearDay(year, yday int) (month, day int, ok bool) {
	leap := isLeap(year)
	m, d := 0, 0
	if leap && yday == 31+29 {
		m, d = int(time.February), 29
	} else {
		y := yday
		if leap && y > 31+29 {
			y--
		}
		if y < 1 || y > 365 {
			return 0, 0, false
		}
		m = (y-1)/31 + 1
		if daysBefore[m] < int32(y) {
			m++
		}
		d = y - int(daysBefore[m-1])
	}
	return m, d, true
}
