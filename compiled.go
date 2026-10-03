package strftime

import (
	"strconv"
	"time"
)

// A Spec is a compiled strftime specification.
//
// It distinguishes literal text from time directives, so format literals
// may contain text such as “2006”, “Mon”, “MST”, “PM” or any UTF-8 runes
// that would otherwise be mistaken for components of a Go time layout.
//
// A Spec is safe for concurrent use by multiple goroutines.
type Spec struct {
	ops []op
}

type opCode uint8

const (
	opLiteral opCode = iota

	opYear          // %Y
	opYear2         // %y
	opMonth         // %m, %-m
	opMonthLong     // %B
	opMonthName     // %b, %h
	opDay           // %d, %-d
	opDaySpace      // %e
	opYearDay       // %j, %-j
	opHour          // %H, %-H
	opHour12        // %I, %-I
	opMinute        // %M, %-M
	opSecond        // %S, %-S
	opFraction      // %L, %f, %N
	opMeridianUpper // %p
	opMeridianLower // %P
	opZoneOffset    // %z, %:z
	opZoneName      // %Z
	opWeekdayLong   // %A
	opWeekdayName   // %a
)

// op is a single compiled element: either a literal text span,
// or one time directive. Compound directives are expanded at compile
// time, so only primitive directives remain.
type op struct {
	code   opCode
	spec   byte // directive that produced this op
	flag   byte // '-' or ':'
	offset int  // byte offset of the directive in the format string

	// Literal span (opLiteral only).
	lit []byte

	// Width of a fractional-second directive: 3 for %L, 6 for %f, 9 for %N.
	width int
}

// CompileError describes a directive that cannot be compiled.
type CompileError struct {
	// Format is the offending specification.
	Format string
	// Directive is the unsupported directive, e.g. "%C".
	Directive string
	// FormatOffset is the byte offset, within Format,
	// of the '%' that begins the offending directive.
	FormatOffset int
	// Message describes why the directive is unsupported, if known.
	Message string
}

func (e *CompileError) Error() string {
	msg := "strftime: unsupported directive: " + e.Directive
	if e.Message != "" {
		msg += " " + e.Message
	}
	return msg
}

// ParseError describes a value that cannot be parsed against a Spec.
type ParseError struct {
	// Value is the input being parsed.
	Value string
	// Directive is the format directive at which parsing failed,
	// or "" for failures not tied to a directive.
	Directive string
	// FormatOffset is the byte offset, within the format string,
	// of the directive at which parsing failed.
	FormatOffset int
	// ValueOffset is the byte offset, within Value, at which parsing failed.
	ValueOffset int
	// Message describes the failure, if known.
	Message string
}

func (e *ParseError) Error() string {
	msg := "strftime: cannot parse"
	if e.Directive != "" {
		msg += " directive " + e.Directive
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

// Compile compiles a strftime specification into a reusable Spec.
//
// The following specifiers are not supported for parsing and are
// therefore rejected:
//
//	%g %k %l %s %u %w %C %G %Q %U %V %W
//
// Unlike Parse, literal text is not translated and may contain any
// characters, including digits, “Jan”, “Mon”, “MST”, “PM”, “pm”,
// and arbitrary UTF-8 runes.
func Compile(format string) (*Spec, error) {
	c := &compiler{format: format}
	c.parser.literal = func(b byte) error {
		c.pending = append(c.pending, b)
		return nil
	}
	c.parser.format = func(spec, flag byte, offset int) error {
		c.pos = offset
		return c.directive(spec, flag, offset)
	}
	if err := c.parser.parse(format); err != nil {
		return nil, err
	}
	c.flush()
	return &Spec{ops: c.ops}, nil
}

type compiler struct {
	format  string
	parser  parser
	ops     []op
	pending []byte // literal bytes accumulated since the last directive
	pos     int    // byte offset of the current directive
}

func (c *compiler) flush() {
	if len(c.pending) == 0 {
		return
	}
	lit := make([]byte, len(c.pending))
	copy(lit, c.pending)
	c.ops = append(c.ops, op{code: opLiteral, lit: lit, offset: -1})
	c.pending = c.pending[:0]
}

// directive compiles one primitive directive.
func (c *compiler) directive(spec, flag byte, offset int) error {
	c.flush()
	// Literal escapes.
	switch spec {
	case '%', 't', 'n':
		switch spec {
		case '%':
			c.pending = append(c.pending, '%')
		case 't':
			c.pending = append(c.pending, '\t')
		case 'n':
			c.pending = append(c.pending, '\n')
		}
		return nil
	case 'L', 'f', 'N':
		if !c.precededBySeparator() {
			return &CompileError{
				Format:       c.format,
				Directive:    directiveText(spec, flag),
				FormatOffset: offset,
				Message:      "must follow '.' or ','",
			}
		}
		width := 3
		if spec == 'f' {
			width = 6
		} else if spec == 'N' {
			width = 9
		}
		c.ops = append(c.ops, op{code: opFraction, spec: spec, flag: flag, offset: offset, width: width})
		return nil
	}

	code, ok := primitiveCode(spec)
	if !ok {
		if expansion, ok := compoundExpansion(spec); ok {
			if flag != 0 {
				return c.unsupported(spec, flag, offset, "")
			}
			return c.expand(expansion, offset)
		}
		return c.unsupported(spec, flag, offset, "")
	}
	if flag != 0 && !validFlag(code, flag) {
		return c.unsupported(spec, flag, offset, "")
	}
	c.ops = append(c.ops, op{code: code, spec: spec, flag: flag, offset: offset})
	return nil
}

// compoundExpansion returns the primitive expansion of a combination directive.
func compoundExpansion(spec byte) (string, bool) {
	switch spec {
	case 'D', 'x':
		return "%m/%d/%y", true
	case 'F':
		return "%Y-%m-%d", true
	case 'T', 'X':
		return "%H:%M:%S", true
	case 'R':
		return "%H:%M", true
	case 'r':
		return "%I:%M:%S %p", true
	case 'v':
		return "%e-%b-%Y", true
	case 'c':
		return "%a %b %e %T %Y", true
	case '+':
		return "%a %b %e %H:%M:%S %Z %Y", true
	}
	return "", false
}

// expand compiles a combination directive's primitive expansion,
// attributing every resulting element to the parent directive's offset.
func (c *compiler) expand(expansion string, offset int) error {
	for i := 0; i < len(expansion); i++ {
		b := expansion[i]
		if b != '%' {
			c.pending = append(c.pending, b)
			continue
		}
		i++
		if i >= len(expansion) {
			c.pending = append(c.pending, '%')
			break
		}
		sub := expansion[i]
		if err := c.directive(sub, 0, offset); err != nil {
			return err
		}
	}
	return nil
}

func (c *compiler) unsupported(spec, flag byte, offset int, message string) error {
	return &CompileError{
		Format:       c.format,
		Directive:    directiveText(spec, flag),
		FormatOffset: offset,
		Message:      message,
	}
}

func directiveText(spec, flag byte) string {
	if flag != 0 {
		return "%" + string(flag) + string(spec)
	}
	return "%" + string(spec)
}

// precededBySeparator reports whether the directive is written directly
// after a '.' or ',' separator: the format byte immediately before the
// directive's '%' must be the separator.
func (c *compiler) precededBySeparator() bool {
	if c.pos <= 0 {
		return false
	}
	prev := c.format[c.pos-1]
	return prev == '.' || prev == ','
}

func primitiveCode(spec byte) (opCode, bool) {
	switch spec {
	case 'Y':
		return opYear, true
	case 'y':
		return opYear2, true
	case 'm':
		return opMonth, true
	case 'B':
		return opMonthLong, true
	case 'b', 'h':
		return opMonthName, true
	case 'd':
		return opDay, true
	case 'e':
		return opDaySpace, true
	case 'j':
		return opYearDay, true
	case 'H':
		return opHour, true
	case 'I':
		return opHour12, true
	case 'M':
		return opMinute, true
	case 'S':
		return opSecond, true
	case 'p':
		return opMeridianUpper, true
	case 'P':
		return opMeridianLower, true
	case 'z':
		return opZoneOffset, true
	case 'Z':
		return opZoneName, true
	case 'A':
		return opWeekdayLong, true
	case 'a':
		return opWeekdayName, true
	}
	return 0, false
}

func validFlag(code opCode, flag byte) bool {
	switch flag {
	case '-':
		switch code {
		case opMonth, opDay, opYearDay, opHour, opHour12, opMinute, opSecond:
			return true
		}
	case ':':
		return code == opZoneOffset
	}
	return false
}

// Format returns the textual representation of t according to the Spec.
func (s *Spec) Format(t time.Time) string {
	return string(s.AppendFormat(nil, t))
}

// AppendFormat is like Format but appends the result to dst.
func (s *Spec) AppendFormat(dst []byte, t time.Time) []byte {
	for i := range s.ops {
		dst = s.ops[i].appendFormat(dst, t)
	}
	return dst
}

// Parse parses value according to the Spec.
// Trailing input after a successful match is an error.
func (s *Spec) Parse(value string) (time.Time, error) {
	return s.ParseIn(value, time.UTC)
}

// ParseIn is like Parse but interprets a value without time zone
// information in the given location.
func (s *Spec) ParseIn(value string, loc *time.Location) (time.Time, error) {
	p := valueParser{spec: s, value: value, loc: loc}
	if err := p.parse(); err != nil {
		return time.Time{}, err
	}
	return p.time()
}

// appendSpecifier appends one primitive directive to dst.
// It returns false for directives it does not recognize, leaving legacy
// callers free to emit the directive verbatim.
func appendSpecifier(dst []byte, spec, flag byte, t time.Time) ([]byte, bool) {
	switch spec {
	case 'A':
		dst = append(dst, t.Weekday().String()...)
	case 'a':
		dst = append(dst, t.Weekday().String()[:3]...)
	case 'B':
		dst = append(dst, t.Month().String()...)
	case 'b', 'h':
		dst = append(dst, t.Month().String()[:3]...)
	case 'm':
		dst = appendInt2(dst, int(t.Month()), flag)
	case 'd':
		dst = appendInt2(dst, int(t.Day()), flag)
	case 'e':
		dst = appendInt2(dst, int(t.Day()), ' ')
	case 'I':
		dst = append12Hour(dst, t, flag)
	case 'l':
		dst = append12Hour(dst, t, ' ')
	case 'H':
		dst = appendInt2(dst, t.Hour(), flag)
	case 'k':
		dst = appendInt2(dst, t.Hour(), ' ')
	case 'M':
		dst = appendInt2(dst, t.Minute(), flag)
	case 'S':
		dst = appendInt2(dst, t.Second(), flag)
	case 'L':
		dst = append(dst, t.Format(".000")[1:]...)
	case 'f':
		dst = append(dst, t.Format(".000000")[1:]...)
	case 'N':
		dst = append(dst, t.Format(".000000000")[1:]...)
	case 'y':
		dst = t.AppendFormat(dst, "06")
	case 'Y':
		dst = t.AppendFormat(dst, "2006")
	case 'C':
		century := century(t)
		if century >= 0 && century < 10 {
			dst = append(dst, '0')
		}
		dst = strconv.AppendInt(dst, int64(century), 10)
	case 'U':
		dst = appendWeekNumber(dst, t, flag, true)
	case 'W':
		dst = appendWeekNumber(dst, t, flag, false)
	case 'V':
		_, w := t.ISOWeek()
		dst = appendInt2(dst, w, flag)
	case 'g':
		y, _ := t.ISOWeek()
		dst = year(y).AppendFormat(dst, "06")
	case 'G':
		y, _ := t.ISOWeek()
		dst = year(y).AppendFormat(dst, "2006")
	case 's':
		dst = strconv.AppendInt(dst, t.Unix(), 10)
	case 'Q':
		dst = strconv.AppendInt(dst, t.UnixMilli(), 10)
	case 'w':
		dst = appendInt1(dst, int(t.Weekday()))
	case 'u':
		if w := t.Weekday(); w == 0 {
			dst = append(dst, '7')
		} else {
			dst = appendInt1(dst, int(w))
		}
	case 'j':
		if flag == '-' {
			dst = strconv.AppendInt(dst, int64(t.YearDay()), 10)
		} else {
			dst = t.AppendFormat(dst, "002")
		}
	default:
		if layout := goLayout(spec, flag, false); layout != "" {
			dst = t.AppendFormat(dst, layout)
		} else {
			return dst, false
		}
	}
	return dst, true
}

func (o op) appendFormat(dst []byte, t time.Time) []byte {
	if o.code == opLiteral {
		return append(dst, o.lit...)
	}
	dst, _ = appendSpecifier(dst, o.spec, o.flag, t)
	return dst
}
