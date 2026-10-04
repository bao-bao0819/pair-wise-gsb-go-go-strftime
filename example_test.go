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

func ExampleLocale_Format() {
	locale, err := strftime.NewLocale(strftime.LocaleSpec{
		Months: []string{
			"janvier", "février", "mars", "avril", "mai", "juin",
			"juillet", "août", "septembre", "octobre", "novembre", "décembre",
		},
		AbbreviatedMonths: []string{
			"janv.", "févr.", "mars", "avr.", "mai", "juin",
			"juil.", "août", "sept.", "oct.", "nov.", "déc.",
		},
		Weekdays: []string{
			"dimanche", "lundi", "mardi", "mercredi",
			"jeudi", "vendredi", "samedi",
		},
		AbbreviatedWeekdays: []string{
			"dim.", "lun.", "mar.", "mer.", "jeu.", "ven.", "sam.",
		},
		AM: "AM",
		PM: "PM",
		Formats: map[byte]string{
			'x': "%d/%m/%Y",
		},
	})
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		return
	}
	t := time.Date(2009, time.August, 7, 18, 5, 4, 0, time.UTC)
	fmt.Println(locale.Format("%A %d %B %Y", t))
	fmt.Println(locale.Format("%x", t))
	// Output:
	// vendredi 07 août 2009
	// 07/08/2009
}
