package strftime

func (p *valueParser) parseDirective(idx int, o *op) error {
	switch o.code {
	case opYear:
		if p.digitRun(4) < 4 {
			return p.failf(o, "expected 4-digit year")
		}
		y := 0
		for k := 0; k < 4; k++ {
			y = y*10 + int(p.value[p.pos+k]-'0')
		}
		p.pos += 4
		return p.setField(fieldYear, y, o)

	case opYear2:
		if p.digitRun(2) < 2 {
			return p.failf(o, "expected 2-digit year")
		}
		y := int(p.value[p.pos]-'0')*10 + int(p.value[p.pos+1]-'0')
		p.pos += 2
		if y >= 69 {
			y += 1900
		} else {
			y += 2000
		}
		return p.setField(fieldYear, y, o)

	case opMonth:
		m, ok := p.getNum(false)
		if !ok {
			return p.failf(o, "expected month")
		}
		if m < 1 || m > 12 {
			return p.failf(o, "month out of range")
		}
		return p.setField(fieldMonth, m, o)

	case opMonthName, opMonthLong:
		tab := longMonthNames
		if o.code == opMonthName {
			tab = shortMonthNames
		}
		idx, ok := p.lookupName(tab)
		if !ok {
			return p.failf(o, "expected month name")
		}
		return p.setField(fieldMonth, idx+1, o)

	case opDay:
		d, ok := p.getNum(false)
		if !ok {
			return p.failf(o, "expected day")
		}
		return p.setField(fieldDay, d, o)

	case opDaySpace:
		if p.pos < len(p.value) && p.value[p.pos] == ' ' {
			p.pos++
		}
		d, ok := p.getNum(false)
		if !ok {
			return p.failf(o, "expected day")
		}
		return p.setField(fieldDay, d, o)

	case opYearDay:
		for i := 0; i < 2; i++ {
			if p.pos < len(p.value) && p.value[p.pos] == ' ' {
				p.pos++
			}
		}
		d, ok := p.getNum3(false)
		if !ok {
			return p.failf(o, "expected day of year")
		}
		return p.setField(fieldYearDay, d, o)

	case opHour:
		h, ok := p.getNum(false)
		if !ok || h < 0 || h > 23 {
			return p.failf(o, "hour out of range")
		}
		return p.setField(fieldHour, h, o)

	case opHour12:
		h, ok := p.getNum(false)
		if !ok || h < 1 || h > 12 {
			return p.failf(o, "hour out of range")
		}
		return p.setField(fieldHour, h, o)

	case opMinute:
		m, ok := p.getNum(false)
		if !ok || m < 0 || m > 59 {
			return p.failf(o, "minute out of range")
		}
		return p.setField(fieldMinute, m, o)

	case opSecond:
		s, ok := p.getNum(false)
		if !ok || s < 0 || s >= 60 {
			return p.failf(o, "second out of range")
		}
		if err := p.setField(fieldSecond, s, o); err != nil {
			return err
		}
		// Consume a fractional second even when none appears in the format,
		// unless a fraction directive follows.
		if p.pos+1 < len(p.value) &&
			(p.value[p.pos] == '.' || p.value[p.pos] == ',') &&
			p.value[p.pos+1] >= '0' && p.value[p.pos+1] <= '9' &&
			!p.fractionFollows(idx) {
			return p.consumeImplicitFraction(o)
		}
		return nil

	case opMeridianUpper:
		return p.parseMeridian(o, false)
	case opMeridianLower:
		return p.parseMeridian(o, true)

	case opZoneOffset:
		return p.parseZoneOffset(o)
	case opZoneName:
		return p.parseZoneName(o)

	case opWeekdayName, opWeekdayLong:
		tab := longDayNames
		if o.code == opWeekdayName {
			tab = shortDayNames
		}
		if _, ok := p.lookupName(tab); !ok {
			return p.failf(o, "expected weekday")
		}
		return nil
	}
	return p.failf(o, "unsupported directive")
}

func (p *valueParser) fractionFollows(idx int) bool {
	// A separator compiled directly after the second, followed by a
	// fractional-second directive, owns the digits.
	if idx+2 >= len(p.spec.ops) {
		return false
	}
	sep := p.spec.ops[idx+1]
	if sep.code != opLiteral || len(sep.lit) != 1 ||
		(sep.lit[0] != '.' && sep.lit[0] != ',') {
		return false
	}
	return p.spec.ops[idx+2].code == opFraction
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i]|('a'-'A') != b[i]|('a'-'A') {
			return false
		}
	}
	return true
}

func (p *valueParser) parseMeridian(o *op, lower bool) error {
	if len(p.value)-p.pos < 2 {
		return p.failf(o, "expected meridian")
	}
	token := p.value[p.pos : p.pos+2]
	var am bool
	var wantAM, wantPM string
	if lower {
		wantAM, wantPM = "am", "pm"
	} else {
		wantAM, wantPM = "AM", "PM"
	}
	switch {
	case equalFoldASCII(token, wantAM):
		am = true
	case equalFoldASCII(token, wantPM):
		am = false
	default:
		return p.failf(o, "expected meridian")
	}
	if p.fields&fieldMeridian != 0 && p.am != am {
		return p.failf(o, "conflicting meridian")
	}
	p.fields |= fieldMeridian
	p.am = am
	if !am {
		p.pm = true
	}
	p.pos += 2
	return nil
}
