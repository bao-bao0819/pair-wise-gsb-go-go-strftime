package strftime

import (
	"strconv"
	"strings"
	"time"
)

// calendarMask records which calendar fields a chunk contributes. Fields are
// compared individually so that an epoch plus e.g. %Y/%m/%d never lets a later
// field silently overwrite an earlier one.
type calendarMask struct {
	year, month, day bool
	hour, minute     bool
	second, nanos    bool
	zone, weekday    bool
	fracWidth        int // 3 (%L), 6 (%f), 9 (%N), or 0
}

// calendarMaskOf derives the field mask of a calendar chunk from its tokens.
func calendarMaskOf(tokens []token) calendarMask {
	var m calendarMask
	for _, tk := range tokens {
		if tk.kind != tokSpec {
			continue
		}
		switch tk.spec {
		case 'Y', 'y':
			m.year = true
		case 'C':
			m.year = true
		case 'm', 'B', 'b', 'h':
			m.month = true
		case 'd', 'e':
			m.day = true
		case 'j':
			// Day-of-year implies a complete date.
			m.year = true
			m.month = true
			m.day = true
		case 'H', 'k', 'I', 'l':
			m.hour = true
		case 'M':
			m.minute = true
		case 'S':
			m.second = true
		case 'L', 'f', 'N':
			m.nanos = true
			switch tk.spec {
			case 'L':
				m.fracWidth = 3
			case 'f':
				m.fracWidth = 6
			case 'N':
				m.fracWidth = 9
			}
		case 'z', 'Z':
			m.zone = true
		case 'A', 'a', 'u', 'w':
			m.weekday = true
		case '+':
			m.year, m.month, m.day = true, true, true
			m.hour, m.minute, m.second = true, true, true
			m.zone, m.weekday = true, true
		case 'c':
			m.year, m.month, m.day = true, true, true
			m.hour, m.minute, m.second = true, true, true
			m.weekday = true
		case 'v':
			m.year, m.month, m.day = true, true, true
		case 'F':
			m.year, m.month, m.day = true, true, true
		case 'D', 'x':
			m.year, m.month, m.day = true, true, true
		case 'r':
			m.hour, m.minute, m.second = true, true, true
		case 'T', 'X':
			m.hour, m.minute, m.second = true, true, true
		case 'R':
			m.hour, m.minute = true, true
		case 'G', 'V', 'U', 'W':
			// Week-based fields are unsupported for parsing via goLayout, but
			// be conservative: they do not pin a unique date.
		}
	}
	return m
}

// matchCalendar verifies that every field contributed by chunks describes the
// epoch instant. Comparison happens in each chunk's own location: a zone
// shifts the displayed wall fields but not the instant.
func matchCalendar(value string, epoch time.Time, chunks []parsedChunk, masks []calendarMask) error {
	for i, pc := range chunks {
		loc := pc.time.Location()
		if loc == nil {
			loc = time.UTC
		}
		want := epoch.In(loc)
		got := pc.time
		m := masks[i]

		mismatch := func(directive, field string, gotVal, wantVal int) error {
			return &valueError{
				directive: directive,
				value:     strconv.Itoa(gotVal),
				where:     pc.offset,
				cause: field + " does not match epoch " +
					epoch.Format(time.RFC3339Nano) +
					" (want " + strconv.Itoa(wantVal) + ")",
			}
		}

		if m.year && got.Year() != want.Year() {
			return mismatch(fieldDirective(chunks[i].tokens, 'Y'), "year", got.Year(), want.Year())
		}
		if m.month && int(got.Month()) != int(want.Month()) {
			return mismatch(fieldDirective(chunks[i].tokens, 'm'), "month", int(got.Month()), int(want.Month()))
		}
		if m.day && got.Day() != want.Day() {
			return mismatch(fieldDirective(chunks[i].tokens, 'd'), "day", got.Day(), want.Day())
		}
		if m.weekday {
			// Standalone time.Parse weekday handling is unreliable, so derive
			// it directly from the rendered name.
			text := value[pc.offset:pc.end]
			if wd, ok := parseWeekday(chunks[i].tokens, text); ok {
				if wd != want.Weekday() {
					return mismatch(fieldDirective(chunks[i].tokens, 'A'), "weekday", int(wd), int(want.Weekday()))
				}
			}
		}
		if m.hour && got.Hour() != want.Hour() {
			return mismatch(fieldDirective(chunks[i].tokens, 'H'), "hour", got.Hour(), want.Hour())
		}
		if m.minute && got.Minute() != want.Minute() {
			return mismatch(fieldDirective(chunks[i].tokens, 'M'), "minute", got.Minute(), want.Minute())
		}
		if m.second && got.Second() != want.Second() {
			return mismatch(fieldDirective(chunks[i].tokens, 'S'), "second", got.Second(), want.Second())
		}
		if m.nanos && got.Nanosecond() != want.Nanosecond() {
			// A chunk using %L/%f only asserts that precision: compare at the
			// width the format actually provides, not at nanoseconds.
			width := m.fracWidth
			if width == 0 {
				width = 9
			}
			gf := truncateFraction(got.Nanosecond(), width)
			wf := truncateFraction(want.Nanosecond(), width)
			if gf != wf {
				fracSpec := byte('N')
				switch width {
				case 3:
					fracSpec = 'L'
				case 6:
					fracSpec = 'f'
				}
				return mismatch(fieldDirective(chunks[i].tokens, fracSpec), "fraction", gf, wf)
			}
		}
	}
	return nil
}

func truncateFraction(nanos, width int) int {
	div := 1
	for i := 0; i < 9-width; i++ {
		div *= 10
	}
	return nanos / div
}

// parseWeekday recovers the weekday from a chunk containing %A/%a, because a
// standalone time.Parse returns an off-by-one weekday when no date is given.
func parseWeekday(tokens []token, value string) (time.Weekday, bool) {
	for _, tk := range tokens {
		if tk.kind != tokSpec || (tk.spec != 'A' && tk.spec != 'a') {
			continue
		}
		long := tk.spec == 'A'
		// The weekday token appears in the rendered value; find it by name.
		v := value
		for _, name := range weekdayNames(long) {
			if strings.Contains(v, name) {
				return weekdayIndex(name, long), true
			}
		}
	}
	return 0, false
}

func weekdayNames(long bool) []string {
	names := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	if long {
		return names
	}
	short := make([]string, len(names))
	for i, n := range names {
		short[i] = n[:3]
	}
	return short
}

func weekdayIndex(name string, long bool) time.Weekday {
	for i, n := range weekdayNames(long) {
		if n == name {
			return time.Weekday(i)
		}
	}
	return 0
}

// fieldDirective returns the raw directive text for error reporting, falling
// back to a synthetic %spec.
func fieldDirective(tokens []token, spec byte) string {
	for _, tk := range tokens {
		if tk.kind == tokSpec && tk.spec == spec {
			return tk.text
		}
	}
	return "%" + string(spec)
}
