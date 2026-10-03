package strftime_test

import (
	"testing"
	"time"

	"github.com/ncruces/go-strftime"
)

func TestParse_ISOWeekDate(t *testing.T) {
	utc := time.UTC
	minus6 := time.FixedZone("", -6*3600)

	tests := []struct {
		format string
		value  string
		want   time.Time
	}{
		// Extended and basic formats.
		{"%G-W%V-%u", "2007-W47-1", time.Date(2007, 11, 19, 0, 0, 0, 0, utc)},
		{"%GW%V%u", "2007W471", time.Date(2007, 11, 19, 0, 0, 0, 0, utc)},
		{"%G-W%V-%u", "2009-W32-5", time.Date(2009, 8, 7, 0, 0, 0, 0, utc)},
		// Start of the civil year belongs to the previous week-based year.
		{"%G-W%V-%u", "2020-W53-5", time.Date(2021, 1, 1, 0, 0, 0, 0, utc)},
		// End of the civil year belongs to the next week-based year.
		{"%G-W%V-%u", "2019-W01-1", time.Date(2018, 12, 31, 0, 0, 0, 0, utc)},
		// Week 53 of a leap year, Sunday (%u = 7).
		{"%G-W%V-%u", "2020-W53-7", time.Date(2021, 1, 3, 0, 0, 0, 0, utc)},
		// Leap year, Sunday of the last week.
		{"%G-W%V-%u", "2016-W52-7", time.Date(2017, 1, 1, 0, 0, 0, 0, utc)},
		// Two-digit week-based year.
		{"%g-W%V-%u", "20-W53-5", time.Date(2021, 1, 1, 0, 0, 0, 0, utc)},
		// Time and time zone directives combine as usual.
		{"%G-W%V-%uT%T%:z", "2007-W47-1T08:37:48-06:00",
			time.Date(2007, 11, 19, 8, 37, 48, 0, minus6)},
		{"%G-W%V-%u %T.%L %Z", "2009-W32-5 06:05:04.300 UTC",
			time.Date(2009, 8, 7, 6, 5, 4, 300000000, utc)},
		// Calendar date and week date must agree: year-month-day.
		{"%Y-%m-%d %G-W%V-%u", "2021-01-01 2020-W53-5",
			time.Date(2021, 1, 1, 0, 0, 0, 0, utc)},
		// Calendar date and week date must agree: day of the year.
		{"%Y-%j %G-W%V-%u", "2021-001 2020-W53-5",
			time.Date(2021, 1, 1, 0, 0, 0, 0, utc)},
		{"%F %G-W%V-%u", "2018-12-31 2019-W01-1",
			time.Date(2018, 12, 31, 0, 0, 0, 0, utc)},
	}

	for _, test := range tests {
		got, err := strftime.Parse(test.format, test.value)
		if err != nil {
			t.Errorf("Parse(%q, %q) = %v", test.format, test.value, err)
		} else if !got.Equal(test.want) ||
			got.Year() != test.want.Year() ||
			got.YearDay() != test.want.YearDay() {
			t.Errorf("Parse(%q, %q) = %v, want %v",
				test.format, test.value, got, test.want)
		} else if _, offset := got.Zone(); test.want.Location() != time.UTC &&
			offset != -6*3600 {
			t.Errorf("Parse(%q, %q) zone offset = %d",
				test.format, test.value, offset)
		}
	}
}

func TestParse_ISOWeekDate_Error(t *testing.T) {
	tests := []struct {
		format string
		value  string
	}{
		// Week 53 does not exist in these years.
		{"%G-W%V-%u", "2021-W53-1"},
		{"%G-W%V-%u", "2019-W53-7"},
		// Week number out of range.
		{"%G-W%V-%u", "2020-W00-1"},
		{"%G-W%V-%u", "2020-W54-1"},
		// Weekday out of range.
		{"%G-W%V-%u", "2020-W01-0"},
		{"%G-W%V-%u", "2020-W01-8"},
		// Missing fields.
		{"%G-W%V", "2020-W53"},
		{"%V-%u", "53-5"},
		{"%G %u", "2020 5"},
		{"%g", "20"},
		{"%u", "5"},
		// Calendar date disagrees with the week date.
		{"%Y-%m-%d %G-W%V-%u", "2020-12-31 2020-W53-5"},
		{"%Y-%j %G-W%V-%u", "2020-366 2020-W53-5"},
		{"%F %G-W%V-%u", "2019-01-01 2019-W01-1"},
		// Value does not match the format.
		{"%G-W%V-%u", "2020-W53"},
		{"%G-W%V-%u", "2020-W53-5x"},
	}

	for _, test := range tests {
		got, err := strftime.Parse(test.format, test.value)
		if err == nil {
			t.Errorf("Parse(%q, %q) = %v, want error",
				test.format, test.value, got)
		} else if !got.IsZero() {
			t.Errorf("Parse(%q, %q) = (%v, %v), want zero time",
				test.format, test.value, got, err)
		}
	}
}

func TestParse_ISOWeekDate_Pivot(t *testing.T) {
	defer func(pivot int) { strftime.PivotYear = pivot }(strftime.PivotYear)

	// The default window is [1969, 2069).
	got, err := strftime.Parse("%g-W%V-%u", "68-W01-1")
	if err != nil {
		t.Fatal(err)
	} else if year := strftime.Format("%G", got); year != "2068" {
		t.Errorf("Parse(%q) week-based year = %q, want %q", "68-W01-1", year, "2068")
	}

	// The window is configurable, never based on the current year.
	strftime.PivotYear = 2020
	for value, want := range map[string]string{
		"19-W01-1": "2119",
		"20-W01-1": "2020",
		"99-W01-1": "2099",
	} {
		got, err := strftime.Parse("%g-W%V-%u", value)
		if err != nil {
			t.Errorf("Parse(%q) = %v", value, err)
		} else if year := strftime.Format("%G", got); year != want {
			t.Errorf("Parse(%q) week-based year = %q, want %q", value, year, want)
		}
	}
}

func TestParse_ISOWeekDate_RoundTrip(t *testing.T) {
	const format = "%G-W%V-%u"

	// Every day of several consecutive full years,
	// including leap years and years with 53 weeks.
	for year := 2015; year <= 2025; year++ {
		days := 365
		if time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC).YearDay() == 366 {
			days = 366
		}
		for day := 1; day <= days; day++ {
			date := time.Date(year, time.January, day, 0, 0, 0, 0, time.UTC)
			str := strftime.Format(format, date)

			got, err := strftime.Parse(format, str)
			if err != nil {
				t.Errorf("Parse(%q, %q) = %v", format, str, err)
			} else if got.Year() != date.Year() ||
				got.Month() != date.Month() ||
				got.Day() != date.Day() {
				t.Errorf("Parse(%q, %q) = %v, want %v",
					format, str, got, date)
			}
		}
	}
}

func TestParse_ISOWeekDate_RoundTrip_TimeZone(t *testing.T) {
	const format = "%G-W%V-%uT%T%:z"
	zone := time.FixedZone("CET", 3600)

	for day := 1; day <= 366; day += 17 {
		date := time.Date(2020, time.January, day, 6, 5, 4, 0, zone)
		str := strftime.Format(format, date)

		got, err := strftime.Parse(format, str)
		if err != nil {
			t.Errorf("Parse(%q, %q) = %v", format, str, err)
		} else if !got.Equal(date) {
			t.Errorf("Parse(%q, %q) = %v, want %v",
				format, str, got, date)
		} else if _, offset := got.Zone(); offset != 3600 {
			t.Errorf("Parse(%q, %q) zone offset = %d, want %d",
				format, str, offset, 3600)
		}
	}
}
