package strftime

import (
	"strconv"
	"time"
)

// Fields independently set by directives, used to reject conflicting
// repetitions such as two different years in the same format.
const (
	fieldYear = 1 << iota
	fieldMonth
	fieldDay
	fieldYearDay
	fieldHour
	fieldMinute
	fieldSecond
	fieldNSecond
	fieldZoneOffset
	fieldMeridian
)

var longDayNames = []string{
	"Sunday",
	"Monday",
	"Tuesday",
	"Wednesday",
	"Thursday",
	"Friday",
	"Saturday",
}

var shortDayNames = []string{
	"Sun",
	"Mon",
	"Tue",
	"Wed",
	"Thu",
	"Fri",
	"Sat",
}

var shortMonthNames = []string{
	"Jan",
	"Feb",
	"Mar",
	"Apr",
	"May",
	"Jun",
	"Jul",
	"Aug",
	"Sep",
	"Oct",
	"Nov",
	"Dec",
}

var longMonthNames = []string{
	"January",
	"February",
	"March",
	"April",
	"May",
	"June",
	"July",
	"August",
	"September",
	"October",
	"November",
	"December",
}

type valueParser struct {
	spec  *Spec
	value string
	pos   int
	loc   *time.Location

	year       int
	month      int
	day        int
	yday       int
	hour       int
	min        int
	sec        int
	nsec       int
	zoneOffset int
	zoneName   string
	zoneUTC    bool
	am         bool
	pm         bool
	fields     int
}

func (p *valueParser) parse() error {
	for i := range p.spec.ops {
		o := &p.spec.ops[i]
		switch o.code {
		case opLiteral:
			if err := p.skipLiteral(o.lit, o); err != nil {
				return err
			}
		case opFraction:
			if err := p.parseFraction(o); err != nil {
				return err
			}
		default:
			if err := p.parseDirective(i, o); err != nil {
				return err
			}
		}
	}

	if p.pos != len(p.value) {
		return &ParseError{
			Value:        p.value,
			FormatOffset: -1,
			ValueOffset:  p.pos,
			Message:      "extra text: " + strconv.Quote(p.value[p.pos:]),
		}
	}
	return nil
}

func (p *valueParser) failf(o *op, message string) error {
	pe := &ParseError{
		Value:       p.value,
		ValueOffset: p.pos,
		Message:     message,
	}
	if o != nil {
		pe.Directive = directiveText(o.spec, o.flag)
		pe.FormatOffset = o.offset
	}
	return pe
}

// skipLiteral matches a literal span against the input.
// A run of spaces in the format consumes a run of spaces in the input
// (matching Go's time.Parse); every other byte must match exactly.
func (p *valueParser) skipLiteral(lit []byte, o *op) error {
	i := 0
	for i < len(lit) {
		if lit[i] == ' ' {
			if p.pos >= len(p.value) || p.value[p.pos] != ' ' {
				return p.failf(o, "expected space")
			}
			for i < len(lit) && lit[i] == ' ' {
				i++
			}
			for p.pos < len(p.value) && p.value[p.pos] == ' ' {
				p.pos++
			}
			continue
		}
		if p.pos >= len(p.value) || p.value[p.pos] != lit[i] {
			return p.failf(o, "expected literal "+strconv.Quote(string(lit)))
		}
		i++
		p.pos++
	}
	return nil
}

// getNum reads one or two digits. With fixed set, two digits are required.
func (p *valueParser) getNum(fixed bool) (int, bool) {
	adv := p.digitRun(2)
	if adv == 0 || fixed && adv != 2 {
		return 0, false
	}
	v := 0
	for k := 0; k < adv; k++ {
		v = v*10 + int(p.value[p.pos+k]-'0')
	}
	p.pos += adv
	return v, true
}

// getNum3 reads one to three digits. With fixed set, three are required.
func (p *valueParser) getNum3(fixed bool) (int, bool) {
	adv := p.digitRun(3)
	if adv == 0 || fixed && adv != 3 {
		return 0, false
	}
	v := 0
	for k := 0; k < adv; k++ {
		v = v*10 + int(p.value[p.pos+k]-'0')
	}
	p.pos += adv
	return v, true
}

func (p *valueParser) digitRun(max int) int {
	adv := 0
	for adv < max && p.pos+adv < len(p.value) {
		c := p.value[p.pos+adv]
		if c < '0' || c > '9' {
			break
		}
		adv++
	}
	return adv
}

func (p *valueParser) lookupName(tab []string) (int, bool) {
	for i, name := range tab {
		if p.matchName(name) {
			p.pos += len(name)
			return i, true
		}
	}
	return 0, false
}

// matchName matches an ASCII name at the current position, ignoring case.
func (p *valueParser) matchName(name string) bool {
	if len(p.value)-p.pos < len(name) {
		return false
	}
	for i := 0; i < len(name); i++ {
		c1 := p.value[p.pos+i]
		c2 := name[i]
		l1 := c1 | ('a' - 'A')
		l2 := c2 | ('a' - 'A')
		if l1 != l2 || l1 < 'a' || l1 > 'z' {
			return false
		}
	}
	return true
}

func (p *valueParser) setField(bit, value int, o *op) error {
	if p.fields&bit != 0 && p.fieldValue(bit) != value {
		return p.failf(o, "conflicting value")
	}
	p.fields |= bit
	p.setFieldValue(bit, value)
	return nil
}

func (p *valueParser) fieldValue(bit int) int {
	switch bit {
	case fieldYear:
		return p.year
	case fieldMonth:
		return p.month
	case fieldDay:
		return p.day
	case fieldYearDay:
		return p.yday
	case fieldHour:
		return p.hour
	case fieldMinute:
		return p.min
	case fieldSecond:
		return p.sec
	case fieldNSecond:
		return p.nsec
	case fieldZoneOffset:
		return p.zoneOffset
	}
	return 0
}

func (p *valueParser) setFieldValue(bit, value int) {
	switch bit {
	case fieldYear:
		p.year = value
	case fieldMonth:
		p.month = value
	case fieldDay:
		p.day = value
	case fieldYearDay:
		p.yday = value
	case fieldHour:
		p.hour = value
	case fieldMinute:
		p.min = value
	case fieldSecond:
		p.sec = value
	case fieldNSecond:
		p.nsec = value
	case fieldZoneOffset:
		p.zoneOffset = value
	}
}
