package phtax

import (
	"fmt"
	"sort"
	"strings"
)

// EntryKind is what one recorded line is.
type EntryKind string

const (
	// Receipt is money earned from the business or the profession: an
	// invoice collected, a retainer, a platform payout. It is the "sales,
	// revenues, receipts, fees" figure on the return.
	Receipt EntryKind = "receipt"

	// SalesReturn is a credit note, a refund to a client, or a discount
	// given after the fact. It is the "sales returns, allowances and
	// discounts" figure, which is deducted from gross to give net sales.
	SalesReturn EntryKind = "return"

	// NonOperating is taxable income that is not from the business or
	// profession itself and is not subject to final tax: the return calls
	// it "other non-operating income". It is added to gross under both
	// regimes, including the 8% one.
	NonOperating EntryKind = "nonoperating"

	// Expense is a deductible cost. It changes the tax only under the
	// graduated rates with itemised deductions; under the optional standard
	// deduction and under the 8% rate it changes nothing, and the tool says
	// so rather than appearing to use it.
	Expense EntryKind = "expense"
)

// ExpenseCategory is which line of the return an expense belongs on. The two
// are deducted alike; they are reported separately because the return
// separates them.
type ExpenseCategory string

const (
	// CostOfServices is the direct cost of the work sold: a subcontractor,
	// a stock image licence bought for a client, a freelancer hired to
	// cover an overflow. BIR Form No. 1701 item 11, "Cost of
	// Sales/Services", which the form notes is "applicable only if
	// availing Itemized Deductions".
	CostOfServices ExpenseCategory = "cost"

	// OrdinaryDeduction is a running cost of the practice: internet,
	// software, rent, professional fees, taxes and licences. BIR Form No.
	// 1701 item 13, "Ordinary Allowable Itemized Deductions".
	OrdinaryDeduction ExpenseCategory = "deduction"
)

// An Entry is one line in the year's record.
type Entry struct {
	ID       string          `json:"id"`
	Date     Date            `json:"date"`
	Kind     EntryKind       `json:"kind"`
	Amount   Centavos        `json:"amount"`
	Client   string          `json:"client,omitempty"`
	Note     string          `json:"note,omitempty"`
	Withheld Centavos        `json:"withheld,omitempty"` // creditable tax withheld, per BIR Form No. 2307
	Category ExpenseCategory `json:"category,omitempty"` // expenses only
}

// Validate refuses an entry that cannot be used, rather than letting a stray
// figure through into a tax computation.
func (e Entry) Validate(year int) error {
	if !e.Date.Valid() {
		return fmt.Errorf("%q is not a date in the calendar", e.Date.String())
	}
	if e.Date.Year != year {
		return fmt.Errorf("the entry dated %s is not in the %d taxable year", e.Date, year)
	}
	switch e.Kind {
	case Receipt, SalesReturn, NonOperating, Expense:
	case "":
		return fmt.Errorf("say what the entry is: a receipt, a return, non-operating income or an expense")
	default:
		return fmt.Errorf("%q is not a kind of entry", e.Kind)
	}
	if e.Amount < 0 {
		return fmt.Errorf("the amount is negative; record money coming back from you as a %q, not as a negative receipt", SalesReturn)
	}
	if e.Amount == 0 {
		return fmt.Errorf("the amount is nil")
	}
	if e.Withheld < 0 {
		return fmt.Errorf("the tax withheld is negative")
	}
	if e.Withheld > 0 && e.Kind != Receipt && e.Kind != NonOperating {
		return fmt.Errorf("tax is withheld on income, not on a %q", e.Kind)
	}
	if e.Withheld > e.Amount {
		return fmt.Errorf("the tax withheld, %s, is more than the amount, %s", e.Withheld, e.Amount)
	}
	if e.Kind == Expense {
		switch e.Category {
		case CostOfServices, OrdinaryDeduction:
		case "":
			return fmt.Errorf("say whether the expense is a cost of services or an ordinary deduction")
		default:
			return fmt.Errorf("%q is not a category of expense", e.Category)
		}
	} else if e.Category != "" {
		return fmt.Errorf("only an expense has a category, and this is a %q", e.Kind)
	}
	return nil
}

// QuarterTotals are one quarter's own figures, before anything cumulative.
type QuarterTotals struct {
	Quarter           int      `json:"quarter"`
	Receipts          Centavos `json:"receipts"`
	SalesReturns      Centavos `json:"salesReturns"`
	NetSales          Centavos `json:"netSales"`
	NonOperating      Centavos `json:"nonOperating"`
	CostOfServices    Centavos `json:"costOfServices"`
	OrdinaryDeduction Centavos `json:"ordinaryDeductions"`
	Withheld          Centavos `json:"withheld"`
	Entries           int      `json:"entries"`
}

// GrossForEightPercent is the base the 8% rate works on before the allowance:
// sales net of returns, allowances and discounts, plus other non-operating
// income. BIR Form No. 1701 schedule 3.B item 26 asks for receipts "net of
// sales returns, allowances and discounts" and item 27 adds non-operating
// income.
func (t QuarterTotals) GrossForEightPercent() Centavos { return t.NetSales + t.NonOperating }

// Expenses is everything deductible in the quarter, whichever line of the
// return it sits on.
func (t QuarterTotals) Expenses() Centavos { return t.CostOfServices + t.OrdinaryDeduction }

// Totals adds the entries up by quarter. Entries outside the year are
// refused, because a figure in the wrong year silently changes two returns.
func Totals(year int, entries []Entry) ([QuartersPerYear]QuarterTotals, error) {
	var out [QuartersPerYear]QuarterTotals
	for i := range out {
		out[i].Quarter = i + 1
	}
	for _, e := range entries {
		if err := e.Validate(year); err != nil {
			return out, err
		}
		n, err := QuarterForDate(e.Date)
		if err != nil {
			return out, err
		}
		t := &out[n-1]
		t.Entries++
		t.Withheld += e.Withheld
		switch e.Kind {
		case Receipt:
			t.Receipts += e.Amount
		case SalesReturn:
			t.SalesReturns += e.Amount
		case NonOperating:
			t.NonOperating += e.Amount
		case Expense:
			if e.Category == CostOfServices {
				t.CostOfServices += e.Amount
			} else {
				t.OrdinaryDeduction += e.Amount
			}
		}
	}
	for i := range out {
		out[i].NetSales = out[i].Receipts - out[i].SalesReturns
	}
	return out, nil
}

// SortEntries orders entries by date, then by the order they were recorded,
// so that a listing reads like a ledger.
func SortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Date.Before(entries[j].Date)
	})
}

// ------------------------------------------------------------- pasted entries

// ParsedRows is the outcome of reading pasted rows: the entries that could be
// read and a line-numbered complaint for each one that could not.
type ParsedRows struct {
	Entries  []Entry  `json:"entries"`
	Problems []string `json:"problems"`
}

// ParseRows reads entries out of pasted text, which is what somebody with a
// spreadsheet of this year's invoices actually has. One entry per line:
//
//	date, amount, kind, client or note, tax withheld, expense category
//
// Only the date and the amount are required. The kind defaults to a receipt.
// The separator may be a comma or a tab, a quoted field may contain the
// separator, a heading row is skipped, and a blank line is ignored. A line that
// cannot be read is reported by its number and left out; nothing is guessed.
func ParseRows(text string, year int) ParsedRows {
	var out ParsedRows
	for i, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := i + 1
		if strings.TrimSpace(raw) == "" {
			continue
		}
		cols := splitRow(raw)
		col := func(n int) string {
			if n < len(cols) {
				return cols[n]
			}
			return ""
		}
		if looksLikeHeading(col(0)) {
			continue
		}
		if hint := splitAmountHint(cols); hint != "" {
			out.Problems = append(out.Problems, fmt.Sprintf("line %d: %s", line, hint))
			continue
		}
		d, err := ParseDate(col(0))
		if err != nil {
			out.Problems = append(out.Problems, fmt.Sprintf("line %d: %v", line, err))
			continue
		}
		amt, err := ParseAmount(col(1))
		if err != nil {
			out.Problems = append(out.Problems,
				fmt.Sprintf("line %d: the amount %q could not be read: %v", line, col(1), err))
			continue
		}
		kind, err := ParseKind(col(2))
		if err != nil {
			out.Problems = append(out.Problems, fmt.Sprintf("line %d: %v", line, err))
			continue
		}
		e := Entry{Date: d, Kind: kind, Amount: amt, Note: col(3)}
		if w := col(4); w != "" {
			wc, err := ParseAmount(w)
			if err != nil {
				out.Problems = append(out.Problems, fmt.Sprintf("line %d: the tax withheld %q could not be read: %v", line, w, err))
				continue
			}
			e.Withheld = wc
		}
		if kind == Expense {
			e.Category = OrdinaryDeduction
			if c := strings.ToLower(col(5)); strings.HasPrefix(c, "cost") {
				e.Category = CostOfServices
			}
		}
		if err := e.Validate(year); err != nil {
			out.Problems = append(out.Problems, fmt.Sprintf("line %d: %v", line, err))
			continue
		}
		out.Entries = append(out.Entries, e)
	}
	return out
}

// splitRow splits one pasted line into fields. The separator is a tab if the
// line has one and a comma otherwise, and a field wrapped in double quotes may
// contain the separator — which matters, because the one field most likely to
// hold a comma is the amount, and a spreadsheet quotes it.
func splitRow(raw string) []string {
	sep := byte(',')
	if strings.IndexByte(raw, '\t') >= 0 {
		sep = '\t'
	}
	var (
		out  []string
		cur  strings.Builder
		quot bool
	)
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c == '"' && quot && i+1 < len(raw) && raw[i+1] == '"':
			cur.WriteByte('"')
			i++
		case c == '"':
			quot = !quot
		case c == sep && !quot:
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	return append(out, strings.TrimSpace(cur.String()))
}

// splitAmountHint spots the one mistake that makes a row genuinely
// unreadable — an unquoted thousands separator, which splits the amount across
// two fields — and names it, rather than guessing where the amount ends. A
// group of exactly three digits where the kind belongs is never anything else.
func splitAmountHint(cols []string) string {
	if len(cols) < 3 {
		return ""
	}
	amount, third := strings.TrimPrefix(cols[1], "₱"), cols[2]
	if len(third) == 3 && allDigits(third) && allDigits(amount) {
		return "the amount looks split across two fields by an unquoted thousands separator (" +
			cols[1] + "," + third + "). Write it as one number, or put it in quotes."
	}
	return ""
}

func looksLikeHeading(first string) bool {
	switch strings.ToLower(first) {
	case "date", "dates", "transaction date", "invoice date":
		return true
	}
	return false
}

// ParseKind reads the kind of an entry from the words a person is likely to
// type for it. An empty kind is a receipt, because that is what most lines in
// a freelancer's year are.
func ParseKind(s string) (EntryKind, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "receipt", "receipts", "income", "sale", "sales", "fee", "fees", "invoice", "revenue":
		return Receipt, nil
	case "return", "returns", "refund", "credit", "credit note", "discount", "allowance":
		return SalesReturn, nil
	case "nonoperating", "non-operating", "non operating", "other", "other income":
		return NonOperating, nil
	case "expense", "expenses", "cost", "costs", "deduction":
		return Expense, nil
	}
	return "", fmt.Errorf("%q is not a kind of entry; use receipt, return, nonoperating or expense", s)
}
