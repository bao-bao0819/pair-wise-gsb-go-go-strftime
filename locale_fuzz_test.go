//go:build go1.18

package strftime_test

import (
	"testing"

	"github.com/ncruces/go-strftime"
)

func FuzzLocaleParse(f *testing.F) {
	l, err := strftime.NewLocale(zhData())
	if err != nil {
		f.Fatal(err)
	}
	seeds := []string{
		"2009-08-07",
		"星期五 2009-08-07",
		"2009年08月07日",
		"十一月 10",
		"十二",
		"06:05:04 上午",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	formats := []string{
		"%Y-%m-%d",
		"%A %Y-%m-%d",
		"%Y年%m月%d日",
		"%B %d",
		"%b",
		"%H:%M:%S %p",
	}
	f.Fuzz(func(t *testing.T, value string) {
		for _, format := range formats {
			tt, err := l.Parse(format, value)
			if err != nil && !tt.IsZero() {
				t.Errorf("Parse(%q,%q) non-zero on error", format, value)
			}
		}
	})
}

func FuzzNewLocale(f *testing.F) {
	f.Add("一月x")
	f.Fuzz(func(t *testing.T, name string) {
		months := []string{
			"一月", "二月", "三月", "四月", "五月", "六月",
			"七月", "八月", "九月", "十月", "十一月", "十二月",
		}
		months[0] = name
		d := zhData()
		d.Months = months
		// Must never panic; error is acceptable.
		_, _ = strftime.NewLocale(d)
	})
}
