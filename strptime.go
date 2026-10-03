package strftime

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WeekYearPivot is the first year of the 100-year window
// used to expand two-digit week-based years (%g) when parsing.
//
// The window is fixed and configurable, it does not depend on the current year.
// The default is 1970, so %g expands to a week-based year in 1970-2069.
var WeekYearPivot = 1970

// hasWeekSpecifiers reports if the format contains
// ISO 8601 week-based date specifiers,
// which time.Parse cannot handle.
func hasWeekSpecifiers(format string) (found bool) {
	var parser parser
	parser.literal = func(byte) error { return nil }
	parser.format = func(spec, flag byte) error {
		switch spec {
		case 'G', 'g', 'V', 'u', 'w':
			found = true
		}
		return nil
	}
	parser.parse(format)
	return
}

// parseWeek parses value according to format,
// handling the ISO 8601 week-based date specifiers
// (%G, %g, %V, %u and %w).
func parseWeek(format, value string) (time.Time, error) {
	s := &strptime{value: value}
	if err := s.parse(format); err != nil {
		return time.Time{}, err
	}
	if s.value != "" {
		return time.Time{}, fmt.Errorf("strftime: cannot parse %q as %q: extra characters", value, format)
	}
	return s.resolve(format)
}

type strptime struct {
	value string

	year    int
	month   time.Month
	day     int
	yday    int
	wyear   int
	week    int
	weekday int // 1 (Monday) to 7 (Sunday)

	hour int
	min  int
	sec  int
	nsec int
	pm   bool

	hasYear     bool
	hasMonth    bool
	hasDay      bool
	hasYDay     bool
	hasWYear    bool
	hasWeek     bool
	hasWeekday  bool
	hasMeridiem bool

	zoneName    string
	zoneOff     int
	hasZoneName bool
	hasZoneOff  bool
}

func (s *strptime) parse(format string) error {
	var parser parser
	parser.literal = s.literalByte
	parser.format = s.directive
	return parser.parse(format)
}

func (s *strptime) directive(spec, flag byte) error {
	switch spec {
	default:
		return formatError{}

	case '%':
		return s.literalByte('%')
	case 'n':
		return s.literalByte('\n')
	case 't':
		return s.literalByte('\t')

	case 'Y':
		year, err := s.integer(true, 0)
		if err != nil {
			return err
		}
		s.year, s.hasYear = year, true
		return nil
	case 'y':
		n, err := s.fixedInt(2)
		if err != nil {
			return err
		}
		if n < 69 {
			s.year = 2000 + n
		} else {
			s.year = 1900 + n
		}
		s.hasYear = true
		return nil
	case 'm':
		month, err := s.ranged("month", 1, 12, 2)
		if err != nil {
			return err
		}
		s.month, s.hasMonth = time.Month(month), true
		return nil
	case 'd':
		day, err := s.ranged("day of the month", 1, 31, 2)
		if err != nil {
			return err
		}
		s.day, s.hasDay = day, true
		return nil
	case 'e':
		if strings.HasPrefix(s.value, " ") {
			s.value = s.value[1:]
		}
		day, err := s.ranged("day of the month", 1, 31, 2)
		if err != nil {
			return err
		}
		s.day, s.hasDay = day, true
		return nil
	case 'j':
		yday, err := s.ranged("day of the year", 1, 366, 3)
		if err != nil {
			return err
		}
		s.yday, s.hasYDay = yday, true
		return nil

	case 'G':
		wyear, err := s.integer(true, 0)
		if err != nil {
			return err
		}
		s.wyear, s.hasWYear = wyear, true
		return nil
	case 'g':
		n, err := s.fixedInt(2)
		if err != nil {
			return err
		}
		s.wyear = WeekYearPivot + mod(n-WeekYearPivot, 100)
		s.hasWYear = true
		return nil
	case 'V':
		week, err := s.ranged("week number", 1, 53, 2)
		if err != nil {
			return err
		}
		s.week, s.hasWeek = week, true
		return nil
	case 'u':
		weekday, err := s.ranged("weekday", 1, 7, 1)
		if err != nil {
			return err
		}
		return s.setWeekday(weekday)
	case 'w':
		weekday, err := s.ranged("weekday", 0, 6, 1)
		if err != nil {
			return err
		}
		if weekday == 0 {
			weekday = 7
		}
		return s.setWeekday(weekday)

	case 'H':
		hour, err := s.ranged("hour", 0, 23, 2)
		if err != nil {
			return err
		}
		s.hour = hour
		return nil
	case 'I':
		hour, err := s.ranged("hour", 1, 12, 2)
		if err != nil {
			return err
		}
		s.hour = hour
		return nil
	case 'M':
		min, err := s.ranged("minute", 0, 59, 2)
		if err != nil {
			return err
		}
		s.min = min
		return nil
	case 'S':
		sec, err := s.ranged("second", 0, 60, 2)
		if err != nil {
			return err
		}
		s.sec = sec
		return nil
	case 'L':
		nsec, err := s.fixedInt(3)
		if err != nil {
			return err
		}
		s.nsec = nsec * int(time.Millisecond)
		return nil
	case 'f':
		nsec, err := s.fixedInt(6)
		if err != nil {
			return err
		}
		s.nsec = nsec * int(time.Microsecond)
		return nil
	case 'N':
		nsec, err := s.fixedInt(9)
		if err != nil {
			return err
		}
		s.nsec = nsec
		return nil
	case 'p', 'P':
		return s.meridiem()

	case 'b', 'h':
		return s.monthName(false)
	case 'B':
		return s.monthName(true)
	case 'a', 'A':
		return s.weekdayName()
	case 'z':
		return s.zoneOffset(flag == ':')
	case 'Z':
		return s.parseZoneName()

	case 'F':
		return s.parse("%Y-%m-%d")
	case 'D', 'x':
		return s.parse("%m/%d/%y")
	case 'v':
		return s.parse("%e-%b-%Y")
	case 'r':
		return s.parse("%I:%M:%S %p")
	case 'R':
		return s.parse("%H:%M")
	case 'T', 'X':
		return s.parse("%H:%M:%S")
	case 'c':
		return s.parse("%a %b %e %H:%M:%S %Y")
	case '+':
		return s.parse("%a %b %e %H:%M:%S %Z %Y")
	}
}

// resolve combines the parsed fields into a time.Time,
// reconciling the week-based and calendar dates.
func (s *strptime) resolve(format string) (time.Time, error) {
	if s.hasYDay && (s.hasMonth || s.hasDay) {
		return time.Time{}, fmt.Errorf("strftime: format %q:"+
			" day of the year (%%j) cannot be combined with month and day", format)
	}

	calComplete := s.hasYear && (s.hasYDay || s.hasMonth && s.hasDay)

	year, month, day := s.year, s.month, s.day
	if !s.hasMonth {
		month = time.January
	}
	if !s.hasDay {
		day = 1
	}

	if s.hasWYear || s.hasWeek {
		if !s.hasWYear {
			return time.Time{}, fmt.Errorf("strftime: format %q:"+
				" missing week-based year (%%G or %%g)", format)
		}
		if !s.hasWeek {
			return time.Time{}, fmt.Errorf("strftime: format %q:"+
				" missing week number (%%V)", format)
		}
		if !s.hasWeekday {
			return time.Time{}, fmt.Errorf("strftime: format %q:"+
				" missing weekday (%%u or %%w)", format)
		}
		if weeks := isoWeeksInYear(s.wyear); s.week > weeks {
			return time.Time{}, fmt.Errorf("strftime:"+
				" week-based year %d has no week %d", s.wyear, s.week)
		}

		wy, wm, wd := isoWeekDate(s.wyear, s.week, s.weekday)
		if s.hasYear || s.hasMonth || s.hasDay || s.hasYDay {
			if !calComplete {
				return time.Time{}, fmt.Errorf("strftime: format %q:"+
					" incomplete calendar date", format)
			}
			cy, cm, cd, err := s.calendarDate()
			if err != nil {
				return time.Time{}, err
			}
			if cy != wy || cm != wm || cd != wd {
				return time.Time{}, fmt.Errorf("strftime:"+
					" calendar date %04d-%02d-%02d does not match week date %04d-W%02d-%d",
					cy, cm, cd, s.wyear, s.week, s.weekday)
			}
		}
		year, month, day = wy, wm, wd
	} else {
		if s.hasWeekday && !calComplete {
			return time.Time{}, fmt.Errorf("strftime: format %q:"+
				" weekday (%%u or %%w) requires a calendar date", format)
		}
		if s.hasYDay {
			t := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, s.yday-1)
			if t.Year() != year {
				return time.Time{}, fmt.Errorf("strftime:"+
					" day of the year %d out of range for year %d", s.yday, year)
			}
			month, day = t.Month(), t.Day()
		}
		t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		if t.Year() != year || t.Month() != month || t.Day() != day {
			return time.Time{}, fmt.Errorf("strftime:"+
				" date %04d-%02d-%02d out of range", year, month, day)
		}
		if s.hasWeekday && isoWeekday(t) != s.weekday {
			return time.Time{}, fmt.Errorf("strftime:"+
				" %04d-%02d-%02d does not fall on weekday %d", year, month, day, s.weekday)
		}
	}

	hour := s.hour
	if s.hasMeridiem {
		if s.pm && hour < 12 {
			hour += 12
		} else if !s.pm && hour == 12 {
			hour = 0
		}
	}

	loc := time.UTC
	switch {
	case s.hasZoneOff && (s.zoneOff != 0 || s.zoneName != "" && s.zoneName != "UTC"):
		loc = time.FixedZone(s.zoneName, s.zoneOff)
	case s.hasZoneName && s.zoneName != "UTC":
		loc = time.FixedZone(s.zoneName, 0)
	}
	return time.Date(year, month, day, hour, s.min, s.sec, s.nsec, loc), nil
}

// calendarDate resolves the calendar date
// from %Y, %m and %d, or %Y and %j.
func (s *strptime) calendarDate() (int, time.Month, int, error) {
	if s.hasYDay {
		t := time.Date(s.year, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, s.yday-1)
		if t.Year() != s.year {
			return 0, 0, 0, fmt.Errorf("strftime:"+
				" day of the year %d out of range for year %d", s.yday, s.year)
		}
		return t.Year(), t.Month(), t.Day(), nil
	}
	t := time.Date(s.year, s.month, s.day, 0, 0, 0, 0, time.UTC)
	if t.Year() != s.year || t.Month() != s.month || t.Day() != s.day {
		return 0, 0, 0, fmt.Errorf("strftime:"+
			" date %04d-%02d-%02d out of range", s.year, s.month, s.day)
	}
	return s.year, s.month, s.day, nil
}

func (s *strptime) setWeekday(weekday int) error {
	if s.hasWeekday && s.weekday != weekday {
		return fmt.Errorf("strftime: conflicting weekdays %d and %d", s.weekday, weekday)
	}
	s.weekday, s.hasWeekday = weekday, true
	return nil
}

func (s *strptime) literalByte(b byte) error {
	if len(s.value) == 0 || s.value[0] != b {
		return fmt.Errorf("strftime: cannot parse %q: expected %q", s.value, string(b))
	}
	s.value = s.value[1:]
	return nil
}

func (s *strptime) integer(signed bool, maxDigits int) (int, error) {
	v := s.value
	i := 0
	neg := false
	if signed && i < len(v) && (v[i] == '+' || v[i] == '-') {
		neg = v[i] == '-'
		i++
	}
	start := i
	for i < len(v) && '0' <= v[i] && v[i] <= '9' && (maxDigits == 0 || i-start < maxDigits) {
		i++
	}
	if i == start {
		return 0, fmt.Errorf("strftime: cannot parse %q: expected a number", v)
	}
	n, err := strconv.Atoi(v[start:i])
	if err != nil {
		return 0, fmt.Errorf("strftime: cannot parse %q: %v", v, err)
	}
	s.value = v[i:]
	if neg {
		n = -n
	}
	return n, nil
}

func (s *strptime) fixedInt(digits int) (int, error) {
	v := s.value
	if len(v) < digits {
		return 0, fmt.Errorf("strftime: cannot parse %q: expected %d digits", v, digits)
	}
	for i := 0; i < digits; i++ {
		if v[i] < '0' || v[i] > '9' {
			return 0, fmt.Errorf("strftime: cannot parse %q: expected %d digits", v, digits)
		}
	}
	n, _ := strconv.Atoi(v[:digits])
	s.value = v[digits:]
	return n, nil
}

func (s *strptime) ranged(name string, min, max, maxDigits int) (int, error) {
	n, err := s.integer(false, maxDigits)
	if err != nil {
		return 0, err
	}
	if n < min || n > max {
		return 0, fmt.Errorf("strftime: %s %d out of range [%d,%d]", name, n, min, max)
	}
	return n, nil
}

func (s *strptime) monthName(long bool) error {
	for m := time.January; m <= time.December; m++ {
		name := m.String()
		if !long {
			name = name[:3]
		}
		if len(s.value) >= len(name) && strings.EqualFold(s.value[:len(name)], name) {
			s.value = s.value[len(name):]
			s.month, s.hasMonth = m, true
			return nil
		}
	}
	return fmt.Errorf("strftime: cannot parse %q: expected a month name", s.value)
}

func (s *strptime) weekdayName() error {
	for d := time.Sunday; d <= time.Saturday; d++ {
		name := d.String()
		if len(s.value) >= len(name) && strings.EqualFold(s.value[:len(name)], name) {
			s.value = s.value[len(name):]
			return nil
		}
	}
	for d := time.Sunday; d <= time.Saturday; d++ {
		name := d.String()[:3]
		if len(s.value) >= len(name) && strings.EqualFold(s.value[:len(name)], name) {
			s.value = s.value[len(name):]
			return nil
		}
	}
	return fmt.Errorf("strftime: cannot parse %q: expected a weekday name", s.value)
}

func (s *strptime) meridiem() error {
	v := s.value
	switch {
	case len(v) >= 2 && strings.EqualFold(v[:2], "AM"):
		s.pm = false
	case len(v) >= 2 && strings.EqualFold(v[:2], "PM"):
		s.pm = true
	default:
		return fmt.Errorf("strftime: cannot parse %q: expected AM or PM", v)
	}
	s.value = v[2:]
	s.hasMeridiem = true
	return nil
}

func (s *strptime) zoneOffset(colon bool) error {
	if strings.HasPrefix(s.value, "Z") {
		s.value = s.value[1:]
		s.zoneOff, s.hasZoneOff = 0, true
		return nil
	}
	if s.value == "" || s.value[0] != '+' && s.value[0] != '-' {
		return fmt.Errorf("strftime: cannot parse %q: expected a time zone offset", s.value)
	}
	sign := 1
	if s.value[0] == '-' {
		sign = -1
	}
	s.value = s.value[1:]
	hour, err := s.fixedInt(2)
	if err != nil {
		return err
	}
	if colon {
		if err := s.literalByte(':'); err != nil {
			return err
		}
	}
	min, err := s.fixedInt(2)
	if err != nil {
		return err
	}
	if hour > 23 || min > 59 {
		return fmt.Errorf("strftime: time zone offset %02d:%02d out of range", hour, min)
	}
	s.zoneOff = sign * (hour*3600 + min*60)
	s.hasZoneOff = true
	return nil
}

func (s *strptime) parseZoneName() error {
	v := s.value
	i := 0
	for i < len(v) && isLetter(v[i]) {
		i++
	}
	if i == 0 {
		return fmt.Errorf("strftime: cannot parse %q: expected a time zone name", v)
	}
	s.zoneName, s.hasZoneName = v[:i], true
	s.value = v[i:]
	return nil
}

func isLetter(b byte) bool {
	return 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
}

func mod(a, n int) int {
	return (a%n + n) % n
}

func isoWeekday(t time.Time) int {
	weekday := int(t.Weekday())
	if weekday == 0 {
		return 7
	}
	return weekday
}

// isoWeeksInYear returns the number of ISO 8601 weeks
// in the given week-based year (52 or 53).
func isoWeeksInYear(year int) int {
	switch jan1 := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC).Weekday(); jan1 {
	case time.Thursday:
		return 53
	case time.Wednesday:
		if y := year; y%4 == 0 && (y%100 != 0 || y%400 == 0) {
			return 53
		}
	}
	return 52
}

// isoWeekDate returns the calendar date of the given
// ISO 8601 week-based year, week number and weekday (1 is Monday).
func isoWeekDate(year, week, weekday int) (int, time.Month, int) {
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	monday := jan4.AddDate(0, 0, -mod(int(jan4.Weekday())+6, 7))
	date := monday.AddDate(0, 0, (week-1)*7+weekday-1)
	return date.Year(), date.Month(), date.Day()
}
