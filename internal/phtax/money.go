package phtax

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Centavos is an amount of money held as a whole number of centavos, so that a
// year of entries adds up exactly. A peso is 100 centavos.
//
// Nothing in this package works in floating point: a quarter's figures are
// added, subtracted and credited against each other dozens of times over a
// year, and a hundredth of a centavo of drift in the wrong place turns a nil
// balance into a peso payable.
type Centavos int64

// Peso is one peso in centavos.
const Peso Centavos = 100

// Pesos turns a whole number of pesos into centavos.
func Pesos(n int64) Centavos { return Centavos(n) * Peso }

// String writes the amount the way a return does: a thousands separator, two
// decimal places, a minus sign in front of a negative.
func (c Centavos) String() string {
	neg := c < 0
	if neg {
		c = -c
	}
	whole := int64(c) / 100
	frac := int64(c) % 100
	s := group(whole) + "." + fmt.Sprintf("%02d", frac)
	if neg {
		return "-" + s
	}
	return s
}

// Plain writes the amount with no thousands separator, which is what a
// machine-readable field wants.
func (c Centavos) Plain() string {
	neg := c < 0
	if neg {
		c = -c
	}
	s := fmt.Sprintf("%d.%02d", int64(c)/100, int64(c)%100)
	if neg {
		return "-" + s
	}
	return s
}

func group(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// errBadAmount is returned for text that is not an amount at all.
var errBadAmount = errors.New("that is not an amount")

// ParseAmount reads an amount the way a person types it: with or without a
// peso sign, with or without thousands separators, with nothing, one or two
// decimal places, and in parentheses for a negative, which is how an
// accounting package exports one.
//
// More than two decimal places is refused rather than silently rounded,
// because an amount of money in the Philippines has two.
func ParseAmount(s string) (Centavos, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, nil
	}
	t = strings.NewReplacer("₱", "", "PHP", "", "php", "", "P", "", ",", "", " ", "", " ", "").Replace(t)
	neg := false
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		neg, t = true, t[1:len(t)-1]
	}
	t = strings.TrimPrefix(t, "+")
	if strings.HasPrefix(t, "-") {
		neg, t = !neg, t[1:]
	}
	if t == "" {
		return 0, errBadAmount
	}
	whole, frac := t, ""
	if i := strings.IndexByte(t, '.'); i >= 0 {
		whole, frac = t[:i], t[i+1:]
	}
	if whole == "" {
		whole = "0"
	}
	if len(frac) > 2 {
		return 0, fmt.Errorf("%q has more than two decimal places; an amount in pesos has at most two", s)
	}
	for len(frac) < 2 {
		frac += "0"
	}
	if !allDigits(whole) || !allDigits(frac) {
		return 0, fmt.Errorf("%q is not an amount", s)
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is too large to be an amount", s)
	}
	f, _ := strconv.ParseInt(frac, 10, 64)
	if w > (1<<62)/100 {
		return 0, fmt.Errorf("%q is too large to be an amount", s)
	}
	out := Centavos(w*100 + f)
	if neg {
		out = -out
	}
	return out, nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// RoundToPeso rounds an amount to a whole number of pesos the way the return
// itself tells you to: "DO NOT enter Centavos; 49 Centavos or Less drop down;
// 50 or more round up" (BIR Form No. 1701, January 2018 ENCS). A negative
// amount rounds away from zero at the same half-way point, so that an
// overpayment is not quietly shaved.
//
// This is used only for the figures offered for transcription onto the return.
// Everything the tool computes with stays in centavos.
func RoundToPeso(c Centavos) Centavos {
	if c < 0 {
		return -RoundToPeso(-c)
	}
	return ((c + 50) / 100) * 100
}

// applyRate takes a rate given in basis points (one hundredth of one per cent,
// so 800 is 8% and 4000 is 40%) and returns that share of the amount, rounded
// half away from zero to the nearest centavo.
//
// Basis points keep every statutory rate an exact integer: 8%, 3%, 1%, 40%,
// 15%, 20%, 25%, 30%, 32% and 35% are all whole numbers of basis points, so no
// rate in this package is ever an inexact decimal.
func applyRate(amount Centavos, basisPoints int) Centavos {
	if amount == 0 || basisPoints == 0 {
		return 0
	}
	neg := amount < 0
	if neg {
		amount = -amount
	}
	n := int64(amount) * int64(basisPoints)
	out := Centavos((n + 5000) / 10000)
	if neg {
		return -out
	}
	return out
}

// atLeastZero clamps an amount at nil. A return has no field for a negative
// amount of tax due, so a figure that would be negative is nil and the excess
// is reported separately as an overpayment.
func atLeastZero(c Centavos) Centavos {
	if c < 0 {
		return 0
	}
	return c
}
