package strftime_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ncruces/go-strftime"
)

// A format whose literal prefix contains text that the old Go-layout
// conversion explicitly forbids: digits (2006), weekday (Mon), zone (MST),
// meridian (PM), and non-ASCII runes (中文).
const literalPrefixFormat = `version2006 Mon MST PM 中文 prefix %Y-%m-%d %H:%M:%S`

func TestCompile_LiteralPrefixRoundTrip(t *testing.T) {
	spec, err := strftime.Compile(literalPrefixFormat)
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}

	want := time.Date(2024, time.March, 9, 7, 6, 5, 0, time.UTC)
	text := spec.Format(want)
	const expect = "version2006 Mon MST PM 中文 prefix 2024-03-09 07:06:05"
	if text != expect {
		t.Fatalf("Format() = %q, want %q", text, expect)
	}

	got, err := spec.Parse(text)
	if err != nil {
		t.Fatalf("Parse(%q) = %v", text, err)
	}
	if !got.Equal(want) {
		t.Fatalf("Parse(%q) = %v, want %v", text, got, want)
	}
}

func TestCompile_OldParseRejectsReservedLiterals(t *testing.T) {
	// The legacy implementation rejected these literals at compile time.
	if _, err := strftime.Parse(literalPrefixFormat, ""); err == nil {
		t.Fatalf("legacy Parse should reject reserved literals, got nil")
	}
	// The compiled implementation accepts them.
	if _, err := strftime.Compile(literalPrefixFormat); err != nil {
		t.Fatalf("Compile() = %v", err)
	}
}

func TestCompile_CompoundExpansion(t *testing.T) {
	ref := time.Date(2009, time.August, 7, 6, 5, 4, 300000000, time.UTC)
	tests := []struct {
		format string
		text   string
	}{
		{"%c", "Fri Aug  7 06:05:04 2009"},
		{"%+", "Fri Aug  7 06:05:04 UTC 2009"},
		{"%D", "08/07/09"},
		{"%F", "2009-08-07"},
		{"%v", " 7-Aug-2009"},
		{"%r", "06:05:04 AM"},
		{"%R", "06:05"},
		{"%T", "06:05:04"},
		{"%x", "08/07/09"},
		{"%X", "06:05:04"},
	}
	for _, tt := range tests {
		spec, err := strftime.Compile(tt.format)
		if err != nil {
			t.Errorf("Compile(%q) = %v", tt.format, err)
			continue
		}
		if got := spec.Format(ref); got != tt.text {
			t.Errorf("Format(%q) = %q, want %q", tt.format, got, tt.text)
		}
		// Compounds must also accept unpadded input.
		unpadded := map[string]string{
			"%c": "Fri Aug 7 6:5:4 2009",
			"%D": "8/7/09",
			"%F": "2009-8-7",
			"%v": "7-Aug-2009",
			"%r": "6:5:4 AM",
			"%R": "6:5",
			"%T": "6:5:4",
		}
		if in, ok := unpadded[tt.format]; ok {
			if _, err := spec.Parse(in); err != nil {
				t.Errorf("Parse(%q, %q) = %v", tt.format, in, err)
			}
		}
		// Round trip.
		back, err := spec.Parse(tt.text)
		if err != nil {
			t.Errorf("Parse(%q) = %v", tt.format, err)
			continue
		}
		if got := spec.Format(back); got != tt.text {
			t.Errorf("round trip(%q) = %q, want %q", tt.format, got, tt.text)
		}
	}
}

func TestCompile_TrailingInput(t *testing.T) {
	spec, err := strftime.Compile("%Y-%m-%d")
	if err != nil {
		t.Fatal(err)
	}
	_, err = spec.Parse("2024-01-02extra")
	if err == nil {
		t.Fatal("Parse() unexpectedly ignored trailing input")
	}
	var pe *strftime.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error is %T, want *strftime.ParseError", err)
	}
	if pe.ValueOffset != len("2024-01-02") {
		t.Errorf("ValueOffset = %d, want %d", pe.ValueOffset, len("2024-01-02"))
	}
}

func TestCompile_PercentAndLiterals(t *testing.T) {
	spec, err := strftime.Compile(`100%% done%%%n%t"2006"`)
	if err != nil {
		t.Fatal(err)
	}
	const want = "100% done%\n\t\"2006\""
	if got := spec.Format(time.Time{}); got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
	if _, err := spec.Parse(want); err != nil {
		t.Fatalf("Parse() = %v", err)
	}
}

func TestCompile_RepeatedFieldsMustAgree(t *testing.T) {
	tests := []struct {
		format, value string
	}{
		{"%Y %Y", "2024 2025"},
		{"%m %m", "01 02"},
		{"%d %d", "01 02"},
		{"%j %j", "001 002"},
		{"%H %H", "01 02"},
		{"%M %M", "01 02"},
		{"%S %S", "01 02"},
		{"%p %p", "AM PM"},
		{"%Y-%m-%d %j", "2024-01-10 050"},
	}
	for _, tt := range tests {
		spec, err := strftime.Compile(tt.format)
		if err != nil {
			t.Errorf("Compile(%q) = %v", tt.format, err)
			continue
		}
		if _, err := spec.Parse(tt.value); err == nil {
			t.Errorf("Parse(%q, %q) expected conflict error", tt.format, tt.value)
		}
	}

	// Consistent repetitions are accepted.
	spec, _ := strftime.Compile("%Y %m %d")
	if _, err := spec.Parse("2024 01 02"); err != nil {
		t.Errorf("consistent repetition: %v", err)
	}
}

func TestCompile_ErrorOffsets(t *testing.T) {
	_, err := strftime.Compile("%Y %C")
	var ce *strftime.CompileError
	if !errors.As(err, &ce) {
		t.Fatalf("Compile error is %T, want *strftime.CompileError", err)
	}
	if ce.Directive != "%C" || ce.FormatOffset != 3 {
		t.Errorf("CompileError = %q @%d, want %%C @3", ce.Directive, ce.FormatOffset)
	}

	// A rejected combination directive points at the combination.
	_, err = strftime.Compile("%Y%Q")
	if !errors.As(err, &ce) || ce.FormatOffset != 2 || ce.Directive != "%Q" {
		t.Errorf("combination error = %q @%d", ce.Directive, ce.FormatOffset)
	}

	// Input errors identify the directive and input offset.
	spec, _ := strftime.Compile("%Y-%m-%d")
	_, err = spec.Parse("2024-xx-01")
	var pe *strftime.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("Parse error is %T, want *strftime.ParseError", err)
	}
	if pe.Directive != "%m" || pe.FormatOffset != 3 || pe.ValueOffset != 5 {
		t.Errorf("ParseError directive=%q fmt@%d in@%d", pe.Directive, pe.FormatOffset, pe.ValueOffset)
	}
}

func TestCompile_UnsupportedSpecifiers(t *testing.T) {
	for _, format := range []string{"%g", "%k", "%l", "%s", "%u", "%w", "%C", "%G", "%Q", "%U", "%V", "%W", "%q"} {
		if _, err := strftime.Compile(format); err == nil {
			t.Errorf("Compile(%q) expected error", format)
		}
	}
	// Fractional seconds require a separator.
	if _, err := strftime.Compile("%L"); err == nil {
		t.Error("Compile(%L) without separator expected error")
	}
	if _, err := strftime.Compile("%S.%L"); err != nil {
		t.Errorf("Compile(%%S.%%L) = %v", err)
	}
}

func TestSpec_ConcurrentUse(t *testing.T) {
	spec, err := strftime.Compile("[%Y-%m-%d %H:%M:%S.%L] %z")
	if err != nil {
		t.Fatal(err)
	}

	const goroutines, iterations = 64, 500
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			base := time.Date(2024, 1, 2, 3, 4, 5, 600000000, time.UTC).
				Add(time.Duration(g) * time.Second)
			for i := 0; i < iterations; i++ {
				tm := base.Add(time.Duration(i) * time.Millisecond)
				text := spec.Format(tm)
				back, err := spec.Parse(text)
				if err != nil {
					errs <- err
					return
				}
				if !back.Equal(tm) {
					t.Errorf("g=%d i=%d: Parse(Format()) = %v, want %v", g, i, back, tm)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestSpec_AppendFormatReusesBuffer(t *testing.T) {
	spec, err := strftime.Compile("%Y-%m-%d")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 0, 32)
	buf = append(buf, "prefix "...)
	buf = spec.AppendFormat(buf, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))
	if got, want := string(buf), "prefix 2024-01-02"; got != want {
		t.Errorf("AppendFormat() = %q, want %q", got, want)
	}
}

func TestSpec_FractionDirectives(t *testing.T) {
	ref := time.Date(2009, 8, 7, 6, 5, 4, 300000000, time.UTC)
	for _, tt := range []struct {
		format, text string
	}{
		{"%S.%L", "04.300"},
		{"%S,%f", "04,300000"},
		{"%S.%N", "04.300000000"},
	} {
		spec, err := strftime.Compile(tt.format)
		if err != nil {
			t.Errorf("Compile(%q) = %v", tt.format, err)
			continue
		}
		if got := spec.Format(ref); got != tt.text {
			t.Errorf("Format(%q) = %q, want %q", tt.format, got, tt.text)
		}
		got, err := spec.Parse(tt.text)
		if err != nil {
			t.Errorf("Parse(%q) = %v", tt.format, err)
			continue
		}
		if got.Nanosecond() != ref.Nanosecond() {
			t.Errorf("Parse(%q) nsec = %d, want %d", tt.format, got.Nanosecond(), ref.Nanosecond())
		}
	}
}
