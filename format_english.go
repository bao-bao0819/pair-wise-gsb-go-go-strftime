package strftime

import (
	"strconv"
	"time"
)

// appendFormatEnglish renders a format using the built-in English names.
// This is the original package-level implementation, kept as a fast
// path so the default API avoids locale lookup overhead.
func appendFormatEnglish(dst []byte, fmt string, t time.Time) []byte {
	var parser parser

	parser.literal = func(b byte) error {
		dst = append(dst, b)
		return nil
	}

	parser.format = func(spec, flag byte) error {
		switch spec {
		case 'A':
			dst = append(dst, t.Weekday().String()...)
			return nil
		case 'a':
			dst = append(dst, t.Weekday().String()[:3]...)
			return nil
		case 'B':
			dst = append(dst, t.Month().String()...)
			return nil
		case 'b', 'h':
			dst = append(dst, t.Month().String()[:3]...)
			return nil
		case 'm':
			dst = appendInt2(dst, int(t.Month()), flag)
			return nil
		case 'd':
			dst = appendInt2(dst, int(t.Day()), flag)
			return nil
		case 'e':
			dst = appendInt2(dst, int(t.Day()), ' ')
			return nil
		case 'I':
			dst = append12Hour(dst, t, flag)
			return nil
		case 'l':
			dst = append12Hour(dst, t, ' ')
			return nil
		case 'H':
			dst = appendInt2(dst, t.Hour(), flag)
			return nil
		case 'k':
			dst = appendInt2(dst, t.Hour(), ' ')
			return nil
		case 'M':
			dst = appendInt2(dst, t.Minute(), flag)
			return nil
		case 'S':
			dst = appendInt2(dst, t.Second(), flag)
			return nil
		case 'L':
			dst = append(dst, t.Format(".000")[1:]...)
			return nil
		case 'f':
			dst = append(dst, t.Format(".000000")[1:]...)
			return nil
		case 'N':
			dst = append(dst, t.Format(".000000000")[1:]...)
			return nil
		case 'y':
			dst = t.AppendFormat(dst, "06")
			return nil
		case 'Y':
			dst = t.AppendFormat(dst, "2006")
			return nil
		case 'C':
			c := century(t)
			if c >= 0 && c < 10 {
				dst = append(dst, '0')
			}
			dst = strconv.AppendInt(dst, int64(c), 10)
			return nil
		case 'U':
			dst = appendWeekNumber(dst, t, flag, true)
			return nil
		case 'W':
			dst = appendWeekNumber(dst, t, flag, false)
			return nil
		case 'V':
			_, w := t.ISOWeek()
			dst = appendInt2(dst, w, flag)
			return nil
		case 'g':
			y, _ := t.ISOWeek()
			dst = year(y).AppendFormat(dst, "06")
			return nil
		case 'G':
			y, _ := t.ISOWeek()
			dst = year(y).AppendFormat(dst, "2006")
			return nil
		case 's':
			dst = strconv.AppendInt(dst, t.Unix(), 10)
			return nil
		case 'Q':
			dst = strconv.AppendInt(dst, t.UnixMilli(), 10)
			return nil
		case 'w':
			dst = appendInt1(dst, int(t.Weekday()))
			return nil
		case 'u':
			if w := t.Weekday(); w == 0 {
				dst = append(dst, '7')
			} else {
				dst = appendInt1(dst, int(w))
			}
			return nil
		case 'j':
			if flag == '-' {
				dst = strconv.AppendInt(dst, int64(t.YearDay()), 10)
			} else {
				dst = t.AppendFormat(dst, "002")
			}
			return nil
		}

		if layout := goLayout(spec, flag, false); layout != "" {
			dst = t.AppendFormat(dst, layout)
			return nil
		}

		dst = append(dst, '%')
		if flag != 0 {
			dst = append(dst, flag)
		}
		dst = append(dst, spec)
		return nil
	}

	parser.parse(fmt)
	return dst
}
