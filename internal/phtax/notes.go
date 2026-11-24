package phtax

import "fmt"

// The remarks in this file are the part of the tool a user reads before they
// read a figure. Each one names what changes and, where there is one, the
// provision it comes from. None of them is advice: they say what the rule is
// and tell the reader to confirm it.

// quarterNotes are the remarks that belong to one quarter's position.
func quarterNotes(p QuarterPosition, el Election, regime Regime, rates Rates, breach ThresholdBreach) []Note {
	var out []Note

	if p.This.Entries == 0 && p.Gross == 0 {
		out = append(out, Note{Info,
			"Nothing is recorded for this quarter or any before it, so every figure here is nil.", ""})
	}

	if p.Overpayment > 0 {
		out = append(out, Note{Warn, fmt.Sprintf(
			"The tax already credited is %s%s more than the tax due on the year to date. "+
				"The return's own line is \"Tax Payable/(Overpayment)\", so this goes on it in brackets and "+
				"there is nothing to pay this quarter. A quarterly overpayment is carried forward against the "+
				"rest of the year, not refunded now; the annual return is where you choose between a refund, a "+
				"tax credit certificate and carrying it over.",
			pesoSign, p.Overpayment), ""})
	}

	if p.PaidPriorQuartersSoft && p.PaidPriorQuarters > 0 {
		out = append(out, Note{Warn, fmt.Sprintf(
			"%s%s is credited as tax paid in earlier quarters, but that is this tool's own figure, not yours. "+
				"The return asks for what you actually paid. Record the payment for each earlier quarter so this "+
				"line is right.", pesoSign, p.PaidPriorQuarters), ""})
	}

	if regime == EightPercent {
		if el.HasCompensation {
			out = append(out, Note{Warn,
				fmt.Sprintf("You have compensation income as well as business income, so the %s%s reduction does not apply: "+
					"the 8%% is on the whole of gross. The reduction is for an individual earning purely from business "+
					"or a profession.", pesoSign, rates.EightPercentAllowance),
				"BIR Form No. 1701 (January 2018 ENCS) item 29: \"not applicable if with compensation income\""})
		} else if p.Gross > 0 && p.Gross <= rates.EightPercentAllowance {
			out = append(out, Note{Info,
				fmt.Sprintf("Gross for the year to date is still within the %s%s reduction, so the tax due to date is nil. "+
					"The return is still filed.", pesoSign, rates.EightPercentAllowance), ""})
		}
		if el.VATRegistered {
			out = append(out, Note{Stop,
				"The 8% option is open only to a person who is not registered for value-added tax. " +
					"As a VAT-registered taxpayer you are on the graduated rates.",
				"NIRC sec. 24(A)(2)(b)"})
		}
		if breach.Crossed && p.Quarter >= breach.Quarter {
			out = append(out, Note{Stop, fmt.Sprintf(
				"Gross passed %s%s during quarter %d. A taxpayer who elected the 8%% rate and then exceeds the "+
					"threshold during the year is on the graduated rates for the whole year, with the 8%% already "+
					"paid allowed as a credit, and must update registration within the month following the month "+
					"the threshold was exceeded. The figures above are no longer the ones to file.",
				pesoSign, breach.Threshold, breach.Quarter),
				"R.R. No. 8-2018; NIRC secs. 24(A)(2)(b), 109(1)(BB) and 116"})
		}
	} else {
		if el.Deduction == OSD && p.CostOfServices+p.ItemizedDeduction > 0 {
			out = append(out, Note{Info, fmt.Sprintf(
				"You have recorded %s%s of costs, but the year is on the optional standard deduction, so the "+
					"deduction taken is %s%s — %s of net sales — and the recorded costs change nothing. "+
					"Switch the year to itemised deductions if the recorded costs are higher and you can support them.",
				pesoSign, p.CostOfServices+p.ItemizedDeduction, pesoSign, p.OSDAmount, rateString(rates.OSDRateBP)),
				"NIRC sec. 34(L)"})
		}
		if p.NetIncome < 0 {
			out = append(out, Note{Warn, fmt.Sprintf(
				"The year to date is a loss of %s%s, so the taxable income is nil and the tax due to date is nil. "+
					"The return is still filed.", pesoSign, -p.NetIncome), ""})
		}
		if breach.Crossed && p.Quarter >= breach.Quarter && !el.VATRegistered {
			out = append(out, Note{Stop, fmt.Sprintf(
				"Gross passed %s%s during quarter %d, so value-added tax registration is required and the "+
					"percentage tax gives way to VAT at %s. Register within the month following the month the "+
					"threshold was exceeded. This tool does not compute VAT.",
				pesoSign, breach.Threshold, breach.Quarter, rateString(rates.VATRateBP)),
				"NIRC secs. 109(1)(BB), 116 and 236(G)"})
		}
	}

	if el.VATRegistered {
		out = append(out, Note{Info,
			"As a VAT-registered taxpayer you file the quarterly VAT return as well. This tool does not compute VAT.",
			"NIRC sec. 114"})
	}

	if p.DeadlineIsWeekend {
		out = append(out, Note{Info, fmt.Sprintf(
			"The statutory date, %s, falls on a %s. Confirm the working date with the Bureau; this tool does not "+
				"move a deadline for a weekend or a holiday.", p.Deadline.Long(), p.Deadline.Weekday()), ""})
	}

	if p.IsAnnual {
		out = append(out, Note{Info,
			"There is no fourth quarterly return. The fourth quarter is settled on the annual return, which is " +
				"this same cumulative computation over the whole year with the three quarterly payments credited.",
			"NIRC sec. 74(B)"})
	}
	return out
}

// yearNotes are the remarks about the year as a whole.
func yearNotes(year int, el Election, rates Rates, breach ThresholdBreach, annual QuarterPosition) []Note {
	out := []Note{
		{Info,
			"The regime is an input here, not a recommendation: this tool assumes you have already elected the 8% " +
				"rate or the graduated rates and tracks the year under it.",
			"NIRC sec. 24(A)(2)(b): the election is irrevocable for the taxable year"},
	}

	if el.Regime == EightPercent {
		out = append(out, Note{Info,
			"The 8% election has to be signified for each taxable year — on BIR Form No. 1905 or on the first " +
				"quarterly return of the year — and it is irrevocable for that year. It takes the place of both " +
				"the graduated income tax and the percentage tax, so no 2551Q is due.",
			"NIRC sec. 24(A)(2)(b); R.R. No. 8-2018"})
	} else if !el.VATRegistered {
		out = append(out, Note{Info,
			"Under the graduated rates the percentage tax is a separate quarterly return, BIR Form No. 2551Q, on " +
				"the quarter's own gross — it is not cumulative and nothing earlier is credited against it.",
			"NIRC secs. 116 and 128(A)(1)"})
	}

	if el.HasCompensation {
		out = append(out, Note{Info,
			"A mixed-income earner files the annual return on BIR Form No. 1701, not 1701A, and compensation income " +
				"goes on the annual return rather than the quarterly ones. This tool tracks the business side only.",
			""})
	}

	if sch, ok := rates.Schedule(year); ok {
		out = append(out, Note{Info,
			fmt.Sprintf("The %d taxable year uses the graduated table %s.", year, sch.span()), sch.Reference})
	}

	if breach.Crossed {
		out = append(out, Note{Stop, fmt.Sprintf(
			"Gross for the year reached %s%s, above the %s%s registration threshold, in quarter %d.",
			pesoSign, breach.Gross, pesoSign, breach.Threshold, breach.Quarter),
			"NIRC secs. 109(1)(BB) and 236(G)"})
	}

	if annual.Overpayment > 0 {
		out = append(out, Note{Warn, fmt.Sprintf(
			"For the year as a whole the credits exceed the tax due by %s%s. On the annual return that is an "+
				"overpayment, and the return makes you choose once and for all between a refund, a tax credit "+
				"certificate and carrying it over to the next year.",
			pesoSign, annual.Overpayment),
			"BIR Form No. 1701 (January 2018 ENCS), the box below item 32: \"Once the choice is made, the same is irrevocable\""})
	}

	out = append(out, Note{Warn,
		"This tool works out figures from what you record. It does not file anything, it does not compute penalties, " +
			"surcharge or interest on a late return, and it is not tax advice. Confirm every rate, threshold and " +
			"date with the Bureau of Internal Revenue or a qualified practitioner before you file.", ""})
	return out
}

// calendar is the year's filing dates, with how far away each one is.
func calendar(year int, rates Rates, today Date) []DeadlineItem {
	d := rates.Deadlines.normalise()
	out := make([]DeadlineItem, 0, 9)

	add := func(form, what, covers string, due Date, ref string) {
		it := DeadlineItem{
			Form: form, What: what, Covers: covers, Due: due,
			DaysAway: today.DaysUntil(due), IsWeekend: due.IsWeekend(), Reference: ref,
		}
		switch {
		case it.DaysAway < 0:
			it.Status = "overdue"
		case it.DaysAway == 0:
			it.Status = "today"
		case it.DaysAway <= 14:
			it.Status = "soon"
		default:
			it.Status = "upcoming"
		}
		out = append(out, it)
	}

	for n := 1; n <= QuarterlyReturns; n++ {
		q, err := QuarterOf(year, n)
		if err != nil {
			continue
		}
		due, err := d.IncomeTaxDue(year, n)
		if err != nil {
			continue
		}
		ref := "NIRC sec. 74(B)"
		if n == 1 {
			ref = "NIRC sec. 74(A) as amended by the TRAIN law"
		}
		add("1701Q", fmt.Sprintf("Quarterly income tax return, quarter %d", n),
			fmt.Sprintf("%s to %s (%s), reported as the year to date", q.Start.Long(), q.End.Long(), q.Months), due, ref)
	}
	for n := 1; n <= QuartersPerYear; n++ {
		q, err := QuarterOf(year, n)
		if err != nil {
			continue
		}
		due, err := d.PercentageTaxDue(year, n)
		if err != nil {
			continue
		}
		add("2551Q", fmt.Sprintf("Quarterly percentage tax return, quarter %d", n),
			fmt.Sprintf("%s to %s (%s), that quarter only", q.Start.Long(), q.End.Long(), q.Months), due,
			"NIRC sec. 128(A)(1): within 25 days after the quarter ends")
	}
	add("1701/1701A", "Annual income tax return",
		fmt.Sprintf("the whole of %d, with the three quarterly payments credited", year),
		d.AnnualDue(year), "NIRC sec. 51(C)(1)")
	add("0605", "Second instalment of the annual tax, if elected",
		fmt.Sprintf("up to half the %d tax due, deferred from the annual return", year),
		d.SecondInstalmentDue(year), "NIRC sec. 56(A)(2) as amended by the TRAIN law")

	sortDeadlines(out)
	return out
}

func sortDeadlines(items []DeadlineItem) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].Due.Before(items[j-1].Due); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
