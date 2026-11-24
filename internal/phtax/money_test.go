package phtax

import "testing"

func TestParseAmount(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Centavos
		bad  bool
	}{
		{name: "empty is nil", in: "", want: 0},
		{name: "spaces are nil", in: "   ", want: 0},
		{name: "whole pesos", in: "1000", want: 100_000},
		{name: "one decimal place", in: "1000.5", want: 100_050},
		{name: "two decimal places", in: "1000.55", want: 100_055},
		{name: "thousands separators", in: "1,234,567.89", want: 123_456_789},
		{name: "peso sign", in: "₱250,000", want: 25_000_000},
		{name: "PHP prefix", in: "PHP 1,500.25", want: 150_025},
		{name: "bare P prefix", in: "P3,000,000", want: 300_000_000},
		{name: "leading decimal point", in: ".75", want: 75},
		{name: "explicit plus", in: "+42", want: 4_200},
		{name: "minus", in: "-42.10", want: -4_210},
		{name: "parentheses are a negative", in: "(1,234.56)", want: -123_456},
		{name: "minus inside parentheses cancels", in: "(-5)", want: 500},
		{name: "trailing point", in: "7.", want: 700},
		{name: "three decimal places refused", in: "1.005", bad: true},
		{name: "letters refused", in: "one thousand", bad: true},
		{name: "two points refused", in: "1.2.3", bad: true},
		{name: "a lone minus refused", in: "-", bad: true},
		{name: "a lone point is nil pesos nil centavos", in: ".", want: 0},
		{name: "absurdly large refused", in: "999999999999999999999", bad: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseAmount(c.in)
			if c.bad {
				if err == nil {
					t.Fatalf("ParseAmount(%q) = %v, want an error", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAmount(%q): %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("ParseAmount(%q) = %d centavos, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestCentavosString(t *testing.T) {
	cases := []struct {
		in          Centavos
		want, plain string
	}{
		{0, "0.00", "0.00"},
		{1, "0.01", "0.01"},
		{99, "0.99", "0.99"},
		{100, "1.00", "1.00"},
		{100_000, "1,000.00", "1000.00"},
		{25_000_000, "250,000.00", "250000.00"},
		{300_000_000, "3,000,000.00", "3000000.00"},
		{123_456_789, "1,234,567.89", "1234567.89"},
		{-123_456_789, "-1,234,567.89", "-1234567.89"},
		{-5, "-0.05", "-0.05"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("Centavos(%d).String() = %q, want %q", c.in, got, c.want)
		}
		if got := c.in.Plain(); got != c.plain {
			t.Errorf("Centavos(%d).Plain() = %q, want %q", c.in, got, c.plain)
		}
	}
}

// The return says: "DO NOT enter Centavos; 49 Centavos or Less drop down; 50 or
// more round up". Forty-nine down, fifty up, on both sides of nil.
func TestRoundToPeso(t *testing.T) {
	cases := []struct{ in, want Centavos }{
		{0, 0},
		{1, 0},
		{49, 0},
		{50, 100},
		{51, 100},
		{99, 100},
		{100, 100},
		{149, 100},
		{150, 200},
		{-49, 0},
		{-50, -100},
		{-150, -200},
		{-149, -100},
		{123_449, 123_400},
		{123_450, 123_500},
	}
	for _, c := range cases {
		if got := RoundToPeso(c.in); got != c.want {
			t.Errorf("RoundToPeso(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestApplyRate(t *testing.T) {
	cases := []struct {
		name   string
		amount Centavos
		bp     int
		want   Centavos
	}{
		{"nil amount", 0, 800, 0},
		{"nil rate", 100_000, 0, 0},
		{"8% of 250,000", Pesos(250_000), 800, Pesos(20_000)},
		{"3% of 100,000", Pesos(100_000), 300, Pesos(3_000)},
		{"1% of 100,000", Pesos(100_000), 100, Pesos(1_000)},
		{"40% of 1,234,567.89", 123_456_789, 4000, 49_382_716},
		{"12% of 1 centavo rounds up at a half", 1, 1200, 0},
		{"8% of 100 pesos", Pesos(100), 800, Pesos(8)},
		{"15% of 1 peso is 15 centavos", Pesos(1), 1500, 15},
		{"a rate on a negative keeps the sign", -Pesos(100), 800, -Pesos(8)},
		// 7 centavos at 8% is 0.56 of a centavo, which rounds to 1.
		{"half a centavo rounds away from zero", 7, 800, 1},
		// 6 centavos at 8% is 0.48 of a centavo, which rounds to nil.
		{"under half a centavo rounds to nil", 6, 800, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := applyRate(c.amount, c.bp); got != c.want {
				t.Fatalf("applyRate(%d, %d) = %d, want %d", c.amount, c.bp, got, c.want)
			}
		})
	}
}

func TestAtLeastZero(t *testing.T) {
	for _, c := range []struct{ in, want Centavos }{{-1, 0}, {0, 0}, {1, 1}, {-100_000, 0}} {
		if got := atLeastZero(c.in); got != c.want {
			t.Errorf("atLeastZero(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestPesos(t *testing.T) {
	if got := Pesos(250_000); got != 25_000_000 {
		t.Fatalf("Pesos(250000) = %d centavos, want 25000000", got)
	}
	if EightPercentAllowance != Pesos(250_000) {
		t.Fatalf("the 8%% allowance is %s, want 250,000.00", EightPercentAllowance)
	}
	if VATThreshold != Pesos(3_000_000) {
		t.Fatalf("the threshold is %s, want 3,000,000.00", VATThreshold)
	}
}
