package phtax

import "fmt"

// This file lays the computed figures out as a list of named amounts to
// transcribe onto a return. It is deliberately a list and not a copy of the
// return: it is not the form, it is not a substitute for the form, and nothing
// here files, submits or validates anything. The return itself comes from the
// Bureau of Internal Revenue, on paper or through its own filing software.

// FigureKind is how a line should be read.
type FigureKind string

const (
	// Amount is a figure to transcribe.
	Amount FigureKind = "amount"
	// Subtotal is a figure derived from the lines above it.
	Subtotal FigureKind = "subtotal"
	// Total is the figure the block exists to produce.
	Total FigureKind = "total"
	// Choice is not an amount but a box to tick or a word to write.
	Choice FigureKind = "choice"
)

// A FigureLine is one named figure for transcription.
//
// Peso is the figure rounded to a whole peso, which is what the return asks
// for: "DO NOT enter Centavos; 49 Centavos or Less drop down; 50 or more round
// up". Exact is the same figure in centavos, so that a reader can see what was
// rounded.
type FigureLine struct {
	Label string     `json:"label"`
	Kind  FigureKind `json:"kind"`
	Peso  Centavos   `json:"peso"`
	Exact Centavos   `json:"exact"`
	Text  string     `json:"text,omitempty"` // for a Choice line
	Item  string     `json:"item,omitempty"` // the return's own item number, where it is known
	Note  string     `json:"note,omitempty"`
}

// A FigureBlock is a group of lines under one heading.
type FigureBlock struct {
	Title string       `json:"title"`
	Lines []FigureLine `json:"lines"`
}

// FormFigures is everything to transcribe for one return.
type FormFigures struct {
	Form             string        `json:"form"`
	Quarter          int           `json:"quarter"`
	Covers           string        `json:"covers"`
	Deadline         Date          `json:"deadline"`
	Blocks           []FigureBlock `json:"blocks"`
	ItemNumbersKnown bool          `json:"itemNumbersKnown"`
	Caveat           string        `json:"caveat"`
}

const notTheFormCaveat = "These are figures to transcribe, not the return. Get the return itself from the " +
	"Bureau of Internal Revenue — on paper or in its own filing software — and check every label against it. " +
	"This tool does not file, submit or validate anything."

const itemNumbersUnknown = "The item numbers of this return could not be sourced from a published copy of the " +
	"form, so these figures are named and not numbered — match them to the return by their labels. The " +
	"item numbers given for BIR Form No. 1701 were read from the form itself (January 2018 ENCS)."

// Figures lays out the figures for one quarter's return. Quarter 4 is the
// annual return.
func (y YearPosition) Figures(quarter int) (FormFigures, error) {
	if quarter < 1 || quarter > QuartersPerYear {
		return FormFigures{}, fmt.Errorf("there are %d quarters in a year, not a quarter %d", QuartersPerYear, quarter)
	}
	p := y.Quarters[quarter-1]
	f := FormFigures{
		Form:     p.Form,
		Quarter:  quarter,
		Deadline: p.Deadline,
		Caveat:   notTheFormCaveat,
	}
	if p.IsAnnual {
		f.Covers = fmt.Sprintf("the whole of %d", y.Year)
	} else {
		f.Covers = fmt.Sprintf("%s to %s, reported as the year to date",
			QuarterStart(y.Year, 1).Long(), p.Period.End.Long())
	}
	f.ItemNumbersKnown = p.Form == "1701"
	if !f.ItemNumbersKnown {
		f.Caveat += " " + itemNumbersUnknown
	}

	f.Blocks = append(f.Blocks, electionBlock(y, p))
	if p.Regime == EightPercent {
		f.Blocks = append(f.Blocks, eightPercentBlock(y, p, f.ItemNumbersKnown))
	} else {
		f.Blocks = append(f.Blocks, graduatedBlock(y, p, f.ItemNumbersKnown))
	}
	f.Blocks = append(f.Blocks, creditsBlock(y, p, f.ItemNumbersKnown))
	f.Blocks = append(f.Blocks, payableBlock(y, p, f.ItemNumbersKnown))
	if p.PercentageTax != nil {
		f.Blocks = append(f.Blocks, percentageBlock(*p.PercentageTax))
	}
	return f, nil
}

// QuarterStart is the first day of a quarter, which the year-to-date heading
// needs.
func QuarterStart(year, n int) Date {
	q, err := QuarterOf(year, n)
	if err != nil {
		return Date{}
	}
	return q.Start
}

func line(label string, c Centavos, kind FigureKind, item, note string) FigureLine {
	return FigureLine{Label: label, Kind: kind, Peso: RoundToPeso(c), Exact: c, Item: item, Note: note}
}

func choice(label, text, item, note string) FigureLine {
	return FigureLine{Label: label, Kind: Choice, Text: text, Item: item, Note: note}
}

func item(known bool, n string) string {
	if known {
		return n
	}
	return ""
}

func electionBlock(y YearPosition, p QuarterPosition) FigureBlock {
	b := FigureBlock{Title: "What the return has to say about the year"}
	b.Lines = append(b.Lines, choice("Taxable year", itoa(y.Year), "", ""))
	if p.IsAnnual {
		b.Lines = append(b.Lines, choice("Return", "BIR Form No. "+p.Form, "",
			annualFormWhy(y.Election)))
	} else {
		b.Lines = append(b.Lines, choice("Return", "BIR Form No. 1701Q, quarter "+itoa(p.Quarter), "",
			"The quarterly return reports the year to date and credits what the earlier quarters paid."))
	}
	switch p.Regime {
	case EightPercent:
		b.Lines = append(b.Lines, choice("Tax rate elected",
			rateString(p.TaxRateBP)+" in lieu of the graduated rates and the percentage tax", "",
			"NIRC sec. 24(A)(2)(b). The election is irrevocable for the taxable year."))
	default:
		method := "the optional standard deduction, " + rateString(p.OSDRateBP) + " of net sales"
		if p.Deduction == Itemized {
			method = "itemised deductions"
		}
		b.Lines = append(b.Lines, choice("Tax rate elected", "the graduated rates", "", "NIRC sec. 24(A)(2)(a)"))
		b.Lines = append(b.Lines, choice("Method of deduction", method, "", "NIRC sec. 34"))
	}
	return b
}

func annualFormWhy(e Election) string {
	switch {
	case e.HasCompensation:
		return "A mixed-income earner files BIR Form No. 1701, whichever rate was elected."
	case e.Regime == Graduated && e.Deduction == Itemized:
		return "Itemising deductions means BIR Form No. 1701 rather than 1701A."
	}
	return "An individual earning purely from business or a profession, on the 8% rate or the optional standard deduction, files BIR Form No. 1701A."
}

func eightPercentBlock(y YearPosition, p QuarterPosition, known bool) FigureBlock {
	b := FigureBlock{Title: "Income and tax due, year to date"}
	b.Lines = append(b.Lines,
		line("Sales / revenues / receipts / fees", p.Receipts, Amount, "", ""),
		line("Less: sales returns, allowances and discounts", p.SalesReturns, Amount, "", ""),
		line("Sales / receipts net of returns, allowances and discounts", p.NetSales, Subtotal, item(known, "26"), ""),
		line("Add: other non-operating income", p.NonOperating, Amount, item(known, "27"), ""),
		line("Total income", p.Gross, Subtotal, item(known, "28"), ""),
	)
	allowanceNote := "Deducted once against the year to date, not once a quarter."
	if y.Election.HasCompensation {
		allowanceNote = "Nil, because there is compensation income as well as business income."
	}
	b.Lines = append(b.Lines,
		line("Less: allowable reduction for an individual earning purely from business or a profession",
			p.Allowance, Amount, item(known, "29"), allowanceNote),
		line("Taxable income, year to date", p.Taxable, Subtotal, item(known, "30"), ""),
		line("Tax due to date at "+rateString(p.TaxRateBP), p.TaxDue, Total, item(known, "31"), ""),
	)
	return b
}

func graduatedBlock(y YearPosition, p QuarterPosition, known bool) FigureBlock {
	b := FigureBlock{Title: "Income and tax due, year to date"}
	b.Lines = append(b.Lines,
		line("Sales / revenues / receipts / fees", p.Receipts, Amount, item(known, "8"), ""),
		line("Less: sales returns, allowances and discounts", p.SalesReturns, Amount, item(known, "9"), ""),
		line("Net sales / revenues / receipts / fees", p.NetSales, Subtotal, item(known, "10"), ""),
	)
	if p.Deduction == Itemized {
		b.Lines = append(b.Lines,
			line("Less: cost of sales / services", p.CostOfServices, Amount, item(known, "11"),
				"The return notes this line as applicable only when itemising."),
			line("Gross income from operation", p.NetSales-p.CostOfServices, Subtotal, item(known, "12"), ""),
			line("Less: ordinary allowable itemised deductions", p.ItemizedDeduction, Amount, item(known, "13"), ""),
			line("Net income from operation", p.NetSales-p.DeductionTaken, Subtotal, item(known, "18"), ""),
		)
	} else {
		b.Lines = append(b.Lines,
			line("Less: optional standard deduction", p.OSDAmount, Amount, item(known, "17"),
				"NIRC sec. 34(L). The return computes it from net sales, not from gross."),
			line("Net income from operation", p.NetSales-p.OSDAmount, Subtotal, item(known, "18"), ""),
		)
	}
	b.Lines = append(b.Lines,
		line("Add: total other non-operating income", p.NonOperating, Amount, item(known, "22"), ""),
		line("Taxable income from business, year to date", p.Taxable, Subtotal, item(known, "23"), netIncomeNote(p)),
		line("Tax due to date under the graduated rates", p.TaxDue, Total, item(known, "25"),
			"Marginal rate at this level: "+rateString(p.MarginalBP)),
	)
	return b
}

func netIncomeNote(p QuarterPosition) string {
	if p.NetIncome < 0 {
		return fmt.Sprintf("The year to date is a loss of %s%s, so taxable income is nil.", pesoSign, -p.NetIncome)
	}
	return ""
}

func creditsBlock(y YearPosition, p QuarterPosition, known bool) FigureBlock {
	b := FigureBlock{Title: "Tax credits and payments, year to date"}
	paidLabel := "Tax payments for the previous quarters of this year"
	paidItem := item(known, "2")
	if p.IsAnnual {
		paidLabel = "Tax payments for the first three quarters"
	}
	paidNote := ""
	if p.PaidPriorQuartersSoft {
		paidNote = "This is this tool's own figure, not a payment you recorded. The return asks for what you actually paid."
	}
	b.Lines = append(b.Lines,
		line("Prior year's excess credits", p.PriorYearExcess, Amount, item(known, "1"), ""),
		line(paidLabel, p.PaidPriorQuarters, Amount, paidItem, paidNote),
		line("Creditable tax withheld for the previous quarters", p.WithheldPriorQuarters, Amount, item(known, "3"),
			"From the certificates on BIR Form No. 2307 your clients give you."),
		line("Creditable tax withheld for this quarter", p.WithheldThisQuarter, Amount, item(known, "4"), ""),
	)
	if p.OtherCredits > 0 {
		b.Lines = append(b.Lines, line("Other tax credits and payments", p.OtherCredits, Amount, item(known, "9"), ""))
	}
	b.Lines = append(b.Lines,
		line("Total tax credits and payments", p.TotalCredits, Total, item(known, "10"), ""),
	)
	return b
}

func payableBlock(y YearPosition, p QuarterPosition, known bool) FigureBlock {
	b := FigureBlock{Title: "What is left to pay"}
	payableNote := ""
	if p.Overpayment > 0 {
		payableNote = "Negative, so it goes on the return in brackets as an overpayment. There is nothing to pay."
	}
	dueNote := ""
	if y.Election.HasCompensation {
		dueNote = "This is the tax on your business income only. The return adds the tax on your compensation income to it."
	}
	b.Lines = append(b.Lines,
		line("Tax due", p.TaxDue, Amount, item(known, "22"), dueNote),
		line("Less: total tax credits and payments", p.TotalCredits, Amount, item(known, "23"), ""),
		line("Tax payable / (overpayment)", p.TaxPayable, Total, item(known, "24"), payableNote),
	)
	if p.IsAnnual && y.Instalment.Available {
		b.Lines = append(b.Lines,
			line("Of which, deferrable to the second instalment", y.Instalment.DeferrableNow, Amount, item(known, "25"),
				"Due "+y.Instalment.Due.Long()+". "+y.Instalment.Reference),
			line("Pay with the return", y.Instalment.PayWithReturn, Subtotal, item(known, "26"), ""),
		)
	}
	b.Lines = append(b.Lines, choice("Penalties: surcharge, interest, compromise", "not computed here",
		item(known, "27 to 30"),
		"This tool does not work out a penalty on a late or incorrect return. Ask the Bureau what is due."))
	return b
}

func percentageBlock(pt PercentageTaxQuarter) FigureBlock {
	return FigureBlock{
		Title: "Percentage tax for this quarter only, on BIR Form No. 2551Q",
		Lines: []FigureLine{
			line("Taxable amount: this quarter's sales net of returns, allowances and discounts",
				pt.Base, Amount, "",
				"This return is not cumulative. Nothing from an earlier quarter is credited against it."),
			line("Percentage tax due at "+rateString(pt.RateBP), pt.Due, Total, "", pt.Reference),
			choice("Due", pt.Deadline.Long(), "", "NIRC sec. 128(A)(1): within 25 days after the quarter ends"),
		},
	}
}
