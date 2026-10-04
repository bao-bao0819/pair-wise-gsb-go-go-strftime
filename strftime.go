package strftime

import (
	"bytes"
	"time"
)

// Format returns a textual representation of the time value
// formatted according to the strftime format specification.
func Format(fmt string, t time.Time) string {
	return englishLocale.Format(fmt, t)
}

// AppendFormat is like Format, but appends the textual representation
// to dst and returns the extended buffer.
func AppendFormat(dst []byte, fmt string, t time.Time) []byte {
	return englishLocale.AppendFormat(dst, fmt, t)
}

// Parse converts a textual representation of time to the time value it represents
// according to the strptime format specification.
//
// The following specifiers are not supported for parsing:
//
//	%g %k %l %s %u %w %C %G %Q %U %V %W
//
// You must also avoid digits and these letter sequences
// in fmt literals:
//
//	Jan Mon MST PM pm
func Parse(fmt, value string) (time.Time, error) {
	pattern, err := layout(fmt, true)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(pattern, value)
}

// Layout converts a strftime format specification
// to a Go time pattern specification.
//
// The following specifiers are not supported by Go patterns:
//
//	%f %g %k %l %s %u %w %C %G %L %N %Q %U %V %W
//
// You must also avoid digits and these letter sequences
// in fmt literals:
//
//	Jan Mon MST PM pm
func Layout(fmt string) (string, error) {
	return layout(fmt, false)
}

func layout(fmt string, parsing bool) (string, error) {
	dst := buffer(fmt)
	var parser parser

	parser.literal = func(b byte) error {
		if '0' <= b && b <= '9' {
			return literalErr(b)
		}
		dst = append(dst, b)
		if b == 'M' || b == 'T' || b == 'm' || b == 'n' {
			switch {
			case bytes.HasSuffix(dst, []byte("Jan")):
				return literalErr("Jan")
			case bytes.HasSuffix(dst, []byte("Mon")):
				return literalErr("Mon")
			case bytes.HasSuffix(dst, []byte("MST")):
				return literalErr("MST")
			case bytes.HasSuffix(dst, []byte("PM")):
				return literalErr("PM")
			case bytes.HasSuffix(dst, []byte("pm")):
				return literalErr("pm")
			}
		}
		return nil
	}

	parser.format = func(spec, flag byte) error {
		if layout := goLayout(spec, flag, parsing); layout != "" {
			dst = append(dst, layout...)
			return nil
		}

		switch spec {
		default:
			return formatError{}

		case 'L', 'f', 'N':
			if bytes.HasSuffix(dst, []byte(".")) || bytes.HasSuffix(dst, []byte(",")) {
				switch spec {
				default:
					dst = append(dst, "000"...)
				case 'f':
					dst = append(dst, "000000"...)
				case 'N':
					dst = append(dst, "000000000"...)
				}
				return nil
			}
			return formatError{message: "must follow '.' or ','"}
		}
	}

	if err := parser.parse(fmt); err != nil {
		return "", err
	}
	return string(dst), nil
}

// UTS35 converts a strftime format specification
// to a Unicode Technical Standard #35 Date Format Pattern.
//
// The following specifiers are not supported by UTS35:
//
//	%e %k %l %u %w %C %P %U %W
func UTS35(fmt string) (string, error) {
	const quote = '\''
	var quoted bool
	dst := buffer(fmt)

	var parser parser

	parser.literal = func(b byte) error {
		if b == quote {
			dst = append(dst, quote, quote)
			return nil
		}
		if !quoted && ('a' <= b && b <= 'z' || 'A' <= b && b <= 'Z') {
			dst = append(dst, quote)
			quoted = true
		}
		dst = append(dst, b)
		return nil
	}

	parser.format = func(spec, flag byte) error {
		if quoted {
			dst = append(dst, quote)
			quoted = false
		}
		if pattern := uts35Pattern(spec, flag); pattern != "" {
			dst = append(dst, pattern...)
			return nil
		}
		return formatError{}
	}

	if err := parser.parse(fmt); err != nil {
		return "", err
	}
	if quoted {
		dst = append(dst, quote)
	}
	return string(dst), nil
}

func buffer(format string) (buf []byte) {
	const bufSize = 64
	max := len(format) + 10
	if max < bufSize {
		var b [bufSize]byte
		buf = b[:0]
	} else {
		buf = make([]byte, 0, max)
	}
	return
}

func year(y int) time.Time {
	return time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC)
}

func century(t time.Time) int {
	y := t.Year()
	c := y / 100
	if y%100 < 0 {
		c--
	}
	return c
}

func appendWeekNumber(dst []byte, t time.Time, flag byte, sunday bool) []byte {
	offset := int(t.Weekday())
	if sunday {
		offset = 6 - offset
	} else if offset != 0 {
		offset = 7 - offset
	}
	return appendInt2(dst, (t.YearDay()+offset)/7, flag)
}

func append12Hour(dst []byte, t time.Time, flag byte) []byte {
	h := t.Hour()
	if h == 0 {
		h = 12
	} else if h > 12 {
		h -= 12
	}
	return appendInt2(dst, h, flag)
}

func appendInt1(dst []byte, i int) []byte {
	return append(dst, byte('0'+i))
}

func appendInt2(dst []byte, i int, flag byte) []byte {
	if flag == 0 || i >= 10 {
		return append(dst, smallsString[i*2:i*2+2]...)
	}
	if flag == ' ' {
		dst = append(dst, flag)
	}
	return appendInt1(dst, i)
}

const smallsString = "" +
	"00010203040506070809" +
	"10111213141516171819" +
	"20212223242526272829" +
	"30313233343536373839" +
	"40414243444546474849" +
	"50515253545556575859" +
	"60616263646566676869" +
	"70717273747576777879" +
	"80818283848586878889" +
	"90919293949596979899"
