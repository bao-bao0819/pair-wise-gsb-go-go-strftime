package strftime

import (
	"strconv"
	"time"
	"unicode/utf8"
)

// Compiled is a compiled strftime format specification.
// It separates literal text from time directives, so literal
// bytes are never mistaken for parts of a Go time layout.
//
// A Compiled value is safe for concurrent use by multiple goroutines.
type Compiled struct {
	format string
	tokens []token
}

type tokenKind uint8

const (
	tokLiteral tokenKind = iota
	tokDirective
)

type token struct {
	kind   tokenKind
	offset int // byte offset in the format: for directives, the '%'; for literals, the first byte
	text   string
	spec   byte
	flag   byte
	fop    formatOp
	pop    parseOp
}

// Compile parses a strftime format specification and returns a reusable,
// immutable representation supporting Format and Parse.
//
// Combined directives (%F, %T, %c, %+, …) are expanded at compile time.
// Every directive supported for formatting is accepted, so Compile only
// fails for malformed specifications; directives unsupported for parsing
// (for example %C or %U) are reported by Parse with their format byte
// offset. See CompileParse for a parser-only compilation.
func Compile(format string) (*Compiled, error) {
	return compile(format, false)
}

// CompileParse compiles format for parsing. Directives that cannot be
// parsed, such as %C or %U, produce a *CompileError reporting the
// directive and its byte offset in the format.
func CompileParse(format string) (*Compiled, error) {
	return compile(format, true)
}

func compile(format string, parseOnly bool) (*Compiled, error) {
	tokens, err := compileInto(format, 0, nil, parseOnly, format)
	if err != nil {
		return nil, err
	}
	return &Compiled{format: format, tokens: mergeLiterals(tokens)}, nil
}

// MustCompile is like Compile, but panics if the format cannot be compiled.
func MustCompile(format string) *Compiled {
	c, err := Compile(format)
	if err != nil {
		panic(err)
	}
	return c
}

// String returns the format specification the value was compiled from.
func (c *Compiled) String() string {
	return c.format
}

// nextPopSpec returns the specifier of the next parseable directive token
// strictly after index i, or 0 when none exists.
func (c *Compiled) nextPopSpec(i int) byte {
	for j := i + 1; j < len(c.tokens); j++ {
		tok := &c.tokens[j]
		if tok.kind == tokDirective && tok.pop != nil {
			return tok.spec
		}
	}
	return 0
}

// Format returns a textual representation of the time value.
func (c *Compiled) Format(t time.Time) string {
	return string(c.AppendFormat(nil, t))
}

// AppendFormat is like Format, but appends the textual representation
// to dst and returns the extended buffer.
//
// It never aliases or retains dst after returning, so the same Compiled
// value can be used concurrently with distinct buffers.
func (c *Compiled) AppendFormat(dst []byte, t time.Time) []byte {
	for i := range c.tokens {
		tok := &c.tokens[i]
		if tok.kind == tokLiteral {
			dst = append(dst, tok.text...)
			continue
		}
		dst = tok.fop(dst, t, tok.flag)
	}
	return dst
}

// CompileError describes a format specification that cannot be compiled
// for the requested use (formatting or parsing).
type CompileError struct {
	Format    string // the format specification
	Directive string // the offending directive, or "" for literal errors
	Offset    int    // byte offset of the directive (or offending literal) in Format
	message   string
}

func (e *CompileError) Error() string {
	if e.Directive != "" {
		return "strftime: unsupported directive: " + e.Directive + " at offset " + strconv.Itoa(e.Offset) + " " + e.message
	}
	return "strftime: unsupported literal at offset " + strconv.Itoa(e.Offset) + " " + e.message
}

func compileError(format string, off int, directive, message string) *CompileError {
	return &CompileError{
		Format:    format,
		Directive: directive,
		Offset:    off,
		message:   message,
	}
}

// scanDirective returns the directive text starting at a '%' (which must be
// at format[start]), along with its specifier and flag. When the directive
// is not recognized (bad flag/modifier/specifier), ok is false and the
// returned text is the run of bytes that must be emitted verbatim.
func scanDirective(format string, start int) (text string, spec, flag byte, ok bool) {
	i := start + 1
	n := len(format)
	if i >= n {
		return format[start:], 0, 0, false
	}
	// Optional flag.
	if format[i] == '-' || format[i] == ':' {
		flag = format[i]
		i++
		if i >= n {
			return format[start:], 0, 0, false
		}
	}
	// Optional E/O modifier.
	mod := byte(0)
	if format[i] == 'E' || format[i] == 'O' {
		mod = format[i]
		i++
		if i >= n {
			return format[start:], 0, 0, false
		}
	}
	spec = format[i]
	if mod != 0 && !okModifier(mod, spec) {
		return format[start : i+1], 0, 0, false
	}
	_, nbytes := utf8.DecodeRuneInString(format[i:])
	return format[start : i+nbytes], spec, flag, true
}

func compileInto(format string, base int, tokens []token, parseOnly bool, root string) ([]token, error) {
	var litStart = -1
	flush := func(end int) {
		if litStart >= 0 {
			tokens = append(tokens, token{
				kind:   tokLiteral,
				offset: base + litStart,
				text:   format[litStart:end],
			})
			litStart = -1
		}
	}

	for i := 0; i < len(format); {
		b := format[i]
		if b != '%' {
			if litStart < 0 {
				litStart = i
			}
			_, size := utf8.DecodeRuneInString(format[i:])
			i += size
			continue
		}

		text, spec, flag, ok := scanDirective(format, i)
		if !ok {
			// Verbatim run (dangling '%', '%-', unsupported modifier, …).
			if litStart < 0 {
				litStart = i
			}
			i += len(text)
			continue
		}

		if combo := comboExpansion(spec, flag); combo != nil {
			flush(i)
			var err error
			tokens, err = compileCombo(string(combo), base+i, text, tokens, parseOnly, root)
			if err != nil {
				return nil, err
			}
			i += len(text)
			continue
		}

		if lit, special := directiveLiteral(spec); special {
			flush(i)
			tokens = append(tokens, token{
				kind:   tokLiteral,
				offset: base + i,
				text:   lit,
			})
			i += len(text)
			continue
		}

		fop := formatOps[spec]
		pop := parseOps[spec]
		if parseOnly && pop == nil {
			flush(i)
			return nil, compileError(root, base+i, text, "is not supported for parsing")
		}
		if fop == nil {
			if litStart < 0 {
				litStart = i
			}
			i += len(text)
			continue
		}

		flush(i)
		tokens = append(tokens, token{
			kind:   tokDirective,
			offset: base + i,
			text:   text,
			spec:   spec,
			flag:   flag,
			fop:    fop,
			pop:    pop,
		})
		i += len(text)
	}
	flush(len(format))
	return tokens, nil
}

// compileCombo expands a combined directive. Tokens produced by the
// expansion are attributed to the outer directive at base (its '%'),
// because that is the directive the user wrote and sees in errors.
func compileCombo(combo string, base int, outer string, tokens []token, parseOnly bool, root string) ([]token, error) {
	expanded, err := compileInto(combo, 0, nil, parseOnly, root)
	if err != nil {
		return nil, err
	}
	for j := range expanded {
		expanded[j].offset = base
		if expanded[j].kind == tokDirective {
			expanded[j].text = outer
		}
	}
	return append(tokens, expanded...), nil
}

func mergeLiterals(tokens []token) []token {
	if len(tokens) < 2 {
		return tokens
	}
	merged := tokens[:0]
	for _, tok := range tokens {
		if tok.kind == tokLiteral && len(merged) > 0 &&
			merged[len(merged)-1].kind == tokLiteral &&
			merged[len(merged)-1].offset+len(merged[len(merged)-1].text) == tok.offset {
			merged[len(merged)-1].text += tok.text
			continue
		}
		merged = append(merged, tok)
	}
	return merged
}

// comboExpansion returns the strftime expansion of a combined directive,
// or nil when the directive is not a combination.
func comboExpansion(spec, flag byte) []byte {
	switch spec {
	case '+':
		return []byte("%a %b %e %H:%M:%S %Z %Y")
	case 'c':
		return []byte("%a %b %e %T %Y")
	case 'v':
		return []byte("%e-%b-%Y")
	case 'F':
		return []byte("%Y-%m-%d")
	case 'D', 'x':
		return []byte("%m/%d/%y")
	case 'r':
		return []byte("%I:%M:%S %p")
	case 'T', 'X':
		return []byte("%H:%M:%S")
	case 'R':
		return []byte("%H:%M")
	}
	return nil
}

// directiveLiteral reports directives that expand to fixed literal text.
func directiveLiteral(spec byte) (string, bool) {
	switch spec {
	case '%':
		return "%", true
	case 'n':
		return "\n", true
	case 't':
		return "\t", true
	}
	return "", false
}
