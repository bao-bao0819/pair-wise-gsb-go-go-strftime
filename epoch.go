package strftime

import (
	"strconv"
	"time"
)

// unixToInternal mirrors the offset used by package time between the Unix
// epoch and its internal year-1 epoch. An int64 overflow of
// unixSec+unixToInternal is precisely the boundary of what time.Time can
// express.
const unixToInternal int64 = (1969*365 + 1969/4 - 1969/100 + 1969/400) * secondsPerDay

const secondsPerDay int64 = 24 * 60 * 60

const (
	maxUnixSec = 1<<63 - 1 - unixToInternal
	minUnixSec = -maxUnixSec - 1
)

type epochKind int

const (
	epochSeconds epochKind = iota
	epochMillis
)

type token struct {
	kind tokenKind
	off  int
	text string // raw format text for error reporting / literals
	spec byte
	flag byte
}

type tokenKind int

const (
	tokLiteral tokenKind = iota
	tokSpec
	tokEpoch
)

type epochFraction struct {
	width int // digits: 3 (%L), 6 (%f), 9 (%N)
	off   int
}

type epochSegment struct {
	kind     epochKind
	off      int
	fraction epochFraction
}

type epochChunk struct {
	raw    []token // tokens contributing to the chunk (for error text)
	layout string
	mask   calendarMask
}

type epochElement struct {
	epoch        *epochSegment // nil for ordinary chunks
	chunk        int           // index in epochPattern.chunks (-1 for epochs)
	epochNext    bool          // the following element is an epoch
	epochNextSeg *epochSegment // the following epoch segment
}

type epochPattern struct {
	chunks   []epochChunk
	segments []*epochSegment
	order    []epochElement
}

// tokenize splits a format into literal and directive tokens, following the
// same flag/modifier rules as parser.parse.
func tokenize(fmt string) []token {
	var tokens []token
	const (
		initial = iota
		percent
		flagged
		modified
	)

	var flag, modifier byte
	state := initial
	start := 0

	flushLiteral := func(end int) {
		if end > start {
			tokens = append(tokens, token{kind: tokLiteral, off: start, text: fmt[start:end]})
		}
	}

	for i := 0; i < len(fmt); i++ {
		b := fmt[i]
		switch state {
		default:
			if b == '%' {
				flushLiteral(i)
				state = percent
				start = i
				continue
			}

		case percent:
			if b == '-' || b == ':' {
				state = flagged
				flag = b
				continue
			}
			if b == 'E' || b == 'O' {
				state = modified
				modifier = b
				flag = 0
				continue
			}
			tokens = append(tokens, token{kind: directiveKind(b), off: start, text: fmt[start : i+1], spec: b})
			state = initial
			start = i + 1

		case flagged:
			if b == 'E' || b == 'O' {
				state = modified
				modifier = b
				continue
			}
			tokens = append(tokens, token{kind: directiveKind(b), off: start, text: fmt[start : i+1], spec: b, flag: flag})
			state = initial
			start = i + 1

		case modified:
			if okModifier(modifier, b) {
				tokens = append(tokens, token{kind: directiveKind(b), off: start, text: fmt[start : i+1], spec: b, flag: flag})
			} else {
				// Unknown modifier sequence: the parser treats it as literal.
				tokens = append(tokens, token{kind: tokLiteral, off: start, text: fmt[start : i+1]})
			}
			state = initial
			start = i + 1
		}
	}

	if state == initial {
		flushLiteral(len(fmt))
	} else {
		tokens = append(tokens, token{kind: tokLiteral, off: start, text: fmt[start:]})
	}
	return tokens
}

func directiveKind(spec byte) tokenKind {
	if spec == 's' || spec == 'Q' {
		return tokEpoch
	}
	return tokSpec
}

// hasEpoch reports whether the format contains a %s or %Q directive.
func hasEpoch(fmt string) bool {
	for _, t := range tokenize(fmt) {
		if t.kind == tokEpoch {
			return true
		}
	}
	return false
}

// compileEpoch validates the tokens and groups them into ordinary calendar
// chunks and epoch segments. A %s may be immediately followed by a '.'/','
// separator and a single %L/%f/%N directive, which then expresses the decimal
// fraction of that epoch. %Q already carries milliseconds, so attaching a
// fraction to it is a conflict.
func compileEpoch(tokens []token) (*epochPattern, error) {
	var p epochPattern
	var raw []token
	var lastEpoch *epochSegment // epoch available for fraction attachment
	var sepToken *token         // pending '.'/',' after that epoch

	addChunk := func() error {
		if len(raw) == 0 {
			return nil
		}
		layout, err := layoutTokens(raw)
		if err != nil {
			return err
		}
		p.chunks = append(p.chunks, epochChunk{
			raw:    append([]token(nil), raw...),
			layout: layout,
			mask:   calendarMaskOf(raw),
		})
		p.order = append(p.order, epochElement{chunk: len(p.chunks) - 1})
		raw = raw[:0]
		return nil
	}

	for i := 0; i < len(tokens); i++ {
		tk := tokens[i]

		// %s/%Q always starts/ends calendar chunks.
		if tk.kind == tokEpoch {
			if err := addChunk(); err != nil {
				return nil, err
			}
			kind := epochSeconds
			if tk.spec == 'Q' {
				kind = epochMillis
			}
			seg := &epochSegment{kind: kind, off: tk.off}
			p.segments = append(p.segments, seg)
			p.order = append(p.order, epochElement{epoch: seg})
			lastEpoch = seg
			sepToken = nil
			continue
		}

		// A '.'/',' literal directly after the epoch opens a pending
		// attachment, which only succeeds if the next token is %L/%f/%N.
		if tk.kind == tokLiteral && lastEpoch != nil && sepToken == nil &&
			len(tk.text) >= 1 && (tk.text[len(tk.text)-1] == '.' || tk.text[len(tk.text)-1] == ',') &&
			tk.off+len(tk.text)-1 == lastEpoch.off+len(directiveText(lastEpoch)) {
			sep := tk.text[len(tk.text)-1]
			st := token{
				kind: tokLiteral,
				off:  tk.off + len(tk.text) - 1,
				text: string(sep),
			}
			tk.text = tk.text[:len(tk.text)-1]
			if len(tk.text) > 0 {
				raw = append(raw, tk)
			}
			if err := addChunk(); err != nil {
				return nil, err
			}
			sepToken = &st
			lastEpoch = nil
			continue
		}

		if tk.kind == tokSpec && sepToken != nil &&
			(tk.spec == 'L' || tk.spec == 'f' || tk.spec == 'N') &&
			tk.off == sepToken.off+1 {
			seg := epochSegmentByOffset(p.segments, sepToken.off)
			if seg == nil {
				raw = append(raw, *sepToken, tk)
				sepToken = nil
				continue
			}
			if seg.kind == epochMillis {
				return nil, &formatError{
					message:   "%Q is already milliseconds; an attached decimal fraction conflicts with it",
					directive: tk.text,
				}
			}
			if seg.fraction.width != 0 {
				return nil, &formatError{
					message:   "duplicate decimal fraction directive for epoch " + directiveText(seg),
					directive: tk.text,
				}
			}
			switch tk.spec {
			case 'L':
				seg.fraction = epochFraction{width: 3, off: tk.off}
			case 'f':
				seg.fraction = epochFraction{width: 6, off: tk.off}
			case 'N':
				seg.fraction = epochFraction{width: 9, off: tk.off}
			}
			sepToken = nil
			continue
		}

		// A pending separator that was not consumed by a fraction directive
		// becomes an ordinary literal and closes the epoch.
		if sepToken != nil {
			raw = append(raw, *sepToken)
			sepToken = nil
		}
		raw = append(raw, tk)
		lastEpoch = nil
	}

	if sepToken != nil {
		raw = append(raw, *sepToken)
	}
	if err := addChunk(); err != nil {
		return nil, err
	}
	for i := 0; i+1 < len(p.order); i++ {
		if p.order[i].epoch == nil && p.order[i+1].epoch != nil {
			p.order[i].epochNext = true
			p.order[i].epochNextSeg = p.order[i+1].epoch
		}
	}
	return &p, nil
}

func epochSegmentByOffset(segs []*epochSegment, sepOff int) *epochSegment {
	for i := len(segs) - 1; i >= 0; i-- {
		if segs[i].off+len(directiveText(segs[i])) == sepOff {
			return segs[i]
		}
	}
	return nil
}

func directiveText(seg *epochSegment) string {
	if seg.kind == epochMillis {
		return "%Q"
	}
	return "%s"
}

// layoutTokens converts ordinary tokens to a Go layout using the same rules as
// layout(). Tokens have already been validated at parse time.
func layoutTokens(tokens []token) (string, error) {
	dst := buffer(formatText(tokens))
	var parser parser
	parser.literal = func(b byte) error {
		if '0' <= b && b <= '9' {
			return literalErr(b)
		}
		dst = append(dst, b)
		return nil
	}
	parser.format = func(spec, flag byte) error {
		if l := goLayout(spec, flag, true); l != "" {
			dst = append(dst, l...)
			return nil
		}
		switch spec {
		case 'L', 'f', 'N':
			if len(dst) > 0 && (dst[len(dst)-1] == '.' || dst[len(dst)-1] == ',') {
				switch spec {
				case 'f':
					dst = append(dst, "000000"...)
				case 'N':
					dst = append(dst, "000000000"...)
				default:
					dst = append(dst, "000"...)
				}
				return nil
			}
			return &formatError{message: "must follow '.' or ','"}
		default:
			return &formatError{}
		}
	}
	for _, tk := range tokens {
		var err error
		if tk.kind == tokLiteral {
			err = parser.literals(tk.text)
		} else {
			err = parser.format(tk.spec, tk.flag)
		}
		if err != nil {
			if fe, ok := err.(*formatError); ok && fe.directive == "" {
				fe.directive = tk.text
			}
			return "", err
		}
	}
	return string(dst), nil
}

func formatText(tokens []token) string {
	var b []byte
	for _, t := range tokens {
		b = append(b, t.text...)
	}
	return string(b)
}

// epochToTime converts a parsed epoch to a UTC time.Time using integer
// arithmetic only (no float64), reporting inputs outside the range time.Time
// can express. For negative %s values, a non-zero fraction lies before the
// integer second: -0.250 becomes -1s + 0.750s.
func epochToTime(kind epochKind, integer, nanos int64, neg bool) (time.Time, error) {
	sec := integer
	switch kind {
	case epochSeconds:
		if nanos != 0 && neg {
			sec = integer - 1
			if sec >= integer {
				return time.Time{}, &valueError{cause: "epoch seconds out of time.Time range"}
			}
			nanos = int64(time.Second) - nanos
		}
	case epochMillis:
		q, r := integer/1000, integer%1000
		if r < 0 {
			q--
			r += 1000
		}
		sec = q
		nanos = r * int64(time.Millisecond)
	}
	if sec < minUnixSec || sec > maxUnixSec {
		return time.Time{}, &valueError{cause: "epoch out of time.Time range"}
	}
	return time.Unix(sec, nanos).UTC(), nil
}

type valueError struct {
	directive string
	value     string
	where     int
	cause     string
}

func (e *valueError) Error() string {
	s := "strftime: invalid value"
	if e.directive != "" {
		s += " for " + e.directive
	}
	if e.value != "" {
		s += ": " + strconv.Quote(e.value)
	}
	s += " at input offset " + strconv.Itoa(e.where)
	if e.cause != "" {
		s += " (" + e.cause + ")"
	}
	return s
}
