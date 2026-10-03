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

func ExampleCompile() {
	// Literals may contain text such as "2006", "Mon" or "MST".
	spec, err := strftime.Compile("version2006 Mon MST %Y-%m-%d")
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		return
	}
	t, err := spec.Parse("version2006 Mon MST 2024-03-09")
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		return
	}
	fmt.Print(t.Format(time.DateOnly))
	// Output:
	// 2024-03-09
}
