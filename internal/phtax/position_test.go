package phtax

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// quarterReceipts puts one receipt in the middle of each quarter it is given a
// non-zero figure for, so a test can describe a year in one line.
func quarterReceipts(year int, pesos ...int64) []Entry {
	months := []time.Month{time.February, time.May, time.August, time.November}
	var out []Entry
	for i, p := range pesos {
		if p == 0 || i >= len(months) {
			continue
		}
		out = append(out, Entry{Date: NewDate(year, months[i], 15), Kind: Receipt, Amount: Pesos(p)})
	}
	return out
}

func mustCompute(t *testing.T, in YearInput) YearPosition {
	t.Helper()
	if in.Rates.EightPercentRateBP == 0 {
		in.Rates = DefaultRates()
	}
	if in.Today.IsZero() {
		in.Today = NewDate(in.Year, time.December, 31)
	}
	y, err := ComputeYear(in)
	if err != nil {
		t.Fatalf("ComputeYear: %v", err)
	}
	if len(y.Quarters) != QuartersPerYear {
		t.Fatalf("got %d quarters, want %d", len(y.Quarters), QuartersPerYear)
	}
	return y
}

type want struct {
	quarter                              int
	gross, taxable, taxDue               Centavos
	credits, payable, toPay, overpayment Centavos
}

func check(t *testing.T, y YearPosition, w want) {
	t.Helper()
	p := y.Quarters[w.quarter-1]
	for _, c := range []struct {
		label     string
		got, want Centavos
	}{
		{"gross", p.Gross, w.gross},
		{"taxable", p.Taxable, w.taxable},
		{"tax due", p.TaxDue, w.taxDue},
		{"total credits", p.TotalCredits, w.credits},
		{"tax payable", p.TaxPayable, w.payable},
		{"amount to pay", p.AmountToPay, w.toPay},
		{"overpayment", p.Overpayment, w.overpayment},
	} {
		if c.got != c.want {
			t.Errorf("quarter %d %s = %s, want %s", w.quarter, c.label, c.got, c.want)
		}
	}
	if p.AmountToPay < 0 || p.Overpayment < 0 {
		t.Errorf("quarter %d produced a negative payment or overpayment: %s / %s", w.quarter, p.AmountToPay, p.Overpayment)
	}
	if p.AmountToPay > 0 && p.Overpayment > 0 {
		t.Errorf("quarter %d is both owed and overpaid: %s / %s", w.quarter, p.AmountToPay, p.Overpayment)
	}
	if p.TaxPayable != p.AmountToPay-p.Overpayment {
		t.Errorf("quarter %d: payable %s does not reconcile with %s to pay and %s overpaid",
			w.quarter, p.TaxPayable, p.AmountToPay, p.Overpayment)
	}
}

// ---------------------------------------------------------------- the 8% path

// Quarter one on its own: nothing is credited, because nothing came before.
func TestEightPercentFirstQuarterOnly(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent},
		Entries:  quarterReceipts(2026, 300_000),
	})
	check(t, y, want{quarter: 1,
		gross: Pesos(300_000), taxable: Pesos(50_000), taxDue: Pesos(4_000),
		credits: 0, payable: Pesos(4_000), toPay: Pesos(4_000)})
	if y.Quarters[0].Allowance != Pesos(250_000) {
		t.Errorf("the allowance applied was %s, want 250,000.00", y.Quarters[0].Allowance)
	}
	if y.Quarters[0].Deadline != NewDate(2026, time.May, 15) {
		t.Errorf("quarter one is due %s, want 2026-05-15", y.Quarters[0].Deadline)
	}
	// Nothing was recorded for the later quarters, so the year-to-date
	// figures stop moving but the credits keep accumulating.
	check(t, y, want{quarter: 2,
		gross: Pesos(300_000), taxable: Pesos(50_000), taxDue: Pesos(4_000),
		credits: Pesos(4_000), payable: 0, toPay: 0})
}

// The whole year, quarter by quarter. Each quarter's tax is on the year to
// date, and what the earlier quarters paid is credited, so the four payments
// add up to the year's tax exactly once.
func TestEightPercentCumulativeThroughTheYear(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent},
		Entries:  quarterReceipts(2026, 300_000, 200_000, 100_000, 400_000),
	})
	check(t, y, want{quarter: 1, gross: Pesos(300_000), taxable: Pesos(50_000), taxDue: Pesos(4_000),
		credits: 0, payable: Pesos(4_000), toPay: Pesos(4_000)})
	check(t, y, want{quarter: 2, gross: Pesos(500_000), taxable: Pesos(250_000), taxDue: Pesos(20_000),
		credits: Pesos(4_000), payable: Pesos(16_000), toPay: Pesos(16_000)})
	check(t, y, want{quarter: 3, gross: Pesos(600_000), taxable: Pesos(350_000), taxDue: Pesos(28_000),
		credits: Pesos(20_000), payable: Pesos(8_000), toPay: Pesos(8_000)})
	check(t, y, want{quarter: 4, gross: Pesos(1_000_000), taxable: Pesos(750_000), taxDue: Pesos(60_000),
		credits: Pesos(28_000), payable: Pesos(32_000), toPay: Pesos(32_000)})

	var paid Centavos
	for _, p := range y.Quarters {
		paid += p.AmountToPay
	}
	if paid != y.Annual.TaxDue {
		t.Fatalf("the four payments add up to %s but the year's tax is %s", paid, y.Annual.TaxDue)
	}
	if y.Annual.Form != "1701A" {
		t.Errorf("a purely self-employed 8%% filer should use 1701A, got %q", y.Annual.Form)
	}
	if y.Annual.Deadline != NewDate(2027, time.April, 15) {
		t.Errorf("the annual return is due %s, want 2027-04-15", y.Annual.Deadline)
	}
	// There is no percentage tax under the 8% election; that is the point.
	for _, p := range y.Quarters {
		if p.PercentageTax != nil {
			t.Errorf("quarter %d was given a percentage tax under the 8%% election", p.Quarter)
		}
	}
}

// The 250,000 allowance is deducted once against the year to date, not once a
// quarter. Deducting it four times would cost the taxpayer nothing and the
// Bureau 60,000 pesos at 8%.
func TestEightPercentAllowanceIsDeductedOnce(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent},
		Entries:  quarterReceipts(2026, 250_000, 250_000, 250_000, 250_000),
	})
	for _, p := range y.Quarters {
		if p.Allowance != Pesos(250_000) {
			t.Errorf("quarter %d applied an allowance of %s, want 250,000.00", p.Quarter, p.Allowance)
		}
	}
	if y.Annual.Taxable != Pesos(750_000) {
		t.Fatalf("the year's taxable income is %s, want 750,000.00 (1,000,000 less one allowance)", y.Annual.Taxable)
	}
	if y.Annual.TaxDue != Pesos(60_000) {
		t.Fatalf("the year's tax is %s, want 60,000.00", y.Annual.TaxDue)
	}
}

func TestEightPercentAllowanceBoundaries(t *testing.T) {
	cases := []struct {
		name                            string
		gross                           int64
		wantAllowance, wantTaxable, tax Centavos
	}{
		{"well inside the allowance", 100_000, Pesos(100_000), 0, 0},
		{"exactly at the allowance", 250_000, Pesos(250_000), 0, 0},
		{"one peso over", 250_001, Pesos(250_000), Pesos(1), 8},
		{"a hundred pesos over", 250_100, Pesos(250_000), Pesos(100), Pesos(8)},
		{"nothing recorded", 0, 0, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			y := mustCompute(t, YearInput{
				Year:     2026,
				Election: Election{Regime: EightPercent},
				Entries:  quarterReceipts(2026, c.gross),
			})
			p := y.Quarters[0]
			if p.Allowance != c.wantAllowance {
				t.Errorf("allowance = %s, want %s", p.Allowance, c.wantAllowance)
			}
			if p.Taxable != c.wantTaxable {
				t.Errorf("taxable = %s, want %s", p.Taxable, c.wantTaxable)
			}
			if p.TaxDue != c.tax {
				t.Errorf("tax due = %s, want %s", p.TaxDue, c.tax)
			}
			if p.Taxable < 0 {
				t.Errorf("the allowance drove taxable income below nil: %s", p.Taxable)
			}
		})
	}
}

// A mixed-income earner gets no allowance against business income: the return
// says the reduction is "not applicable if with compensation income".
func TestEightPercentWithCompensationGetsNoAllowance(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent, HasCompensation: true},
		Entries:  quarterReceipts(2026, 300_000),
	})
	p := y.Quarters[0]
	if p.Allowance != 0 {
		t.Fatalf("allowance = %s, want nil for a mixed-income earner", p.Allowance)
	}
	if p.Taxable != Pesos(300_000) || p.TaxDue != Pesos(24_000) {
		t.Fatalf("taxable %s, tax %s; want 300,000.00 and 24,000.00", p.Taxable, p.TaxDue)
	}
	if y.Annual.Form != "1701" {
		t.Errorf("a mixed-income earner files 1701, got %q", y.Annual.Form)
	}
	if !hasNote(p.Notes, "compensation income") {
		t.Error("the quarter should say why the allowance is nil")
	}
}

// --------------------------------------------------- credits exceeding the tax

// A loss quarter after a profit quarter: the cumulative tax falls below what
// has already been paid, and the quarter must report an overpayment rather
// than a negative payment.
func TestLossQuarterAfterProfitQuarter(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: Graduated, Deduction: Itemized},
		Entries: []Entry{
			{Date: NewDate(2026, time.February, 1), Kind: Receipt, Amount: Pesos(800_000)},
			{Date: NewDate(2026, time.February, 2), Kind: Expense, Amount: Pesos(100_000), Category: OrdinaryDeduction},
			{Date: NewDate(2026, time.May, 1), Kind: Expense, Amount: Pesos(600_000), Category: OrdinaryDeduction},
			{Date: NewDate(2026, time.August, 1), Kind: Expense, Amount: Pesos(200_000), Category: OrdinaryDeduction},
		},
	})

	// Quarter one: 700,000 of net income, taxed at 22,500 + 20% of 300,000.
	check(t, y, want{quarter: 1, gross: Pesos(800_000), taxable: Pesos(700_000), taxDue: Pesos(82_500),
		credits: 0, payable: Pesos(82_500), toPay: Pesos(82_500)})

	// Quarter two: the year to date is down to 100,000 of net income, which
	// is inside the nil band, so the tax to date is nil and the whole of
	// quarter one's payment is an overpayment.
	check(t, y, want{quarter: 2, gross: Pesos(800_000), taxable: Pesos(100_000), taxDue: 0,
		credits: Pesos(82_500), payable: -Pesos(82_500), toPay: 0, overpayment: Pesos(82_500)})

	// Quarter three: the year to date is now an actual loss. Taxable income
	// is nil, never negative, and the overpayment stands.
	p3 := y.Quarters[2]
	if p3.NetIncome != -Pesos(100_000) {
		t.Errorf("quarter three net income = %s, want a loss of 100,000.00", p3.NetIncome)
	}
	check(t, y, want{quarter: 3, gross: Pesos(800_000), taxable: 0, taxDue: 0,
		credits: Pesos(82_500), payable: -Pesos(82_500), toPay: 0, overpayment: Pesos(82_500)})
	if !hasNote(p3.Notes, "loss of") {
		t.Error("a loss-making year to date should be remarked on")
	}
	if !hasNote(p3.Notes, "Overpayment") && !hasNote(p3.Notes, "overpayment") {
		t.Error("an overpaid quarter should be remarked on")
	}

	// A quarter that paid nothing contributes nothing to the next quarter's
	// credits, so the credit does not grow on its own.
	if y.Quarters[2].PaidPriorQuarters != Pesos(82_500) {
		t.Errorf("quarter three credits %s of earlier payments, want 82,500.00", y.Quarters[2].PaidPriorQuarters)
	}
	if y.Annual.Overpayment != Pesos(82_500) {
		t.Errorf("the year ends %s overpaid, want 82,500.00", y.Annual.Overpayment)
	}
	if !hasNote(y.Notes, "irrevocable") {
		t.Error("the year's notes should say the annual choice about an overpayment is irrevocable")
	}
}

// Prior-quarter tax exceeding the current cumulative liability, without a
// loss: a large credit note in quarter two pulls cumulative gross back down
// under the 8% election.
func TestPriorQuarterTaxExceedsCumulativeLiability(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent},
		Entries: []Entry{
			{Date: NewDate(2026, time.February, 1), Kind: Receipt, Amount: Pesos(1_000_000)},
			{Date: NewDate(2026, time.May, 1), Kind: SalesReturn, Amount: Pesos(600_000)},
		},
	})
	check(t, y, want{quarter: 1, gross: Pesos(1_000_000), taxable: Pesos(750_000), taxDue: Pesos(60_000),
		credits: 0, payable: Pesos(60_000), toPay: Pesos(60_000)})
	check(t, y, want{quarter: 2, gross: Pesos(400_000), taxable: Pesos(150_000), taxDue: Pesos(12_000),
		credits: Pesos(60_000), payable: -Pesos(48_000), toPay: 0, overpayment: Pesos(48_000)})
}

// An overpayment that is larger than everything the rest of the year can
// absorb still never turns into a negative amount to pay.
func TestOverpaymentNeverBecomesANegativePayment(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent, PriorYearExcess: Pesos(500_000)},
		Entries:  quarterReceipts(2026, 300_000, 300_000, 300_000, 300_000),
	})
	for _, p := range y.Quarters {
		if p.AmountToPay != 0 {
			t.Errorf("quarter %d wants %s paid although 500,000.00 is carried in", p.Quarter, p.AmountToPay)
		}
		if p.Overpayment <= 0 {
			t.Errorf("quarter %d shows no overpayment although the carried credit swallows the tax", p.Quarter)
		}
	}
	// 1,200,000 less the allowance is 950,000; 8% of that is 76,000; the
	// carried credit of 500,000 leaves 424,000 unused.
	if y.Annual.Overpayment != Pesos(424_000) {
		t.Fatalf("the year ends %s overpaid, want 424,000.00", y.Annual.Overpayment)
	}
}

// --------------------------------------------------------- recorded payments

// What goes on the return is what was actually paid, not what this tool thinks
// should have been. Where the taxpayer has recorded a payment it is used, and
// the "assumed" flag goes away.
func TestRecordedPaymentsAreUsedInsteadOfComputedOnes(t *testing.T) {
	el := Election{Regime: EightPercent}
	el.Payments[0] = QuarterPayment{Recorded: true, Amount: Pesos(3_000)}
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: el,
		Entries:  quarterReceipts(2026, 300_000, 200_000),
	})
	// Quarter one's own figure is still 4,000 — the tool does not bend to
	// the payment — but quarter two credits the 3,000 that was paid.
	check(t, y, want{quarter: 1, gross: Pesos(300_000), taxable: Pesos(50_000), taxDue: Pesos(4_000),
		credits: 0, payable: Pesos(4_000), toPay: Pesos(4_000)})
	check(t, y, want{quarter: 2, gross: Pesos(500_000), taxable: Pesos(250_000), taxDue: Pesos(20_000),
		credits: Pesos(3_000), payable: Pesos(17_000), toPay: Pesos(17_000)})
	if y.Quarters[1].PaidPriorQuartersSoft {
		t.Error("quarter two should not be marked as assuming a payment it was told about")
	}
	if hasNote(y.Quarters[1].Notes, "this tool's own figure") {
		t.Error("quarter two should not warn about an assumed payment")
	}
	// Quarter three has one recorded payment and one assumed, so it is
	// still marked as assuming.
	if !y.Quarters[2].PaidPriorQuartersSoft {
		t.Error("quarter three credits an assumed payment and should say so")
	}
	if !hasNote(y.Quarters[2].Notes, "this tool's own figure") {
		t.Error("quarter three should warn about the assumed payment")
	}
}

func TestZeroRecordedPaymentMeansNothingWasPaid(t *testing.T) {
	el := Election{Regime: EightPercent}
	for i := range el.Payments {
		el.Payments[i] = QuarterPayment{Recorded: true, Amount: 0}
	}
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: el,
		Entries:  quarterReceipts(2026, 300_000, 200_000, 100_000, 400_000),
	})
	// Nothing was paid all year, so the annual return carries the whole of
	// the year's tax.
	check(t, y, want{quarter: 4, gross: Pesos(1_000_000), taxable: Pesos(750_000), taxDue: Pesos(60_000),
		credits: 0, payable: Pesos(60_000), toPay: Pesos(60_000)})
}

// ------------------------------------------------------- the graduated path

func TestGraduatedWithOptionalStandardDeduction(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: Graduated, Deduction: OSD},
		Entries:  quarterReceipts(2026, 300_000, 300_000, 300_000, 300_000),
	})
	// 40% of net sales each time, then the 2023 graduated table.
	check(t, y, want{quarter: 1, gross: Pesos(300_000), taxable: Pesos(180_000), taxDue: 0,
		credits: 0, payable: 0, toPay: 0})
	check(t, y, want{quarter: 2, gross: Pesos(600_000), taxable: Pesos(360_000), taxDue: Pesos(16_500),
		credits: 0, payable: Pesos(16_500), toPay: Pesos(16_500)})
	check(t, y, want{quarter: 3, gross: Pesos(900_000), taxable: Pesos(540_000), taxDue: Pesos(50_500),
		credits: Pesos(16_500), payable: Pesos(34_000), toPay: Pesos(34_000)})
	check(t, y, want{quarter: 4, gross: Pesos(1_200_000), taxable: Pesos(720_000), taxDue: Pesos(86_500),
		credits: Pesos(50_500), payable: Pesos(36_000), toPay: Pesos(36_000)})

	if y.Quarters[3].OSDAmount != Pesos(480_000) {
		t.Errorf("the year's optional standard deduction is %s, want 480,000.00", y.Quarters[3].OSDAmount)
	}
	if y.Annual.Form != "1701A" {
		t.Errorf("a purely self-employed filer on the optional standard deduction uses 1701A, got %q", y.Annual.Form)
	}
}

func TestGraduatedItemisedVersusOptionalStandardDeduction(t *testing.T) {
	entries := []Entry{
		{Date: NewDate(2026, time.February, 1), Kind: Receipt, Amount: Pesos(1_000_000)},
		{Date: NewDate(2026, time.February, 2), Kind: Expense, Amount: Pesos(200_000), Category: CostOfServices},
		{Date: NewDate(2026, time.February, 3), Kind: Expense, Amount: Pesos(100_000), Category: OrdinaryDeduction},
	}
	osd := mustCompute(t, YearInput{Year: 2026, Entries: entries,
		Election: Election{Regime: Graduated, Deduction: OSD}})
	item := mustCompute(t, YearInput{Year: 2026, Entries: entries,
		Election: Election{Regime: Graduated, Deduction: Itemized}})

	// The standard deduction is 400,000, the recorded costs only 300,000, so
	// the standard deduction wins here.
	if osd.Annual.DeductionTaken != Pesos(400_000) {
		t.Errorf("the standard deduction taken is %s, want 400,000.00", osd.Annual.DeductionTaken)
	}
	if item.Annual.DeductionTaken != Pesos(300_000) {
		t.Errorf("the itemised deduction taken is %s, want 300,000.00", item.Annual.DeductionTaken)
	}
	if osd.Annual.TaxDue >= item.Annual.TaxDue {
		t.Errorf("the standard deduction should cost less here: %s vs %s", osd.Annual.TaxDue, item.Annual.TaxDue)
	}
	// Recorded costs that the chosen method ignores must be said out loud.
	if !hasNote(osd.Quarters[0].Notes, "change nothing") {
		t.Error("ignoring recorded costs under the standard deduction should be remarked on")
	}
	if hasNote(item.Quarters[0].Notes, "change nothing") {
		t.Error("itemised deductions do use the recorded costs")
	}
	if item.Annual.Form != "1701" {
		t.Errorf("itemising means form 1701, got %q", item.Annual.Form)
	}
}

// The percentage tax is per quarter and credits nothing — unlike the income
// tax return it sits beside.
func TestPercentageTaxIsPerQuarterNotCumulative(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: Graduated, Deduction: OSD},
		Entries:  quarterReceipts(2026, 100_000, 200_000, 300_000, 400_000),
	})
	wantBase := []Centavos{Pesos(100_000), Pesos(200_000), Pesos(300_000), Pesos(400_000)}
	wantDue := []Centavos{Pesos(3_000), Pesos(6_000), Pesos(9_000), Pesos(12_000)}
	wantDeadline := []Date{
		NewDate(2026, time.April, 25), NewDate(2026, time.July, 25),
		NewDate(2026, time.October, 25), NewDate(2027, time.January, 25),
	}
	for i, p := range y.Quarters {
		pt := p.PercentageTax
		if pt == nil {
			t.Fatalf("quarter %d has no percentage tax", i+1)
		}
		if pt.Base != wantBase[i] {
			t.Errorf("quarter %d percentage tax base = %s, want %s (this quarter only)", i+1, pt.Base, wantBase[i])
		}
		if pt.RateBP != 300 {
			t.Errorf("quarter %d percentage tax rate = %d basis points, want 300", i+1, pt.RateBP)
		}
		if pt.Due != wantDue[i] {
			t.Errorf("quarter %d percentage tax due = %s, want %s", i+1, pt.Due, wantDue[i])
		}
		if pt.Deadline != wantDeadline[i] {
			t.Errorf("quarter %d percentage tax is due %s, want %s", i+1, pt.Deadline, wantDeadline[i])
		}
	}
}

// The rate changed on 1 July 2023, which is the first day of a quarter, so
// 2023 has two quarters at 1% and two at 3%.
func TestPercentageTaxRateChangeMidYear(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2023,
		Election: Election{Regime: Graduated, Deduction: OSD},
		Entries:  quarterReceipts(2023, 100_000, 100_000, 100_000, 100_000),
	})
	wantRate := []int{100, 100, 300, 300}
	wantDue := []Centavos{Pesos(1_000), Pesos(1_000), Pesos(3_000), Pesos(3_000)}
	for i, p := range y.Quarters {
		if p.PercentageTax.RateBP != wantRate[i] {
			t.Errorf("2023 quarter %d rate = %d basis points, want %d", i+1, p.PercentageTax.RateBP, wantRate[i])
		}
		if p.PercentageTax.Due != wantDue[i] {
			t.Errorf("2023 quarter %d percentage tax = %s, want %s", i+1, p.PercentageTax.Due, wantDue[i])
		}
	}
	// The same, the other way round, in 2020.
	y20 := mustCompute(t, YearInput{
		Year:     2020,
		Election: Election{Regime: Graduated, Deduction: OSD},
		Entries:  quarterReceipts(2020, 100_000, 100_000, 100_000, 100_000),
	})
	for i, wantBP := range []int{300, 300, 100, 100} {
		if got := y20.Quarters[i].PercentageTax.RateBP; got != wantBP {
			t.Errorf("2020 quarter %d rate = %d basis points, want %d", i+1, got, wantBP)
		}
	}
}

// A year before 2023 is taxed on the earlier, heavier table.
func TestYearPicksTheRightGraduatedTable(t *testing.T) {
	year := func(y int) Centavos {
		return mustCompute(t, YearInput{
			Year:     y,
			Election: Election{Regime: Graduated, Deduction: OSD},
			Entries:  quarterReceipts(y, 500_000, 500_000, 500_000, 500_000),
		}).Annual.TaxDue
	}
	// 2,000,000 of net sales less a 40% standard deduction is 1,200,000.
	// On the 2018 table: 130,000 plus 30% of 400,000. On the 2023 table:
	// 102,500 plus 25% of 400,000.
	if got := year(2020); got != Pesos(250_000) {
		t.Errorf("the 2020 tax is %s, want 250,000.00 on the 2018 table", got)
	}
	if got := year(2026); got != Pesos(202_500) {
		t.Errorf("the 2026 tax is %s, want 202,500.00 on the 2023 table", got)
	}
	if !hasNote(mustCompute(t, YearInput{Year: 2020, Election: Election{Regime: Graduated}}).Notes,
		"2018 to 2022") {
		t.Error("the year should say which graduated table it used")
	}
}

func TestVATRegisteredHasNoPercentageTax(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: Graduated, Deduction: OSD, VATRegistered: true},
		Entries:  quarterReceipts(2026, 100_000),
	})
	for _, p := range y.Quarters {
		if p.PercentageTax != nil {
			t.Errorf("quarter %d charges a percentage tax to a VAT-registered taxpayer", p.Quarter)
		}
	}
	if !hasNote(y.Quarters[0].Notes, "does not compute VAT") {
		t.Error("the tool should say plainly that it does not compute VAT")
	}
}

// ------------------------------------------------------------- withholding

func TestWithholdingIsCreditedAndSplitByQuarter(t *testing.T) {
	var entries []Entry
	for i, m := range []time.Month{time.February, time.May, time.August, time.November} {
		_ = i
		entries = append(entries, Entry{Date: NewDate(2026, m, 15), Kind: Receipt,
			Amount: Pesos(500_000), Withheld: Pesos(10_000)})
	}
	y := mustCompute(t, YearInput{Year: 2026, Election: Election{Regime: EightPercent}, Entries: entries})

	check(t, y, want{quarter: 1, gross: Pesos(500_000), taxable: Pesos(250_000), taxDue: Pesos(20_000),
		credits: Pesos(10_000), payable: Pesos(10_000), toPay: Pesos(10_000)})
	check(t, y, want{quarter: 2, gross: Pesos(1_000_000), taxable: Pesos(750_000), taxDue: Pesos(60_000),
		credits: Pesos(30_000), payable: Pesos(30_000), toPay: Pesos(30_000)})
	check(t, y, want{quarter: 3, gross: Pesos(1_500_000), taxable: Pesos(1_250_000), taxDue: Pesos(100_000),
		credits: Pesos(70_000), payable: Pesos(30_000), toPay: Pesos(30_000)})
	check(t, y, want{quarter: 4, gross: Pesos(2_000_000), taxable: Pesos(1_750_000), taxDue: Pesos(140_000),
		credits: Pesos(110_000), payable: Pesos(30_000), toPay: Pesos(30_000)})

	// The return splits withholding into earlier quarters and this one.
	for i, p := range y.Quarters {
		if p.WithheldThisQuarter != Pesos(10_000) {
			t.Errorf("quarter %d withheld this quarter = %s, want 10,000.00", i+1, p.WithheldThisQuarter)
		}
		if p.WithheldPriorQuarters != Pesos(10_000)*Centavos(i) {
			t.Errorf("quarter %d withheld earlier = %s, want %s", i+1, p.WithheldPriorQuarters, Pesos(10_000)*Centavos(i))
		}
	}

	// The invariant: what is handed over across the year equals the year's
	// tax less everything that was already withheld.
	var paid Centavos
	for _, p := range y.Quarters {
		paid += p.AmountToPay
	}
	if paid != y.Annual.TaxDue-Pesos(40_000) {
		t.Fatalf("payments total %s; want the year's tax %s less 40,000.00 withheld", paid, y.Annual.TaxDue)
	}
}

// ------------------------------------------------- crossing the threshold

func TestCrossingTheRegistrationThresholdUnderTheEightPercentElection(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent, Deduction: OSD},
		Entries:  quarterReceipts(2026, 1_000_000, 1_000_000, 1_500_000),
	})
	if !y.Threshold.Crossed {
		t.Fatal("gross reached 3,500,000.00 and the threshold was not reported as crossed")
	}
	if y.Threshold.Quarter != 3 {
		t.Errorf("the threshold was crossed in quarter %d, want 3", y.Threshold.Quarter)
	}
	if y.Threshold.Gross != Pesos(3_500_000) || y.Threshold.Threshold != Pesos(3_000_000) {
		t.Errorf("the breach reads %s against %s", y.Threshold.Gross, y.Threshold.Threshold)
	}
	if len(y.Recomputed) != QuartersPerYear {
		t.Fatalf("the year was not worked again under the graduated rates (%d quarters)", len(y.Recomputed))
	}
	if !strings.Contains(y.RecomputedWhy, "falls away") {
		t.Errorf("the reason for the recomputation reads %q", y.RecomputedWhy)
	}
	if !hasNote(y.Quarters[2].Notes, "no longer the ones to file") {
		t.Error("the quarter that crossed should say its own figures are no longer the ones to file")
	}
	if hasNote(y.Quarters[0].Notes, "no longer the ones to file") {
		t.Error("a quarter before the crossing should not be flagged")
	}

	// The recomputation credits what was paid under the 8% election, not the
	// graduated figures that were never paid.
	if y.Recomputed[1].PaidPriorQuarters != y.Quarters[0].AmountToPay {
		t.Errorf("the recomputation credits %s for quarter one; %s was paid under the election",
			y.Recomputed[1].PaidPriorQuarters, y.Quarters[0].AmountToPay)
	}
	// 3,500,000 net sales less a 40% standard deduction is 2,100,000, taxed
	// at 402,500 plus 30% of 100,000.
	if got := y.Recomputed[3].TaxDue; got != Pesos(432_500) {
		t.Errorf("the recomputed year's tax is %s, want 432,500.00", got)
	}
	if y.Recomputed[0].PercentageTax == nil {
		t.Error("the graduated recomputation should carry the percentage tax")
	}
}

func TestCrossingTheThresholdUnderTheGraduatedRates(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: Graduated, Deduction: OSD},
		Entries:  quarterReceipts(2026, 1_600_000, 1_600_000),
	})
	if !y.Threshold.Crossed || y.Threshold.Quarter != 2 {
		t.Fatalf("the threshold should be crossed in quarter two, got %+v", y.Threshold)
	}
	if len(y.Recomputed) != 0 {
		t.Error("a graduated year needs no recomputation; it is already on the graduated rates")
	}
	if !hasNote(y.Quarters[1].Notes, "gives way to VAT") {
		t.Error("crossing the threshold should warn that VAT replaces the percentage tax")
	}
}

func TestEightPercentWhileVATRegisteredIsRefusedOutright(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent, VATRegistered: true},
		Entries:  quarterReceipts(2026, 300_000),
	})
	if !hasNote(y.Quarters[0].Notes, "not registered for value-added tax") {
		t.Error("the 8% option is not open to a VAT-registered person and the tool should say so")
	}
	if len(y.Recomputed) != QuartersPerYear {
		t.Fatal("the year should be worked again under the graduated rates")
	}
	if !strings.Contains(y.RecomputedWhy, "registered for value-added tax") {
		t.Errorf("the reason reads %q", y.RecomputedWhy)
	}
}

func TestNothingBelowTheThresholdIsFlagged(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent},
		Entries:  quarterReceipts(2026, 3_000_000),
	})
	if y.Threshold.Crossed {
		t.Fatal("gross of exactly 3,000,000.00 is at the threshold, not over it")
	}
	one := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent},
		Entries:  quarterReceipts(2026, 3_000_001),
	})
	if !one.Threshold.Crossed {
		t.Fatal("one peso over the threshold is over the threshold")
	}
}

// The marginal rate is what the NEXT peso would meet, which at a bracket bound
// is already the rate above — while the tax on the income itself is still the
// rate below. The two tests differ by one peso and it matters on the screen.
func TestMarginalRateAtABracketBound(t *testing.T) {
	cases := []struct {
		taxable        int64
		wantTax        Centavos
		wantMarginalBP int
	}{
		{249_999, 0, 0},
		{250_000, 0, 1500},
		{250_001, 15, 1500},
		{399_999, Pesos(22_500) - 15, 1500},
		{400_000, Pesos(22_500), 2000},
		{800_000, Pesos(102_500), 2500},
		{2_000_000, Pesos(402_500), 3000},
		{8_000_000, Pesos(2_202_500), 3500},
	}
	for _, c := range cases {
		// Net sales of n with no deductions give taxable income of n.
		y := mustCompute(t, YearInput{
			Year:     2026,
			Election: Election{Regime: Graduated, Deduction: Itemized},
			Entries:  quarterReceipts(2026, c.taxable),
		})
		p := y.Quarters[0]
		if p.Taxable != Pesos(c.taxable) {
			t.Fatalf("taxable = %s, want %s", p.Taxable, Pesos(c.taxable))
		}
		if p.TaxDue != c.wantTax {
			t.Errorf("tax on %s = %s, want %s", p.Taxable, p.TaxDue, c.wantTax)
		}
		if p.MarginalBP != c.wantMarginalBP {
			t.Errorf("marginal rate at %s = %d basis points, want %d",
				p.Taxable, p.MarginalBP, c.wantMarginalBP)
		}
	}
}

// ------------------------------------------------------- the second instalment

func TestSecondInstalment(t *testing.T) {
	el := Election{Regime: Graduated, Deduction: OSD}
	for i := range el.Payments {
		el.Payments[i] = QuarterPayment{Recorded: true, Amount: 0}
	}
	y := mustCompute(t, YearInput{Year: 2026, Election: el,
		Entries: quarterReceipts(2026, 300_000, 300_000, 300_000, 300_000)})

	if !y.Instalment.Available {
		t.Fatal("86,500.00 of tax due is well over the 2,000.00 threshold")
	}
	if y.Instalment.HalfOfTaxDue != Pesos(43_250) {
		t.Errorf("half the tax due is %s, want 43,250.00", y.Instalment.HalfOfTaxDue)
	}
	if y.Instalment.DeferrableNow != Pesos(43_250) || y.Instalment.PayWithReturn != Pesos(43_250) {
		t.Errorf("the split is %s deferred and %s now", y.Instalment.DeferrableNow, y.Instalment.PayWithReturn)
	}
	if y.Instalment.Due != NewDate(2027, time.October, 15) {
		t.Errorf("the second instalment is due %s, want 2027-10-15", y.Instalment.Due)
	}

	// Nothing can be deferred that is not owed: with the quarters paid, the
	// annual balance is smaller than half the year's tax.
	paid := mustCompute(t, YearInput{Year: 2026, Election: Election{Regime: Graduated, Deduction: OSD},
		Entries: quarterReceipts(2026, 300_000, 300_000, 300_000, 300_000)})
	if paid.Instalment.DeferrableNow != paid.Annual.AmountToPay {
		t.Errorf("deferrable %s exceeds the %s actually payable", paid.Instalment.DeferrableNow, paid.Annual.AmountToPay)
	}
}

func TestSecondInstalmentThreshold(t *testing.T) {
	// 8% of 25,000 is exactly 2,000, which is not "in excess of" 2,000.
	at := mustCompute(t, YearInput{Year: 2026, Election: Election{Regime: EightPercent},
		Entries: quarterReceipts(2026, 275_000)})
	if at.Annual.TaxDue != Pesos(2_000) {
		t.Fatalf("the year's tax is %s, want exactly 2,000.00", at.Annual.TaxDue)
	}
	if at.Instalment.Available {
		t.Error("tax due of exactly 2,000.00 does not open the two-instalment election")
	}
	over := mustCompute(t, YearInput{Year: 2026, Election: Election{Regime: EightPercent},
		Entries: quarterReceipts(2026, 275_100)})
	if !over.Instalment.Available {
		t.Errorf("tax due of %s is in excess of 2,000.00", over.Annual.TaxDue)
	}
}

// --------------------------------------------------------------- projection

func TestProjection(t *testing.T) {
	cases := []struct {
		name             string
		today            Date
		receipts         []int64
		available        bool
		quarters         int
		projectedGross   Centavos
		projectedTax     Centavos
		crossesThreshold bool
	}{
		{
			name: "two quarters in", today: NewDate(2026, time.July, 15),
			receipts: []int64{300_000, 300_000}, available: true, quarters: 2,
			projectedGross: Pesos(1_200_000), projectedTax: Pesos(76_000),
		},
		{
			name: "on course to cross the threshold", today: NewDate(2026, time.July, 15),
			receipts: []int64{900_000, 900_000}, available: true, quarters: 2,
			projectedGross: Pesos(3_600_000), projectedTax: Pesos(268_000), crossesThreshold: true,
		},
		{
			name: "too early in the year", today: NewDate(2026, time.January, 5),
			receipts: []int64{300_000}, available: false,
		},
		{
			name: "the year is over, so there is nothing to project", today: NewDate(2027, time.January, 5),
			receipts: []int64{300_000, 300_000, 300_000, 300_000}, available: false,
		},
		{
			name: "nothing recorded yet", today: NewDate(2026, time.July, 15),
			receipts: []int64{}, available: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			y := mustCompute(t, YearInput{
				Year: 2026, Today: c.today,
				Election: Election{Regime: EightPercent},
				Entries:  quarterReceipts(2026, c.receipts...),
			})
			p := y.Projection
			if p.Available != c.available {
				t.Fatalf("available = %v, want %v", p.Available, c.available)
			}
			if !c.available {
				return
			}
			if p.QuartersCounted != c.quarters {
				t.Errorf("counted %d quarters, want %d", p.QuartersCounted, c.quarters)
			}
			if p.ProjectedGross != c.projectedGross {
				t.Errorf("projected gross = %s, want %s", p.ProjectedGross, c.projectedGross)
			}
			if p.ProjectedTax != c.projectedTax {
				t.Errorf("projected tax = %s, want %s", p.ProjectedTax, c.projectedTax)
			}
			if p.CrossesThreshold != c.crossesThreshold {
				t.Errorf("crosses the threshold = %v, want %v", p.CrossesThreshold, c.crossesThreshold)
			}
		})
	}
}

func TestProjectionUnderTheGraduatedRates(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year: 2026, Today: NewDate(2026, time.July, 15),
		Election: Election{Regime: Graduated, Deduction: OSD},
		Entries:  quarterReceipts(2026, 300_000, 300_000),
	})
	// 1,200,000 projected, less a 40% standard deduction, is 720,000, taxed
	// at 22,500 plus 20% of 320,000.
	if y.Projection.ProjectedTax != Pesos(86_500) {
		t.Fatalf("the projected tax is %s, want 86,500.00", y.Projection.ProjectedTax)
	}
	item := mustCompute(t, YearInput{
		Year: 2026, Today: NewDate(2026, time.July, 15),
		Election: Election{Regime: Graduated, Deduction: Itemized},
		Entries: append(quarterReceipts(2026, 300_000, 300_000),
			Entry{Date: NewDate(2026, time.March, 1), Kind: Expense, Amount: Pesos(100_000), Category: OrdinaryDeduction}),
	})
	// 1,200,000 projected less 200,000 of projected costs is 1,000,000,
	// taxed at 102,500 plus 25% of 200,000.
	if item.Projection.ProjectedTax != Pesos(152_500) {
		t.Fatalf("the projected tax with itemised deductions is %s, want 152,500.00", item.Projection.ProjectedTax)
	}
}

// ------------------------------------------------------------------- the rest

func TestEmptyYear(t *testing.T) {
	y := mustCompute(t, YearInput{Year: 2026, Election: Election{Regime: EightPercent}})
	for _, p := range y.Quarters {
		if p.Gross != 0 || p.TaxDue != 0 || p.AmountToPay != 0 || p.Overpayment != 0 {
			t.Errorf("quarter %d is not nil on an empty year: %+v", p.Quarter, p)
		}
		if !hasNote(p.Notes, "Nothing is recorded") {
			t.Errorf("quarter %d should say nothing is recorded", p.Quarter)
		}
	}
	if y.Instalment.Available {
		t.Error("there is nothing to defer on an empty year")
	}
	if y.Threshold.Crossed {
		t.Error("an empty year crosses nothing")
	}
}

func TestDefaultsWhenTheElectionIsBlank(t *testing.T) {
	y := mustCompute(t, YearInput{Year: 2026, Entries: quarterReceipts(2026, 300_000)})
	if y.Quarters[0].Regime != Graduated {
		t.Errorf("a blank regime should fall back to the graduated rates, got %q", y.Quarters[0].Regime)
	}
	if y.Quarters[0].Deduction != OSD {
		t.Errorf("a blank deduction method should fall back to the standard deduction, got %q", y.Quarters[0].Deduction)
	}
	if !hasNote(y.Notes, "not a recommendation") {
		t.Error("the year should say the regime is an input, not a recommendation")
	}
}

func TestAYearWithNoRateTableIsRefused(t *testing.T) {
	_, err := ComputeYear(YearInput{Year: 2015, Rates: DefaultRates(), Election: Election{Regime: Graduated}})
	if err == nil {
		t.Fatal("a year with no bracket table on record should be refused, not guessed at")
	}
	if !strings.Contains(err.Error(), "2015") {
		t.Errorf("the refusal should name the year: %v", err)
	}
}

func TestEntriesOutsideTheYearAreRefused(t *testing.T) {
	_, err := ComputeYear(YearInput{
		Year: 2026, Rates: DefaultRates(), Election: Election{Regime: EightPercent},
		Entries: []Entry{{Date: NewDate(2025, time.December, 31), Kind: Receipt, Amount: Pesos(1_000)}},
	})
	if err == nil {
		t.Fatal("an entry from another year should be refused rather than silently dropped")
	}
}

func TestOverriddenRatesAreUsed(t *testing.T) {
	r := DefaultRates()
	r.EightPercentRateBP = 1000
	r.EightPercentAllowance = Pesos(300_000)
	y := mustCompute(t, YearInput{
		Year: 2026, Rates: r,
		Election: Election{Regime: EightPercent},
		Entries:  quarterReceipts(2026, 500_000),
	})
	// 10% of 200,000 under the overridden figures.
	if got := y.Quarters[0].TaxDue; got != Pesos(20_000) {
		t.Fatalf("with a 10%% rate and a 300,000.00 allowance the tax is %s, want 20,000.00", got)
	}
	if y.Quarters[0].TaxRateBP != 1000 {
		t.Errorf("the quarter reports a rate of %d basis points, want 1000", y.Quarters[0].TaxRateBP)
	}
}

func TestOverriddenThresholdMovesTheWarning(t *testing.T) {
	r := DefaultRates()
	r.VATThreshold = Pesos(500_000)
	y := mustCompute(t, YearInput{
		Year: 2026, Rates: r,
		Election: Election{Regime: EightPercent},
		Entries:  quarterReceipts(2026, 600_000),
	})
	if !y.Threshold.Crossed || y.Threshold.Quarter != 1 {
		t.Fatalf("the overridden threshold should be crossed in quarter one, got %+v", y.Threshold)
	}
}

func TestDeadlineCalendar(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year: 2026, Today: NewDate(2026, time.May, 10),
		Election: Election{Regime: Graduated, Deduction: OSD},
	})
	if len(y.Deadlines) != 9 {
		t.Fatalf("the year has %d filing dates, want 9 (three 1701Q, four 2551Q, the annual return and the second instalment)", len(y.Deadlines))
	}
	for i := 1; i < len(y.Deadlines); i++ {
		if y.Deadlines[i].Due.Before(y.Deadlines[i-1].Due) {
			t.Fatalf("the calendar is out of order at %d: %s before %s", i, y.Deadlines[i].Due, y.Deadlines[i-1].Due)
		}
	}
	byForm := map[string][]DeadlineItem{}
	for _, d := range y.Deadlines {
		byForm[d.Form] = append(byForm[d.Form], d)
		if d.Reference == "" {
			t.Errorf("%s on %s has no reference", d.Form, d.Due)
		}
	}
	if len(byForm["1701Q"]) != 3 {
		t.Errorf("there are %d quarterly income tax returns, want 3", len(byForm["1701Q"]))
	}
	if len(byForm["2551Q"]) != 4 {
		t.Errorf("there are %d percentage tax returns, want 4", len(byForm["2551Q"]))
	}
	// On 10 May 2026 the first quarter's percentage tax return is already
	// overdue and the income tax return is five days off.
	status := map[string]string{}
	for _, d := range y.Deadlines {
		status[d.Form+" "+d.Due.String()] = d.Status
	}
	if status["2551Q 2026-04-25"] != "overdue" {
		t.Errorf("the April percentage tax return should be overdue, got %q", status["2551Q 2026-04-25"])
	}
	if status["1701Q 2026-05-15"] != "soon" {
		t.Errorf("the May income tax return should be due soon, got %q", status["1701Q 2026-05-15"])
	}
	if status["1701/1701A 2027-04-15"] != "upcoming" {
		t.Errorf("the annual return should be upcoming, got %q", status["1701/1701A 2027-04-15"])
	}
	onTheDay := mustCompute(t, YearInput{Year: 2026, Today: NewDate(2026, time.May, 15),
		Election: Election{Regime: Graduated, Deduction: OSD}})
	for _, d := range onTheDay.Deadlines {
		if d.Form == "1701Q" && d.Due == NewDate(2026, time.May, 15) && d.Status != "today" {
			t.Errorf("on the day itself the status should be \"today\", got %q", d.Status)
		}
	}
}

func TestWeekendDeadlineIsFlaggedNotMoved(t *testing.T) {
	// 15 August 2026 is a Saturday.
	y := mustCompute(t, YearInput{Year: 2026, Election: Election{Regime: EightPercent}})
	q2 := y.Quarters[1]
	if q2.Deadline != NewDate(2026, time.August, 15) {
		t.Fatalf("the deadline was moved to %s; this tool states the statutory date", q2.Deadline)
	}
	if !q2.DeadlineIsWeekend {
		t.Fatal("a Saturday deadline should be flagged")
	}
	if !hasNote(q2.Notes, "falls on a Saturday") {
		t.Error("the weekend should be remarked on, with a word to confirm the working date")
	}
}

func TestAnnualPositionMatchesTheFourthQuarter(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent},
		Entries:  quarterReceipts(2026, 300_000, 200_000, 100_000, 400_000),
	})
	if !reflect.DeepEqual(y.Annual, y.Quarters[3]) {
		t.Fatal("the annual position should be the fourth quarter's cumulative position")
	}
	if !y.Annual.IsAnnual {
		t.Error("the fourth position is the annual return")
	}
	if !hasNote(y.Annual.Notes, "no fourth quarterly return") {
		t.Error("the annual position should explain that there is no fourth 1701Q")
	}
}

// hasNote looks in the whole of a note — what it says and what it cites —
// because that is what a reader sees.
func hasNote(notes []Note, substr string) bool {
	for _, n := range notes {
		if strings.Contains(n.Text, substr) || strings.Contains(n.Reference, substr) {
			return true
		}
	}
	return false
}
