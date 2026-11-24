package phtax

import "fmt"

// Regime is the way the taxpayer has elected to be taxed on business or
// professional income.
//
// Choosing between them is a separate job from the one this package does.
// Here the regime is an input: the taxpayer has already elected it, on
// registration or on the first quarterly return of the year, and under NIRC
// sec. 24(A)(2)(b) that election is irrevocable for the taxable year.
type Regime string

const (
	// EightPercent is the flat 8% on gross, in lieu of the graduated income
	// tax and the percentage tax.
	EightPercent Regime = "eight-percent"
	// Graduated is the graduated income tax on net income, with the
	// percentage tax payable separately.
	Graduated Regime = "graduated"
)

// DeductionMethod is how net income is arrived at under the graduated rates.
type DeductionMethod string

const (
	// Itemized deducts the costs actually recorded. NIRC sec. 34(A)-(J).
	Itemized DeductionMethod = "itemized"
	// OSD takes the optional standard deduction instead. NIRC sec. 34(L).
	OSD DeductionMethod = "osd"
)

// Severity is how loudly a note should be read.
type Severity string

const (
	// Info is something worth knowing.
	Info Severity = "info"
	// Warn is something that changes what to file.
	Warn Severity = "warn"
	// Stop is something that makes the computation as elected wrong.
	Stop Severity = "stop"
)

// A Note is a remark against a quarter or a year, with the provision behind
// it where there is one.
type Note struct {
	Severity  Severity `json:"severity"`
	Text      string   `json:"text"`
	Reference string   `json:"reference,omitempty"`
}

// A QuarterPayment is what was actually paid with one quarter's return. It is
// the taxpayer's own record: the figure that goes on the next quarter's return
// as "tax payment/s for the previous quarter/s" is what was paid, not what
// this tool thinks should have been.
type QuarterPayment struct {
	Recorded bool     `json:"recorded"`
	Amount   Centavos `json:"amount"`
	FiledOn  Date     `json:"filedOn"`
}

// An Election is everything the taxpayer has chosen or been landed with for
// the year, other than the figures themselves.
type Election struct {
	Regime          Regime                          `json:"regime"`
	Deduction       DeductionMethod                 `json:"deduction"`
	HasCompensation bool                            `json:"hasCompensation"`
	VATRegistered   bool                            `json:"vatRegistered"`
	PriorYearExcess Centavos                        `json:"priorYearExcess"`
	OtherCredits    Centavos                        `json:"otherCredits"`
	Payments        [QuartersPerYear]QuarterPayment `json:"payments"`
}

// A PercentageTaxQuarter is the quarterly percentage tax position.
//
// It is not cumulative. The income tax return reports the year to date and
// credits what was paid before; the percentage tax return under NIRC sec.
// 128(A)(1) reports one quarter and credits nothing. Treating the two alike is
// the commonest way to get a quarter wrong, so they are kept apart here.
type PercentageTaxQuarter struct {
	Quarter   int      `json:"quarter"`
	Base      Centavos `json:"base"`
	RateBP    int      `json:"rateBasisPoints"`
	Due       Centavos `json:"due"`
	Deadline  Date     `json:"deadline"`
	Reference string   `json:"reference"`
}

// A QuarterPosition is the whole income tax position as at the end of one
// quarter: the year to date, the tax on it, the credits against it, and what
// is left to pay. Quarter 4 is the annual return, which is the same
// computation over the whole year.
type QuarterPosition struct {
	Quarter  int     `json:"quarter"`
	Period   Quarter `json:"period"`
	IsAnnual bool    `json:"isAnnual"`
	Form     string  `json:"form"`
	Regime   Regime  `json:"regime"`

	// This quarter's own figures, which only the percentage tax uses.
	This QuarterTotals `json:"this"`

	// The year to date.
	Receipts     Centavos `json:"receipts"`
	SalesReturns Centavos `json:"salesReturns"`
	NetSales     Centavos `json:"netSales"`
	NonOperating Centavos `json:"nonOperating"`
	Gross        Centavos `json:"gross"`

	// The graduated path.
	Deduction         DeductionMethod `json:"deduction,omitempty"`
	CostOfServices    Centavos        `json:"costOfServices"`
	ItemizedDeduction Centavos        `json:"itemizedDeduction"`
	OSDAmount         Centavos        `json:"osdAmount"`
	OSDRateBP         int             `json:"osdRateBasisPoints,omitempty"`
	DeductionTaken    Centavos        `json:"deductionTaken"`
	NetIncome         Centavos        `json:"netIncome"` // signed: a loss is negative

	// The 8% path.
	Allowance Centavos `json:"allowance"`

	// Both paths meet here.
	Taxable    Centavos `json:"taxable"` // never negative; a loss is nil
	TaxDue     Centavos `json:"taxDue"`
	TaxRateBP  int      `json:"taxRateBasisPoints,omitempty"` // 8% regime only
	MarginalBP int      `json:"marginalRateBasisPoints"`

	// The credits, in the order the return lists them.
	PriorYearExcess       Centavos `json:"priorYearExcess"`
	PaidPriorQuarters     Centavos `json:"paidPriorQuarters"`
	PaidPriorQuartersSoft bool     `json:"paidPriorQuartersAssumed"`
	WithheldPriorQuarters Centavos `json:"withheldPriorQuarters"`
	WithheldThisQuarter   Centavos `json:"withheldThisQuarter"`
	OtherCredits          Centavos `json:"otherCredits"`
	TotalCredits          Centavos `json:"totalCredits"`

	// The result. TaxPayable is signed, the way the return's own
	// "Tax Payable/(Overpayment)" line is. AmountToPay and Overpayment are
	// the two halves of it, so that nothing downstream has to remember that
	// a negative payable is not a payment.
	TaxPayable  Centavos `json:"taxPayable"`
	AmountToPay Centavos `json:"amountToPay"`
	Overpayment Centavos `json:"overpayment"`

	// Filing.
	Deadline          Date `json:"deadline"`
	DeadlineIsWeekend bool `json:"deadlineIsWeekend"`

	PercentageTax *PercentageTaxQuarter `json:"percentageTax,omitempty"`

	Notes []Note `json:"notes,omitempty"`
}

// SecondInstalment is the split the annual return allows.
type SecondInstalment struct {
	Available     bool     `json:"available"`
	HalfOfTaxDue  Centavos `json:"halfOfTaxDue"`
	DeferrableNow Centavos `json:"deferrableNow"`
	PayWithReturn Centavos `json:"payWithReturn"`
	Due           Date     `json:"due"`
	MinimumTaxDue Centavos `json:"minimumTaxDue"`
	Reference     string   `json:"reference"`
}

// A Projection is where the year lands if the rest of it looks like the part
// already recorded. It is a straight-line annualisation of the quarters that
// have ended, and nothing more: it is not a forecast, and a freelancer's year
// is rarely even.
type Projection struct {
	Available        bool     `json:"available"`
	QuartersCounted  int      `json:"quartersCounted"`
	GrossToDate      Centavos `json:"grossToDate"`
	ProjectedGross   Centavos `json:"projectedGross"`
	ProjectedTax     Centavos `json:"projectedTax"`
	CrossesThreshold bool     `json:"crossesThreshold"`
}

// A DeadlineItem is one filing date in the year's calendar.
type DeadlineItem struct {
	Form      string `json:"form"`
	What      string `json:"what"`
	Covers    string `json:"covers"`
	Due       Date   `json:"due"`
	DaysAway  int    `json:"daysAway"`
	Status    string `json:"status"` // overdue, today, soon, upcoming
	IsWeekend bool   `json:"isWeekend"`
	Reference string `json:"reference"`
}

// A ThresholdBreach records the quarter in which cumulative gross passed the
// registration threshold.
type ThresholdBreach struct {
	Crossed   bool     `json:"crossed"`
	Quarter   int      `json:"quarter"`
	Gross     Centavos `json:"gross"`
	Threshold Centavos `json:"threshold"`
}

// A YearPosition is the whole year: four positions, the year's notes, the
// filing calendar, and — where the election has come apart — the same year
// worked again under the graduated rates.
type YearPosition struct {
	Year          int                            `json:"year"`
	Election      Election                       `json:"election"`
	Totals        [QuartersPerYear]QuarterTotals `json:"totals"`
	Quarters      []QuarterPosition              `json:"quarters"`
	Annual        QuarterPosition                `json:"annual"`
	Instalment    SecondInstalment               `json:"instalment"`
	Projection    Projection                     `json:"projection"`
	Threshold     ThresholdBreach                `json:"threshold"`
	Recomputed    []QuarterPosition              `json:"recomputed,omitempty"`
	RecomputedWhy string                         `json:"recomputedWhy,omitempty"`
	AnnualForm    string                         `json:"annualForm"`
	Deadlines     []DeadlineItem                 `json:"deadlines"`
	Notes         []Note                         `json:"notes,omitempty"`
}

// YearInput is everything ComputeYear needs.
type YearInput struct {
	Year     int
	Election Election
	Entries  []Entry
	Rates    Rates
	Today    Date
}

// ComputeYear works the whole year out. It is pure: the same input gives the
// same output, and nothing here reads a clock, a file or a network.
func ComputeYear(in YearInput) (YearPosition, error) {
	rates := in.Rates.normalise()
	if _, ok := rates.Schedule(in.Year); !ok {
		return YearPosition{}, fmt.Errorf("no graduated rate table is on record for the %d taxable year; add one before computing it", in.Year)
	}
	el := in.Election.normalise()
	totals, err := Totals(in.Year, in.Entries)
	if err != nil {
		return YearPosition{}, err
	}
	today := in.Today
	if !today.Valid() {
		today = Today()
	}

	out := YearPosition{Year: in.Year, Election: el, Totals: totals}

	// Where did cumulative gross pass the threshold?
	var cum Centavos
	for n := 1; n <= QuartersPerYear; n++ {
		cum += totals[n-1].GrossForEightPercent()
		if cum > rates.VATThreshold && !out.Threshold.Crossed {
			out.Threshold = ThresholdBreach{Crossed: true, Quarter: n, Gross: cum, Threshold: rates.VATThreshold}
		}
	}

	out.Quarters = computePositions(in.Year, el, el.Regime, el.Deduction, totals, rates, out.Threshold, nil)
	out.Annual = out.Quarters[QuartersPerYear-1]
	out.AnnualForm = annualForm(el)
	out.Instalment = instalment(out.Annual, rates, in.Year)
	out.Projection = project(in.Year, el, totals, rates, today)
	out.Deadlines = calendar(in.Year, rates, today)
	out.Notes = yearNotes(in.Year, el, rates, out.Threshold, out.Annual)

	// An election that has come apart is worked again the other way, with
	// what was paid under it credited, because that is what the law then
	// requires of the taxpayer.
	if el.Regime == EightPercent && (out.Threshold.Crossed || el.VATRegistered) {
		// Where a quarter's payment was never recorded, the recomputation
		// falls back to what was due under the election the taxpayer was
		// actually on at the time — the 8% figure — and not to the
		// graduated figure, which was never paid.
		var paidUnderElection []Centavos
		for _, q := range out.Quarters {
			paidUnderElection = append(paidUnderElection, q.AmountToPay)
		}
		out.Recomputed = computePositions(in.Year, el, Graduated, el.Deduction, totals, rates, out.Threshold, paidUnderElection)
		if el.VATRegistered {
			out.RecomputedWhy = "You are registered for value-added tax, so the 8% option is not open to you. " +
				"The same year is worked here under the graduated rates, with what you have already paid credited."
		} else {
			out.RecomputedWhy = fmt.Sprintf(
				"Gross passed %s%s in quarter %d, so the 8%% election falls away for the whole year. "+
					"The same year is worked here under the graduated rates, with the 8%% you have already paid credited against it.",
				pesoSign, rates.VATThreshold, out.Threshold.Quarter)
		}
	}
	return out, nil
}

const pesoSign = "₱"

func (e Election) normalise() Election {
	if e.Regime != EightPercent && e.Regime != Graduated {
		e.Regime = Graduated
	}
	if e.Deduction != Itemized && e.Deduction != OSD {
		e.Deduction = OSD
	}
	if e.PriorYearExcess < 0 {
		e.PriorYearExcess = 0
	}
	if e.OtherCredits < 0 {
		e.OtherCredits = 0
	}
	for i := range e.Payments {
		if e.Payments[i].Amount < 0 {
			e.Payments[i].Amount = 0
		}
	}
	return e
}

func annualForm(e Election) string {
	// BIR Form No. 1701A is for an individual earning purely from business
	// or a profession and taking either the 8% rate or the optional
	// standard deduction. Anyone with compensation income as well, and
	// anyone itemising, files BIR Form No. 1701.
	if e.HasCompensation {
		return "1701"
	}
	if e.Regime == Graduated && e.Deduction == Itemized {
		return "1701"
	}
	return "1701A"
}

// computePositions walks the four quarters forward. The forward walk is the
// point: each quarter's credits include what the quarters before it paid, so
// quarter three cannot be worked out without having worked out one and two.
// fallbackPaid, when given, is what to credit for a quarter whose payment the
// taxpayer never recorded. A nil fallback means "credit what this same
// computation says was due", which is the best guess available for the elected
// regime; a recomputation passes the figures from the regime the taxpayer was
// actually on.
func computePositions(year int, el Election, regime Regime, deduction DeductionMethod,
	totals [QuartersPerYear]QuarterTotals, rates Rates, breach ThresholdBreach,
	fallbackPaid []Centavos) []QuarterPosition {

	out := make([]QuarterPosition, 0, QuartersPerYear)

	var (
		cumReceipts, cumReturns, cumNonOp Centavos
		cumCost, cumOrdinary, cumWithheld Centavos
		paidBefore                        Centavos
		paidBeforeSoft                    bool
		withheldBefore                    Centavos
	)

	for n := 1; n <= QuartersPerYear; n++ {
		t := totals[n-1]
		q, _ := QuarterOf(year, n)

		cumReceipts += t.Receipts
		cumReturns += t.SalesReturns
		cumNonOp += t.NonOperating
		cumCost += t.CostOfServices
		cumOrdinary += t.OrdinaryDeduction
		withheldBefore = cumWithheld
		cumWithheld += t.Withheld

		p := QuarterPosition{
			Quarter: n, Period: q, IsAnnual: n == QuartersPerYear, Regime: regime,
			This:         t,
			Receipts:     cumReceipts,
			SalesReturns: cumReturns,
			NetSales:     cumReceipts - cumReturns,
			NonOperating: cumNonOp,
		}
		p.Gross = p.NetSales + p.NonOperating
		p.Form = "1701Q"
		if p.IsAnnual {
			p.Form = annualForm(el)
		}

		switch regime {
		case EightPercent:
			p.TaxRateBP = rates.EightPercentRateBP
			p.MarginalBP = rates.EightPercentRateBP
			// The allowance is deducted once, against the year to date,
			// and never below nil: that is what a cumulative return does,
			// and it is why the first quarter of a small year shows no tax.
			if !el.HasCompensation {
				p.Allowance = rates.EightPercentAllowance
				if p.Gross < p.Allowance {
					p.Allowance = atLeastZero(p.Gross)
				}
			}
			p.Taxable = atLeastZero(p.Gross - p.Allowance)
			p.TaxDue = applyRate(p.Taxable, rates.EightPercentRateBP)
		default:
			p.Deduction = deduction
			p.CostOfServices = cumCost
			p.ItemizedDeduction = cumOrdinary
			p.OSDRateBP = rates.OSDRateBP
			p.OSDAmount = applyRate(p.NetSales, rates.OSDRateBP)
			if deduction == OSD {
				// The optional standard deduction replaces both the cost
				// of services and the itemised deductions: the return
				// marks the cost-of-sales line "applicable only if
				// availing Itemized Deductions".
				p.DeductionTaken = p.OSDAmount
				p.NetIncome = p.NetSales - p.OSDAmount + p.NonOperating
			} else {
				p.DeductionTaken = cumCost + cumOrdinary
				p.NetIncome = p.NetSales - p.DeductionTaken + p.NonOperating
			}
			p.Taxable = atLeastZero(p.NetIncome)
			p.TaxDue, _ = rates.GraduatedTax(p.Taxable, year)
			p.MarginalBP = rates.marginalBP(p.Taxable, year)
			if !el.VATRegistered {
				p.PercentageTax = percentageTax(year, n, t, rates)
			}
		}

		// Credits, year to date, in the order the return lists them.
		p.PriorYearExcess = el.PriorYearExcess
		p.PaidPriorQuarters = paidBefore
		p.PaidPriorQuartersSoft = paidBeforeSoft
		p.WithheldPriorQuarters = withheldBefore
		p.WithheldThisQuarter = t.Withheld
		p.OtherCredits = el.OtherCredits
		p.TotalCredits = p.PriorYearExcess + p.PaidPriorQuarters + cumWithheld + p.OtherCredits

		p.TaxPayable = p.TaxDue - p.TotalCredits
		p.AmountToPay = atLeastZero(p.TaxPayable)
		p.Overpayment = atLeastZero(-p.TaxPayable)

		p.Deadline, _ = rates.Deadlines.IncomeTaxDue(year, n)
		p.DeadlineIsWeekend = p.Deadline.IsWeekend()

		p.Notes = quarterNotes(p, el, regime, rates, breach)
		out = append(out, p)

		// What the next quarter may credit. Quarter four is the annual
		// return and credits the first three, so the fourth quarter's own
		// payment is never carried anywhere.
		if n < QuartersPerYear {
			switch rec := el.Payments[n-1]; {
			case rec.Recorded:
				paidBefore += rec.Amount
			case fallbackPaid != nil && n-1 < len(fallbackPaid):
				paidBefore += fallbackPaid[n-1]
				paidBeforeSoft = true
			default:
				paidBefore += p.AmountToPay
				paidBeforeSoft = true
			}
		}
	}
	return out
}

// marginalBP is the rate the next peso of taxable income would meet. The test
// is "at or above", not "above", which is where it differs from GraduatedTax:
// at taxable income of exactly 250,000 the tax is nil, because no peso has yet
// crossed the bound — but the next peso has, so the marginal rate is already
// the one above.
func (r Rates) marginalBP(taxable Centavos, year int) int {
	s, ok := r.Schedule(year)
	if !ok {
		return 0
	}
	bp := 0
	for _, b := range s.Brackets {
		if taxable >= b.Over {
			bp = b.RateBP
		}
	}
	return bp
}

func percentageTax(year, n int, t QuarterTotals, rates Rates) *PercentageTaxQuarter {
	q, err := QuarterOf(year, n)
	if err != nil {
		return nil
	}
	// The rate is read at the quarter's start. Both changes to the rate fell
	// on the first day of a quarter, so no quarter has ever straddled two
	// rates; if one ever does, this is where it has to be split.
	bp, period, ok := rates.PercentageTaxRateBP(q.Start)
	if !ok {
		return nil
	}
	due, _ := rates.Deadlines.PercentageTaxDue(year, n)
	// The base is the quarter's own sales, net of returns, allowances and
	// discounts. Other non-operating income is not a sale or receipt of the
	// business and is left out of it.
	base := atLeastZero(t.NetSales)
	return &PercentageTaxQuarter{
		Quarter: n, Base: base, RateBP: bp,
		Due:      applyRate(base, bp),
		Deadline: due, Reference: period.Reference,
	}
}

func instalment(annual QuarterPosition, rates Rates, year int) SecondInstalment {
	half := annual.TaxDue / 2
	si := SecondInstalment{
		HalfOfTaxDue:  half,
		Due:           rates.Deadlines.SecondInstalmentDue(year),
		MinimumTaxDue: rates.SecondInstalmentMinimum,
		Reference: "NIRC sec. 56(A)(2) as amended by the TRAIN law: where the tax due exceeds " + pesoSign +
			rates.SecondInstalmentMinimum.String() + ", an individual may elect to pay in two equal instalments, " +
			"the first when the return is filed and the second on or before 15 October following the close of the calendar year. " +
			"BIR Form No. 1701 (January 2018 ENCS) item 25 allows 50% or less of the tax due to be deferred.",
	}
	if annual.TaxDue <= rates.SecondInstalmentMinimum {
		return si
	}
	si.Available = true
	si.DeferrableNow = half
	if si.DeferrableNow > annual.AmountToPay {
		si.DeferrableNow = annual.AmountToPay
	}
	si.PayWithReturn = annual.AmountToPay - si.DeferrableNow
	return si
}

// project annualises the quarters that have ended at the straight-line pace of
// what is recorded in them.
func project(year int, el Election, totals [QuartersPerYear]QuarterTotals, rates Rates, today Date) Projection {
	elapsed := 0
	for n := 1; n <= QuartersPerYear; n++ {
		q, err := QuarterOf(year, n)
		if err != nil {
			continue
		}
		if today.After(q.End) {
			elapsed = n
		}
	}
	if elapsed == 0 || elapsed == QuartersPerYear {
		return Projection{}
	}
	var gross Centavos
	for n := 1; n <= elapsed; n++ {
		gross += totals[n-1].GrossForEightPercent()
	}
	if gross <= 0 {
		return Projection{}
	}
	projected := gross * Centavos(QuartersPerYear) / Centavos(elapsed)
	p := Projection{
		Available: true, QuartersCounted: elapsed,
		GrossToDate: gross, ProjectedGross: projected,
		CrossesThreshold: projected > rates.VATThreshold,
	}
	if el.Regime == EightPercent {
		allowance := rates.EightPercentAllowance
		if el.HasCompensation {
			allowance = 0
		}
		p.ProjectedTax = applyRate(atLeastZero(projected-allowance), rates.EightPercentRateBP)
		return p
	}
	// Under the graduated rates the projection has to go through the same
	// deduction the year is using, or it says nothing useful.
	net := projected
	if el.Deduction == OSD {
		net = projected - applyRate(projected, rates.OSDRateBP)
	} else {
		var expenses Centavos
		for n := 1; n <= elapsed; n++ {
			expenses += totals[n-1].Expenses()
		}
		net = projected - expenses*Centavos(QuartersPerYear)/Centavos(elapsed)
	}
	p.ProjectedTax, _ = rates.GraduatedTax(atLeastZero(net), year)
	return p
}
