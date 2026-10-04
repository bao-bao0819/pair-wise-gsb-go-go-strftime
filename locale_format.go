package strftime

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// Format returns a textual representation of the time value
// formatted according to the strftime format specification,
// using the month, weekday, and meridiem names of the [Locale].
func (l *Locale) Format(fmt string, t time.Time) string {
	buf := buffer(fmt)
	return string(l.AppendFormat(buf, fmt, t))
}

// AppendFormat is like [Locale.Format], but appends the textual
// representation to dst and returns the extended buffer.
func (l *Locale) AppendFormat(dst []byte, fmt string, t time.Time) []byte {
	if l.english {
		return appendFormatEnglish(dst, fmt, t)
	}
	r := localeRenderer{l: l, t: t}
	var p parser
	p.literal = func(b byte) error {
		dst = append(dst, b)
		return nil
	}
	p.format = func(spec, flag byte) error {
		var err error
		dst, err = r.format(dst, spec, flag, nil)
		return err
	}
	if err := p.parse(fmt); err != nil {
		dst = append(dst, fmt...)
	}
	return dst
}

type localeRenderer struct {
	l *Locale
	t time.Time
}

// format renders a single directive, expanding combination
// directives recursively. stack detects custom-format cycles.
func (r *localeRenderer) format(dst []byte, spec, flag byte, stack []byte) ([]byte, error) {
	// A flagged directive is never a combination; render the primitive.
	if flag == 0 {
		if value, custom := r.l.lookupFormat(spec); value != "" {
			if custom {
				for _, k := range stack {
					if k == spec {
						return dst, errors.New("cycle in combination formats")
					}
				}
				stack = append(stack, spec)
			}
			return r.renderSpec(dst, value, stack)
		}
	}
	return r.primitive(dst, spec, flag), nil
}

// renderSpec parses and renders an expanded/raw format body.
func (r *localeRenderer) renderSpec(dst []byte, body string, stack []byte) ([]byte, error) {
	var firstErr error
	var p parser
	p.literal = func(b byte) error {
		dst = append(dst, b)
		return nil
	}
	p.format = func(spec, flag byte) error {
		var err error
		dst, err = r.format(dst, spec, flag, stack)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		return err
	}
	_ = p.parse(body)
	return dst, firstErr
}

func (r *localeRenderer) primitive(dst []byte, spec, flag byte) []byte {
	t := r.t
	l := r.l
	switch spec {
	case 'A':
		return append(dst, l.weekdayName(t.Weekday(), false)...)
	case 'a':
		return append(dst, l.weekdayName(t.Weekday(), true)...)
	case 'B':
		return append(dst, l.monthName(t.Month(), false)...)
	case 'b', 'h':
		return append(dst, l.monthName(t.Month(), true)...)
	case 'm':
		return appendInt2(dst, int(t.Month()), flag)
	case 'd':
		return appendInt2(dst, t.Day(), flag)
	case 'e':
		return appendInt2(dst, t.Day(), ' ')
	case 'I':
		return append12Hour(dst, t, flag)
	case 'l':
		return append12Hour(dst, t, ' ')
	case 'H':
		return appendInt2(dst, t.Hour(), flag)
	case 'k':
		return appendInt2(dst, t.Hour(), ' ')
	case 'M':
		return appendInt2(dst, t.Minute(), flag)
	case 'S':
		return appendInt2(dst, t.Second(), flag)
	case 'L':
		return append(dst, t.Format(".000")[1:]...)
	case 'f':
		return append(dst, t.Format(".000000")[1:]...)
	case 'N':
		return append(dst, t.Format(".000000000")[1:]...)
	case 'y':
		return t.AppendFormat(dst, "06")
	case 'Y':
		return t.AppendFormat(dst, "2006")
	case 'C':
		c := century(t)
		if c >= 0 && c < 10 {
			dst = append(dst, '0')
		}
		return strconv.AppendInt(dst, int64(c), 10)
	case 'U':
		return appendWeekNumber(dst, t, flag, true)
	case 'W':
		return appendWeekNumber(dst, t, flag, false)
	case 'V':
		_, w := t.ISOWeek()
		return appendInt2(dst, w, flag)
	case 'g':
		y, _ := t.ISOWeek()
		return year(y).AppendFormat(dst, "06")
	case 'G':
		y, _ := t.ISOWeek()
		return year(y).AppendFormat(dst, "2006")
	case 's':
		return strconv.AppendInt(dst, t.Unix(), 10)
	case 'Q':
		return strconv.AppendInt(dst, t.UnixMilli(), 10)
	case 'w':
		return appendInt1(dst, int(t.Weekday()))
	case 'u':
		if wd := t.Weekday(); wd == 0 {
			return append(dst, '7')
		}
		return appendInt1(dst, int(t.Weekday()))
	case 'j':
		if flag == '-' {
			return strconv.AppendInt(dst, int64(t.YearDay()), 10)
		}
		return t.AppendFormat(dst, "002")
	case 'p':
		if l.english {
			return append(dst, t.Format("PM")...)
		}
		return append(dst, l.meridian(t.Hour())...)
	case 'P':
		if l.english {
			return append(dst, t.Format("pm")...)
		}
		return append(dst, strings.ToLower(l.meridian(t.Hour()))...)
	}
	if layout := goLayout(spec, flag, false); layout != "" {
		return t.AppendFormat(dst, layout)
	}
	dst = append(dst, '%')
	if flag != 0 {
		dst = append(dst, flag)
	}
	return append(dst, spec)
}
