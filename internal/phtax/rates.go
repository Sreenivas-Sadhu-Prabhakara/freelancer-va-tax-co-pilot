package phtax

// Every rate, threshold, allowance and filing date this tool uses is declared
// in this file and nowhere else, each with the provision it comes from. They
// are the figures in force at the time of writing, they are indicative, and
// they are all overridable: a rate can change, and a tool that hard-codes one
// in the middle of a calculation goes quietly wrong when it does.
//
// Confirm the current figures with the Bureau of Internal Revenue. Where a
// figure could not be sourced confidently, Constants() says so rather than
// presenting a guess as settled.

// ---------------------------------------------------------------- the figures

const (
	// EightPercentRateBP is the flat income-tax rate a qualifying
	// self-employed individual may elect, in basis points: 8%.
	//
	// NIRC sec. 24(A)(2)(b), as amended by the TRAIN law (R.A. No. 10963).
	// It is in lieu of both the graduated income tax under sec. 24(A) and
	// the percentage tax under sec. 116.
	EightPercentRateBP = 800

	// EightPercentAllowance is the amount deducted from gross sales,
	// receipts and other non-operating income before the 8% rate is
	// applied, and only for an individual earning purely from business or
	// the practice of a profession.
	//
	// NIRC sec. 24(A)(2)(b). BIR Form No. 1701 (January 2018 ENCS),
	// Part V schedule 3.B item 29, states it as the "[a]llowable reduction
	// from gross sales/receipts and other non-operating income of purely
	// self-employed individuals and/or professionals in the amount of
	// P250,000 (not applicable if with compensation income)".
	EightPercentAllowance = Centavos(250_000 * 100)

	// VATThreshold is the ceiling on gross sales or receipts for the twelve
	// months, above which a person must register for value-added tax — and
	// therefore also the ceiling on electing the 8% rate and on paying the
	// percentage tax instead of VAT.
	//
	// NIRC secs. 109(1)(BB) and 116, as amended by the TRAIN law. The TRAIN
	// law provides for the figure to be adjusted to its present value every
	// three years using the consumer price index; no adjustment to this
	// threshold had been issued at the time of writing, so it stands at
	// three million pesos. Confirm it before you rely on it.
	VATThreshold = Centavos(3_000_000 * 100)

	// OSDRateBP is the optional standard deduction, in basis points: 40% of
	// net sales or receipts, claimed instead of itemising deductions.
	//
	// NIRC sec. 34(L), as amended by the TRAIN law. BIR Form No. 1701
	// (January 2018 ENCS) item 17 computes it as 40% of item 10, which is
	// sales net of returns, allowances and discounts — not 40% of gross.
	OSDRateBP = 4000

	// VATRateBP is the rate of value-added tax, in basis points: 12%. This
	// tool never computes VAT; the rate is here only so that a warning
	// about crossing the threshold can name it.
	//
	// NIRC sec. 106(A)/108(A).
	VATRateBP = 1200

	// SecondInstalmentMinimum is the amount of annual tax due above which an
	// individual may elect to pay in two instalments rather than all at
	// once.
	//
	// NIRC sec. 56(A)(2), as amended by the TRAIN law: "When a tax due is
	// in excess of Two thousand pesos (P2,000), the taxpayer other than a
	// corporation, may elect to pay the tax in two (2) equal installments",
	// the first when the return is filed and "the second installment on or
	// before October 15 following the close of the calendar year".
	SecondInstalmentMinimum = Centavos(2_000 * 100)
)

// A Bracket is one row of the graduated income-tax table: tax on taxable
// income above Over is Base plus RateBP of the excess over Over.
type Bracket struct {
	Over   Centavos `json:"over"`
	Base   Centavos `json:"base"`
	RateBP int      `json:"rateBasisPoints"`
}

// A BracketSchedule is the graduated table in force for a range of taxable
// years. LastYear of 0 means it is the current table with no end in sight.
type BracketSchedule struct {
	FirstYear int       `json:"firstYear"`
	LastYear  int       `json:"lastYear"`
	Reference string    `json:"reference"`
	Brackets  []Bracket `json:"brackets"`
}

// graduatedSchedules are the graduated rates on the taxable income of an
// individual under NIRC sec. 24(A)(2)(a), as amended by the TRAIN law. The law
// set one table for 2018 through 2022 and a second, lower one from 2023
// onwards; both are kept, because a late or amended return for an earlier year
// has to use the table that applied to that year.
//
// The first 250,000 pesos of taxable income is taxed at nil under both tables:
// that is the exemption a graduated-rate filer gets, and it is not the same
// thing as the 250,000-peso allowance under the 8% option, although the two
// amounts coincide.
var graduatedSchedules = []BracketSchedule{
	{
		FirstYear: 2018, LastYear: 2022,
		Reference: "NIRC sec. 24(A)(2)(a), TRAIN law (R.A. No. 10963), first schedule: taxable years 2018 to 2022",
		Brackets: []Bracket{
			{Over: 0, Base: 0, RateBP: 0},
			{Over: Pesos(250_000), Base: 0, RateBP: 2000},
			{Over: Pesos(400_000), Base: Pesos(30_000), RateBP: 2500},
			{Over: Pesos(800_000), Base: Pesos(130_000), RateBP: 3000},
			{Over: Pesos(2_000_000), Base: Pesos(490_000), RateBP: 3200},
			{Over: Pesos(8_000_000), Base: Pesos(2_410_000), RateBP: 3500},
		},
	},
	{
		FirstYear: 2023, LastYear: 0,
		Reference: "NIRC sec. 24(A)(2)(a), TRAIN law (R.A. No. 10963), second schedule: taxable year 2023 onwards",
		Brackets: []Bracket{
			{Over: 0, Base: 0, RateBP: 0},
			{Over: Pesos(250_000), Base: 0, RateBP: 1500},
			{Over: Pesos(400_000), Base: Pesos(22_500), RateBP: 2000},
			{Over: Pesos(800_000), Base: Pesos(102_500), RateBP: 2500},
			{Over: Pesos(2_000_000), Base: Pesos(402_500), RateBP: 3000},
			{Over: Pesos(8_000_000), Base: Pesos(2_202_500), RateBP: 3500},
		},
	},
}

// A PercentageTaxPeriod is the percentage-tax rate in force over a range of
// dates. The rate has moved twice, and both moves fell on the first day of a
// quarter, so a quarter never straddles two rates.
type PercentageTaxPeriod struct {
	From      Date   `json:"from"`
	To        Date   `json:"to"` // zero means open-ended
	RateBP    int    `json:"rateBasisPoints"`
	Reference string `json:"reference"`
}

// percentageTaxPeriods are the rates of the percentage tax on a person exempt
// from value-added tax under NIRC sec. 116.
//
// The rate is 3%. The CREATE law (R.A. No. 11534, sec. 13) cut it to 1% for
// three years, from 1 July 2020 to 30 June 2023, and the Bureau confirmed the
// return to 3% from 1 July 2023 in RMC No. 69-2023. Both changes took effect
// on the first day of a taxable quarter.
//
// A self-employed individual who has elected the 8% income tax is exempt from
// this tax altogether, which is the whole point of the election.
var percentageTaxPeriods = []PercentageTaxPeriod{
	{From: Date{}, To: NewDate(2020, 6, 30), RateBP: 300,
		Reference: "NIRC sec. 116 as amended by the TRAIN law: 3% of gross quarterly sales or receipts"},
	{From: NewDate(2020, 7, 1), To: NewDate(2023, 6, 30), RateBP: 100,
		Reference: "CREATE law (R.A. No. 11534) sec. 13: 1% from 1 July 2020 to 30 June 2023"},
	{From: NewDate(2023, 7, 1), To: Date{}, RateBP: 300,
		Reference: "BIR RMC No. 69-2023: the rate reverts to 3% from 1 July 2023"},
}

// ------------------------------------------------------------------- the type

// Rates holds every figure a computation uses. DefaultRates returns the ones
// above; a caller may change any of them, and the tool's own settings file
// carries overrides so that a change in the law does not need a new binary.
type Rates struct {
	EightPercentRateBP      int                   `json:"eightPercentRateBasisPoints"`
	EightPercentAllowance   Centavos              `json:"eightPercentAllowance"`
	VATThreshold            Centavos              `json:"vatThreshold"`
	OSDRateBP               int                   `json:"osdRateBasisPoints"`
	VATRateBP               int                   `json:"vatRateBasisPoints"`
	SecondInstalmentMinimum Centavos              `json:"secondInstalmentMinimum"`
	Graduated               []BracketSchedule     `json:"graduated"`
	PercentageTax           []PercentageTaxPeriod `json:"percentageTax"`
	Deadlines               Deadlines             `json:"deadlines"`
}

// DefaultRates returns the figures in force at the time of writing. The slices
// are copied, so a caller that edits one does not edit the package's own.
func DefaultRates() Rates {
	r := Rates{
		EightPercentRateBP:      EightPercentRateBP,
		EightPercentAllowance:   EightPercentAllowance,
		VATThreshold:            VATThreshold,
		OSDRateBP:               OSDRateBP,
		VATRateBP:               VATRateBP,
		SecondInstalmentMinimum: SecondInstalmentMinimum,
		Graduated:               make([]BracketSchedule, len(graduatedSchedules)),
		PercentageTax:           append([]PercentageTaxPeriod(nil), percentageTaxPeriods...),
		Deadlines:               DefaultDeadlines(),
	}
	for i, s := range graduatedSchedules {
		s.Brackets = append([]Bracket(nil), s.Brackets...)
		r.Graduated[i] = s
	}
	return r
}

// normalise fills in anything a loaded settings file left empty, so that a
// data file written by an older version, or edited by hand, cannot produce a
// computation against a nil rate.
func (r Rates) normalise() Rates {
	d := DefaultRates()
	if r.EightPercentRateBP == 0 {
		r.EightPercentRateBP = d.EightPercentRateBP
	}
	if r.EightPercentAllowance == 0 {
		r.EightPercentAllowance = d.EightPercentAllowance
	}
	if r.VATThreshold <= 0 {
		r.VATThreshold = d.VATThreshold
	}
	if r.OSDRateBP == 0 {
		r.OSDRateBP = d.OSDRateBP
	}
	if r.VATRateBP == 0 {
		r.VATRateBP = d.VATRateBP
	}
	if r.SecondInstalmentMinimum <= 0 {
		r.SecondInstalmentMinimum = d.SecondInstalmentMinimum
	}
	if len(r.Graduated) == 0 {
		r.Graduated = d.Graduated
	}
	if len(r.PercentageTax) == 0 {
		r.PercentageTax = d.PercentageTax
	}
	r.Deadlines = r.Deadlines.normalise()
	return r
}

// Schedule returns the graduated table that applies to a taxable year, and
// whether one was found. A year before the first table is refused rather than
// computed with the wrong rates.
func (r Rates) Schedule(year int) (BracketSchedule, bool) {
	var best BracketSchedule
	found := false
	for _, s := range r.Graduated {
		if year < s.FirstYear {
			continue
		}
		if s.LastYear != 0 && year > s.LastYear {
			continue
		}
		if !found || s.FirstYear > best.FirstYear {
			best, found = s, true
		}
	}
	return best, found
}

// GraduatedTax is the income tax on an individual's taxable income for a year
// under the graduated rates. A loss, or income within the nil band, is nil
// tax, never a negative.
func (r Rates) GraduatedTax(taxable Centavos, year int) (Centavos, bool) {
	s, ok := r.Schedule(year)
	if !ok {
		return 0, false
	}
	if taxable <= 0 {
		return 0, true
	}
	var row Bracket
	for _, b := range s.Brackets {
		if taxable > b.Over {
			row = b
		}
	}
	return row.Base + applyRate(taxable-row.Over, row.RateBP), true
}

// PercentageTaxRateBP is the percentage-tax rate in force on a date, and
// whether a period covering that date was found.
func (r Rates) PercentageTaxRateBP(on Date) (int, PercentageTaxPeriod, bool) {
	for _, p := range r.PercentageTax {
		if !p.From.IsZero() && on.Before(p.From) {
			continue
		}
		if !p.To.IsZero() && p.To.Before(on) {
			continue
		}
		return p.RateBP, p, true
	}
	return 0, PercentageTaxPeriod{}, false
}

// ------------------------------------------------------- the published figures

// Confidence says how well a figure could be sourced.
type Confidence string

const (
	// Sourced means the figure was read from the statute, a regulation, or
	// the Bureau's own form.
	Sourced Confidence = "sourced"
	// Unconfirmed means the figure is the one in general use and is
	// consistent across secondary sources, but the primary text or the
	// current version of it could not be obtained.
	Unconfirmed Confidence = "unconfirmed"
)

// A Constant is one published figure, for the assumptions panel and for the
// -constants listing. Nothing in the computation reads these: they describe
// the fields of Rates, so that what the tool says it uses and what it uses
// cannot drift apart without a test noticing.
type Constant struct {
	Name       string     `json:"name"`
	Value      string     `json:"value"`
	What       string     `json:"what"`
	Reference  string     `json:"reference"`
	Confidence Confidence `json:"confidence"`
	Field      string     `json:"field"` // the Rates field it overrides, "" if not overridable
}

// Constants describes every figure in a Rates, in the order a reader wants
// them.
func (r Rates) Constants() []Constant {
	r = r.normalise()
	out := []Constant{
		{
			Name: "8% income tax rate", Value: rateString(r.EightPercentRateBP),
			What:       "the flat rate a qualifying self-employed individual may elect, in lieu of the graduated income tax and the percentage tax",
			Reference:  "NIRC sec. 24(A)(2)(b), as amended by the TRAIN law (R.A. No. 10963)",
			Confidence: Sourced, Field: "eightPercentRateBasisPoints",
		},
		{
			Name: "8% allowable reduction", Value: "₱" + r.EightPercentAllowance.String(),
			What:       "deducted from gross sales, receipts and other non-operating income before the 8% rate is applied; only for an individual earning purely from business or a profession, not for one who also has compensation income",
			Reference:  "NIRC sec. 24(A)(2)(b); BIR Form No. 1701 (January 2018 ENCS) Part V schedule 3.B item 29",
			Confidence: Sourced, Field: "eightPercentAllowance",
		},
		{
			Name: "VAT registration threshold", Value: "₱" + r.VATThreshold.String(),
			What:       "gross sales or receipts for the twelve months above which VAT registration is required; also the ceiling on electing the 8% rate and on paying the percentage tax instead of VAT",
			Reference:  "NIRC secs. 109(1)(BB) and 116, as amended by the TRAIN law. The law provides for a three-yearly adjustment to present value; no adjustment had been issued at the time of writing",
			Confidence: Sourced, Field: "vatThreshold",
		},
		{
			Name: "Optional standard deduction", Value: rateString(r.OSDRateBP) + " of net sales/receipts",
			What:       "claimed instead of itemising deductions, under the graduated rates; 40% of sales net of returns, allowances and discounts, not of gross",
			Reference:  "NIRC sec. 34(L), as amended by the TRAIN law; BIR Form No. 1701 (January 2018 ENCS) item 17",
			Confidence: Sourced, Field: "osdRateBasisPoints",
		},
		{
			Name: "Value-added tax rate", Value: rateString(r.VATRateBP),
			What:       "named only in the warning about crossing the registration threshold; this tool never computes VAT",
			Reference:  "NIRC secs. 106(A) and 108(A)",
			Confidence: Sourced, Field: "vatRateBasisPoints",
		},
		{
			Name: "Two-instalment threshold", Value: "₱" + r.SecondInstalmentMinimum.String() + " of annual tax due",
			What:       "above this, up to half the annual tax may be deferred to a second instalment instead of being paid in full with the return",
			Reference:  "NIRC sec. 56(A)(2), as amended by the TRAIN law: \"When a tax due is in excess of Two thousand pesos (P2,000), the taxpayer other than a corporation, may elect to pay the tax in two (2) equal installments\"",
			Confidence: Sourced, Field: "secondInstalmentMinimum",
		},
	}
	for _, p := range r.PercentageTax {
		out = append(out, Constant{
			Name:  "Percentage tax " + p.span(),
			Value: rateString(p.RateBP) + " of the quarter's gross sales/receipts",
			What: "payable quarterly on BIR Form No. 2551Q by a non-VAT person under the graduated rates; " +
				"a person who has elected the 8% income tax does not pay it",
			Reference: p.Reference, Confidence: Sourced, Field: "percentageTax",
		})
	}
	for _, s := range r.Graduated {
		out = append(out, Constant{
			Name:       "Graduated rates " + s.span(),
			Value:      s.summary(),
			What:       "the income tax on an individual's taxable income under the graduated rates",
			Reference:  s.Reference,
			Confidence: Sourced, Field: "graduated",
		})
	}
	return append(out, r.Deadlines.constants()...)
}

func rateString(bp int) string {
	whole, frac := bp/100, bp%100
	s := itoa(whole)
	switch {
	case frac == 0:
	case frac%10 == 0:
		s += "." + itoa(frac/10)
	default:
		s += "." + pad2(frac)
	}
	return s + "%"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func pad2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func (p PercentageTaxPeriod) span() string {
	switch {
	case p.From.IsZero() && p.To.IsZero():
		return "(all periods)"
	case p.From.IsZero():
		return "(to " + p.To.String() + ")"
	case p.To.IsZero():
		return "(from " + p.From.String() + ")"
	}
	return "(" + p.From.String() + " to " + p.To.String() + ")"
}

func (s BracketSchedule) span() string {
	if s.LastYear == 0 {
		return "(" + itoa(s.FirstYear) + " onwards)"
	}
	if s.LastYear == s.FirstYear {
		return "(" + itoa(s.FirstYear) + ")"
	}
	return "(" + itoa(s.FirstYear) + " to " + itoa(s.LastYear) + ")"
}

func (s BracketSchedule) summary() string {
	out := ""
	for i, b := range s.Brackets {
		if i > 0 {
			out += "; "
		}
		if b.Over == 0 {
			out += "nil to ₱" + s.nextBound(i) + ": " + rateString(b.RateBP)
			continue
		}
		out += "over ₱" + b.Over.String() + ": " + rateString(b.RateBP)
	}
	return out
}

func (s BracketSchedule) nextBound(i int) string {
	if i+1 < len(s.Brackets) {
		return s.Brackets[i+1].Over.String()
	}
	return "—"
}
