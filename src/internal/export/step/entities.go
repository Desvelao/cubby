package step

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// writer allocates sequential STEP entity IDs (#1, #2, ...) and emits
// "#id=ENTITY(...);" lines, tracking the first write error so callers don't
// need to check every call.
type writer struct {
	bw  *bufio.Writer
	n   int
	err error
}

func newWriter(w io.Writer) *writer {
	return &writer{bw: bufio.NewWriter(w)}
}

// emit writes a fully-formed entity body under a freshly allocated id and
// returns that id, for other entities to reference.
func (wr *writer) emit(body string) int {
	wr.n++
	id := wr.n
	if wr.err != nil {
		return id
	}
	_, wr.err = fmt.Fprintf(wr.bw, "#%d=%s;\n", id, body)
	return id
}

func (wr *writer) raw(line string) {
	if wr.err != nil {
		return
	}
	_, wr.err = fmt.Fprintln(wr.bw, line)
}

func (wr *writer) flush() error {
	if wr.err != nil {
		return wr.err
	}
	return wr.bw.Flush()
}

func ref(id int) string { return fmt.Sprintf("#%d", id) }

func refList(ids []int) string {
	s := "("
	for i, id := range ids {
		if i > 0 {
			s += ","
		}
		s += ref(id)
	}
	return s + ")"
}

// formatReal renders v as an ISO 10303-21 REAL literal: digits, a mandatory
// '.', optional fraction and an optional uppercase E exponent ("3.", "0.5",
// "1.E-7"). It uses the shortest round-trip representation, positional for
// ordinary magnitudes and E notation for extreme ones. Negative zero prints
// as "0.". Non-finite values have no REAL representation and return an error.
func formatReal(v float64) (string, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "", fmt.Errorf("step: cannot write non-finite REAL value %v", v)
	}
	if v == 0 {
		return "0.", nil
	}
	if a := math.Abs(v); a >= 1e-4 && a < 1e15 {
		s := strconv.FormatFloat(v, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += "."
		}
		return s, nil
	}
	s := strconv.FormatFloat(v, 'E', -1, 64) // e.g. "-1.5E-07", "1E+21"
	mant, exp, _ := strings.Cut(s, "E")
	if !strings.Contains(mant, ".") {
		mant += "."
	}
	neg := strings.HasPrefix(exp, "-")
	exp = strings.TrimLeft(exp, "+-0")
	if exp == "" {
		exp = "0"
	}
	if neg {
		exp = "-" + exp
	}
	return mant + "E" + exp, nil
}

// real formats v with formatReal, recording any error as the writer's first
// error so callers don't need to check every value.
func (wr *writer) real(v float64) string {
	s, err := formatReal(v)
	if err != nil && wr.err == nil {
		wr.err = err
	}
	if err != nil {
		return "0."
	}
	return s
}

func (wr *writer) point(x, y, z float64) string {
	return fmt.Sprintf("CARTESIAN_POINT('',(%s,%s,%s))", wr.real(x), wr.real(y), wr.real(z))
}

func (wr *writer) direction(x, y, z float64) string {
	return fmt.Sprintf("DIRECTION('',(%s,%s,%s))", wr.real(x), wr.real(y), wr.real(z))
}
