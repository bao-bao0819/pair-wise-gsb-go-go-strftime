package strftime

import (
	"strconv"
	"time"
)

type formatOp func(dst []byte, t time.Time, flag byte) []byte

var formatOps = [256]formatOp{
	'A': func(dst []byte, t time.Time, _ byte) []byte {
		return append(dst, t.Weekday().String()...)
	},
	'a': func(dst []byte, t time.Time, _ byte) []byte {
		return append(dst, t.Weekday().String()[:3]...)
	},
	'B': func(dst []byte, t time.Time, _ byte) []byte {
		return append(dst, t.Month().String()...)
	},
	'b': formatMonthAbbr,
	'h': formatMonthAbbr,
	'm': func(dst []byte, t time.Time, flag byte) []byte {
		return appendInt2(dst, int(t.Month()), flag)
	},
	'd': func(dst []byte, t time.Time, flag byte) []byte {
		return appendInt2(dst, t.Day(), flag)
	},
	'e': func(dst []byte, t time.Time, _ byte) []byte {
		return appendInt2(dst, t.Day(), ' ')
	},
	'I': func(dst []byte, t time.Time, flag byte) []byte {
		return append12Hour(dst, t, flag)
	},
	'l': func(dst []byte, t time.Time, _ byte) []byte {
		return append12Hour(dst, t, ' ')
	},
	'H': func(dst []byte, t time.Time, flag byte) []byte {
		return appendInt2(dst, t.Hour(), flag)
	},
	'k': func(dst []byte, t time.Time, _ byte) []byte {
		return appendInt2(dst, t.Hour(), ' ')
	},
	'M': func(dst []byte, t time.Time, flag byte) []byte {
		return appendInt2(dst, t.Minute(), flag)
	},
	'S': func(dst []byte, t time.Time, flag byte) []byte {
		return appendInt2(dst, t.Second(), flag)
	},
	'L': func(dst []byte, t time.Time, _ byte) []byte {
		return append(dst, t.Format(".000")[1:]...)
	},
	'f': func(dst []byte, t time.Time, _ byte) []byte {
		return append(dst, t.Format(".000000")[1:]...)
	},
	'N': func(dst []byte, t time.Time, _ byte) []byte {
		return append(dst, t.Format(".000000000")[1:]...)
	},
	'y': func(dst []byte, t time.Time, _ byte) []byte {
		return t.AppendFormat(dst, "06")
	},
	'Y': func(dst []byte, t time.Time, _ byte) []byte {
		return t.AppendFormat(dst, "2006")
	},
	'C': func(dst []byte, t time.Time, _ byte) []byte {
		c := century(t)
		if c >= 0 && c < 10 {
			dst = append(dst, '0')
		}
		return strconv.AppendInt(dst, int64(c), 10)
	},
	'U': func(dst []byte, t time.Time, flag byte) []byte {
		return appendWeekNumber(dst, t, flag, true)
	},
	'W': func(dst []byte, t time.Time, flag byte) []byte {
		return appendWeekNumber(dst, t, flag, false)
	},
	'V': func(dst []byte, t time.Time, flag byte) []byte {
		_, w := t.ISOWeek()
		return appendInt2(dst, w, flag)
	},
	'g': func(dst []byte, t time.Time, _ byte) []byte {
		y, _ := t.ISOWeek()
		return year(y).AppendFormat(dst, "06")
	},
	'G': func(dst []byte, t time.Time, _ byte) []byte {
		y, _ := t.ISOWeek()
		return year(y).AppendFormat(dst, "2006")
	},
	's': func(dst []byte, t time.Time, _ byte) []byte {
		return strconv.AppendInt(dst, t.Unix(), 10)
	},
	'Q': func(dst []byte, t time.Time, _ byte) []byte {
		return strconv.AppendInt(dst, t.UnixMilli(), 10)
	},
	'w': func(dst []byte, t time.Time, _ byte) []byte {
		return appendInt1(dst, int(t.Weekday()))
	},
	'u': func(dst []byte, t time.Time, _ byte) []byte {
		if w := t.Weekday(); w == 0 {
			return append(dst, '7')
		} else {
			return appendInt1(dst, int(w))
		}
	},
	'j': func(dst []byte, t time.Time, flag byte) []byte {
		if flag == '-' {
			return strconv.AppendInt(dst, int64(t.YearDay()), 10)
		}
		return t.AppendFormat(dst, "002")
	},
	'p': func(dst []byte, t time.Time, _ byte) []byte {
		if t.Hour() >= 12 {
			return append(dst, "PM"...)
		}
		return append(dst, "AM"...)
	},
	'P': func(dst []byte, t time.Time, _ byte) []byte {
		if t.Hour() >= 12 {
			return append(dst, "pm"...)
		}
		return append(dst, "am"...)
	},
	'Z': func(dst []byte, t time.Time, _ byte) []byte {
		name, _ := t.Zone()
		return append(dst, name...)
	},
	'z': func(dst []byte, t time.Time, flag byte) []byte {
		if flag == ':' {
			return t.AppendFormat(dst, "-07:00")
		}
		return t.AppendFormat(dst, "-0700")
	},
}

func formatMonthAbbr(dst []byte, t time.Time, _ byte) []byte {
	return append(dst, t.Month().String()[:3]...)
}
