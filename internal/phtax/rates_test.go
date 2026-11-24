package phtax

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// The graduated table is checked at every boundary of both schedules, on the
// boundary itself and one peso either side of it, because a bracket that is
// one peso out is the kind of error nothing else catches.
func TestGraduatedTax2023Onwards(t *testing.T) {
	r := DefaultRates()
	cases := []struct {
		name    string
		taxable Centavos
		want    Centavos
	}{
		{"nil", 0, 0},
		{"a loss is nil tax", -Pesos(500_000), 0},
		{"within the nil band", Pesos(100_000), 0},
		{"exactly at the nil band's top", Pesos(250_000), 0},
		{"one peso over the nil band", Pesos(250_001), 15},
		{"15% band, midway", Pesos(300_000), Pesos(7_500)},
		{"15% band, at its top", Pesos(400_000), Pesos(22_500)},
		{"20% band, one peso in", Pesos(400_001), Pesos(22_500) + 20},
		{"20% band, at its top", Pesos(800_000), Pesos(102_500)},
		{"25% band, one peso in", Pesos(800_001), Pesos(102_500) + 25},
		{"25% band, at its top", Pesos(2_000_000), Pesos(402_500)},
		{"30% band, one peso in", Pesos(2_000_001), Pesos(402_500) + 30},
		{"30% band, at its top", Pesos(8_000_000), Pesos(2_202_500)},
		{"35% band, one peso in", Pesos(8_000_001), Pesos(2_202_500) + 35},
		{"35% band, well in", Pesos(10_000_000), Pesos(2_202_500) + Pesos(700_000)},
		// 15% of 50 centavos is 7.5 centavos, which rounds away from zero.
		{"centavos carry through", Pesos(250_000) + 50, 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := r.GraduatedTax(c.taxable, 2026)
			if !ok {
				t.Fatal("no bracket schedule found for 2026")
			}
			if got != c.want {
				t.Fatalf("GraduatedTax(%s, 2026) = %s, want %s", c.taxable, got, c.want)
			}
		})
	}
}

func TestGraduatedTax2018To2022(t *testing.T) {
	r := DefaultRates()
	cases := []struct {
		taxable Centavos
		want    Centavos
	}{
		{Pesos(250_000), 0},
		{Pesos(400_000), Pesos(30_000)},
		{Pesos(800_000), Pesos(130_000)},
		{Pesos(2_000_000), Pesos(490_000)},
		{Pesos(8_000_000), Pesos(2_410_000)},
		{Pesos(8_000_001), Pesos(2_410_000) + 35},
	}
	for _, year := range []int{2018, 2019, 2020, 2021, 2022} {
		for _, c := range cases {
			got, ok := r.GraduatedTax(c.taxable, year)
			if !ok {
				t.Fatalf("no bracket schedule found for %d", year)
			}
			if got != c.want {
				t.Errorf("GraduatedTax(%s, %d) = %s, want %s", c.taxable, year, got, c.want)
			}
		}
	}
	// The two schedules must differ, or one of them is wrong.
	old, _ := r.GraduatedTax(Pesos(500_000), 2022)
	now, _ := r.GraduatedTax(Pesos(500_000), 2023)
	if old <= now {
		t.Fatalf("the 2023 table should tax %s less than the 2018 table did: %s vs %s", Pesos(500_000), now, old)
	}
}

func TestScheduleByYear(t *testing.T) {
	r := DefaultRates()
	cases := []struct {
		year      int
		wantFirst int
		found     bool
	}{
		{2017, 0, false},
		{2018, 2018, true},
		{2022, 2018, true},
		{2023, 2023, true},
		{2026, 2023, true},
		{2040, 2023, true},
	}
	for _, c := range cases {
		s, ok := r.Schedule(c.year)
		if ok != c.found {
			t.Fatalf("Schedule(%d) found = %v, want %v", c.year, ok, c.found)
		}
		if ok && s.FirstYear != c.wantFirst {
			t.Fatalf("Schedule(%d) is the table from %d, want the one from %d", c.year, s.FirstYear, c.wantFirst)
		}
	}
	if _, ok := r.GraduatedTax(Pesos(500_000), 2017); ok {
		t.Fatal("a year with no table on record should not produce a tax")
	}
}

// The percentage tax has moved twice, and both moves fell on the first day of a
// quarter. The boundary dates are the whole point.
func TestPercentageTaxRate(t *testing.T) {
	r := DefaultRates()
	cases := []struct {
		on   Date
		want int
	}{
		{NewDate(2019, time.January, 1), 300},
		{NewDate(2020, time.June, 30), 300},
		{NewDate(2020, time.July, 1), 100},
		{NewDate(2021, time.December, 31), 100},
		{NewDate(2023, time.June, 30), 100},
		{NewDate(2023, time.July, 1), 300},
		{NewDate(2026, time.November, 24), 300},
	}
	for _, c := range cases {
		got, period, ok := r.PercentageTaxRateBP(c.on)
		if !ok {
			t.Fatalf("no percentage tax period covers %s", c.on)
		}
		if got != c.want {
			t.Fatalf("PercentageTaxRateBP(%s) = %d basis points, want %d", c.on, got, c.want)
		}
		if period.Reference == "" {
			t.Fatalf("the period covering %s has no reference", c.on)
		}
	}
}

func TestRateString(t *testing.T) {
	for _, c := range []struct {
		bp   int
		want string
	}{
		{800, "8%"}, {300, "3%"}, {100, "1%"}, {4000, "40%"},
		{1500, "15%"}, {3200, "32%"}, {1200, "12%"}, {50, "0.5%"}, {125, "1.25%"}, {0, "0%"},
	} {
		if got := rateString(c.bp); got != c.want {
			t.Errorf("rateString(%d) = %q, want %q", c.bp, got, c.want)
		}
	}
}

// normalise is what stands between a hand-edited data file and a computation
// against a nil rate.
func TestNormaliseFillsEveryGap(t *testing.T) {
	got := Rates{}.normalise()
	want := DefaultRates()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("an empty Rates did not normalise to the defaults:\n got %+v\nwant %+v", got, want)
	}
	// A deliberate override survives normalising.
	over := Rates{EightPercentRateBP: 1000, VATThreshold: Pesos(5_000_000)}.normalise()
	if over.EightPercentRateBP != 1000 {
		t.Fatalf("an overridden 8%% rate was lost: %d", over.EightPercentRateBP)
	}
	if over.VATThreshold != Pesos(5_000_000) {
		t.Fatalf("an overridden threshold was lost: %s", over.VATThreshold)
	}
	if over.OSDRateBP != OSDRateBP {
		t.Fatalf("overriding one figure should not disturb another: OSD is %d", over.OSDRateBP)
	}
}

// DefaultRates must hand out copies, or one caller's override becomes
// everybody's.
func TestDefaultRatesAreNotShared(t *testing.T) {
	a := DefaultRates()
	a.Graduated[1].Brackets[1].RateBP = 9999
	b := DefaultRates()
	if b.Graduated[1].Brackets[1].RateBP == 9999 {
		t.Fatal("editing the rates a caller was given changed the package's own table")
	}
}

// Every figure the tool computes with has to appear in the published list with
// a reference, or the assumptions panel is a lie.
func TestConstantsCoverEveryRate(t *testing.T) {
	cs := DefaultRates().Constants()
	if len(cs) < 12 {
		t.Fatalf("only %d constants are published; every rate, threshold and date should be there", len(cs))
	}
	for _, c := range cs {
		if c.Name == "" || c.Value == "" || c.What == "" {
			t.Errorf("a constant is incompletely described: %+v", c)
		}
		if c.Reference == "" {
			t.Errorf("%q has no statutory reference", c.Name)
		}
		if c.Confidence != Sourced && c.Confidence != Unconfirmed {
			t.Errorf("%q has no confidence: %q", c.Name, c.Confidence)
		}
	}
	// Each overridable field named by a constant must really be a field of
	// Rates, so the override panel cannot offer a name nothing reads.
	fields := map[string]bool{}
	rt := reflect.TypeOf(Rates{})
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		if tag != "" {
			fields[strings.Split(tag, ",")[0]] = true
		}
	}
	for _, c := range cs {
		if c.Field == "" {
			continue
		}
		if !fields[c.Field] {
			t.Errorf("%q says it overrides %q, which is not a field of Rates", c.Name, c.Field)
		}
	}
	// And the other way: every field of Rates must be published.
	named := map[string]bool{}
	for _, c := range cs {
		named[c.Field] = true
	}
	for f := range fields {
		if !named[f] {
			t.Errorf("the rate %q is used but never published with a reference", f)
		}
	}
}

func TestConstantsNameTheKeyFigures(t *testing.T) {
	joined := ""
	for _, c := range DefaultRates().Constants() {
		joined += c.Name + "|" + c.Value + "|" + c.Reference + "\n"
	}
	for _, want := range []string{
		"8%", "250,000.00", "3,000,000.00", "40%", "3%", "1%", "12%",
		"24(A)(2)(b)", "34(L)", "109(1)(BB)", "128(A)(1)", "74(A)", "74(B)", "51(C)(1)", "56(A)(2)",
		"R.A. No. 10963", "R.A. No. 11534", "RMC No. 69-2023",
		"15 May", "15 August", "15 November", "15 April", "15 October",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the published constants never mention %q", want)
		}
	}
}
