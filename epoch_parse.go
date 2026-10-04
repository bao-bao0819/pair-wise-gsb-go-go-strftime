package strftime

import (
	"strconv"
	"time"
)

type parsedChunk struct {
	tokens []token
	time   time.Time
	offset int // offset, in the input value, where this chunk started
	end    int // offset, in the input value, where this chunk ended
}

// parseEpoch implements Parse for formats containing %s/%Q. Parsing never
// round-trips through float64; epoch arithmetic is exact integer nanoseconds.
func parseEpoch(p *epochPattern, value string) (time.Time, error) {
	var (
		chunks []parsedChunk
		masks  []calendarMask
		epoch  time.Time
		haveEp bool
		off    int
	)

	for _, el := range p.order {
		if el.epoch != nil {
			t, n, err := consumeEpoch(value, off, el.epoch)
			if err != nil {
				return time.Time{}, err
			}
			if haveEp && !t.Equal(epoch) {
				return time.Time{}, &valueError{
					directive: directiveText(el.epoch),
					value:     value[off : off+n],
					where:     off,
					cause:     "epoch fields do not represent the same instant",
				}
			}
			epoch, haveEp = t, true
			off += n
			continue
		}

		chunk := p.chunks[el.chunk]
		parsed, newOff, err := consumeChunk(chunk, value, off, el.epochNext, el.epochNextSeg)
		if err != nil {
			return time.Time{}, err
		}
		chunks = append(chunks, parsedChunk{
			tokens: chunk.raw,
			time:   parsed,
			offset: off,
			end:    newOff,
		})
		masks = append(masks, chunk.mask)
		off = newOff
	}

	if off != len(value) {
		return time.Time{}, &valueError{
			value: value[off:],
			where: off,
			cause: "extra text",
		}
	}
	if !haveEp {
		return time.Time{}, &valueError{cause: "no epoch directive"}
	}

	if err := matchCalendar(value, epoch, chunks, masks); err != nil {
		return time.Time{}, err
	}

	loc := time.UTC
	for i := len(chunks) - 1; i >= 0; i-- {
		if masks[i].zone {
			loc = chunks[i].time.Location()
			break
		}
	}
	return epoch.In(loc), nil
}

// consumeEpoch scans an epoch number, its optional attached decimal fraction,
// and converts the result to a UTC time.Time.
func consumeEpoch(value string, start int, seg *epochSegment) (time.Time, int, error) {
	s := value[start:]
	integer, n, neg, err := scanEpochInteger(s, start, seg)
	if err != nil {
		return time.Time{}, 0, err
	}

	var nanos int64
	consumed := n
	if seg.fraction.width != 0 {
		if n >= len(s) || (s[n] != '.' && s[n] != ',') {
			return time.Time{}, 0, &valueError{
				directive: directiveText(seg),
				where:     start + n,
				cause:     "missing decimal separator before fraction",
			}
		}
		ns, fn, ferr := scanEpochFraction(s[n+1:], start+n+1, seg)
		if ferr != nil {
			return time.Time{}, 0, ferr
		}
		nanos = ns
		consumed = n + 1 + fn
	}

	t, terr := epochToTime(seg.kind, integer, nanos, neg)
	if terr != nil {
		if ve, ok := terr.(*valueError); ok {
			ve.directive = directiveText(seg)
			ve.where = start
			ve.value = value[start : start+consumed]
		}
		return time.Time{}, 0, terr
	}
	return t, consumed, nil
}

// consumeChunk parses one ordinary chunk from value[start:]. When an epoch
// follows, progressively longer prefixes are attempted; the first prefix that
// parses exactly and is followed by a scannable epoch wins.
func consumeChunk(chunk epochChunk, value string, start int, epochNext bool, seg *epochSegment) (time.Time, int, error) {
	rest := value[start:]
	if !epochNext {
		t, err := time.Parse(chunk.layout, rest)
		if err != nil {
			return time.Time{}, 0, wrapParseError(chunk, rest, start, err)
		}
		return t, start + len(rest), nil
	}

	var best time.Time
	bestEnd := -1
	for end := 1; end <= len(rest); end++ {
		prefix := rest[:end]
		t, err := time.Parse(chunk.layout, prefix)
		if err != nil {
			continue
		}
		if _, _, _, eerr := scanEpochInteger(rest[end:], start+end, seg); eerr == nil {
			// Go layout matching is exact; the longest prefix that still
			// parses is the greedy match, so trailing literals (e.g. a
			// separating space) are consumed by the chunk.
			best, bestEnd = t, end
		}
	}
	if bestEnd >= 0 {
		return best, start + bestEnd, nil
	}
	return time.Time{}, 0, wrapParseError(chunk, rest, start, &time.ParseError{
		Value:     rest,
		Layout:    chunk.layout,
		ValueElem: rest,
		Message:   ": cannot find boundary before epoch",
	})
}

func wrapParseError(chunk epochChunk, rest string, start int, err error) error {
	where := start
	value := rest
	cause := err.Error()
	if pe, ok := err.(*time.ParseError); ok {
		if pe.ValueElem != "" {
			value = pe.ValueElem
			if idx := indexAt(rest, pe.ValueElem); idx >= 0 {
				where = start + idx
			}
		}
		cause = pe.Message
	}
	return &valueError{
		directive: chunkLabel(chunk.raw),
		value:     value,
		where:     where,
		cause:     cause,
	}
}

func chunkLabel(tokens []token) string {
	for _, tk := range tokens {
		if tk.kind != tokLiteral {
			return tk.text
		}
	}
	if len(tokens) > 0 {
		return strconv.Quote(tokens[0].text)
	}
	return ""
}

func indexAt(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}

// scanEpochInteger consumes an optional sign and one or more digits, returning
// the integer, bytes consumed and whether the sign was negative.
func scanEpochInteger(value string, off int, seg *epochSegment) (integer int64, n int, neg bool, err error) {
	if n < len(value) && (value[n] == '+' || value[n] == '-') {
		if value[n] == '-' {
			neg = true
		}
		n++
	}
	digits := n
	for digits < len(value) && isDigit(value[digits]) {
		digits++
	}
	if digits == n {
		return 0, 0, false, &valueError{
			directive: directiveText(seg),
			where:     off,
			cause:     "missing epoch digits",
		}
	}
	integer, convErr := strconv.ParseInt(value[:digits], 10, 64)
	if convErr != nil {
		return 0, digits, false, &valueError{
			directive: directiveText(seg),
			where:     off,
			value:     value[:digits],
			cause:     convErr.Error(),
		}
	}
	return integer, digits, neg, nil
}

// scanEpochFraction consumes decimal fraction digits and converts them to
// nanoseconds using integer place-value arithmetic. Fewer digits are padded by
// place value; more digits than the directive width are rejected rather than
// silently truncated.
func scanEpochFraction(value string, off int, seg *epochSegment) (nanos int64, n int, err error) {
	width := seg.fraction.width
	for n < len(value) && isDigit(value[n]) {
		n++
	}
	if n == 0 {
		return 0, 0, &valueError{
			directive: fractionDirective(seg),
			where:     off,
			cause:     "missing fractional digits",
		}
	}
	if n > width {
		return 0, n, &valueError{
			directive: fractionDirective(seg),
			where:     off,
			value:     value[:n],
			cause:     "too many fractional digits (would lose precision)",
		}
	}
	integer, convErr := strconv.ParseInt(value[:n], 10, 64)
	if convErr != nil {
		return 0, n, &valueError{
			directive: fractionDirective(seg),
			where:     off,
			value:     value[:n],
			cause:     convErr.Error(),
		}
	}
	// Interpret the digits as the first n places of a `width`-digit fraction
	// (pad missing places with zeros), then convert to nanoseconds.
	scaled := integer
	for i := 0; i < width-n; i++ {
		scaled *= 10
	}
	for i := 0; i < 9-width; i++ {
		scaled *= 10
	}
	return scaled, n, nil
}

func fractionDirective(seg *epochSegment) string {
	switch seg.fraction.width {
	case 3:
		return "%L"
	case 6:
		return "%f"
	case 9:
		return "%N"
	}
	return ""
}

func isDigit(b byte) bool { return '0' <= b && b <= '9' }
