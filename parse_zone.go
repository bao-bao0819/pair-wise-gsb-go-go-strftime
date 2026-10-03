package strftime

import (
	"time"
)

func parseZoneName(s *parseState, value string, _ byte) (string, error) {
	if len(value) >= 3 && value[:3] == "UTC" {
		if s.zoneNameSet && s.zoneName != "UTC" {
			return value, errConflict
		}
		s.zoneName, s.zoneNameSet = "UTC", true
		s.zone = time.UTC
		return value[3:], nil
	}

	n, ok := scanTimeZone(value)
	if !ok {
		return value, errBad
	}
	name := value[:n]
	if s.zoneNameSet && s.zoneName != name {
		return value, errConflict
	}
	s.zoneName, s.zoneNameSet = name, true
	// A name of the form GMT±h carries an explicit hour offset.
	if len(name) > 3 && name[:3] == "GMT" {
		if hr, ok := atoiSigned(name[3:]); ok {
			s.offset, s.offsetSet = hr*3600, true
			s.zoneNumericName = true
		}
	}
	return value[n:], nil
}

func parseZoneOffset(s *parseState, value string, flag byte) (string, error) {
	if flag == ':' {
		if len(value) >= 1 && value[0] == 'Z' {
			return s.setOffset(0, value[1:])
		}
		if len(value) < 6 || value[3] != ':' {
			return value, errBad
		}
		hr, ok1 := twoDigits(value[1:3])
		mm, ok2 := twoDigits(value[4:6])
		if !ok1 || !ok2 || (value[0] != '+' && value[0] != '-') || hr > 24 || mm > 60 {
			return value, errBad
		}
		offset := (hr*60 + mm) * 60
		if value[0] == '-' {
			offset = -offset
		}
		return s.setOffset(offset, value[6:])
	}

	if len(value) >= 1 && value[0] == 'Z' {
		return s.setOffset(0, value[1:])
	}
	if len(value) < 5 {
		return value, errBad
	}
	hr, ok1 := twoDigits(value[1:3])
	mm, ok2 := twoDigits(value[3:5])
	if !ok1 || !ok2 || (value[0] != '+' && value[0] != '-') || hr > 24 || mm > 60 {
		return value, errBad
	}
	offset := (hr*60 + mm) * 60
	if value[0] == '-' {
		offset = -offset
	}
	return s.setOffset(offset, value[5:])
}

func (s *parseState) setOffset(offset int, rem string) (string, error) {
	if s.offsetSet && s.offset != offset {
		return rem, errConflict
	}
	s.offset, s.offsetSet = offset, true
	return rem, nil
}

func twoDigits(s string) (int, bool) {
	if len(s) < 2 || !isASCIIDigit(s[0]) || !isASCIIDigit(s[1]) {
		return 0, false
	}
	return int(s[0]-'0')*10 + int(s[1]-'0'), true
}

// scanTimeZone mirrors time.parseTimeZone: it returns the length of a
// zone abbreviation at the start of value.
func scanTimeZone(value string) (int, bool) {
	if len(value) < 3 {
		return 0, false
	}
	// ChST and MeST are the only zones with a lower-case letter.
	if len(value) >= 4 && (value[:4] == "ChST" || value[:4] == "MeST") {
		return 4, true
	}
	// GMT may carry an hour offset.
	if value[:3] == "GMT" {
		return scanGMT(value), true
	}
	// Unnamed zones may use a +/-00 form.
	if value[0] == '+' || value[0] == '-' {
		n := scanSignedOffset(value)
		return n, n > 0
	}

	var nUpper int
	for nUpper = 0; nUpper < 6; nUpper++ {
		if nUpper >= len(value) {
			break
		}
		if c := value[nUpper]; c < 'A' || 'Z' < c {
			break
		}
	}
	switch nUpper {
	case 3:
		return 3, true
	case 4:
		if value[3] == 'T' || value[:4] == "WITA" {
			return 4, true
		}
	case 5:
		if value[4] == 'T' {
			return 5, true
		}
	}
	return 0, false
}

func scanGMT(value string) int {
	if len(value) == 3 {
		return 3
	}
	return 3 + scanSignedOffset(value[3:])
}

func scanSignedOffset(value string) int {
	if len(value) == 0 || (value[0] != '-' && value[0] != '+') {
		return 0
	}
	i := 1
	for i < len(value) && isASCIIDigit(value[i]) {
		i++
	}
	if i == 1 {
		return 0
	}
	hr := 0
	for j := 1; j < i; j++ {
		hr = hr*10 + int(value[j]-'0')
	}
	if hr > 23 {
		return 0
	}
	return i
}

// resolveZoneName locates a named zone, falling back to a fake fixed zone
// with an unknown offset, mirroring time.parse.
func resolveZoneName(name string) *time.Location {
	if name == "UTC" {
		return time.UTC
	}
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	offset := 0
	if len(name) > 3 && name[:3] == "GMT" {
		if n, ok := atoiSigned(name[3:]); ok {
			offset = n * 3600
		}
	}
	return time.FixedZone(name, offset)
}

func atoiSigned(s string) (int, bool) {
	if len(s) == 0 {
		return 0, false
	}
	neg := false
	switch s[0] {
	case '-':
		neg = true
		s = s[1:]
	case '+':
		s = s[1:]
	}
	n := 0
	if len(s) == 0 {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if !isASCIIDigit(s[i]) {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}
