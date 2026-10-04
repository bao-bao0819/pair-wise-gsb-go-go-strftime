package strftime_test

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ncruces/go-strftime"
)

func TestParse_Unix(t *testing.T) {
	tests := []struct {
		format string
		value  string
		want   time.Time
	}{
		// Around zero.
		{"%s", "0", time.Unix(0, 0).UTC()},
		{"%s", "-0", time.Unix(0, 0).UTC()},
		{"%s", "+0", time.Unix(0, 0).UTC()},
		{"%s", "1", time.Unix(1, 0).UTC()},
		{"%s", "-1", time.Unix(-1, 0).UTC()},
		{"%s", "1249625104", time.Unix(1249625104, 0).UTC()},
		{"%s", "-1249625104", time.Unix(-1249625104, 0).UTC()},

		// Leading zeros and other numeric forms.
		{"%s", "007", time.Unix(7, 0).UTC()},
		{"%s", "-007", time.Unix(-7, 0).UTC()},
		{"%s", "+7", time.Unix(7, 0).UTC()},
		{"%Q", "001234", time.Unix(1, 234000000).UTC()},

		// Fractional seconds: the sign applies to the whole value.
		{"%s.%N", "0.000000001", time.Unix(0, 1).UTC()},
		{"%s.%N", "-0.000000001", time.Unix(-1, 999999999).UTC()},
		{"%s.%L", "0.250", time.Unix(0, 250000000).UTC()},
		{"%s.%L", "-0.250", time.Unix(-1, 750000000).UTC()},
		{"%s.%f", "-0.000250", time.Unix(-1, 999750000).UTC()},
		{"%s,%N", "1,5", time.Unix(1, 500000000).UTC()},
		{"%s.%N", "1234567890.123456789", time.Unix(1234567890, 123456789).UTC()},
		{"%s.%N", "-1234567890.123456789", time.Unix(-1234567891, 876543211).UTC()},

		// Short fractions are padded by decimal place value.
		{"%s.%L", "1.5", time.Unix(1, 500000000).UTC()},
		{"%s.%f", "1.25", time.Unix(1, 250000000).UTC()},
		{"%s.%N", "1.00000025", time.Unix(1, 250).UTC()},
		{"%s.%L", "1.5000", time.Unix(1, 500000000).UTC()},

		// Milliseconds.
		{"%Q", "0", time.Unix(0, 0).UTC()},
		{"%Q", "123456789", time.Unix(123456, 789000000).UTC()},
		{"%Q", "-123456789", time.Unix(-123457, 211000000).UTC()},
		{"%Q", "-250", time.Unix(-1, 750000000).UTC()},

		// Near int64 boundaries.
		{"%s", "9223371974719179007", time.Unix(9223371974719179007, 0).UTC()},
		{"%s", "-9223372036854775808", time.Unix(math.MinInt64, 0).UTC()},
		{"%s.%N", "9223371974719179007.999999999", time.Unix(9223371974719179007, 999999999).UTC()},
		{"%Q", "9223372036854775807", time.Unix(9223372036854775, 807000000).UTC()},
		{"%Q", "-9223372036854775808", time.Unix(-9223372036854776, 192000000).UTC()},
	}
	for _, test := range tests {
		got, err := strftime.Parse(test.format, test.value)
		if err != nil {
			t.Errorf("Parse(%q, %q) error: %v", test.format, test.value, err)
		} else if !got.Equal(test.want) {
			t.Errorf("Parse(%q, %q) = %v, want %v", test.format, test.value, got, test.want)
		}
	}
}

func TestParse_UnixError(t *testing.T) {
	tests := []struct {
		format string
		value  string
		want   string
	}{
		{"%s", "", "%s"},
		{"%s", "abc", "%s"},
		{"%s", "1.5", "%s"},
		{"%s", "99999999999999999999", "out of range"},
		{"%s", "9223372036854775808", "out of range"},
		{"%s", "9223371974719179008", "out of range"},
		{"%s", "-9223372036854775809", "out of range"},
		{"%s.%N", "-9223372036854775808.1", "out of range"},
		{"%Q", "9223372036854775808", "out of range"},

		{"%s.%N", "1", "%N"},
		{"%s.%N", "1.", "%N"},
		{"%s.%N", "1.x", "%N"},
		{"%s.%L", "1.5005", "%L"},
		{"%s.%N", "1.0000000005", "%N"},

		{"%Q.%N", "1.5", "%Q"},
		{"%Q.%f", "1.5", "%Q"},
		{"%s.%N.%L", "1.5.5", "duplicate"},
		{"%s%s", "11", "separated"},

		{"%s %s", "abc def", "does not match"},
		{"%Y-%m-%d %s", "not-a-date 123", "%Y"},
	}
	for _, test := range tests {
		got, err := strftime.Parse(test.format, test.value)
		if err == nil {
			t.Errorf("Parse(%q, %q) expected error", test.format, test.value)
		} else if !strings.Contains(err.Error(), test.want) {
			t.Errorf("Parse(%q, %q) = %q, want %q", test.format, test.value, err, test.want)
		}
		if !got.IsZero() {
			t.Errorf("Parse(%q, %q) = %v, want zero time", test.format, test.value, got)
		}
	}
}

func TestParse_UnixConsistent(t *testing.T) {
	sec := reference.Unix()
	ms := sec * 1000
	whole := time.Unix(sec, 0).UTC()

	tests := []struct {
		format string
		value  string
		want   time.Time
	}{
		{"%s %s", fmt.Sprintf("%d %d", sec, sec), whole},
		{"%s %Q", fmt.Sprintf("%d %d", sec, ms), whole},
		{"%Q %s", fmt.Sprintf("%d %d", ms, sec), whole},
		{"%Y-%m-%d %s", fmt.Sprintf("2009-08-07 %d", sec), whole},
		{"%H:%M:%S %s", fmt.Sprintf("06:05:04 %d", sec), whole},
		{"%Y-%m-%d %H:%M:%S %s.%N", fmt.Sprintf("2009-08-07 06:05:04 %d.300000000", sec), reference},
		{"%FT%T%:z %s", fmt.Sprintf("2009-08-07T06:05:04+00:00 %d", sec), whole},
		{"%FT%T%:z %s", fmt.Sprintf("2009-08-07T07:05:04+01:00 %d", sec), whole},
		{"%FT%T%:z %s.%N", fmt.Sprintf("2009-08-07T06:05:04.3+00:00 %d.300000000", sec), reference},
		{"%s %z", fmt.Sprintf("%d +0900", sec), whole},
	}
	for _, test := range tests {
		got, err := strftime.Parse(test.format, test.value)
		if err != nil {
			t.Errorf("Parse(%q, %q) error: %v", test.format, test.value, err)
		} else if !got.Equal(test.want) {
			t.Errorf("Parse(%q, %q) = %v, want %v", test.format, test.value, got, test.want)
		}
	}

	got, err := strftime.Parse("%s %z", fmt.Sprintf("%d +0900", sec))
	if err != nil {
		t.Fatal(err)
	}
	if _, offset := got.Zone(); offset != 9*3600 {
		t.Errorf("Parse(%q) zone offset = %d, want %d", "%s %z", offset, 9*3600)
	}
}

func TestParse_UnixConflict(t *testing.T) {
	sec := reference.Unix()

	tests := []struct {
		format string
		value  string
		want   string
	}{
		{"%s %s", "123 124", "%s"},
		{"%s %Q", "123 123001", "%Q"},
		{"%Q %s", "123001 123", "%s"},
		{"%Y-%m-%d %s", fmt.Sprintf("2009-08-08 %d", sec), "%d"},
		{"%Y-%m-%d %s", fmt.Sprintf("2010-08-07 %d", sec), "%Y"},
		{"%H:%M:%S %s", fmt.Sprintf("06:05:05 %d", sec), "%S"},
		{"%H:%M:%S %s", fmt.Sprintf("07:05:04 %d", sec), "%H"},
		{"%T %s.%N", fmt.Sprintf("06:05:04.301 %d.300000000", sec), "%S"},
		{"%FT%T%:z %s", fmt.Sprintf("2009-08-07T07:05:04+02:00 %d", sec), "%T"},
	}
	for _, test := range tests {
		got, err := strftime.Parse(test.format, test.value)
		if err == nil {
			t.Errorf("Parse(%q, %q) expected error", test.format, test.value)
		} else if !strings.Contains(err.Error(), test.want) {
			t.Errorf("Parse(%q, %q) = %q, want %q", test.format, test.value, err, test.want)
		}
		if !got.IsZero() {
			t.Errorf("Parse(%q, %q) = %v, want zero time", test.format, test.value, got)
		}
	}
}
