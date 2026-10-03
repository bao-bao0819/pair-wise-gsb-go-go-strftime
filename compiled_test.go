package strftime_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ncruces/go-strftime"
)

var ref = time.Date(2009, 8, 7, 6, 5, 4, 300000000, time.UTC)

// A literal prefix that contains every token the Go layout engine treats as
// a directive: digits ("2006"), a weekday ("Mon"), a zone ("MST"), a
// meridian ("PM") and multi-byte UTF-8 text ("中文").
func TestCompiled_LiteralRoundTrip(t *testing.T) {
	format := "字面 2006 Mon MST PM v%Y-%m-%d %H:%M:%S"
	c, err := strftime.Compile(format)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	want := "字面 2006 Mon MST PM v2009-08-07 06:05:04"
	if got := c.Format(ref); got != want {
		t.Fatalf("Format = %q, want %q", got, want)
	}

	got, err := c.Parse(want)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if y, m, d := got.Date(); y != 2009 || m != 8 || d != 7 {
		t.Fatalf("date = %v", got)
	}
	if h, mi, s := got.Clock(); h != 6 || mi != 5 || s != 4 {
		t.Fatalf("clock = %v", got)
	}
	if got2 := c.Format(got); got2 != want {
		t.Fatalf("round trip = %q, want %q", got2, want)
	}
}

func TestCompiled_TrailingInput(t *testing.T) {
	c := strftime.MustCompile("%Y-%m-%d")
	if _, err := c.Parse("2009-08-07extra"); err == nil {
		t.Fatal("expected error for trailing input")
	} else {
		var pe *strftime.ParseError
		if !errors.As(err, &pe) || pe.Offset != 10 {
			t.Fatalf("err = %v", err)
		}
		if !strings.Contains(err.Error(), "extra text") {
			t.Fatalf("err = %v", err)
		}
	}
	if _, err := c.Parse("2009-08-07"); err != nil {
		t.Fatalf("exact match: %v", err)
	}
}

func TestCompiled_PercentPercent(t *testing.T) {
	c := strftime.MustCompile("100%% done %Y")
	want := "100% done 2009"
	if got := c.Format(ref); got != want {
		t.Fatalf("Format = %q", got)
	}
	if got, err := c.Parse(want); err != nil || got.Year() != 2009 {
		t.Fatalf("Parse = %v %v", got, err)
	}
}

func TestCompiled_CompileErrorOffset(t *testing.T) {
	// CompileParse rejects parse-unsupported directives at compile time,
	// reporting the directive and its byte offset in the format.
	if _, err := strftime.CompileParse("ab%C"); err == nil {
		t.Fatal("expected CompileError")
	} else {
		var ce *strftime.CompileError
		if !errors.As(err, &ce) || ce.Offset != 2 || ce.Directive != "%C" {
			t.Fatalf("err = %v", err)
		}
	}
	// Format-only compilation still accepts %C.
	if c, err := strftime.Compile("ab%C"); err != nil || c == nil {
		t.Fatalf("Compile(ab%%C) = %v %v", c, err)
	}

	// %C is valid for formatting but not for parsing; the Compiled parse
	// path reports the directive and its format byte offset.
	c := strftime.MustCompile("ab%C")
	if _, err := c.Parse("ab20"); err == nil {
		t.Fatal("expected compile-style error")
	} else if !strings.Contains(err.Error(), "%C") ||
		!strings.Contains(err.Error(), "offset 2") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompiled_InputErrorPointsAtDirective(t *testing.T) {
	c := strftime.MustCompile("%Y-%m-%d")
	_, err := c.Parse("2009-xx-07")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "%m") ||
		!strings.Contains(err.Error(), "input offset 5") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompiled_DuplicateFieldsMustAgree(t *testing.T) {
	c := strftime.MustCompile("%Y-%m-%d %H:%M:%S %Y")
	if _, err := c.Parse("2009-08-07 06:05:04 2009"); err != nil {
		t.Errorf("consistent duplicate: %v", err)
	}
	if _, err := c.Parse("2009-08-07 06:05:04 2010"); err == nil {
		t.Error("conflicting year accepted")
	}

	c2 := strftime.MustCompile("%m %m")
	if _, err := c2.Parse("08 08"); err != nil {
		t.Errorf("consistent month: %v", err)
	}
	if _, err := c2.Parse("08 09"); err == nil {
		t.Error("conflicting month accepted")
	}

	// Day of year must agree with month/day.
	c3 := strftime.MustCompile("%Y-%m-%d %j")
	if _, err := c3.Parse("2009-08-07 219"); err != nil {
		t.Errorf("consistent yday: %v", err)
	}
	if _, err := c3.Parse("2009-08-07 220"); err == nil {
		t.Error("conflicting yday accepted")
	}
}

func TestCompiled_CompoundExpansion(t *testing.T) {
	// %F/%T are expanded at compile time; their literal parts ("-", ":", "T")
	// are matched verbatim.
	c := strftime.MustCompile("%FT%T")
	got, err := c.Parse("2009-08-07T06:05:04")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Format(got) != "2009-08-07T06:05:04" {
		t.Fatalf("Format = %q", c.Format(got))
	}

	// Numeric fields stay lenient (1-2 digits), like the old Go-layout parse.
	got2, err := c.Parse("2009-8-7T6:5:4")
	if err != nil {
		t.Fatalf("Parse lenient: %v", err)
	}
	if got2 != time.Date(2009, 8, 7, 6, 5, 4, 0, time.UTC) {
		t.Fatalf("got %v", got2)
	}
}

func TestCompiled_CompoundErrorPointsOutward(t *testing.T) {
	c := strftime.MustCompile("%FT%T")
	_, err := c.Parse("2009-08-07Txx:05:04")
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *strftime.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("not a ParseError: %v", err)
	}
	// The failure belongs to the outer %T directive, at the hour offset.
	if pe.Directive != "%T" || pe.Offset != 11 {
		t.Fatalf("Directive=%q Offset=%d (%v)", pe.Directive, pe.Offset, err)
	}
}

func TestCompiled_UTF8LiteralOffsets(t *testing.T) {
	c := strftime.MustCompile("中文字面%Y")
	if got := c.Format(ref); got != "中文字面2009" {
		t.Fatalf("Format = %q", got)
	}
	got, err := c.Parse("中文字面2009")
	if err != nil || got.Year() != 2009 {
		t.Fatalf("Parse = %v %v", got, err)
	}
	_, err = c.Parse("中文字面abcd")
	var pe *strftime.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("not a ParseError: %v", err)
	}
	// "中文字面" is 12 bytes; the error offset is a byte offset.
	if pe.Offset != 12 || pe.Directive != "%Y" {
		t.Fatalf("Offset=%d Directive=%q (%v)", pe.Offset, pe.Directive, err)
	}
}

// A single Compiled value used concurrently must not share or corrupt
// per-call buffers.
func TestCompiled_Concurrent(t *testing.T) {
	c := strftime.MustCompile("2006 Mon MST PM 中文 %Y-%m-%dT%H:%M:%S.%N%:z")

	times := []time.Time{
		time.Date(2009, 8, 7, 6, 5, 4, 300000000, time.UTC),
		time.Date(1999, 12, 31, 23, 59, 59, 1, time.FixedZone("", 8*3600)),
		time.Date(1, 2, 3, 4, 5, 6, 7, time.UTC),
	}

	var wg sync.WaitGroup
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				tm := times[(g+i)%len(times)]
				var buf []byte
				buf = c.AppendFormat(buf, tm)
				var other []byte
				other = c.AppendFormat(other, times[(g+i+1)%len(times)])
				got, err := c.Parse(string(buf))
				if err != nil {
					t.Errorf("Parse(%q): %v", buf, err)
					return
				}
				if !got.Equal(tm) {
					t.Errorf("got %v, want %v (formatted %q)", got, tm, buf)
					return
				}
				// A second independent call must produce its own value;
				// shared internal storage would scramble either buffer.
				wantOther := c.Format(times[(g+i+1)%len(times)])
				if string(other) != wantOther {
					t.Errorf("concurrent buffer = %q, want %q", other, wantOther)
					return
				}
				if c.Format(tm) != string(buf) {
					t.Errorf("first buffer mutated: %q", buf)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestCompiled_LiteralDirectives(t *testing.T) {
	c := strftime.MustCompile("a%nb%tc%%d")
	want := "a\nb\tc%d"
	if got := c.Format(time.Time{}); got != want {
		t.Fatalf("Format = %q, want %q", got, want)
	}
	got, err := c.Parse(want)
	if err != nil || !got.Equal(time.Date(0, time.January, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Parse = %v %v", got, err)
	}
}

func TestCompiled_UnknownDirectiveFormatCompat(t *testing.T) {
	// Format keeps the old verbatim behavior for unknown directives.
	if got := strftime.Format("%q x", time.Time{}); got != "%q x" {
		t.Fatalf("Format = %q", got)
	}
}
