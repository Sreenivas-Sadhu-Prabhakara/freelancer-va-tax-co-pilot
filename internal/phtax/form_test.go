package phtax

import (
	"strings"
	"testing"
	"time"
)

func figures(t *testing.T, y YearPosition, quarter int) FormFigures {
	t.Helper()
	f, err := y.Figures(quarter)
	if err != nil {
		t.Fatalf("Figures(%d): %v", quarter, err)
	}
	return f
}

func lineNamed(f FormFigures, substr string) (FigureLine, bool) {
	for _, b := range f.Blocks {
		for _, l := range b.Lines {
			if strings.Contains(l.Label, substr) {
				return l, true
			}
		}
	}
	return FigureLine{}, false
}

// lineExact is for a label that is a prefix of another one, where a substring
// match would find the wrong line.
func lineExact(f FormFigures, label string) (FigureLine, bool) {
	for _, b := range f.Blocks {
		for _, l := range b.Lines {
			if l.Label == label {
				return l, true
			}
		}
	}
	return FigureLine{}, false
}

func blockNamed(f FormFigures, substr string) (FigureBlock, bool) {
	for _, b := range f.Blocks {
		if strings.Contains(b.Title, substr) {
			return b, true
		}
	}
	return FigureBlock{}, false
}

func TestFiguresForTheQuarterlyReturnUnderTheEightPercentElection(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent},
		Entries:  quarterReceipts(2026, 300_000, 200_000),
	})
	f := figures(t, y, 2)

	if f.Form != "1701Q" {
		t.Errorf("form = %q, want 1701Q", f.Form)
	}
	if f.Deadline != NewDate(2026, time.August, 15) {
		t.Errorf("deadline = %s, want 2026-08-15", f.Deadline)
	}
	if !strings.Contains(f.Covers, "year to date") {
		t.Errorf("the quarterly return covers %q; it should say it is the year to date", f.Covers)
	}
	if f.ItemNumbersKnown {
		t.Error("the 1701Q item numbers could not be sourced, so none should be claimed")
	}
	for _, b := range f.Blocks {
		for _, l := range b.Lines {
			if l.Item != "" {
				t.Errorf("the 1701Q line %q claims item number %q", l.Label, l.Item)
			}
		}
	}

	for _, want := range []struct {
		label string
		pesos Centavos
	}{
		{"Sales / revenues / receipts / fees", Pesos(500_000)},
		{"allowable reduction", Pesos(250_000)},
		{"Taxable income, year to date", Pesos(250_000)},
		{"Tax due to date at 8%", Pesos(20_000)},
		{"Tax payments for the previous quarters", Pesos(4_000)},
		{"Total tax credits and payments", Pesos(4_000)},
		{"Tax payable / (overpayment)", Pesos(16_000)},
	} {
		l, ok := lineNamed(f, want.label)
		if !ok {
			t.Errorf("no line named %q", want.label)
			continue
		}
		if l.Peso != want.pesos {
			t.Errorf("%q = %s, want %s", want.label, l.Peso, want.pesos)
		}
	}

	// The 8% return has no percentage tax block.
	if _, ok := blockNamed(f, "Percentage tax"); ok {
		t.Error("the 8% election carries no percentage tax")
	}
	// Nor a graduated deduction line.
	if _, ok := lineNamed(f, "optional standard deduction"); ok {
		t.Error("the 8% election takes no deduction")
	}
	if !strings.Contains(f.Caveat, "not the return") {
		t.Errorf("the caveat should say plainly that this is not the return: %q", f.Caveat)
	}
	if !strings.Contains(f.Caveat, "does not file") {
		t.Errorf("the caveat should say plainly that nothing is filed: %q", f.Caveat)
	}
	if !strings.Contains(f.Caveat, "could not be sourced") {
		t.Errorf("the caveat should admit the item numbers are unsourced: %q", f.Caveat)
	}
}

func TestFiguresForTheAnnualReturnCarryItemNumbers(t *testing.T) {
	// Itemised deductions mean form 1701, which is the one form whose item
	// numbers were read from the form itself.
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: Graduated, Deduction: Itemized},
		Entries: []Entry{
			{Date: NewDate(2026, time.February, 1), Kind: Receipt, Amount: Pesos(1_000_000)},
			{Date: NewDate(2026, time.February, 2), Kind: SalesReturn, Amount: Pesos(50_000)},
			{Date: NewDate(2026, time.February, 3), Kind: Expense, Amount: Pesos(200_000), Category: CostOfServices},
			{Date: NewDate(2026, time.February, 4), Kind: Expense, Amount: Pesos(100_000), Category: OrdinaryDeduction},
			{Date: NewDate(2026, time.March, 1), Kind: NonOperating, Amount: Pesos(20_000)},
		},
	})
	f := figures(t, y, 4)

	if f.Form != "1701" {
		t.Fatalf("form = %q, want 1701", f.Form)
	}
	if !f.ItemNumbersKnown {
		t.Fatal("the 1701 item numbers were read from the form and should be offered")
	}
	if strings.Contains(f.Caveat, "could not be sourced") {
		t.Error("the 1701 caveat should not claim the item numbers are unsourced")
	}
	if f.Deadline != NewDate(2027, time.April, 15) {
		t.Errorf("deadline = %s, want 2027-04-15", f.Deadline)
	}

	for _, want := range []struct{ label, item string }{
		{"Sales / revenues / receipts / fees", "8"},
		{"Less: sales returns, allowances and discounts", "9"},
		{"Net sales / revenues / receipts / fees", "10"},
		{"Less: cost of sales / services", "11"},
		{"Gross income from operation", "12"},
		{"Less: ordinary allowable itemised deductions", "13"},
		{"Net income from operation", "18"},
		{"Add: total other non-operating income", "22"},
		{"Taxable income from business, year to date", "23"},
		{"Tax due to date under the graduated rates", "25"},
		{"Prior year's excess credits", "1"},
		{"Tax payments for the first three quarters", "2"},
		{"Creditable tax withheld for the previous quarters", "3"},
		{"Total tax credits and payments", "10"},
		{"Tax payable / (overpayment)", "24"},
	} {
		l, ok := lineNamed(f, want.label)
		if !ok {
			t.Errorf("no line named %q", want.label)
			continue
		}
		if l.Item != want.item {
			t.Errorf("%q is item %q, want item %q", want.label, l.Item, want.item)
		}
	}
	// "Tax due" on its own is Part II item 22, which the figure for the
	// graduated schedule (item 25) feeds into.
	if l, ok := lineExact(f, "Tax due"); !ok || l.Item != "22" {
		t.Errorf("the Part II tax-due line is item %q, want 22 (found: %v)", l.Item, ok)
	}

	// 950,000 net sales less 300,000 of costs plus 20,000 of other income.
	l, _ := lineNamed(f, "Taxable income from business, year to date")
	if l.Peso != Pesos(670_000) {
		t.Errorf("taxable income = %s, want 670,000.00", l.Peso)
	}
	if _, ok := blockNamed(f, "Percentage tax"); !ok {
		t.Error("a graduated non-VAT year carries a percentage tax block")
	}
}

func TestFiguresRoundToWholePesosButKeepTheCentavos(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent},
		Entries: []Entry{
			// 250,000.00 plus 6.25 of taxable income: 8% of 6.25 is 0.50,
			// which the return's own rule rounds up to a peso.
			{Date: NewDate(2026, time.February, 1), Kind: Receipt, Amount: Pesos(250_006) + 25},
		},
	})
	l, ok := lineNamed(figures(t, y, 1), "Tax due to date at 8%")
	if !ok {
		t.Fatal("no tax due line")
	}
	if l.Exact != 50 {
		t.Fatalf("the exact tax is %s, want 0.50", l.Exact)
	}
	if l.Peso != Pesos(1) {
		t.Fatalf("the figure for transcription is %s, want 1.00 — 50 centavos rounds up", l.Peso)
	}

	down := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent},
		Entries: []Entry{
			// 8% of 6.00 is 0.48, which rounds down to nil.
			{Date: NewDate(2026, time.February, 1), Kind: Receipt, Amount: Pesos(250_006)},
		},
	})
	l2, _ := lineNamed(figures(t, down, 1), "Tax due to date at 8%")
	if l2.Exact != 48 || l2.Peso != 0 {
		t.Fatalf("0.48 should transcribe as nil, got exact %s and peso %s", l2.Exact, l2.Peso)
	}
}

func TestFiguresDescribeTheElection(t *testing.T) {
	cases := []struct {
		name     string
		election Election
		wants    []string
	}{
		{"the 8% election", Election{Regime: EightPercent},
			[]string{"8% in lieu of the graduated rates and the percentage tax", "irrevocable"}},
		{"graduated with the standard deduction", Election{Regime: Graduated, Deduction: OSD},
			[]string{"the graduated rates", "optional standard deduction, 40% of net sales"}},
		{"graduated with itemised deductions", Election{Regime: Graduated, Deduction: Itemized},
			[]string{"the graduated rates", "itemised deductions"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			y := mustCompute(t, YearInput{Year: 2026, Election: c.election,
				Entries: quarterReceipts(2026, 300_000)})
			f := figures(t, y, 1)
			b, ok := blockNamed(f, "What the return has to say")
			if !ok {
				t.Fatal("no block describing the election")
			}
			joined := ""
			for _, l := range b.Lines {
				joined += l.Label + "|" + l.Text + "|" + l.Note + "\n"
			}
			for _, w := range c.wants {
				if !strings.Contains(joined, w) {
					t.Errorf("the election block never mentions %q:\n%s", w, joined)
				}
			}
		})
	}
}

func TestFiguresShowTheSecondInstalmentOnlyOnTheAnnualReturn(t *testing.T) {
	el := Election{Regime: Graduated, Deduction: OSD}
	for i := range el.Payments {
		el.Payments[i] = QuarterPayment{Recorded: true, Amount: 0}
	}
	y := mustCompute(t, YearInput{Year: 2026, Election: el,
		Entries: quarterReceipts(2026, 300_000, 300_000, 300_000, 300_000)})

	annual := figures(t, y, 4)
	l, ok := lineNamed(annual, "deferrable to the second instalment")
	if !ok {
		t.Fatal("the annual return should offer the second instalment")
	}
	if l.Peso != Pesos(43_250) {
		t.Errorf("deferrable = %s, want 43,250.00", l.Peso)
	}
	if !strings.Contains(l.Note, "15 October 2027") {
		t.Errorf("the instalment note should name the date: %q", l.Note)
	}
	if pay, ok := lineNamed(annual, "Pay with the return"); !ok || pay.Peso != Pesos(43_250) {
		t.Errorf("pay with the return = %s, want 43,250.00", pay.Peso)
	}
	if _, ok := lineNamed(figures(t, y, 1), "deferrable to the second instalment"); ok {
		t.Error("a quarterly return has no second instalment")
	}
}

func TestFiguresSayPenaltiesAreNotComputed(t *testing.T) {
	y := mustCompute(t, YearInput{Year: 2026, Election: Election{Regime: EightPercent},
		Entries: quarterReceipts(2026, 300_000)})
	l, ok := lineNamed(figures(t, y, 1), "Penalties")
	if !ok {
		t.Fatal("the penalties line should be present and say it is not computed")
	}
	if l.Kind != Choice || l.Text != "not computed here" {
		t.Fatalf("the penalties line reads %q / %q", l.Kind, l.Text)
	}
}

func TestFiguresMarkAnOverpaymentAsSuch(t *testing.T) {
	y := mustCompute(t, YearInput{
		Year:     2026,
		Election: Election{Regime: EightPercent},
		Entries: []Entry{
			{Date: NewDate(2026, time.February, 1), Kind: Receipt, Amount: Pesos(1_000_000)},
			{Date: NewDate(2026, time.May, 1), Kind: SalesReturn, Amount: Pesos(600_000)},
		},
	})
	l, ok := lineNamed(figures(t, y, 2), "Tax payable / (overpayment)")
	if !ok {
		t.Fatal("no payable line")
	}
	if l.Peso != -Pesos(48_000) {
		t.Errorf("the payable line = %s, want -48,000.00", l.Peso)
	}
	if !strings.Contains(l.Note, "brackets") {
		t.Errorf("an overpayment should be explained: %q", l.Note)
	}
}

func TestFiguresFlagAnAssumedPriorPayment(t *testing.T) {
	y := mustCompute(t, YearInput{Year: 2026, Election: Election{Regime: EightPercent},
		Entries: quarterReceipts(2026, 300_000, 200_000)})
	l, _ := lineNamed(figures(t, y, 2), "Tax payments for the previous quarters")
	if !strings.Contains(l.Note, "not a payment you recorded") {
		t.Errorf("an assumed payment should be flagged on the figure itself: %q", l.Note)
	}
	el := Election{Regime: EightPercent}
	el.Payments[0] = QuarterPayment{Recorded: true, Amount: Pesos(4_000)}
	rec := mustCompute(t, YearInput{Year: 2026, Election: el,
		Entries: quarterReceipts(2026, 300_000, 200_000)})
	l2, _ := lineNamed(figures(t, rec, 2), "Tax payments for the previous quarters")
	if l2.Note != "" {
		t.Errorf("a recorded payment needs no warning: %q", l2.Note)
	}
}

func TestFiguresMentionCompensationIncome(t *testing.T) {
	y := mustCompute(t, YearInput{Year: 2026,
		Election: Election{Regime: EightPercent, HasCompensation: true},
		Entries:  quarterReceipts(2026, 300_000)})
	f := figures(t, y, 1)
	l, _ := lineNamed(f, "allowable reduction")
	if !strings.Contains(l.Note, "compensation income") {
		t.Errorf("the nil allowance should be explained: %q", l.Note)
	}
	due, ok := lineExact(f, "Tax due")
	if !ok {
		t.Fatal("no tax due line in the payable block")
	}
	if !strings.Contains(due.Note, "business income only") {
		t.Errorf("the tax due line should say it covers business income only: %q", due.Note)
	}
}

func TestFiguresRefuseABadQuarter(t *testing.T) {
	y := mustCompute(t, YearInput{Year: 2026, Election: Election{Regime: EightPercent}})
	for _, n := range []int{-1, 0, 5} {
		if _, err := y.Figures(n); err == nil {
			t.Errorf("Figures(%d) should be refused", n)
		}
	}
}

func TestEveryFigureLineIsNamed(t *testing.T) {
	for _, el := range []Election{
		{Regime: EightPercent},
		{Regime: Graduated, Deduction: OSD},
		{Regime: Graduated, Deduction: Itemized},
	} {
		y := mustCompute(t, YearInput{Year: 2026, Election: el,
			Entries: quarterReceipts(2026, 400_000, 400_000, 400_000, 400_000)})
		for q := 1; q <= QuartersPerYear; q++ {
			f := figures(t, y, q)
			if len(f.Blocks) == 0 {
				t.Fatalf("%v quarter %d produced no figures", el.Regime, q)
			}
			for _, b := range f.Blocks {
				if b.Title == "" {
					t.Errorf("%v quarter %d has an unnamed block", el.Regime, q)
				}
				if len(b.Lines) == 0 {
					t.Errorf("%v quarter %d block %q is empty", el.Regime, q, b.Title)
				}
				for _, l := range b.Lines {
					if l.Label == "" {
						t.Errorf("%v quarter %d block %q has an unlabelled line", el.Regime, q, b.Title)
					}
					if l.Kind == Choice && l.Text == "" {
						t.Errorf("%v quarter %d line %q is a choice with nothing to choose", el.Regime, q, l.Label)
					}
					if l.Kind != Choice && l.Peso != RoundToPeso(l.Exact) {
						t.Errorf("%v quarter %d line %q: %s does not round to %s",
							el.Regime, q, l.Label, l.Exact, l.Peso)
					}
				}
			}
		}
	}
}
