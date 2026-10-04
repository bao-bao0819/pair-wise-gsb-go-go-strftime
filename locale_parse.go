package strftime

import (
	"errors"
	"time"
)

// Parse converts a textual representation of time to the time value
// it represents according to the strptime format specification,
// matching month, weekday, and meridiem names using the [Locale].
//
// When both a weekday (%A/%a) and a date are given, the weekday must
// agree with the date, otherwise an error is returned.
func (l *Locale) Parse(fmt, value string) (time.Time, error) {
	if l.english {
		return Parse(fmt, value)
	}

	expanded, err := l.expand(fmt, nil)
	if err != nil {
		return time.Time{}, err
	}

	tokens, err := tokenize(expanded)
	if err != nil {
		return time.Time{}, err
	}
	// A trailing fraction after %S belongs to an explicit %L/%f/%N
	// directive when one follows the point; otherwise %S consumes it.
	for i, tk := range tokens {
		if tk.spec != 'S' {
			continue
		}
		j := i + 1
		if j < len(tokens) && !tokens[j].isSpec && (tokens[j].text == "." || tokens[j].text == ",") {
			k := j + 1
			if k < len(tokens) && tokens[k].isSpec && (tokens[k].spec == 'L' || tokens[k].spec == 'f' || tokens[k].spec == 'N') {
				tokens[i].skipFraction = true
			}
		}
	}

	p := &strptime{
		locale: l,
		value:  value,
	}

	for _, tk := range tokens {
		var err error
		if tk.isSpec {
			p.skipFraction = tk.skipFraction
			err = p.format(tk.spec, tk.flag)
			if fe, ok := err.(formatError); ok {
				fe.directive = directiveText(tk)
				err = fe
			}
		} else {
			for i := 0; i < len(tk.text); i++ {
				if err = p.literal(tk.text[i]); err != nil {
					break
				}
			}
		}
		if err != nil {
			return time.Time{}, err
		}
	}
	if p.pos != len(value) {
		return time.Time{}, errors.New("strftime: extra text: " + value[p.pos:])
	}
	return p.build()
}

func directiveText(tk token) string {
	if tk.flag != 0 {
		return "%" + string(tk.flag) + string(tk.spec)
	}
	return "%" + string(tk.spec)
}

type token struct {
	isSpec       bool
	text         string
	spec, flag   byte
	skipFraction bool
}

// tokenize splits a strftime format (with combinations expanded)
// into literal text and directive tokens.
func tokenize(fmt string) ([]token, error) {
	var tokens []token
	var literal []byte
	flush := func() {
		if len(literal) > 0 {
			tokens = append(tokens, token{text: string(literal)})
			literal = nil
		}
	}

	for i := 0; i < len(fmt); {
		if fmt[i] != '%' {
			literal = append(literal, fmt[i])
			i++
			continue
		}

		j := i + 1
		var flag byte
		if j < len(fmt) && (fmt[j] == '-' || fmt[j] == ':') {
			flag = fmt[j]
			j++
		}
		var modifier byte
		if j < len(fmt) && (fmt[j] == 'E' || fmt[j] == 'O') {
			modifier = fmt[j]
			j++
		}
		if j >= len(fmt) {
			// Trailing incomplete sequence: treat as literal.
			literal = append(literal, fmt[i:]...)
			i = len(fmt)
			continue
		}
		if modifier != 0 && !okModifier(modifier, fmt[j]) {
			// Invalid modifier combination: the whole sequence is literal.
			literal = append(literal, fmt[i:j+1]...)
			i = j + 1
			continue
		}
		flush()
		tokens = append(tokens, token{isSpec: true, spec: fmt[j], flag: flag})
		i = j + 1
	}
	flush()
	return tokens, nil
}

type strptime struct {
	locale *Locale
	value  string
	pos    int

	year    int
	month   int
	day     int
	yearday int
	hour    int
	min     int
	sec     int
	nsec    int

	weekday  int
	hour12   int
	meridiem int // 0 = AM, 1 = PM, -1 = absent

	zoneName     string
	zoneOff      int
	hasZone      bool
	hasOff       bool
	lastByte     byte
	skipFraction bool

	flags uint16
}

const (
	hasYear = 1 << iota
	hasMonth
	hasDay
	hasYearday
	hasHour
	hasMin
	hasSec
	hasWeekday
	hasHour12
	hasMeridiem
)

func (p *strptime) literal(b byte) error {
	if p.pos < len(p.value) && p.value[p.pos] == b {
		p.pos++
		p.lastByte = b
		return nil
	}
	return errors.New("strftime: cannot parse " + quoteByte(b) + " at position")
}

func quoteByte(b byte) string {
	if b >= ' ' && b < 0x7f {
		return string([]byte{'\'', b, '\''})
	}
	return string([]byte{'0', 'x', hexDigit(b >> 4), hexDigit(b & 0xF)})
}

func hexDigit(b byte) byte {
	if b < 10 {
		return '0' + b
	}
	return 'a' + b - 10
}

func (p *strptime) format(spec, flag byte) error {
	switch spec {
	case 'Y':
		return p.parseYear(flag)
	case 'y':
		return p.parseYear2()
	case 'm':
		return p.parseIntRange(&p.month, &p.flags, hasMonth, 1, 12, 2)
	case 'd', 'e':
		return p.parseDay()
	case 'j':
		if err := p.parseIntRange(&p.yearday, &p.flags, hasYearday, 1, 366, 3); err != nil {
			return err
		}
		maxDay := time.Date(p.year, time.December, 31, 0, 0, 0, 0, time.UTC).YearDay()
		if p.yearday > maxDay {
			return errors.New("strftime: day-of-year out of range")
		}
		return nil
	case 'H':
		return p.parseIntRange(&p.hour, &p.flags, hasHour, 0, 23, 2)
	case 'I':
		return p.parseIntRange(&p.hour12, &p.flags, hasHour12, 1, 12, 2)
	case 'M':
		return p.parseIntRange(&p.min, &p.flags, hasMin, 0, 59, 2)
	case 'S':
		if err := p.parseIntRange(&p.sec, &p.flags, hasSec, 0, 59, 2); err != nil {
			return err
		}
		if !p.skipFraction && p.pos < len(p.value) && (p.value[p.pos] == '.' || p.value[p.pos] == ',') {
			dot := p.pos
			p.pos++
			if err := p.scanNanos(9); err != nil {
				p.pos = dot
				return err
			}
		}
		p.skipFraction = false
		return nil
	case 'L', 'f', 'N':
		return p.parseFixedFraction(spec)
	case 'U', 'V', 'W', 'g', 'G', 'C', 's', 'Q', 'u', 'w', 'k', 'l':
		return formatError{}
	case 'B', 'b', 'h':
		if err := p.parseName(&p.month, &p.flags, hasMonth, &p.locale.months); err != nil {
			return err
		}
		p.month++ // matcher values are zero-based; months are one-based
		return nil
	case 'A', 'a':
		return p.parseName(&p.weekday, &p.flags, hasWeekday, &p.locale.weekdays)
	case 'p', 'P':
		v, size, ok := p.locale.meridiem.matchAt(p.value, p.pos)
		if !ok {
			return errors.New("strftime: cannot parse meridiem")
		}
		p.meridiem = v
		p.flags |= hasMeridiem
		p.pos += size
		return nil
	case 'z':
		return p.parseZone(flag)
	case 'Z':
		return p.parseZoneName()
	case 'n':
		return p.literal('\n')
	case 't':
		return p.literal('\t')
	case '%':
		return p.literal('%')
	}
	return formatError{}
}

// parseDay reads an optional leading blank followed by 1 or 2 digits,
// as used by %d and %e.
func (p *strptime) parseDay() error {
	start := p.pos
	if p.pos < len(p.value) && p.value[p.pos] == ' ' {
		p.pos++
	}
	v, n := p.scanDigits(2)
	if n == 0 {
		p.pos = start
		return errors.New("strftime: cannot parse day")
	}
	if v < 1 || v > 31 {
		return errors.New("strftime: day out of range")
	}
	p.day = v
	p.flags |= hasDay
	return nil
}

// parseIntRange reads up to maxWidth digits and validates the range.
func (p *strptime) parseIntRange(dst *int, flags *uint16, bit uint16, min, max, maxWidth int) error {
	v, n := p.scanDigits(maxWidth)
	if n == 0 {
		return errors.New("strftime: cannot parse integer")
	}
	if v < min || v > max {
		return errors.New("strftime: value out of range")
	}
	*dst = v
	*flags |= bit
	return nil
}

// scanDigits consumes up to width ASCII digits and returns the value
// and the number of digits consumed.
func (p *strptime) scanDigits(width int) (value, count int) {
	for count < width && p.pos < len(p.value) {
		c := p.value[p.pos]
		if c < '0' || c > '9' {
			break
		}
		value = value*10 + int(c-'0')
		count++
		p.pos++
	}
	return value, count
}

func (p *strptime) parseYear(flag byte) error {
	start := p.pos
	negative := false
	if p.pos < len(p.value) && (p.value[p.pos] == '-' || p.value[p.pos] == '+') {
		negative = p.value[p.pos] == '-'
		p.pos++
	}
	v, n := p.scanDigits(4)
	if n < 4 {
		p.pos = start
		return errors.New("strftime: cannot parse year")
	}
	if negative {
		v = -v
	}
	p.year = v
	p.flags |= hasYear
	return nil
}

func (p *strptime) parseYear2() error {
	v, n := p.scanDigits(2)
	if n < 2 {
		return errors.New("strftime: cannot parse 2-digit year")
	}
	switch {
	case v >= 69:
		v += 1900
	default:
		v += 2000
	}
	p.year = v
	p.flags |= hasYear
	return nil
}

// parseName matches the longest month or weekday name at the current
// position, trying exact comparison first and then case folding.
// Ordinary format literals are matched byte-for-byte elsewhere, so
// they are never consumed here.
func (p *strptime) parseName(dst *int, flags *uint16, bit uint16, m *nameMatcher) error {
	value, size, ok := m.matchAt(p.value, p.pos)
	if !ok {
		return errors.New("strftime: cannot parse " + m.kind + " name")
	}
	*dst = value
	*flags |= bit
	p.pos += size
	return nil
}

// parseFixedFraction handles %L (3), %f (6), and %N (9) digits.
// The point or comma is a literal in the format, already consumed by
// the time just before the directive.
func (p *strptime) parseFixedFraction(spec byte) error {
	width := 3
	switch spec {
	case 'f':
		width = 6
	case 'N':
		width = 9
	}
	if p.lastByte != '.' && p.lastByte != ',' {
		return errors.New("strftime: fractional second must follow '.' or ','")
	}
	return p.scanNanos(width)
}

// scanNanos reads exactly width digits (when width is 1..9) or as
// many as available (when width is 0), and stores the nanosecond
// fraction, padding with zeros to nine digits.
func (p *strptime) scanNanos(width int) error {
	start := p.pos
	var digits []byte
	for p.pos < len(p.value) && (width == 0 || len(digits) < width) {
		c := p.value[p.pos]
		if c < '0' || c > '9' {
			break
		}
		digits = append(digits, c)
		p.pos++
	}
	if len(digits) == 0 {
		return errors.New("strftime: fractional second has no digits")
	}
	if width != 0 && len(digits) != width {
		p.pos = start
		return errors.New("strftime: fractional second has wrong number of digits")
	}
	for len(digits) < 9 {
		digits = append(digits, '0')
	}
	if len(digits) > 9 {
		digits = digits[:9]
	}
	nsec := 0
	for _, c := range digits {
		nsec = nsec*10 + int(c-'0')
	}
	p.nsec = nsec
	return nil
}

func (p *strptime) parseZone(flag byte) error {
	start := p.pos
	if p.pos < len(p.value) && p.value[p.pos] == 'Z' {
		p.pos++
		p.zoneOff, p.hasZone, p.hasOff = 0, true, true
		return nil
	}

	neg := false
	if p.pos < len(p.value) && (p.value[p.pos] == '+' || p.value[p.pos] == '-') {
		neg = p.value[p.pos] == '-'
		p.pos++
	}

	hh, ok := p.readZoneNumber(2)
	if !ok {
		p.pos = start
		return errors.New("strftime: cannot parse zone offset")
	}
	mm := 0
	if p.pos < len(p.value) && p.value[p.pos] == ':' {
		p.pos++
		if m, mok := p.readZoneNumber(2); mok {
			mm = m
		}
	} else if p.pos < len(p.value) && p.value[p.pos] >= '0' && p.value[p.pos] <= '9' {
		if m, mok := p.readZoneNumber(2); mok {
			mm = m
		}
	}
	off := hh*3600 + mm*60
	if neg {
		off = -off
	}
	p.zoneOff, p.hasZone, p.hasOff = off, true, true
	return nil
}

// readZoneNumber reads one or two digits for an offset component.
func (p *strptime) readZoneNumber(max int) (int, bool) {
	start := p.pos
	v, n := 0, 0
	for n < max && p.pos < len(p.value) && p.value[p.pos] >= '0' && p.value[p.pos] <= '9' {
		v = v*10 + int(p.value[p.pos]-'0')
		p.pos++
		n++
	}
	if n == 0 {
		p.pos = start
		return 0, false
	}
	return v, true
}

func (p *strptime) parseZoneName() error {
	start := p.pos
	// Consume a token of letters (ASCII or Unicode), mirroring the
	// standard library's zone abbreviation handling.
	for p.pos < len(p.value) {
		c := p.value[p.pos]
		if isUpperASCII(c) || c >= 0x80 {
			p.pos++
			continue
		}
		// Walk a full non-ASCII rune so multi-byte letters are kept whole.
		break
	}
	if p.pos == start {
		return errors.New("strftime: cannot parse zone name")
	}
	p.zoneName = p.value[start:p.pos]
	p.hasZone = true
	return nil
}

func isUpperASCII(c byte) bool { return 'A' <= c && c <= 'Z' }

// build assembles the parsed fields into a time.Time, applying the
// 12-hour meridiem and validating ranges and weekday/date agreement.
func (p *strptime) build() (time.Time, error) {
	year := p.year
	month := p.month
	day := p.day
	if p.flags&hasMonth == 0 {
		month = 1
	}

	if p.flags&hasYearday != 0 {
		if p.flags&hasDay != 0 || p.flags&hasMonth != 0 {
			// Convert the ordinal day and check agreement.
			t0 := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
			want := t0.AddDate(0, 0, p.yearday-1)
			if p.flags&hasMonth != 0 && int(want.Month()) != month {
				return time.Time{}, errors.New("strftime: day-of-year does not match month")
			}
			if p.flags&hasDay != 0 && want.Day() != day {
				return time.Time{}, errors.New("strftime: day-of-year does not match day")
			}
			month, day = int(want.Month()), want.Day()
		} else {
			t0 := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
			t := t0.AddDate(0, 0, p.yearday-1)
			month, day = int(t.Month()), t.Day()
		}
	}
	if p.flags&hasDay == 0 && p.flags&hasYearday == 0 {
		day = 1
	}

	hour := p.hour
	if p.flags&hasHour12 != 0 {
		h := p.hour12
		if p.meridiem == 1 && h < 12 {
			h += 12
		} else if p.meridiem == 0 && h == 12 {
			h = 0
		}
		hour = h
	}

	t := time.Date(year, time.Month(month), day, hour, p.min, p.sec, p.nsec, time.UTC)
	if t.Month() != time.Month(month) || t.Day() != day || t.Year() != year {
		return time.Time{}, errors.New("strftime: day out of range")
	}

	if p.flags&hasWeekday != 0 && (p.flags&hasDay != 0 || p.flags&hasMonth != 0 || p.flags&hasYearday != 0) {
		if int(t.Weekday()) != p.weekday {
			return time.Time{}, errors.New("strftime: weekday does not match date")
		}
	}

	loc := time.UTC
	if p.hasZone {
		if p.zoneName != "" && !p.hasOff {
			offset := p.zoneOff
			if p.zoneName == "UTC" || p.zoneName == "GMT" {
				offset = 0
			}
			loc = time.FixedZone(p.zoneName, offset)
		} else if p.hasOff || p.zoneName == "" {
			loc = time.FixedZone("", p.zoneOff)
		}
		t = t.In(loc)
	}
	return t, nil
}
