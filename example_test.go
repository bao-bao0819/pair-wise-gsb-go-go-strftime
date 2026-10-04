package strftime_test

import (
	"fmt"
	"os"

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
	a := strftime.Analyze("%D %I:%M %Z")
	fmt.Println(a.Verdict)
	for _, issue := range a.Issues {
		fmt.Println(issue)
	}
	// Output:
	// conditional
	// missing: %I: 12-hour clock without %p: AM/PM is ambiguous
	// lossy: %y: two-digit year needs an external pivot window (Parse assumes 1969-2068)
	// lossy: clock is truncated to minute precision
	// lossy: %Z: a zone name cannot be mapped back to a unique UTC offset
}
