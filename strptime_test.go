package strftime_test

import (
	"testing"
	"time"

	"github.com/ncruces/go-strftime"
)

func TestParse_ISOWeekDate(t *testing.T) {
	tests := []struct {
		format string
		value  string
		want   time.Time
	}{
		// Year start belongs to the previous week-based year.
		{"%G-W%V-%u", "2020-W01-1", time.Date(2019, 12, 30, 0, 0, 0, 0, time.UTC)},
		{"%G-W%V-%u", "2020-W53-5", time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"%G-W%V-%u", "2020-W53-7", time.Date(2021, 1, 3, 0, 0, 0, 0, time.UTC)},
		// Year end belongs to the next week-based year.
		{"%G-W%V-%u", "2019-W01-1", time.Date(2018, 12, 31, 0, 0, 0, 0, time.UTC)},
		{"%G-W%V-%u", "2019-W01-7", time.Date(2019, 1, 6, 0, 0, 0, 0, time.UTC)},
		// Years with a 53rd week.
		{"%G-W%V-%u", "2015-W53-7", time.Date(2016, 1, 3, 0, 0, 0, 0, time.UTC)},
		{"%G-W%V-%u", "2020-W53-1", time.Date(2020, 12, 28, 0, 0, 0, 0, time.UTC)},
		// Sunday (%u is 7, %w is 0) and leap day.
		{"%G-W%V-%u", "2016-W52-7", time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"%G-W%V-%w", "2016-W52-0", time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"%G-W%V-%u", "2020-W09-6", time.Date(2020, 2, 29, 0, 0, 0, 0, time.UTC)},
		// Unpadded week number.
		{"%G-W%-V-%u", "2009-W32-5", time.Date(2009, 8, 7, 0, 0, 0, 0, time.UTC)},
		// Two-digit week-based year.
		{"%g-W%V-%u", "20-W53-5", time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)},
		// Both %u and %w, agreeing.
		{"%G-W%V-%u/%w", "2020-W53-5/5", time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)},
		// Calendar date and week date must agree.
		{"%Y-%m-%d %G-W%V-%u", "2021-01-01 2020-W53-5", time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"%Y %j %G-W%V-%u", "2021 1 2020-W53-5", time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"%F %G-W%V-%u", "2020-02-29 2020-W09-6", time.Date(2020, 2, 29, 0, 0, 0, 0, time.UTC)},
		// Weekday checked against a calendar date.
		{"%Y-%m-%d %u", "2021-01-01 5", time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)},
		// Time and time zone combine as usual.
		{"%G-W%V-%u %T", "2020-W53-5 06:05:04", time.Date(2021, 1, 1, 6, 5, 4, 0, time.UTC)},
		{"%G-W%V-%uT%T%:z", "2020-W53-5T06:05:04Z", time.Date(2021, 1, 1, 6, 5, 4, 0, time.UTC)},
		{"%G-W%V-%u %r", "2020-W53-5 06:05:04 PM", time.Date(2021, 1, 1, 18, 5, 4, 0, time.UTC)},
	}
	for _, test := range tests {
		got, err := strftime.Parse(test.format, test.value)
		if err != nil {
			t.Errorf("Parse(%q, %q) = %v", test.format, test.value, err)
		} else if !got.Equal(test.want) {
			t.Errorf("Parse(%q, %q) = %v, want %v", test.format, test.value, got, test.want)
		}
	}
}

func TestParse_ISOWeekDate_Zone(t *testing.T) {
	got, err := strftime.Parse("%G-W%V-%uT%T%:z", "2020-W53-5T06:05:04+09:00")
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	want := time.Date(2021, 1, 1, 6, 5, 4, 0, time.FixedZone("", 9*3600))
	if !got.Equal(want) {
		t.Errorf("Parse() = %v, want %v", got, want)
	}
	if _, offset := got.Zone(); offset != 9*3600 {
		t.Errorf("Parse() zone offset = %d, want %d", offset, 9*3600)
	}
}

func TestParse_ISOWeekDate_Errors(t *testing.T) {
	tests := []struct {
		format string
		value  string
	}{
		// The week-based year has no 53rd week.
		{"%G-W%V-%u", "2021-W53-1"},
		{"%G-W%V-%u", "2019-W53-7"},
		// Week number and weekday out of range.
		{"%G-W%V-%u", "2020-W00-1"},
		{"%G-W%V-%u", "2020-W54-1"},
		{"%G-W%V-%u", "2020-W01-0"},
		{"%G-W%V-%u", "2020-W01-8"},
		{"%G-W%V-%w", "2020-W01-7"},
		// Missing fields.
		{"%G-W%V", "2020-W53"},
		{"%V-%u", "53-5"},
		{"%G-%u", "2020-5"},
		{"%u", "5"},
		// Calendar date and week date disagree.
		{"%Y-%m-%d %G-W%V-%u", "2021-01-02 2020-W53-5"},
		{"%Y-%m-%d %G-W%V-%u", "2020-12-31 2020-W53-5"},
		{"%Y %j %G-W%V-%u", "2021 2 2020-W53-5"},
		{"%Y %j %G-W%V-%u", "2020 366 2020-W53-5"},
		// Incomplete calendar date.
		{"%G-W%V-%u %m", "2020-W53-5 01"},
		{"%G-W%V-%u %Y", "2020-W53-5 2021"},
		// Weekday disagrees with the calendar date.
		{"%Y-%m-%d %u", "2021-01-01 6"},
		// Conflicting weekdays.
		{"%G-W%V-%u/%w", "2020-W53-7/1"},
		// Extra characters.
		{"%G-W%V-%u", "2020-W53-5x"},
	}
	for _, test := range tests {
		if got, err := strftime.Parse(test.format, test.value); err == nil {
			t.Errorf("Parse(%q, %q) = %v, want error", test.format, test.value, got)
		} else if !got.IsZero() {
			t.Errorf("Parse(%q, %q) = (%v, %v), want zero time", test.format, test.value, got, err)
		}
	}
}

func TestParse_ISOWeekDate_Pivot(t *testing.T) {
	defer func(pivot int) { strftime.WeekYearPivot = pivot }(strftime.WeekYearPivot)

	// Default window is 1970-2069.
	got, err := strftime.Parse("%g-W%V-%u", "20-W53-5")
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	if want := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Parse() = %v, want %v", got, want)
	}

	got, err = strftime.Parse("%g-W%V-%u", "69-W01-1")
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	if year, week := got.ISOWeek(); year != 2069 || week != 1 {
		t.Errorf("Parse() = %v, want ISO week 2069-W01", got)
	}

	// The window is configurable and fixed, it ignores the current year.
	strftime.WeekYearPivot = 1920
	got, err = strftime.Parse("%g-W%V-%u", "69-W01-1")
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	if want := time.Date(1968, 12, 30, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Parse() = %v, want %v", got, want)
	}
}

func TestParse_ISOWeekDate_RoundTrip(t *testing.T) {
	formats := []string{
		"%G-W%V-%u",
		"%Y-%m-%d %G-W%V-%u",
		"%Y %j %G-W%V-%u",
		"%G-W%V-%uT%T%:z",
	}
	start := time.Date(2014, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	for date := start; date.Before(end); date = date.AddDate(0, 0, 1) {
		for _, format := range formats {
			str := strftime.Format(format, date)
			got, err := strftime.Parse(format, str)
			if err != nil {
				t.Errorf("Parse(%q, %q) = %v", format, str, err)
			} else if got.Year() != date.Year() ||
				got.Month() != date.Month() ||
				got.Day() != date.Day() {
				t.Errorf("Parse(%q, %q) = %v, want %v", format, str, got, date)
			}
		}
	}
}
