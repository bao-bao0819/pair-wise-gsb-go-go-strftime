package strftime

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// unixToInternal is the number of seconds between
// January 1, year 1, and January 1, 1970.
const unixToInternal = 62135596800

// maxUnixSec is the largest number of seconds since the Unix epoch
// that a time.Time can represent.
const maxUnixSec = math.MaxInt64 - unixToInternal

// A parseError describes a failure to parse a time value.
type parseError struct {
	directive string
	value     string
	message   string
}

func (e *parseError) Error() string {
	return "strftime: cannot parse " + strconv.Quote(e.value) +
		" as " + e.directive + ": " + e.message
}

// Field identifiers used to cross-check parsed values.
const (
	fieldYear = iota
	fieldMonth
	fieldDay
	fieldYearDay
	fieldHour
	fieldMinute
	fieldSecond
	fieldNanosecond
	fieldZone
)

// A fieldDirective records which directive set which field.
type fieldDirective struct {
	field int
	text  string
}

// An epochSpec is a %s or %Q directive,
// optionally followed by a fractional second directive.
type epochSpec struct {
	directive string // "%s" or "%Q"
	millis    bool
	fracDir   string // "%L", "%f" or "%N"
	fracSep   byte   // '.' or ','
	fracPrec  int    // 3, 6 or 9
}

// An epochSegment is a fragment of a strftime format:
// either an epoch directive, or other format text.
type epochSegment struct {
	epoch     *epochSpec
	format    string
	directive bool // format contains field directives
	fields    []fieldDirective
}

// splitEpoch splits a strftime format around %s and %Q directives.
// It returns nil if the format has no epoch directives.
func splitEpoch(format string) ([]epochSegment, error) {
	var segs []epochSegment
	var cur strings.Builder
	var fields []fieldDirective
	var directive, epoch bool

	flush := func() {
		if cur.Len() > 0 {
			segs = append(segs, epochSegment{
				format:    cur.String(),
				directive: directive,
				fields:    fields,
			})
			cur.Reset()
			fields = nil
			directive = false
		}
	}

	var p parser
	p.literal = func(b byte) error {
		cur.WriteByte(b)
		return nil
	}
	p.format = func(spec, flag byte) error {
		switch spec {
		case 's', 'Q':
			if cur.Len() == 0 && len(segs) > 0 && segs[len(segs)-1].epoch != nil {
				return formatError{message: "epoch directives must be separated"}
			}
			flush()
			segs = append(segs, epochSegment{epoch: &epochSpec{
				directive: "%" + string(spec),
				millis:    spec == 'Q',
			}})
			epoch = true
			return nil

		case 'L', 'f', 'N':
			if n := len(segs); n > 0 && cur.Len() == 1 && segs[n-1].epoch != nil {
				if sep := cur.String(); sep == "." || sep == "," {
					e := segs[n-1].epoch
					switch {
					case e.millis:
						return formatError{message: "cannot add fractional seconds to %Q"}
					case e.fracDir != "":
						return formatError{message: "duplicate fractional seconds"}
					}
					e.fracDir = "%" + string(spec)
					e.fracSep = sep[0]
					switch spec {
					case 'L':
						e.fracPrec = 3
					case 'f':
						e.fracPrec = 6
					case 'N':
						e.fracPrec = 9
					}
					cur.Reset()
					return nil
				}
			}
		}

		switch spec {
		case '%':
			cur.WriteByte('%')
		case 'n':
			cur.WriteByte('\n')
		case 't':
			cur.WriteByte('\t')
		default:
			directive = true
			cur.WriteByte('%')
			text := "%"
			if flag != 0 {
				cur.WriteByte(flag)
				text += string(flag)
			}
			cur.WriteByte(spec)
			text += string(spec)
			for _, field := range specFields(spec) {
				fields = append(fields, fieldDirective{field, text})
			}
		}
		return nil
	}

	if err := p.parse(format); err != nil {
		return nil, err
	}
	flush()
	if !epoch {
		return nil, nil
	}
	return segs, nil
}

// parseEpoch parses a value using a format split around epoch directives.
func parseEpoch(format, value string, segs []epochSegment) (time.Time, error) {
	// Fast path: the entire value is a single epoch number.
	if len(segs) == 1 && segs[0].epoch != nil {
		sec, nsec, err := segs[0].epoch.parseValue(value)
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(sec, nsec).UTC(), nil
	}

	var pattern strings.Builder
	pattern.WriteByte('^')
	groups := make([]int, len(segs))
	group := 0
	for i := range segs {
		seg := &segs[i]
		switch {
		case seg.epoch != nil:
			group++
			groups[i] = group
			pattern.WriteString(`([-+]?\d+)`)
			if seg.epoch.fracDir != "" {
				group++
				pattern.WriteString(regexp.QuoteMeta(string(seg.epoch.fracSep)))
				pattern.WriteString(`(\d+)`)
			}
		case seg.directive:
			group++
			groups[i] = group
			pattern.WriteString(`((?s:.*?))`)
		default:
			pattern.WriteString(regexp.QuoteMeta(seg.format))
		}
	}
	pattern.WriteByte('$')

	re, err := regexp.Compile(pattern.String())
	if err != nil {
		return time.Time{}, err
	}
	match := re.FindStringSubmatch(value)
	if match == nil {
		return time.Time{}, &parseError{format, value, "does not match format"}
	}

	var sec, nsec int64
	var epochDir string
	var zone *time.Location
	type calendar struct {
		parsed time.Time
		fields []fieldDirective
	}
	var cals []calendar

	for i := range segs {
		seg := &segs[i]
		if seg.epoch != nil {
			frac := ""
			if seg.epoch.fracDir != "" {
				frac = match[groups[i]+1]
			}
			s, ns, err := seg.epoch.parse(match[groups[i]], frac)
			if err != nil {
				return time.Time{}, err
			}
			if epochDir != "" && (s != sec || ns != nsec) {
				return time.Time{}, &parseError{
					seg.epoch.directive, value,
					"conflicts with " + epochDir,
				}
			}
			sec, nsec, epochDir = s, ns, seg.epoch.directive
			continue
		}
		if !seg.directive {
			continue
		}
		text := match[groups[i]]
		lay, err := layout(seg.format, true)
		if err != nil {
			return time.Time{}, err
		}
		parsed, err := time.Parse(lay, text)
		if err != nil {
			return time.Time{}, &parseError{seg.format, text, err.Error()}
		}
		fields := seg.fields
		if parsed.Nanosecond() != 0 && !hasField(fields, fieldNanosecond) {
			fields = append(fields, fieldDirective{fieldNanosecond, "%S"})
		}
		for _, fd := range fields {
			if fd.field == fieldZone {
				zone = parsed.Location()
			}
		}
		cals = append(cals, calendar{parsed, fields})
	}

	loc := time.UTC
	if zone != nil {
		loc = zone
	}
	got := time.Unix(sec, nsec).In(loc)
	for _, cal := range cals {
		for _, fd := range cal.fields {
			var epoch, other int64
			switch fd.field {
			case fieldYear:
				epoch, other = int64(got.Year()), int64(cal.parsed.Year())
			case fieldMonth:
				epoch, other = int64(got.Month()), int64(cal.parsed.Month())
			case fieldDay:
				epoch, other = int64(got.Day()), int64(cal.parsed.Day())
			case fieldYearDay:
				epoch, other = int64(got.YearDay()), int64(cal.parsed.YearDay())
			case fieldHour:
				epoch, other = int64(got.Hour()), int64(cal.parsed.Hour())
			case fieldMinute:
				epoch, other = int64(got.Minute()), int64(cal.parsed.Minute())
			case fieldSecond:
				epoch, other = int64(got.Second()), int64(cal.parsed.Second())
			case fieldNanosecond:
				epoch, other = int64(got.Nanosecond()), int64(cal.parsed.Nanosecond())
			default:
				continue
			}
			if epoch != other {
				return time.Time{}, &parseError{
					fd.text, value,
					"conflicts with " + epochDir,
				}
			}
		}
	}
	return got, nil
}

// parseValue parses a value that consists of a single epoch number.
func (e *epochSpec) parseValue(value string) (int64, int64, error) {
	intStr, fracStr := value, ""
	if e.fracDir != "" {
		i := strings.LastIndexByte(value, e.fracSep)
		if i < 0 {
			return 0, 0, &parseError{e.fracDir, value, "invalid syntax"}
		}
		intStr, fracStr = value[:i], value[i+1:]
		if !isDigits(fracStr) {
			return 0, 0, &parseError{e.fracDir, fracStr, "invalid syntax"}
		}
	}
	return e.parse(intStr, fracStr)
}

// parse parses the integer and fractional parts of an epoch number.
// The sign of the integer part applies to the value as a whole,
// so -0.250 is a quarter of a second before the epoch.
func (e *epochSpec) parse(intStr, fracStr string) (int64, int64, error) {
	if e.millis {
		ms, err := parseInt(intStr, e.directive)
		if err != nil {
			return 0, 0, err
		}
		sec := ms / 1000
		nsec := ms % 1000 * int64(time.Millisecond)
		if nsec < 0 {
			sec--
			nsec += int64(time.Second)
		}
		return sec, nsec, nil
	}

	sec, err := parseInt(intStr, e.directive)
	if err != nil {
		return 0, 0, err
	}
	var frac int64
	if e.fracDir != "" {
		digits := fracStr
		if len(digits) > e.fracPrec {
			if !isZeros(digits[e.fracPrec:]) {
				return 0, 0, &parseError{e.fracDir, fracStr, "excess significant digits"}
			}
			digits = digits[:e.fracPrec]
		}
		frac, _ = strconv.ParseInt(digits+strings.Repeat("0", e.fracPrec-len(digits)), 10, 64)
		frac *= int64(time.Second) / pow10(e.fracPrec)
	}
	if frac != 0 && strings.HasPrefix(intStr, "-") {
		if sec == math.MinInt64 {
			return 0, 0, &parseError{e.directive, intStr, "value out of range"}
		}
		sec, frac = sec-1, int64(time.Second)-frac
	}
	if sec > maxUnixSec {
		return 0, 0, &parseError{e.directive, intStr, "value out of range"}
	}
	return sec, frac, nil
}

func parseInt(s, directive string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		message := "invalid syntax"
		if errors.Is(err, strconv.ErrRange) {
			message = "value out of range"
		}
		return 0, &parseError{directive, s, message}
	}
	return n, nil
}

func pow10(n int) int64 {
	p := int64(1)
	for ; n > 0; n-- {
		p *= 10
	}
	return p
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isZeros(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '0' {
			return false
		}
	}
	return true
}

func hasField(fields []fieldDirective, field int) bool {
	for _, fd := range fields {
		if fd.field == field {
			return true
		}
	}
	return false
}

func specFields(spec byte) []int {
	switch spec {
	case 'Y', 'y', 'C', 'G', 'g':
		return []int{fieldYear}
	case 'm', 'b', 'B', 'h':
		return []int{fieldMonth}
	case 'd', 'e':
		return []int{fieldDay}
	case 'j':
		return []int{fieldYearDay}
	case 'H', 'k', 'I', 'l', 'p', 'P':
		return []int{fieldHour}
	case 'M':
		return []int{fieldMinute}
	case 'S':
		return []int{fieldSecond}
	case 'L', 'f', 'N':
		return []int{fieldNanosecond}
	case 'z', 'Z':
		return []int{fieldZone}
	case 'c':
		return []int{fieldYear, fieldMonth, fieldDay, fieldHour, fieldMinute, fieldSecond}
	case 'D', 'x', 'F', 'v':
		return []int{fieldYear, fieldMonth, fieldDay}
	case 'T', 'X', 'r':
		return []int{fieldHour, fieldMinute, fieldSecond}
	case 'R':
		return []int{fieldHour, fieldMinute}
	case '+':
		return []int{fieldYear, fieldMonth, fieldDay, fieldHour, fieldMinute, fieldSecond, fieldZone}
	}
	return nil
}
