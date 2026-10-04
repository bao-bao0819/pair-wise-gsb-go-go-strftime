package strftime_test

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ncruces/go-strftime"
)

const (
	// ext = unixSec + unixToInternal must fit int64; time.Time cannot
	// represent instants beyond these Unix seconds.
	unixToInternal int64 = 62135596800
	maxUnixSec           = int64(math.MaxInt64) - unixToInternal
	minUnixSec           = -maxUnixSec - 1
)

func TestParse_Epoch(t *testing.T) {
	tests := []struct {
		format string
		value  string
		want   time.Time
	}{
		// Zero point, both sides.
		{"%s", "0", time.Unix(0, 0).UTC()},
		{"%s", "+0", time.Unix(0, 0).UTC()},
		{"%s", "-0", time.Unix(0, 0).UTC()},
		{"%s", "1", time.Unix(1, 0).UTC()},
		{"%s", "-1", time.Unix(-1, 0).UTC()},

		// Leading zeros and alternate number spellings.
		{"%s", "0000000000", time.Unix(0, 0).UTC()},
		{"%s", "+0000000001", time.Unix(1, 0).UTC()},
		{"%s", "-0000000001", time.Unix(-1, 0).UTC()},
		{"%Q", "0000000000000", time.Unix(0, 0).UTC()},

		// Milliseconds.
		{"%Q", "123456789", time.Unix(123456, 789_000_000).UTC()},
		{"%Q", "-1", time.Unix(-1, 999_000_000).UTC()},
		{"%Q", "-250", time.Unix(-1, 750_000_000).UTC()},
		{"%Q", "-1000", time.Unix(-1, 0).UTC()},

		// Seconds + independent fraction, nanosecond exactness.
		{"%s.%N", "0.000000001", time.Unix(0, 1).UTC()},
		{"%s.%N", "0.1", time.Unix(0, 100_000_000).UTC()},
		{"%s.%N", "0.10", time.Unix(0, 100_000_000).UTC()},
		{"%s.%N", "0.250", time.Unix(0, 250_000_000).UTC()},
		{"%s,%N", "0,250", time.Unix(0, 250_000_000).UTC()},
		{"%s.%f", "0.250000", time.Unix(0, 250_000_000).UTC()},
		{"%s.%f", "0.25", time.Unix(0, 250_000_000).UTC()}, // first 2 of 6 places
		{"%s.%L", "0.250", time.Unix(0, 250_000_000).UTC()},
		{"%s.%L", "0.25", time.Unix(0, 250_000_000).UTC()}, // place-value pad
		{"%s.%L", "0.1", time.Unix(0, 100_000_000).UTC()},
		{"%s.%N", "0.999999999", time.Unix(0, 999_999_999).UTC()},

		// Negative fractions lie before the epoch.
		{"%s.%N", "-0.000000001", time.Unix(-1, 999_999_999).UTC()},
		{"%s.%N", "-0.250", time.Unix(-1, 750_000_000).UTC()},
		{"%s.%L", "-0.250", time.Unix(-1, 750_000_000).UTC()},
		{"%s.%L", "-0.1", time.Unix(-1, 900_000_000).UTC()},
		{"%s.%f", "-0.25", time.Unix(-1, 750_000_000).UTC()},
		{"%s.%N", "-1.250", time.Unix(-2, 750_000_000).UTC()},

		// int64 millisecond boundary is representable.
		{"%Q", "9223372036854775807", time.UnixMilli(math.MaxInt64).UTC()},
		{"%Q", "-9223372036854775808", time.UnixMilli(math.MinInt64).UTC()},

		// Seconds exactly at the time.Time representable boundary.
		{"%s", strconv.FormatInt(maxUnixSec, 10), time.Unix(maxUnixSec, 0).UTC()},
		{"%s", strconv.FormatInt(minUnixSec, 10), time.Unix(minUnixSec, 0).UTC()},

		// Epoch cross-checked with calendar fields.
		{"%FT%T %s", "2009-08-07T06:05:04 1249625104", time.Unix(1249625104, 0).UTC()},
		{"%s %Y-%m-%dT%H:%M:%S", "1249625104 2009-08-07T06:05:04", time.Unix(1249625104, 0).UTC()},
		{"%s %F", "1249625104 2009-08-07", time.Unix(1249625104, 0).UTC()},
		{"%s %R", "1249625104 06:05", time.Unix(1249625104, 0).UTC()},

		// Zone shifts wall fields, not the instant.
		{"%FT%T%z %s", "2009-08-07T14:05:04+0800 1249625104",
			time.Unix(1249625104, 0).In(time.FixedZone("", 8*3600))},
		{"%s %FT%T%:z", "1249625104 2009-08-07T14:05:04+08:00",
			time.Unix(1249625104, 0).In(time.FixedZone("", 8*3600))},

		// Two epochs that agree.
		{"%s %s", "1249625104 1249625104", time.Unix(1249625104, 0).UTC()},
		{"%s %Q", "1249625104 1249625104000", time.Unix(1249625104, 0).UTC()},

		// Epoch with independent subsecond chunk agreeing exactly.
		{"%s.%N %T.%N", "1249625104.300000000 06:05:04.300000000",
			time.Unix(1249625104, 300_000_000).UTC()},

		// Weekday field agrees with the epoch.
		{"%s %A", "1249625104 Friday", time.Unix(1249625104, 0).UTC()},
		{"%s %a", "1249625104 Fri", time.Unix(1249625104, 0).UTC()},

		// Zone-only chunk selects the result location.
		{"%s %z", "1249625104 +0800",
			time.Unix(1249625104, 0).In(time.FixedZone("", 8*3600))},

		// A subsecond chunk with less precision compares at that precision.
		{"%s.%N %T.%L", "1249625104.300500000 06:05:04.300",
			time.Unix(1249625104, 300_500_000).UTC()},

		// Literal prefix/suffix around the epoch.
		{"at %s", "at 100", time.Unix(100, 0).UTC()},
		{"%s end", "100 end", time.Unix(100, 0).UTC()},
	}

	for _, tt := range tests {
		got, err := strftime.Parse(tt.format, tt.value)
		if err != nil {
			t.Errorf("Parse(%q, %q) error: %v", tt.format, tt.value, err)
			continue
		}
		if !got.Equal(tt.want) || got.Nanosecond() != tt.want.Nanosecond() {
			t.Errorf("Parse(%q, %q) = %v, want %v", tt.format, tt.value,
				got.Format(time.RFC3339Nano), tt.want.Format(time.RFC3339Nano))
			continue
		}
		// No float64 round-trip: UnixNano must survive exactly.
		if tt.want.Unix() >= minUnixSec && tt.want.Unix() <= maxUnixSec {
			if got.UnixNano() != tt.want.UnixNano() {
				t.Errorf("Parse(%q, %q) UnixNano = %d, want %d",
					tt.format, tt.value, got.UnixNano(), tt.want.UnixNano())
			}
		}
	}
}

func TestParse_EpochErrors(t *testing.T) {
	tests := []struct {
		format string
		value  string
		// part of the error message that must be present
		want   string
		offset bool // an input-position error is expected
	}{
		// %Q cannot be combined with an attached fraction.
		{"%Q.%L", "123456789.000", "%Q", false},
		{"%Q.%f", "123456789.000000", "%Q", false},
		{"%Q.%N", "123456789.000000000", "%Q", false},

		// Extra significant digits must not be truncated.
		{"%s.%N", "0.1234567890", "%N", true},
		{"%s.%L", "0.1234", "%L", true},
		{"%s.%f", "0.1234567", "%f", true},

		// Missing fraction digits.
		{"%s.%N", "0.", "%N", true},
		{"%s.%L", "0", "fraction", true},

		// Calendar field disagrees with epoch.
		{"%s %Y", "1249625104 2010", "%Y", true},
		{"%s %F", "1249625104 2009-08-08", "%d", true},
		{"%FT%T %s", "2009-08-07T06:05:05 1249625104", "%S", true},
		{"%s %A", "1249625104 Thursday", "%A", true},
		{"%s.%N %T.%L", "1249625104.300 06:05:04.301", "%L", true},

		// Epoch fields disagree.
		{"%s %s", "1249625104 1249625105", "%s", true},
		{"%s %Q", "1249625104 1249625104001", "%Q", true},

		// Out of time.Time range (must not panic / wrap silently).
		{"%s", strconv.FormatInt(maxUnixSec+1, 10), "range", true},
		{"%s", strconv.FormatInt(minUnixSec-1, 10), "range", true},
		{"%s", "9223372036854775807", "range", true},
		{"%s", "-9223372036854775808", "range", true},
		{"%s", "99999999999999999999", "range", true},
		{"%s.%N", "-9223372036854775808.000000001", "range", true},

		// Malformed input reports the directive and position, not a generic
		// "invalid time".
		{"%s", "abc", "%s", true},
		{"%s.%N", "1.2a", "offset", true},
		{"%s extra", "123 nope", "extra", true},
	}

	for _, tt := range tests {
		got, err := strftime.Parse(tt.format, tt.value)
		if err == nil {
			t.Errorf("Parse(%q, %q) = %v, want error containing %q",
				tt.format, tt.value, got, tt.want)
			continue
		}
		if !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Parse(%q, %q) error = %q, want substring %q",
				tt.format, tt.value, err.Error(), tt.want)
		}
		if tt.offset && !strings.Contains(err.Error(), "offset") {
			t.Errorf("Parse(%q, %q) error %q does not report input position",
				tt.format, tt.value, err.Error())
		}
	}
}

func TestParse_EpochRoundTrip(t *testing.T) {
	// Exact nanosecond values never go through float64.
	samples := []time.Time{
		time.Unix(0, 1),
		time.Unix(0, 999_999_999),
		time.Unix(-1, 0), // whole seconds round-trip exactly
		time.Unix(1249625104, 300_000_000),
		time.Unix(maxUnixSec, 0),
		time.Unix(minUnixSec, 0),
	}
	for _, want := range samples {
		value := strftime.Format("%s.%N", want)
		got, err := strftime.Parse("%s.%N", value)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", value, err)
			continue
		}
		if got.UnixNano() != want.UnixNano() {
			t.Errorf("round-trip %q: got %d, want %d",
				value, got.UnixNano(), want.UnixNano())
		}
	}
}

func TestParse_EpochNegativeFraction(t *testing.T) {
	// A negative fraction counts backward from the integer second, so the
	// result must equal the exact negative Unix nanosecond value.
	tests := []struct {
		value string
		nano  int64
	}{
		{"-0.000000001", -1},
		{"-0.250", -250_000_000},
		{"-0.999999999", -999_999_999},
		{"-1.250", -1_250_000_000},
		{"-123456789.123456789", -123_456_789_123_456_789},
	}
	for _, tt := range tests {
		got, err := strftime.Parse("%s.%N", tt.value)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.value, err)
			continue
		}
		if got.UnixNano() != tt.nano {
			t.Errorf("Parse(%q) UnixNano = %d, want %d",
				tt.value, got.UnixNano(), tt.nano)
		}
	}
}

func TestParse_EpochLargeExactNanos(t *testing.T) {
	// Large Unix second with a non-trivial fraction: float64 would round this.
	value := "1000000000.123456789"
	got, err := strftime.Parse("%s.%N", value)
	if err != nil {
		t.Fatal(err)
	}
	const wantNano = 1_000_000_000_123_456_789
	if got.UnixNano() != wantNano {
		t.Errorf("UnixNano = %d, want %d", got.UnixNano(), wantNano)
	}
	if got.Unix() != 1_000_000_000 || got.Nanosecond() != 123_456_789 {
		t.Errorf("got sec=%d ns=%d", got.Unix(), got.Nanosecond())
	}
}

func TestParse_EpochRange(t *testing.T) {
	// Just inside the representable range.
	inRange := []string{
		strconv.FormatInt(maxUnixSec, 10),
		strconv.FormatInt(minUnixSec, 10),
	}
	for _, v := range inRange {
		if _, err := strftime.Parse("%s", v); err != nil {
			t.Errorf("Parse(%q) in range: %v", v, err)
		}
	}

	// One second beyond on either side must be reported, not wrapped.
	outRange := []string{
		strconv.FormatInt(maxUnixSec+1, 10),
		strconv.FormatInt(minUnixSec-1, 10),
	}
	for _, v := range outRange {
		got, err := strftime.Parse("%s", v)
		if err == nil {
			t.Errorf("Parse(%q) = %v, want range error", v, got)
		}
	}

	// int64 millisecond extremes are expressible (wider than seconds limits).
	for _, v := range []string{strconv.FormatInt(math.MaxInt64, 10), strconv.FormatInt(math.MinInt64, 10)} {
		got, err := strftime.Parse("%Q", v)
		if err != nil {
			t.Errorf("Parse(%%Q,%q): %v", v, err)
			continue
		}
		ms := got.UnixMilli()
		want, _ := strconv.ParseInt(v, 10, 64)
		if ms != want {
			t.Errorf("Parse(%%Q,%q) UnixMilli = %d, want %d", v, ms, want)
		}
	}
}

func TestParse_EpochFractionConflict(t *testing.T) {
	// A %Q combined with an attached fraction directive is rejected.
	for _, format := range []string{"%Q.%L", "%Q.%f", "%Q.%N", "%Q,%L"} {
		if _, err := strftime.Parse(format, "123456789.000"); err == nil {
			t.Errorf("Parse(%q) expected conflict error", format)
		}
	}
}
