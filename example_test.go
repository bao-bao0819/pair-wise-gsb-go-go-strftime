package strftime_test

import (
	"fmt"
	"os"
	"time"

	strftime "github.com/ncruces/go-strftime"
)

func ExampleLayout() {
	layout, err := strftime.Layout("%Y-%m-%d %H:%M:%S")
	if err != nil {
		fmt.Fprint(os.Stderr, err)
	} else {
		fmt.Printf("%q", layout)
	}
	// Output:
	// "2006-01-02 15:04:05"
}

func ExampleAnalyze() {
	r := strftime.Analyze("%I:%M:%S")
	fmt.Println(r.Verdict)
	for _, f := range r.Findings {
		fmt.Println(f.Code)
	}
	// Output:
	// conditional
	// 12_hour_no_meridian
	// missing_date
	// missing_zone
	// subsecond_truncated
}

func ExampleReport_RoundTrip() {
	report := strftime.Analyze("%F %T")
	t := time.Date(2009, 8, 7, 6, 5, 4, 300000000, time.UTC)
	rt := report.RoundTrip(t)
	fmt.Println(report.Verdict, rt.FractionLost, rt.ZoneDifference)
	// Output:
	// conditional 300ms 0s
}
