package strftime_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ncruces/go-strftime"
)

func codeSet(r *strftime.Report) map[strftime.FindingCode]bool {
	m := make(map[strftime.FindingCode]bool)
	for _, f := range r.Findings {
		m[f.Code] = true
	}
	return m
}

// TestAnalyzeExpansion checks that composite directives are analyzed as
// their expansions: %F must behave like %Y-%m-%d, not like an opaque token.
func TestAnalyzeExpansion(t *testing.T) {
	tests := []struct {
		composite   string
		expansion   string
		wantVerdict strftime.Verdict
	}{
		{"%F", "%Y-%m-%d", strftime.Conditional},
		{"%T", "%H:%M:%S", strftime.Conditional},
		{"%R", "%H:%M", strftime.Conditional},
		{"%D", "%m/%d/%y", strftime.Conditional},
		{"%x", "%m/%d/%y", strftime.Conditional},
		{"%X", "%H:%M:%S", strftime.Conditional},
	}
	for _, tt := range tests {
		got := strftime.Analyze(tt.composite)
		want := strftime.Analyze(tt.expansion)
		if got.Verdict != want.Verdict {
			t.Errorf("Analyze(%q).Verdict = %s, want %s (same as %q)",
				tt.composite, got.Verdict, want.Verdict, tt.expansion)
		}
	}

	// %c expands to a full date+time (no zone): its atomic pieces must be
	// reported with the composite named as the source.
	r := strftime.Analyze("%c")
	if !r.Date.Present || !r.Time.Present || r.Zone.Present {
		t.Errorf("%%c date/time/zone = %v/%v/%v", r.Date.Present, r.Time.Present, r.Zone.Present)
	}
	if len(r.Date.Years) != 1 || r.Date.Years[0] != "%Y" {
		t.Errorf("%%c year directives = %v", r.Date.Years)
	}
	// Expanded directives report the atomic token and name %c as Source.
	if r.Date.Years[0] != "%Y" {
		t.Fatalf("expanded year token = %q", r.Date.Years[0])
	}
	// The weekday carried by %c is derivable: the redundant note must
	// point at %a and name %c as its source.
	var redundant *strftime.Finding
	for i := range r.Findings {
		if r.Findings[i].Code == strftime.FindingRedundantField {
			redundant = &r.Findings[i]
		}
	}
	if redundant == nil {
		t.Fatalf("%%c should note the redundant weekday: %v", r.Findings)
	}
	if redundant.Directive != "%a" || redundant.Source != "%c" {
		t.Errorf("redundant finding = %q source=%q, want %%a from %%c",
			redundant.Directive, redundant.Source)
	}

	// %r expands to a 12-hour time *with* a meridian, so the meridian
	// warning must not fire.
	rr := strftime.Analyze("%r")
	if codeSet(rr)[strftime.Finding12HourNoMeridian] {
		t.Errorf("%%r should not report missing meridian: %v", rr.Findings)
	}
}

// TestAnalyzeSameSurfaceLength groups formats that print the same number
// of characters for the reference time but carry different information.
func TestAnalyzeSameSurfaceLength(t *testing.T) {
	ref := time.Date(2009, 8, 7, 6, 5, 4, 300000000, time.UTC)

	groups := []struct {
		surface int
		// formats print the same number of characters for ref but
		// carry different information.
		formats []string
	}{
		{
			surface: 8,
			formats: []string{"%H:%M:%S", "%I:%M:%S", "%Y%m%d", "%m/%d/%y"},
		},
		{
			surface: 26,
			// Same 26 chars: one has fraction but no zone, the other
			// has a zone name but no fraction.
			formats: []string{"%Y-%m-%dT%H:%M:%S.%f", "%Y-%m-%dT%H:%M:%S %:z"},
		},
		{
			surface: 24,
			// Fixed numeric offset vs. no zone at all.
			formats: []string{"%Y-%m-%dT%H:%M:%S%z", "%a %b %d %H:%M:%S %Y"},
		},
	}

	for _, g := range groups {
		t.Run(fmt.Sprint(g.surface), func(t *testing.T) {
			signatures := map[string]strftime.Verdict{}
			for _, f := range g.formats {
				out := strftime.Format(f, ref)
				if len(out) != g.surface {
					t.Fatalf("Format(%q, ref) = %q (len %d), want surface len %d",
						f, out, len(out), g.surface)
				}
				r := strftime.Analyze(f)
				var sig []string
				for c := range codeSet(r) {
					sig = append(sig, string(c))
				}
				sort.Strings(sig)
				key := strings.Join(sig, ",")
				if other, dup := signatures[key]; dup {
					t.Errorf("formats %q and %q both print %d chars and share the analysis %s; "+
						"the group must contrast formats with different information",
						f, "<previous>", g.surface, other)
				}
				signatures[key] = r.Verdict
			}
		})
	}

	// The key assertion: the two 8-char time strings carry different
	// information, reflected in the 12-hour warning.
	plain := strftime.Analyze("%H:%M:%S")
	twelve := strftime.Analyze("%I:%M:%S")
	if codeSet(plain)[strftime.Finding12HourNoMeridian] {
		t.Errorf("%%H:%%M:%%S must not report a meridian problem")
	}
	if !codeSet(twelve)[strftime.Finding12HourNoMeridian] {
		t.Errorf("%%I:%%M:%%S must report a missing meridian: %v", twelve.Findings)
	}
	if plain.Time.Hour24 != true || twelve.Time.Hour12 != true {
		t.Errorf("hour kinds plain=%v/%v twelve=%v/%v",
			plain.Time.Hour24, plain.Time.Hour12, twelve.Time.Hour24, twelve.Time.Hour12)
	}

	// The 10-char date pair differs only by the two-digit-year window.
	full := strftime.Analyze("%F")
	short := strftime.Analyze("%m/%d/%y")
	if codeSet(full)[strftime.FindingTwoDigitYear] {
		t.Errorf("%%F must not report a two-digit year")
	}
	if !codeSet(short)[strftime.FindingTwoDigitYear] {
		t.Errorf("%%m/%%d/%%y must report a two-digit year: %v", short.Findings)
	}

	// %:z uniquely fixes the offset; %f fixes microseconds but no zone.
	offset := strftime.Analyze("%Y-%m-%dT%H:%M:%S%:z")
	fraction := strftime.Analyze("%Y-%m-%dT%H:%M:%S.%f")
	if codeSet(offset)[strftime.FindingMissingZone] || !offset.Zone.Present {
		t.Errorf("offset format must carry a zone: %v", offset.Zone)
	}
	if !codeSet(fraction)[strftime.FindingMissingZone] || fraction.Zone.Present {
		t.Errorf("fraction format must miss a zone: %v", fraction.Zone)
	}
	if fraction.Precision != strftime.MicrosecondPrecision {
		t.Errorf("precision = %s, want microsecond", fraction.Precision)
	}
	if offset.Precision != strftime.SecondPrecision {
		t.Errorf("precision = %s, want second", offset.Precision)
	}
}

func TestRoundTrip(t *testing.T) {
	tz := time.FixedZone("CEST", 2*3600)
	ref := time.Date(2009, 8, 7, 6, 5, 4, 300000000, tz)

	t.Run("exact", func(t *testing.T) {
		r := strftime.Analyze("%Y-%m-%dT%H:%M:%S.%N%:z")
		if r.Verdict != strftime.Reversible {
			t.Fatalf("verdict = %s", r.Verdict)
		}
		rt := r.RoundTrip(ref)
		if rt.ParseError != nil || !rt.Exact || rt.InstantDifference != 0 ||
			rt.ZoneDifference != 0 || rt.FractionLost != 0 {
			t.Fatalf("round trip = %+v", rt)
		}
	})

	t.Run("fraction truncated", func(t *testing.T) {
		r := strftime.Analyze("%F %T")
		rt := r.RoundTrip(ref)
		if rt.ParseError != nil {
			t.Fatal(rt.ParseError)
		}
		// Static conclusion stays conditional even though the sample
		// loses exactly the fraction.
		if r.Verdict != strftime.Conditional {
			t.Fatalf("verdict changed to %s", r.Verdict)
		}
		if rt.FractionLost != 300*time.Millisecond {
			t.Errorf("FractionLost = %v, want 300ms", rt.FractionLost)
		}
		if rt.Differences[strftime.FindingSubsecondTruncated] != 300*time.Millisecond {
			t.Errorf("differences = %v", rt.Differences)
		}
	})

	t.Run("zone assumed UTC", func(t *testing.T) {
		rt := strftime.Analyze("%F %T").RoundTrip(ref)
		if rt.ZoneDifference != -2*time.Hour {
			t.Errorf("ZoneDifference = %v, want -2h", rt.ZoneDifference)
		}
	})

	t.Run("two digit year sample passes, verdict stays", func(t *testing.T) {
		r := strftime.Analyze("%D %T")
		rt := r.RoundTrip(ref)
		if rt.ParseError != nil {
			t.Fatal(rt.ParseError)
		}
		// 2009 is inside Go's 2-digit window for this sample, so the
		// date round-trips - but the static warning must remain.
		if !codeSet(r)[strftime.FindingTwoDigitYear] {
			t.Fatalf("two-digit-year warning missing: %v", r.Findings)
		}
	})

	t.Run("12h morning sample passes, verdict stays", func(t *testing.T) {
		r := strftime.Analyze("%I:%M:%S")
		rt := r.RoundTrip(ref)
		if rt.ParseError != nil {
			t.Fatal(rt.ParseError)
		}
		if !codeSet(r)[strftime.Finding12HourNoMeridian] {
			t.Fatalf("meridian warning missing after round trip: %v", r.Findings)
		}
	})

	t.Run("12h afternoon sample fails to round-trip", func(t *testing.T) {
		pm := time.Date(2009, 8, 7, 18, 5, 4, 0, time.UTC)
		rt := strftime.Analyze("%Y-%m-%d %I:%M:%S").RoundTrip(pm)
		if rt.InstantDifference != 12*time.Hour {
			t.Errorf("InstantDifference = %v, want 12h", rt.InstantDifference)
		}
		if rt.Differences[strftime.Finding12HourNoMeridian] != 12*time.Hour {
			t.Errorf("12h attribution = %v", rt.Differences)
		}
	})

	t.Run("two digit year outside window", func(t *testing.T) {
		old := time.Date(1968, 8, 7, 0, 0, 0, 0, time.UTC)
		r := strftime.Analyze("%m/%d/%y")
		rt := r.RoundTrip(old)
		if rt.ParseError != nil {
			t.Fatal(rt.ParseError)
		}
		if rt.Differences[strftime.FindingTwoDigitYear] == 0 {
			t.Errorf("two-digit-year attribution missing: %v", rt.Differences)
		}
		// Static verdict must not be "repaired" by the sample.
		if r.Verdict != strftime.Conditional {
			t.Fatalf("verdict = %s", r.Verdict)
		}
	})

	t.Run("missing second", func(t *testing.T) {
		rt := strftime.Analyze("%F %H:%M").RoundTrip(
			time.Date(2009, 8, 7, 6, 5, 4, 0, time.UTC))
		if rt.InstantDifference != 4*time.Second {
			t.Errorf("instant difference = %v, want 4s", rt.InstantDifference)
		}
		if rt.Differences[strftime.FindingMissingSecond] == 0 {
			t.Errorf("missing-second attribution = %v", rt.Differences)
		}
	})

	t.Run("millisecond truncation", func(t *testing.T) {
		ts := time.Date(2009, 8, 7, 6, 5, 4, 345678901, time.UTC)
		rt := strftime.Analyze("%Y-%m-%d %H:%M:%S.%L").RoundTrip(ts)
		if rt.FractionLost != 678*time.Microsecond+901*time.Nanosecond {
			t.Errorf("FractionLost = %v", rt.FractionLost)
		}
	})

	t.Run("unparseable", func(t *testing.T) {
		rt := strftime.Analyze("%s").RoundTrip(ref)
		if rt.ParseError == nil {
			t.Errorf("expected parse error, got %+v", rt)
		}
	})
}

func TestAnalyzePrecision(t *testing.T) {
	tests := map[string]strftime.Precision{
		"%Y-%m-%d %H:%M:%S":    strftime.SecondPrecision,
		"%Y-%m-%d %H:%M:%S.%L": strftime.MillisecondPrecision,
		"%Y-%m-%d %H:%M:%S.%f": strftime.MicrosecondPrecision,
		"%Y-%m-%d %H:%M:%S.%N": strftime.NanosecondPrecision,
		"%F":                   strftime.SecondPrecision,
		"%Q":                   strftime.MillisecondPrecision,
	}
	for f, want := range tests {
		if got := strftime.Analyze(f).Precision; got != want {
			t.Errorf("Analyze(%q).Precision = %s, want %s", f, got, want)
		}
	}
}

func TestAnalyzeFields(t *testing.T) {
	r := strftime.Analyze("%Y-%m-%dT%H:%M:%S.%N%:z")
	if r.Date.Years[0] != "%Y" || r.Date.Months[0] != "%m" || r.Date.Days[0] != "%d" {
		t.Errorf("date fields = %+v", r.Date)
	}
	if r.Time.Hours[0] != "%H" || r.Time.Minutes[0] != "%M" || r.Time.Seconds[0] != "%S" {
		t.Errorf("time fields = %+v", r.Time)
	}
	if len(r.Zone.Offset) != 1 || r.Zone.Offset[0] != "%:z" {
		t.Errorf("zone fields = %+v", r.Zone)
	}

	rn := strftime.Analyze("%+")
	if !rn.Zone.Name || len(rn.Zone.Offset) != 0 || rn.Zone.UTC {
		t.Errorf("%%+ zone = %+v", rn.Zone)
	}
	if rn.Date.Weekdays[0] != "%a" || rn.Date.Months[0] != "%b" {
		t.Errorf("%%+ date = %+v", rn.Date)
	}

	ru := strftime.Analyze("%Y-%m-%dT%H:%M:%SZ")
	if !ru.Zone.UTC || ru.Zone.Name || len(ru.Zone.Offset) != 0 {
		t.Errorf("literal Z zone = %+v", ru.Zone)
	}
}

func severityFor(r *strftime.Report, code strftime.FindingCode) (strftime.Severity, bool) {
	for _, f := range r.Findings {
		if f.Code == code {
			return f.Severity, true
		}
	}
	return 0, false
}

func TestAnalyzeVerdict(t *testing.T) {
	tests := []struct {
		format string
		want   strftime.Verdict
		// codes that must be present at the listed severity
		warnings []strftime.FindingCode
		errors   []strftime.FindingCode
		notes    []strftime.FindingCode
		absent   []strftime.FindingCode
	}{
		{
			format: "%Y-%m-%dT%H:%M:%S.%N%:z",
			want:   strftime.Reversible,
		},
		{
			format: "%Y-%m-%dT%H:%M:%S.%NZ",
			want:   strftime.Reversible,
		},
		{
			format:   "%F %T",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.FindingMissingZone, strftime.FindingSubsecondTruncated},
		},
		{
			format:   "%c",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.FindingMissingZone, strftime.FindingSubsecondTruncated},
			notes:    []strftime.FindingCode{strftime.FindingRedundantField},
		},
		{
			// %+ carries %Z: name alone does not fix the offset.
			format:   "%+",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.FindingZoneNameOnly, strftime.FindingSubsecondTruncated},
		},
		{
			// 12-hour hour with no meridian.
			format:   "%I:%M:%S",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.Finding12HourNoMeridian},
		},
		{
			format: "%I:%M:%S %p",
			want:   strftime.Conditional,
			absent: []strftime.FindingCode{strftime.Finding12HourNoMeridian},
		},
		{
			// Two-digit year needs a pivot window.
			format:   "%m/%d/%y %H:%M:%S%z",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.FindingTwoDigitYear, strftime.FindingSubsecondTruncated},
		},
		{
			format:   "%D",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.FindingTwoDigitYear, strftime.FindingMissingTime, strftime.FindingMissingZone},
		},
		{
			// Same 8 printed characters as %H:%M:%S but 12-hour ambiguity.
			format:   "%I:%M:%S",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.Finding12HourNoMeridian},
		},
		{
			// Whole seconds only: sub-second truncation.
			format:   "%Y-%m-%dT%H:%M:%S%:z",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.FindingSubsecondTruncated},
		},
		{
			// Milliseconds: finer detail truncated.
			format:   "%Y-%m-%dT%H:%M:%S.%L%:z",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.FindingSubsecondTruncated},
		},
		{
			format:   "%Y-%m-%dT%H:%M:%S.%f%:z",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.FindingSubsecondTruncated},
		},
		{
			// Zone name only: offset not uniquely recoverable.
			format:   "%Y-%m-%d %H:%M:%S %Z",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.FindingZoneNameOnly},
		},
		{
			// ISO week date missing the weekday.
			format:   "%GW%V",
			want:     strftime.Unparseable,
			errors:   []strftime.FindingCode{strftime.FindingUnsupportedDirective},
			warnings: []strftime.FindingCode{strftime.FindingISOWeekIncomplete},
		},
		{
			format: "%GW%V%u",
			want:   strftime.Unparseable,
			errors: []strftime.FindingCode{strftime.FindingUnsupportedDirective},
			absent: []strftime.FindingCode{strftime.FindingISOWeekIncomplete},
		},
		{
			// Week-number date, components complete but unparseable here.
			format: "%Y %W %w",
			want:   strftime.Unparseable,
			errors: []strftime.FindingCode{strftime.FindingUnsupportedDirective},
			absent: []strftime.FindingCode{strftime.FindingDateSystemConflict, strftime.FindingWeekNumberIncomplete},
		},
		{
			format: "%s",
			want:   strftime.Unparseable,
			errors: []strftime.FindingCode{strftime.FindingUnsupportedDirective},
		},
		{
			format: "%Q %Y",
			want:   strftime.Unparseable,
			errors: []strftime.FindingCode{strftime.FindingUnsupportedDirective, strftime.FindingEpochConflict},
		},
		{
			format: "%I:%M:%S %H:%M",
			want:   strftime.Unparseable,
			errors: []strftime.FindingCode{strftime.FindingTimeSystemConflict},
		},
		{
			format: "%I:%M:%S %p %P",
			want:   strftime.Unparseable,
			errors: []strftime.FindingCode{strftime.FindingMeridianConflict},
		},
		{
			format: "%Y-%m-%dT%H:%M:%SZ%z",
			want:   strftime.Unparseable,
			errors: []strftime.FindingCode{strftime.FindingZoneConflict},
		},
		{
			format: "%S%N",
			want:   strftime.Unparseable,
			errors: []strftime.FindingCode{strftime.FindingInvalidFraction},
		},
		{
			format: "123",
			want:   strftime.Unparseable,
			errors: []strftime.FindingCode{strftime.FindingInvalidLiteral},
		},
		{
			format: "%q",
			want:   strftime.Unparseable,
			errors: []strftime.FindingCode{strftime.FindingUnknownDirective},
		},
		{
			// Ordinal date is a complete date (no calendar conflict).
			format:   "%Y%j",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.FindingMissingTime, strftime.FindingMissingZone},
			absent:   []strftime.FindingCode{strftime.FindingDateSystemConflict, strftime.FindingMissingMonth},
		},
		{
			// Month and day without a year.
			format:   "%m/%d",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.FindingMissingDate},
		},
		{
			// Year only.
			format:   "%Y",
			want:     strftime.Conditional,
			warnings: []strftime.FindingCode{strftime.FindingMissingMonth, strftime.FindingMissingTime, strftime.FindingMissingZone},
		},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			r := strftime.Analyze(tt.format)
			if r.Verdict != tt.want {
				t.Fatalf("Analyze(%q).Verdict = %s, want %s; findings=%v",
					tt.format, r.Verdict, tt.want, r.Findings)
			}
			codes := codeSet(r)
			for _, c := range tt.warnings {
				s, ok := severityFor(r, c)
				if !ok || s != strftime.Warning {
					t.Errorf("code %s: want warning, got %v (ok=%v)", c, s, ok)
				}
			}
			for _, c := range tt.errors {
				s, ok := severityFor(r, c)
				if !ok || s != strftime.Error {
					t.Errorf("code %s: want error, got %v (ok=%v)", c, s, ok)
				}
			}
			for _, c := range tt.notes {
				s, ok := severityFor(r, c)
				if !ok || s != strftime.Note {
					t.Errorf("code %s: want note, got %v (ok=%v)", c, s, ok)
				}
			}
			for _, c := range tt.absent {
				if codes[c] {
					t.Errorf("code %s should be absent; findings=%v", c, r.Findings)
				}
			}
		})
	}
}
