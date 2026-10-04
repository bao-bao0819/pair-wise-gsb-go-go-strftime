# `strftime`/`strptime` compatible time formatting and parsing for Go

[![Go Reference](https://pkg.go.dev/badge/image)](https://pkg.go.dev/github.com/ncruces/go-strftime)
[![Go Coverage](https://github.com/ncruces/go-strftime/wiki/coverage.svg)](https://raw.githack.com/wiki/ncruces/go-strftime/coverage.html)

## Localized month, weekday, and AM/PM names

The package functions always render English names and never read the OS
locale. To localize `%A %a %B %b %p %P` (and combinations like `%c`),
build an explicit `Locale`:

```go
loc, err := strftime.NewLocale(strftime.LocaleData{
    Months:        []string{ /* 12 full names, January first */ },
    MonthAbbrev:   []string{ /* 12 abbreviations */ },
    Weekdays:      []string{ /* 7 full names, Sunday first */ },
    WeekdayAbbrev: []string{ /* 7 abbreviations */ },
    AM:            "vorm.",
    PM:            "nachm.",
    Formats:       map[string]string{"c": "%a %d %b %Y %H:%M:%S"},
})
text := loc.Format("%c", time.Now())
when, err := loc.Parse("%c", text)
```

`NewLocale` copies the inputs and rejects empty entries, names that are
ambiguous under exact or case-insensitive matching, and cyclic
combination formats. Parsing takes the longest name that completes a
directive (multi-byte safe) and verifies that a parsed weekday agrees
with the parsed date. A `Locale` is safe for concurrent use.
