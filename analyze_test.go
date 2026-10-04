package strftime_test

import (
	"testing"
	"time"

	"github.com/ncruces/go-strftime"
)

var analyzeTests = []struct {
	format    string
	length    int // length of Format(format, reference)
	date      bool
	clock     bool
	precision time.Duration
	zone      bool
	verdict   strftime.Verdict
	issue     string // a directive expected in Issues, "" to skip
}{
	// 8 characters, increasing information loss.
	{"%H:%M:%S", 8, false, true, time.Second, false, strftime.Conditional, ""},
	{"%I:%M:%S", 8, false, true, time.Second, false, strftime.Conditional, "%I"},
	{"%y-%m-%d", 8, true, false, 0, false, strftime.Conditional, "%y"},
	{"%m/%d/%y", 8, true, false, 0, false, strftime.Conditional, "%y"},
	{"%D", 8, true, false, 0, false, strftime.Conditional, "%y"},
	{"%Y %j", 8, true, false, 0, false, strftime.Conditional, ""},
	// 10 characters, from conditional to unparseable.
	{"%Y-%m-%d", 10, true, false, 0, false, strftime.Conditional, ""},
	{"%F", 10, true, false, 0, false, strftime.Conditional, ""},
	{"%m/%d/%Y", 10, true, false, 0, false, strftime.Conditional, ""},
	{"%G-W%V-%u", 10, true, false, 0, false, strftime.Unparseable, "%G"},
	{"%Y-W%V-%u", 10, false, false, 0, false, strftime.Unparseable, "%V"},
	{"%s", 10, true, true, time.Second, true, strftime.Unparseable, "%s"},
	// 5 characters.
	{"%H:%M", 5, false, true, time.Minute, false, strftime.Conditional, ""},
	{"%R", 5, false, true, time.Minute, false, strftime.Conditional, ""},
	{"%I:%M", 5, false, true, time.Minute, false, strftime.Conditional, "%I"},
	// Zone information.
	{"%Y-%m-%dT%H:%M:%S%:z", 25, true, true, time.Second, true, strftime.Conditional, ""},
	{"%Y-%m-%dT%H:%M:%S.%N%:z", 35, true, true, time.Nanosecond, true, strftime.Reversible, ""},
	{"%Y-%m-%dT%H:%M:%S.%N %Z", 33, true, true, time.Nanosecond, false, strftime.Conditional, "%Z"},
	// Composite directives.
	{"%c", 24, true, true, time.Second, false, strftime.Conditional, ""},
	{"%+", 28, true, true, time.Second, false, strftime.Conditional, "%Z"},
	{"%r", 11, false, true, time.Second, false, strftime.Conditional, ""},
	{"%v", 11, true, false, 0, false, strftime.Conditional, ""},
	// Epoch and week numbers.
	{"%Q", 13, true, true, time.Millisecond, true, strftime.Unparseable, "%Q"},
	{"%Y-%m-%d %U", 13, true, false, 0, false, strftime.Unparseable, "%U"},
}

func TestAnalyze(t *testing.T) {
	for _, tt := range analyzeTests {
		a := strftime.Analyze(tt.format)
		if got := len(strftime.Format(tt.format, reference)); got != tt.length {
			t.Errorf("Format(%q) length: got %d, want %d", tt.format, got, tt.length)
		}
		if a.Date != tt.date {
			t.Errorf("Analyze(%q).Date: got %t, want %t", tt.format, a.Date, tt.date)
		}
		if a.Time != tt.clock {
			t.Errorf("Analyze(%q).Time: got %t, want %t", tt.format, a.Time, tt.clock)
		}
		if a.Precision != tt.precision {
			t.Errorf("Analyze(%q).Precision: got %s, want %s", tt.format, a.Precision, tt.precision)
		}
		if a.Zone != tt.zone {
			t.Errorf("Analyze(%q).Zone: got %t, want %t", tt.format, a.Zone, tt.zone)
		}
		if a.Verdict != tt.verdict {
			t.Errorf("Analyze(%q).Verdict: got %s, want %s (%v)", tt.format, a.Verdict, tt.verdict, a.Issues)
		}
		for _, issue := range a.Issues {
			if issue.Reason == "" {
				t.Errorf("Analyze(%q): issue without reason: %v", tt.format, issue)
			}
		}
		if tt.issue == "" {
			continue
		}
		found := false
		for _, issue := range a.Issues {
			found = found || issue.Directive == tt.issue
		}
		if !found {
			t.Errorf("Analyze(%q): no issue for %s: %v", tt.format, tt.issue, a.Issues)
		}
	}
}

func TestAnalyzeConflicts(t *testing.T) {
	tests := []struct {
		format string
		issue  string
	}{
		{"%H:%I %p", "%I"},    // 12-hour and 24-hour clocks mixed
		{"%Y-%y-%m-%d", "%y"}, // full and two-digit years mixed
		{"%Y-%m-%d %j", "%j"}, // day of the year mixed with month and day
		{"%G-W%V %F", "%G"},   // ISO week date mixed with calendar date
		{"%S.%L.%N", "%L"},    // multiple sub-second directives
		{"%s %F", "%s"},       // epoch mixed with calendar
		{"%p %H:%M", "%p"},    // meridian without a 12-hour clock
		{"%G-%m-%d", "%G"},    // ISO year without week number
		{"%V-%u", "%V"},       // ISO week without week-based year
		{"%G-W%V", "%V"},      // ISO week date without weekday
	}
	for _, tt := range tests {
		a := strftime.Analyze(tt.format)
		found := false
		for _, issue := range a.Issues {
			found = found || issue.Directive == tt.issue
		}
		if !found {
			t.Errorf("Analyze(%q): no issue for %s: %v", tt.format, tt.issue, a.Issues)
		}
	}
}

func TestAnalyzeRoundTrip(t *testing.T) {
	// A reversible format recovers any time exactly.
	a := strftime.Analyze("%Y-%m-%dT%H:%M:%S.%N%:z")
	r := a.RoundTrip(reference)
	if r.Err != nil || !r.Exact || r.Delta != 0 {
		t.Errorf("RoundTrip: got %+v", r)
	}
	if a.Verdict != strftime.Reversible {
		t.Errorf("Verdict: got %s, want reversible", a.Verdict)
	}

	// A conditional format may round-trip a lucky sample:
	// the reference time has exactly 300ms, so millisecond
	// truncation is invisible. The static verdict still stands.
	a = strftime.Analyze("%Y-%m-%dT%H:%M:%S.%L%:z")
	r = a.RoundTrip(reference)
	if r.Err != nil || !r.Exact {
		t.Errorf("RoundTrip: got %+v", r)
	}
	if a.Verdict != strftime.Conditional {
		t.Errorf("Verdict: got %s, want conditional", a.Verdict)
	}

	// A time-only format parses, but loses the date.
	a = strftime.Analyze("%T")
	r = a.RoundTrip(reference)
	if r.Err != nil || r.Exact {
		t.Errorf("RoundTrip: got %+v", r)
	}

	// An unparseable format fails the round trip.
	a = strftime.Analyze("%s")
	r = a.RoundTrip(reference)
	if r.Err == nil {
		t.Errorf("RoundTrip: got %+v, want error", r)
	}
}

func TestAnalyzeCompositeExpansion(t *testing.T) {
	// Expanding composites must give the same verdict
	// as the equivalent explicit format.
	for _, tt := range [][2]string{
		{"%c", "%a %b %e %T %Y"},
		{"%+", "%a %b %e %H:%M:%S %Z %Y"},
		{"%F", "%Y-%m-%d"},
		{"%D", "%m/%d/%y"},
		{"%r", "%I:%M:%S %p"},
		{"%T", "%H:%M:%S"},
	} {
		a, b := strftime.Analyze(tt[0]), strftime.Analyze(tt[1])
		if a.Verdict != b.Verdict || a.Date != b.Date || a.Time != b.Time ||
			a.Precision != b.Precision || a.Zone != b.Zone {
			t.Errorf("Analyze(%q) = %+v, Analyze(%q) = %+v", tt[0], a, tt[1], b)
		}
	}
}
