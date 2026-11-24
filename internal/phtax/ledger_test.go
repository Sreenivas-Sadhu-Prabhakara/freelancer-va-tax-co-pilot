package phtax

import (
	"strings"
	"testing"
	"time"
)

func receipt(month time.Month, day int, pesos int64) Entry {
	return Entry{Date: NewDate(2026, month, day), Kind: Receipt, Amount: Pesos(pesos)}
}

func TestTotalsByQuarter(t *testing.T) {
	entries := []Entry{
		receipt(time.January, 10, 50_000),
		receipt(time.March, 31, 25_000),
		{Date: NewDate(2026, time.February, 1), Kind: SalesReturn, Amount: Pesos(5_000)},
		{Date: NewDate(2026, time.February, 15), Kind: Expense, Amount: Pesos(3_000), Category: OrdinaryDeduction},
		{Date: NewDate(2026, time.February, 16), Kind: Expense, Amount: Pesos(7_000), Category: CostOfServices},
		{Date: NewDate(2026, time.March, 1), Kind: NonOperating, Amount: Pesos(1_000)},
		{Date: NewDate(2026, time.April, 1), Kind: Receipt, Amount: Pesos(100_000), Withheld: Pesos(10_000)},
		receipt(time.December, 31, 1_000),
	}
	got, err := Totals(2026, entries)
	if err != nil {
		t.Fatalf("Totals: %v", err)
	}

	q1 := got[0]
	if q1.Receipts != Pesos(75_000) {
		t.Errorf("Q1 receipts = %s, want 75,000.00", q1.Receipts)
	}
	if q1.SalesReturns != Pesos(5_000) {
		t.Errorf("Q1 sales returns = %s, want 5,000.00", q1.SalesReturns)
	}
	if q1.NetSales != Pesos(70_000) {
		t.Errorf("Q1 net sales = %s, want 70,000.00", q1.NetSales)
	}
	if q1.NonOperating != Pesos(1_000) {
		t.Errorf("Q1 non-operating = %s, want 1,000.00", q1.NonOperating)
	}
	if q1.CostOfServices != Pesos(7_000) || q1.OrdinaryDeduction != Pesos(3_000) {
		t.Errorf("Q1 expenses split wrongly: cost %s, ordinary %s", q1.CostOfServices, q1.OrdinaryDeduction)
	}
	if q1.Expenses() != Pesos(10_000) {
		t.Errorf("Q1 expenses = %s, want 10,000.00", q1.Expenses())
	}
	if q1.GrossForEightPercent() != Pesos(71_000) {
		t.Errorf("Q1 gross for the 8%% base = %s, want 71,000.00", q1.GrossForEightPercent())
	}
	if q1.Entries != 6 {
		t.Errorf("Q1 holds %d entries, want 6", q1.Entries)
	}

	if got[1].Receipts != Pesos(100_000) || got[1].Withheld != Pesos(10_000) {
		t.Errorf("Q2 = %s receipts, %s withheld", got[1].Receipts, got[1].Withheld)
	}
	if got[2].Entries != 0 {
		t.Errorf("Q3 should be empty, has %d entries", got[2].Entries)
	}
	if got[3].Receipts != Pesos(1_000) {
		t.Errorf("Q4 receipts = %s, want 1,000.00", got[3].Receipts)
	}
	for i, q := range got {
		if q.Quarter != i+1 {
			t.Errorf("quarter %d is numbered %d", i+1, q.Quarter)
		}
	}
}

func TestEntryValidation(t *testing.T) {
	cases := []struct {
		name  string
		entry Entry
		want  string
	}{
		{"a plain receipt is fine", receipt(time.January, 10, 1_000), ""},
		{"a date in another year", receipt(time.January, 10, 1_000).inYear(2025), "not in the 2026 taxable year"},
		{"an impossible date", Entry{Date: NewDate(2026, time.February, 30), Kind: Receipt, Amount: Pesos(1)}, "not a date in the calendar"},
		{"no kind", Entry{Date: NewDate(2026, time.January, 1), Amount: Pesos(1)}, "say what the entry is"},
		{"an unknown kind", Entry{Date: NewDate(2026, time.January, 1), Kind: "bribe", Amount: Pesos(1)}, "not a kind of entry"},
		{"a nil amount", Entry{Date: NewDate(2026, time.January, 1), Kind: Receipt}, "nil"},
		{"a negative amount", Entry{Date: NewDate(2026, time.January, 1), Kind: Receipt, Amount: -Pesos(1)}, "negative"},
		{"withholding on an expense", Entry{Date: NewDate(2026, time.January, 1), Kind: Expense, Category: CostOfServices, Amount: Pesos(100), Withheld: Pesos(10)}, "withheld on income"},
		{"withholding above the amount", Entry{Date: NewDate(2026, time.January, 1), Kind: Receipt, Amount: Pesos(100), Withheld: Pesos(200)}, "more than the amount"},
		{"negative withholding", Entry{Date: NewDate(2026, time.January, 1), Kind: Receipt, Amount: Pesos(100), Withheld: -Pesos(1)}, "negative"},
		{"an expense with no category", Entry{Date: NewDate(2026, time.January, 1), Kind: Expense, Amount: Pesos(100)}, "cost of services or an ordinary deduction"},
		{"an expense with a bad category", Entry{Date: NewDate(2026, time.January, 1), Kind: Expense, Amount: Pesos(100), Category: "misc"}, "not a category"},
		{"a receipt with a category", Entry{Date: NewDate(2026, time.January, 1), Kind: Receipt, Amount: Pesos(100), Category: CostOfServices}, "only an expense has a category"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.entry.Validate(2026)
			if c.want == "" {
				if err != nil {
					t.Fatalf("Validate: %v, want no error", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate accepted the entry, want an error mentioning %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Validate: %v, want an error mentioning %q", err, c.want)
			}
		})
	}
}

func (e Entry) inYear(y int) Entry {
	e.Date.Year = y
	return e
}

func TestTotalsRefusesABadEntry(t *testing.T) {
	_, err := Totals(2026, []Entry{receipt(time.January, 1, 100).inYear(2025)})
	if err == nil {
		t.Fatal("Totals should refuse an entry from another year rather than drop it")
	}
}

func TestParseRows(t *testing.T) {
	text := `Date,Amount,Kind,Note,Withheld
2026-01-15,"25,000",receipt,Acme retainer,2500
2026-02-01	18000	receipt	Tab separated

2026-02-10,"₱3,000.00",expense,Internet,,cost
2026-03-04,500,refund,"Overbilled Acme, refunded"
2026-03-05,1200,nonoperating,Bank interest
nonsense,1,receipt,bad date
2026-03-06,twelve,receipt,bad amount
2026-03-07,100,bribe,bad kind
2025-12-31,100,receipt,wrong year
2026-03-08,3,000,receipt,unquoted thousands separator
`
	got := ParseRows(strings.ReplaceAll(text, `₱`, "₱"), 2026)

	if len(got.Entries) != 5 {
		for _, e := range got.Entries {
			t.Logf("read: %s %s %s", e.Date, e.Kind, e.Amount)
		}
		t.Fatalf("read %d entries, want 5", len(got.Entries))
	}
	if len(got.Problems) != 5 {
		t.Fatalf("reported %d problems, want 5: %v", len(got.Problems), got.Problems)
	}
	// The unreadable amount is named, not guessed at.
	if !strings.Contains(got.Problems[4], "unquoted thousands separator") {
		t.Errorf("the last problem should explain the split amount: %q", got.Problems[4])
	}
	for _, want := range []string{"line 8", "line 9", "line 10", "line 11", "line 12"} {
		found := false
		for _, p := range got.Problems {
			if strings.HasPrefix(p, want+":") {
				found = true
			}
		}
		if !found {
			t.Errorf("no problem reported for %s: %v", want, got.Problems)
		}
	}

	first := got.Entries[0]
	if first.Amount != Pesos(25_000) || first.Withheld != Pesos(2_500) || first.Note != "Acme retainer" {
		t.Errorf("the first row read as %+v", first)
	}
	if got.Entries[1].Amount != Pesos(18_000) {
		t.Errorf("the tab-separated row read as %s", got.Entries[1].Amount)
	}
	exp := got.Entries[2]
	if exp.Kind != Expense || exp.Amount != Pesos(3_000) {
		t.Errorf("the expense row read as %+v", exp)
	}
	if exp.Category != CostOfServices {
		t.Errorf("the expense category read as %q, want %q", exp.Category, CostOfServices)
	}
	if got.Entries[3].Kind != SalesReturn {
		t.Errorf("\"refund\" read as %q, want %q", got.Entries[3].Kind, SalesReturn)
	}
	if got.Entries[3].Note != "Overbilled Acme, refunded" {
		t.Errorf("a quoted note containing a comma read as %q", got.Entries[3].Note)
	}
	if got.Entries[4].Kind != NonOperating {
		t.Errorf("\"nonoperating\" read as %q", got.Entries[4].Kind)
	}
}

func TestParseRowsEmptyAndHeadingOnly(t *testing.T) {
	for _, in := range []string{"", "   \n\n", "date,amount,kind\n"} {
		got := ParseRows(in, 2026)
		if len(got.Entries) != 0 || len(got.Problems) != 0 {
			t.Errorf("ParseRows(%q) = %d entries, %d problems; want nothing at all", in, len(got.Entries), len(got.Problems))
		}
	}
}

func TestParseKind(t *testing.T) {
	cases := []struct {
		in   string
		want EntryKind
		bad  bool
	}{
		{in: "", want: Receipt},
		{in: "receipt", want: Receipt},
		{in: "INVOICE", want: Receipt},
		{in: " fees ", want: Receipt},
		{in: "refund", want: SalesReturn},
		{in: "credit note", want: SalesReturn},
		{in: "non-operating", want: NonOperating},
		{in: "other income", want: NonOperating},
		{in: "expense", want: Expense},
		{in: "deduction", want: Expense},
		{in: "whatever", bad: true},
	}
	for _, c := range cases {
		got, err := ParseKind(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("ParseKind(%q) = %q, want an error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseKind(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseKind(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSortEntries(t *testing.T) {
	in := []Entry{
		receipt(time.December, 1, 3),
		receipt(time.January, 1, 1),
		receipt(time.June, 1, 2),
	}
	SortEntries(in)
	for i, want := range []time.Month{time.January, time.June, time.December} {
		if in[i].Date.Month != want {
			t.Fatalf("after sorting, entry %d is in %s, want %s", i, in[i].Date.Month, want)
		}
	}
}
