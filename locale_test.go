package strftime_test

import (
	"sync"
	"testing"
	"time"

	"github.com/ncruces/go-strftime"
)

// zhData builds the multi-byte Chinese locale fixture, where several
// abbreviations and full names share prefixes ("十" heads "十月",
// "十一月", "十二月").
func zhData() strftime.LocaleData {
	return strftime.LocaleData{
		Months: []string{
			"一月", "二月", "三月", "四月", "五月", "六月",
			"七月", "八月", "九月", "十月", "十一月", "十二月",
		},
		MonthAbbrev: []string{
			"一", "二", "三", "四", "五", "六",
			"七", "八", "九", "十", "十一", "十二",
		},
		Weekdays: []string{
			"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六",
		},
		WeekdayAbbrev: []string{
			"日", "一", "二", "三", "四", "五", "六",
		},
		AM: "上午",
		PM: "下午",
		Formats: map[string]string{
			"c": "%Y年%m月%d日 %A %H:%M:%S",
		},
	}
}

// chineseLocale uses multi-byte names where several abbreviations and
// full names share prefixes ("十" heads "十月", "十一月", "十二月").
func chineseLocale(t *testing.T) *strftime.Locale {
	t.Helper()
	l, err := strftime.NewLocale(zhData())
	if err != nil {
		t.Fatalf("NewLocale: %v", err)
	}
	return l
}

var localeRef = time.Date(2009, 8, 7, 6, 5, 4, 300000000, time.UTC) // Friday

func TestLocale_Format(t *testing.T) {
	l := chineseLocale(t)
	tests := []struct {
		format string
		want   string
	}{
		{"%A", "星期五"},
		{"%a", "五"},
		{"%B", "八月"},
		{"%b", "八"},
		{"%h", "八"},
		{"%c", "2009年08月07日 星期五 06:05:04"},
		{"%Y年%m月%d日", "2009年08月07日"},
		{"%I:%M:%S %p", "06:05:04 上午"},
		{"%H時%M", "06時05"},
		{"%a %b %e %T %Y", "五 八  7 06:05:04 2009"},
		{"%+", "五 八  7 06:05:04 UTC 2009"},
		{"%P", "上午"},
		{"literal %B text", "literal 八月 text"},
		{"%B百分之", "八月百分之"},
	}
	for _, tt := range tests {
		if got := l.Format(tt.format, localeRef); got != tt.want {
			t.Errorf("Locale.Format(%q) = %q, want %q", tt.format, got, tt.want)
		}
	}
}

func TestLocale_FormatPM(t *testing.T) {
	l := chineseLocale(t)
	pm := time.Date(2009, 8, 7, 18, 5, 4, 0, time.UTC)
	if got := l.Format("%I:%M:%S %p %P", pm); got != "06:05:04 下午 下午" {
		t.Errorf("Locale.Format PM = %q", got)
	}
	noon := time.Date(2009, 8, 7, 12, 0, 0, 0, time.UTC)
	if got := l.Format("%p", noon); got != "下午" {
		t.Errorf("Locale.Format noon = %q", got)
	}
	midnight := time.Date(2009, 8, 7, 0, 0, 0, 0, time.UTC)
	if got := l.Format("%p", midnight); got != "上午" {
		t.Errorf("Locale.Format midnight = %q", got)
	}
}

func TestLocale_ParseRoundTrip(t *testing.T) {
	l := chineseLocale(t)
	formats := []string{
		"%c",
		"%A %a %B %b %Y-%m-%d %H:%M:%S",
		"%Y年%m月%d日 %H:%M:%S.%L",
		"%Y年%m月%d日 %H:%M:%S.%f",
		"%Y年%m月%d日 %H:%M:%S.%N",
		"%Y-%m-%dT%H:%M:%S%:z",
		"%Y-%m-%dT%H:%M:%S%z",
		"%Y-%m-%dT%H:%M:%S %Z",
		"%I:%M:%S %p",
		"%A %Y-%m-%d",
		"%a %Y-%m-%d",
		"%B %d %Y",
		"%b %d %Y",
		"%Y/%m/%d %j",
		"%d-%b-%Y",
		"%e %B",
	}
	for _, f := range formats {
		s := l.Format(f, localeRef)
		got, err := l.Parse(f, s)
		if err != nil {
			t.Errorf("Locale.Parse(%q, %q): %v", f, s, err)
			continue
		}
		if back := l.Format(f, got); back != s {
			t.Errorf("roundtrip %q: parsed %v, reformatted %q != %q", f, got, back, s)
		}
	}
}

func TestLocale_ParseLongestPrefix(t *testing.T) {
	l := chineseLocale(t)
	// The matcher takes the longest name that can complete the current
	// directive: "十一月" wins over the shorter abbreviation "十一",
	// and the shared prefix "十" still resolves October alone.
	cases := []struct {
		format, value string
		month         time.Month
	}{
		{"%b|%Y", "十|2009", time.October},
		{"%b|%Y", "十一|2009", time.November},
		{"%b|%Y", "十二|2009", time.December},
		{"%B %d", "十一月 10", time.November},
		{"%B %d", "十二月 10", time.December},
		{"%B %d", "十月 10", time.October},
	}
	for _, tt := range cases {
		got, err := l.Parse(tt.format, tt.value)
		if err != nil {
			t.Errorf("Parse(%q,%q): %v", tt.format, tt.value, err)
			continue
		}
		if got.Month() != tt.month {
			t.Errorf("Parse(%q,%q) month = %v, want %v", tt.format, tt.value, got.Month(), tt.month)
		}
	}
}

func TestLocale_ParseCaseFold(t *testing.T) {
	// Latin names with multibyte fold-equivalent characters.
	l, err := strftime.NewLocale(strftime.LocaleData{
		Months: []string{
			"Janvier", "Février", "Mars", "Avril", "Mai", "Juin",
			"Juillet", "Août", "Septembre", "Octobre", "Novembre", "Décembre",
		},
		MonthAbbrev: []string{
			"jan", "fév", "mar", "avr", "mai", "jun",
			"jul", "aoû", "sep", "oct", "nov", "déc",
		},
		Weekdays: []string{
			"Dimanche", "Lundi", "Mardi", "Mercredi", "Jeudi", "Vendredi", "Samedi",
		},
		WeekdayAbbrev: []string{
			"dim", "lun", "mar", "mer", "jeu", "ven", "sam",
		},
		AM: "matin",
		PM: "soir",
	})
	if err != nil {
		t.Fatalf("NewLocale: %v", err)
	}

	// Upper-case input folds onto the lowercase-accented names.
	got, err := l.Parse("%B %d, %Y", "FÉVRIER 07, 2009")
	if err != nil {
		t.Fatalf("folded parse: %v", err)
	}
	if got.Month() != time.February {
		t.Errorf("folded month = %v", got.Month())
	}

	got, err = l.Parse("%b", "DÉC")
	if err != nil || got.Month() != time.December {
		t.Errorf("folded abbrev: %v %v", got, err)
	}

	if s := l.Format("%B", got); s != "Décembre" {
		t.Errorf("formatted folded month = %q", s)
	}
}

func TestLocale_ParseWeekdayConsistency(t *testing.T) {
	l := chineseLocale(t)
	// 2009-08-07 is a Friday ("星期五").
	if _, err := l.Parse("%A %Y-%m-%d", "星期五 2009-08-07"); err != nil {
		t.Errorf("consistent weekday: %v", err)
	}
	if _, err := l.Parse("%A %Y-%m-%d", "星期四 2009-08-07"); err == nil {
		t.Errorf("inconsistent weekday should fail")
	}
	// Weekday alone is accepted (no date to contradict it).
	if got, err := l.Parse("%A", "星期五"); err != nil || got.Year() != 0 {
		t.Errorf("weekday only should not constrain the date: %v %v", got, err)
	}
}

func TestLocale_OrdinaryLiterals(t *testing.T) {
	l := chineseLocale(t)
	// Latin letters adjacent to directives must match literally and
	// never be swallowed by the month/weekday name matcher.
	got, err := l.Parse("date:%Y-%m-%d;week:%A", "date:2009-08-07;week:星期五")
	if err != nil {
		t.Fatalf("literal parse: %v", err)
	}
	if y, m, d := got.Date(); y != 2009 || m != time.August || d != 7 {
		t.Errorf("literal date = %v", got)
	}
	if s := l.Format("date:%Y-%m-%d;week:%A", localeRef); s != "date:2009-08-07;week:星期五" {
		t.Errorf("literal format = %q", s)
	}
}

func TestLocale_CustomComposite(t *testing.T) {
	data := strftime.LocaleData{
		Months: []string{
			"一月", "二月", "三月", "四月", "五月", "六月",
			"七月", "八月", "九月", "十月", "十一月", "十二月",
		},
		MonthAbbrev: []string{
			"一", "二", "三", "四", "五", "六",
			"七", "八", "九", "十", "十一", "十二",
		},
		Weekdays: []string{
			"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六",
		},
		WeekdayAbbrev: []string{
			"日", "一", "二", "三", "四", "五", "六",
		},
		AM: "上午",
		PM: "下午",
	}

	data.Formats = map[string]string{
		"D": "%Y-%m-%d",
		"2": "%D %H:%M", // references another custom key
		"r": "%H:%M:%S", // overrides the built-in 12-hour combination
	}
	l, err := strftime.NewLocale(data)
	if err != nil {
		t.Fatalf("NewLocale nested: %v", err)
	}
	if got := l.Format("%2", localeRef); got != "2009-08-07 06:05" {
		t.Errorf("nested composite = %q", got)
	}
	if got := l.Format("%r %D", localeRef); got != "06:05:04 2009-08-07" {
		// Custom %r overrides the built-in.
		t.Errorf("override composite = %q", got)
	}
	if p, err := l.Parse("%2", "2009-08-07 06:05"); err != nil || p.Month() != time.August {
		t.Errorf("parse nested composite: %v %v", p, err)
	}

	// Direct self-reference is a cycle.
	data.Formats = map[string]string{"x": "%x"}
	if _, err := strftime.NewLocale(data); err == nil {
		t.Errorf("self-referential composite should fail")
	}

	// Indirect cycle is also rejected.
	data.Formats = map[string]string{"x": "%y", "y": "%x"}
	if _, err := strftime.NewLocale(data); err == nil {
		t.Errorf("indirect cycle should fail")
	}

	// Unknown directive inside a composite is rejected.
	data.Formats = map[string]string{"x": "%Q is %Y"}
	if _, err := strftime.NewLocale(data); err != nil {
		t.Errorf("format-only directive %%Q should be allowed: %v", err)
	}

	// A truly unknown directive is rejected at construction.
	data.Formats = map[string]string{"x": "%1 %Y"}
	if _, err := strftime.NewLocale(data); err == nil {
		t.Errorf("unknown directive in composite should fail")
	}
}

func TestLocale_NewErrors(t *testing.T) {
	good := func() strftime.LocaleData {
		return strftime.LocaleData{
			Months: []string{
				"一月", "二月", "三月", "四月", "五月", "六月",
				"七月", "八月", "九月", "十月", "十一月", "十二月",
			},
			MonthAbbrev: []string{
				"一", "二", "三", "四", "五", "六",
				"七", "八", "九", "十", "十一", "十二",
			},
			Weekdays: []string{
				"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六",
			},
			WeekdayAbbrev: []string{
				"日", "一", "二", "三", "四", "五", "六",
			},
			AM: "上午",
			PM: "下午",
		}
	}

	// Wrong slice lengths.
	d := good()
	d.Months = d.Months[:11]
	if _, err := strftime.NewLocale(d); err == nil {
		t.Errorf("short Months should fail")
	}

	// Empty entry.
	d = good()
	d.Weekdays[3] = ""
	if _, err := strftime.NewLocale(d); err == nil {
		t.Errorf("empty weekday should fail")
	}

	// Empty meridiem.
	d = good()
	d.PM = ""
	if _, err := strftime.NewLocale(d); err == nil {
		t.Errorf("empty PM should fail")
	}

	// Exact duplicate mapping to a different value.
	d = good()
	d.Months[2] = d.Months[1]
	if _, err := strftime.NewLocale(d); err == nil {
		t.Errorf("duplicate month should fail")
	}

	// Case-folded duplicate.
	d = good()
	d.MonthAbbrev[3] = "AUG" // folds onto "aug" (August)
	d.MonthAbbrev[7] = "aug"
	if _, err := strftime.NewLocale(d); err == nil {
		t.Errorf("folded duplicate should fail")
	}

	// Invalid format key.
	d = good()
	d.Formats = map[string]string{"xx": "%Y"}
	if _, err := strftime.NewLocale(d); err == nil {
		t.Errorf("bad format key should fail")
	}

	// Empty format value.
	d = good()
	d.Formats = map[string]string{"c": "   "}
	if _, err := strftime.NewLocale(d); err == nil {
		t.Errorf("empty format should fail")
	}
}

func TestLocale_DefensiveCopy(t *testing.T) {
	d := strftime.LocaleData{
		Months: []string{
			"一月", "二月", "三月", "四月", "五月", "六月",
			"七月", "八月", "九月", "十月", "十一月", "十二月",
		},
		MonthAbbrev: []string{
			"一", "二", "三", "四", "五", "六",
			"七", "八", "九", "十", "十一", "十二",
		},
		Weekdays: []string{
			"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六",
		},
		WeekdayAbbrev: []string{
			"日", "一", "二", "三", "四", "五", "六",
		},
		AM:      "上午",
		PM:      "下午",
		Formats: map[string]string{"c": "%Y"},
	}
	l, err := strftime.NewLocale(d)
	if err != nil {
		t.Fatal(err)
	}
	d.Months[0] = "MUTATED"
	d.MonthAbbrev[0] = "X"
	d.Formats["c"] = "%H"
	d.Formats["z"] = "%M"
	if got := l.Format("%B %c", time.Date(2009, 1, 2, 3, 4, 5, 0, time.UTC)); got != "一月 2009" {
		t.Errorf("locale was affected by caller mutation: %q", got)
	}
}

func TestLocale_Concurrent(t *testing.T) {
	l1 := chineseLocale(t)
	l2, err := strftime.NewLocale(strftime.LocaleData{
		Months: []string{
			"January", "February", "March", "April", "May", "June",
			"July", "August", "September", "October", "November", "December",
		},
		MonthAbbrev: []string{
			"Jan", "Feb", "Mar", "Apr", "May", "Jun",
			"Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
		},
		Weekdays: []string{
			"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday",
		},
		WeekdayAbbrev: []string{
			"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat",
		},
		AM: "AM",
		PM: "PM",
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = l1.Format("%c %A %B", localeRef)
			_, _ = l1.Parse("%Y-%m-%d", "2009-08-07")
		}()
		go func() {
			defer wg.Done()
			_ = l2.Format("%c %A %B", localeRef)
			_, _ = l2.Parse("%Y-%m-%d", "2009-08-07")
		}()
	}
	wg.Wait()
}

func TestEnglishLocale_Unchanged(t *testing.T) {
	// The package-level API must keep producing English names even
	// when a custom locale was constructed in the same program.
	_ = chineseLocale(t)
	cases := map[string]string{
		"%A":    "Friday",
		"%a":    "Fri",
		"%B":    "August",
		"%b":    "Aug",
		"%p":    "AM",
		"%P":    "am",
		"%c":    "Fri Aug  7 06:05:04 2009",
		"%+":    "Fri Aug  7 06:05:04 UTC 2009",
		"%FT%T": "2009-08-07T06:05:04",
	}
	for f, want := range cases {
		if got := strftime.Format(f, localeRef); got != want {
			t.Errorf("Format(%q) = %q, want %q", f, got, want)
		}
	}
	if got, err := strftime.Parse("%Y-%m-%d %A", "2009-08-07 Friday"); err != nil || got.Year() != 2009 {
		t.Errorf("english parse: %v %v", got, err)
	}
}
