package strftime_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ncruces/go-strftime"
)

// chineseSpec uses multi-byte names, and abbreviations
// that are not prefixes of the full names.
func chineseSpec() strftime.LocaleSpec {
	return strftime.LocaleSpec{
		Months: []string{
			"一月", "二月", "三月", "四月", "五月", "六月",
			"七月", "八月", "九月", "十月", "十一月", "十二月",
		},
		AbbreviatedMonths: []string{
			"1月", "2月", "3月", "4月", "5月", "6月",
			"7月", "8月", "9月", "10月", "11月", "12月",
		},
		Weekdays: []string{
			"星期日", "星期一", "星期二", "星期三",
			"星期四", "星期五", "星期六",
		},
		AbbreviatedWeekdays: []string{
			"周日", "周一", "周二", "周三",
			"周四", "周五", "周六",
		},
		AM: "上午",
		PM: "下午",
		Formats: map[byte]string{
			'c': "%Y年%m月%d日 %A %T",
			'x': "%Y年%m月%d日",
		},
	}
}

// prefixSpec uses names with common prefixes:
// "Mar" (abbreviated March), "March" and "Marathon" (May).
func prefixSpec() strftime.LocaleSpec {
	return strftime.LocaleSpec{
		Months: []string{
			"January", "February", "March", "April", "Marathon", "June",
			"July", "August", "September", "October", "November", "December",
		},
		AbbreviatedMonths: []string{
			"Jan", "Feb", "Mar", "Apr", "Mth", "Jun",
			"Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
		},
		Weekdays: []string{
			"Sunday", "Monday", "Tuesday", "Wednesday",
			"Thursday", "Friday", "Saturday",
		},
		AbbreviatedWeekdays: []string{
			"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat",
		},
		AM: "AM",
		PM: "PM",
		Formats: map[byte]string{
			'x': "%d.%m.%Y",
		},
	}
}

func TestLocale_Format(t *testing.T) {
	loc, err := strftime.NewLocale(chineseSpec())
	if err != nil {
		t.Fatal(err)
	}
	reference := time.Date(2009, 8, 7, 18, 5, 4, 0, time.UTC) // Friday
	tests := []struct {
		format string
		want   string
	}{
		{"%A", "星期五"},
		{"%a", "周五"},
		{"%B", "八月"},
		{"%b", "8月"},
		{"%h", "8月"},
		{"%p", "下午"},
		{"%P", "下午"},
		{"%Y年%B", "2009年八月"},
		{"%c", "2009年08月07日 星期五 18:05:04"},
		{"%x", "2009年08月07日"},
		{"%X", "18:05:04"},
		{"%r", "06:05:04 下午"},
		{"%F", "2009-08-07"},
		{" noon %p", " noon 下午"},
	}
	for _, test := range tests {
		if got := loc.Format(test.format, reference); got != test.want {
			t.Errorf("Format(%q) = %q, want %q", test.format, got, test.want)
		}
	}

	morning := time.Date(2009, 11, 10, 6, 5, 4, 0, time.UTC) // Tuesday, November
	if got := loc.Format("%p", morning); got != "上午" {
		t.Errorf("Format(%q) = %q, want %q", "%p", got, "上午")
	}
	// Abbreviations must not be truncated to three bytes.
	if got := loc.Format("%b", morning); got != "11月" {
		t.Errorf("Format(%q) = %q, want %q", "%b", got, "11月")
	}
	if got := loc.Format("%B", morning); got != "十一月" {
		t.Errorf("Format(%q) = %q, want %q", "%B", got, "十一月")
	}
}

func TestLocale_Parse(t *testing.T) {
	loc, err := strftime.NewLocale(chineseSpec())
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2009, 8, 7, 18, 5, 4, 0, time.UTC) // Friday
	for _, value := range []string{
		"2009年08月07日 星期五 18:05:04",
		"2009年8月7日 周五 18:05:04",
	} {
		got, err := loc.Parse("%c", value)
		if err != nil {
			t.Errorf("Parse(%q) = %v", value, err)
		} else if !got.Equal(want) {
			t.Errorf("Parse(%q) = %v, want %v", value, got, want)
		}
	}

	got, err := loc.Parse("%Y年%B%d日 %A %p %I时", "2009年八月7日 星期五 下午 06时")
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2009, 8, 7, 18, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Parse = %v, want %v", got, want)
	}
}

func TestLocale_Parse_weekday(t *testing.T) {
	loc, err := strftime.NewLocale(chineseSpec())
	if err != nil {
		t.Fatal(err)
	}
	// 2009-08-07 was a Friday, not a Monday.
	_, err = loc.Parse("%Y年%m月%d日 %A", "2009年08月07日 星期一")
	if err == nil {
		t.Error("Parse: expected weekday mismatch error")
	}
	if _, err = loc.Parse("%Y年%m月%d日 %A", "2009年08月07日 星期五"); err != nil {
		t.Errorf("Parse: %v", err)
	}
	// Without a day of the month, there is nothing to check against.
	if _, err = loc.Parse("%A", "星期一"); err != nil {
		t.Errorf("Parse: %v", err)
	}
}

func TestLocale_Parse_prefix(t *testing.T) {
	loc, err := strftime.NewLocale(prefixSpec())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		format string
		value  string
		month  time.Month
	}{
		{"%B", "Marathon", time.May}, // longest match wins
		{"%B", "March", time.March},  // not the "Mar" prefix
		{"%B", "Mar", time.March},    // abbreviations complete %B
		{"%b", "March", time.March},  // full names complete %b
		{"%b", "Mth", time.May},      //
		{"%B %d", "Marathon 15", time.May},
	}
	for _, test := range tests {
		got, err := loc.Parse(test.format, test.value)
		if err != nil {
			t.Errorf("Parse(%q, %q) = %v", test.format, test.value, err)
		} else if got.Month() != test.month {
			t.Errorf("Parse(%q, %q) month = %v, want %v",
				test.format, test.value, got.Month(), test.month)
		}
	}
	if _, err := loc.Parse("%B", "Marath"); err == nil {
		t.Error("Parse: expected error for partial name")
	}
}

func TestLocale_Parse_casefold(t *testing.T) {
	spec := prefixSpec()
	spec.CaseFold = true
	spec.Months[2] = "März"
	loc, err := strftime.NewLocale(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"märz", "MÄRZ", "MäRz"} {
		got, err := loc.Parse("%B %Y", value+" 2009")
		if err != nil {
			t.Errorf("Parse(%q) = %v", value, err)
		} else if got.Month() != time.March {
			t.Errorf("Parse(%q) month = %v, want %v", value, got.Month(), time.March)
		}
	}
	if got, err := loc.Parse("%B", "MARATHON"); err != nil || got.Month() != time.May {
		t.Errorf("Parse(MARATHON) = %v, %v", got, err)
	}

	// Without CaseFold, matching is exact.
	loc, err = strftime.NewLocale(prefixSpec())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loc.Parse("%B", "march"); err == nil {
		t.Error("Parse: expected error for exact matching")
	}
}

func TestLocale_literal(t *testing.T) {
	loc, err := strftime.NewLocale(prefixSpec())
	if err != nil {
		t.Fatal(err)
	}
	reference := time.Date(2009, 4, 1, 0, 0, 0, 0, time.UTC)
	// Literal text that looks like a name must not be matched as one.
	if got := loc.Format("March madness in %B", reference); got != "March madness in April" {
		t.Errorf("Format = %q, want %q", got, "March madness in April")
	}
	got, err := loc.Parse("March madness in %B", "March madness in April")
	if err != nil {
		t.Fatal(err)
	}
	if got.Month() != time.April {
		t.Errorf("Parse month = %v, want %v", got.Month(), time.April)
	}
}

func TestLocale_roundtrip(t *testing.T) {
	loc, err := strftime.NewLocale(chineseSpec())
	if err != nil {
		t.Fatal(err)
	}
	reference := time.Date(2009, 11, 10, 23, 1, 2, 0, time.UTC) // Tuesday
	const format = "%Y-%m-%d %A %B %b %I:%M:%S %p"
	value := loc.Format(format, reference)
	got, err := loc.Parse(format, value)
	if err != nil {
		t.Fatalf("Parse(%q) = %v", value, err)
	}
	if !got.Equal(reference) {
		t.Errorf("Parse(%q) = %v, want %v", value, got, reference)
	}
}

func TestLocale_Parse_directives(t *testing.T) {
	loc, err := strftime.NewLocale(chineseSpec())
	if err != nil {
		t.Fatal(err)
	}

	got, err := loc.Parse("%Y-%m-%d %T %z", "2009-08-07 18:05:04 +0230")
	if err != nil {
		t.Fatal(err)
	}
	if _, offset := got.Zone(); offset != 2*3600+30*60 {
		t.Errorf("Parse(%%z) offset = %d", offset)
	}

	got, err = loc.Parse("%Y-%m-%d %T.%L %Z", "2009-08-07 18:05:04.250 UTC")
	if err != nil {
		t.Fatal(err)
	}
	if got.Nanosecond() != 250000000 {
		t.Errorf("Parse(%%L) nsec = %d", got.Nanosecond())
	}

	// Day of the year also pins the date for the weekday check.
	got, err = loc.Parse("%Y %j %A", "2009 219 星期五")
	if err != nil {
		t.Fatal(err)
	}
	if got.Day() != 7 || got.Month() != time.August {
		t.Errorf("Parse(%%j) = %v", got)
	}
	if _, err = loc.Parse("%Y %j %A", "2009 219 星期一"); err == nil {
		t.Error("Parse: expected weekday mismatch error")
	}

	if _, err := loc.Parse("%B %e, %Y", "八月  7, 2009"); err != nil {
		t.Errorf("Parse(%%e) = %v", err)
	}
}

func TestNewLocale_errors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*strftime.LocaleSpec)
	}{
		{"months length", func(s *strftime.LocaleSpec) { s.Months = s.Months[:11] }},
		{"abbreviated length", func(s *strftime.LocaleSpec) { s.AbbreviatedWeekdays = nil }},
		{"weekdays length", func(s *strftime.LocaleSpec) { s.Weekdays = s.Weekdays[:6] }},
		{"empty month", func(s *strftime.LocaleSpec) { s.Months[3] = "" }},
		{"empty weekday", func(s *strftime.LocaleSpec) { s.AbbreviatedWeekdays[0] = "" }},
		{"empty meridiem", func(s *strftime.LocaleSpec) { s.PM = "" }},
		{"ambiguous month", func(s *strftime.LocaleSpec) { s.AbbreviatedMonths[1] = s.Months[0] }},
		{"ambiguous weekday", func(s *strftime.LocaleSpec) { s.Weekdays[2] = s.Weekdays[1] }},
		{"ambiguous meridiem", func(s *strftime.LocaleSpec) { s.PM = s.AM }},
		{"format cycle", func(s *strftime.LocaleSpec) {
			s.Formats = map[byte]string{'c': "%x", 'x': "%c"}
		}},
		{"format self cycle", func(s *strftime.LocaleSpec) {
			s.Formats = map[byte]string{'c': "%c"}
		}},
		{"format transitive cycle", func(s *strftime.LocaleSpec) {
			s.Formats = map[byte]string{'c': "%x", 'x': "%D", 'D': "%c"}
		}},
		{"format bad specifier", func(s *strftime.LocaleSpec) {
			s.Formats = map[byte]string{'q': "%Y"}
		}},
		{"format empty", func(s *strftime.LocaleSpec) {
			s.Formats = map[byte]string{'c': ""}
		}},
	}
	for _, test := range tests {
		spec := chineseSpec()
		test.mutate(&spec)
		if _, err := strftime.NewLocale(spec); err == nil {
			t.Errorf("NewLocale(%s): expected error", test.name)
		} else if !strings.HasPrefix(err.Error(), "strftime: invalid locale: ") {
			t.Errorf("NewLocale(%s) = %v", test.name, err)
		}
	}

	// Ambiguity is checked under the matching strategy.
	spec := prefixSpec()
	spec.AbbreviatedMonths[1] = "JANUARY"
	if _, err := strftime.NewLocale(spec); err != nil {
		t.Errorf("NewLocale(exact) = %v", err)
	}
	spec.CaseFold = true
	if _, err := strftime.NewLocale(spec); err == nil {
		t.Error("NewLocale(fold): expected ambiguity error")
	}
}

func TestNewLocale_copies(t *testing.T) {
	spec := prefixSpec()
	loc, err := strftime.NewLocale(spec)
	if err != nil {
		t.Fatal(err)
	}
	// Mutating the caller's slices and maps must not affect the locale.
	spec.Months[2] = "MUTATED"
	spec.AbbreviatedMonths[2] = "MUTATED"
	spec.Weekdays[1] = "MUTATED"
	spec.AbbreviatedWeekdays[1] = "MUTATED"
	spec.Formats['x'] = "%c"

	reference := time.Date(2009, 3, 2, 0, 0, 0, 0, time.UTC) // Monday
	if got := loc.Format("%A %B %b %x", reference); got != "Monday March Mar 02.03.2009" {
		t.Errorf("Format = %q, want %q", got, "Monday March Mar 02.03.2009")
	}
	if _, err := loc.Parse("%B", "March"); err != nil {
		t.Errorf("Parse = %v", err)
	}
}

func TestLocale_concurrent(t *testing.T) {
	loc1, err := strftime.NewLocale(chineseSpec())
	if err != nil {
		t.Fatal(err)
	}
	loc2, err := strftime.NewLocale(prefixSpec())
	if err != nil {
		t.Fatal(err)
	}
	reference := time.Date(2009, 8, 7, 18, 5, 4, 0, time.UTC) // Friday
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if i%2 == 0 {
					if got := loc1.Format("%A %B %p", reference); got != "星期五 八月 下午" {
						t.Errorf("loc1.Format = %q", got)
					}
				} else {
					if got := loc2.Format("%A %B %p", reference); got != "Friday August PM" {
						t.Errorf("loc2.Format = %q", got)
					}
					if _, err := loc2.Parse("%A, %B %d %Y", "Friday, August 07 2009"); err != nil {
						t.Errorf("loc2.Parse = %v", err)
					}
				}
			}
		}(i)
	}
	wg.Wait()
}
