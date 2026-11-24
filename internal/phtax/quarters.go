package phtax

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// A Date is a calendar day with no time and no zone. Tax dates are calendar
// dates: the fifteenth of May is the fifteenth of May wherever the machine
// thinks it is, and holding one as a timestamp invites a day of drift.
type Date struct {
	Year  int        `json:"year"`
	Month time.Month `json:"month"`
	Day   int        `json:"day"`
}

// NewDate makes a date. It does not normalise: an impossible date stays
// impossible, and Valid reports it.
func NewDate(y int, m time.Month, d int) Date { return Date{Year: y, Month: m, Day: d} }

// Today is the current date in the machine's own zone, which is the zone the
// person filing is in.
func Today() Date {
	t := time.Now()
	return Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

// IsZero reports a date that was never set.
func (d Date) IsZero() bool { return d.Year == 0 && d.Month == 0 && d.Day == 0 }

// Valid reports whether the date exists in the calendar. It catches the 29th
// of February in a common year, which a time.Time would silently roll into
// the 1st of March.
func (d Date) Valid() bool {
	if d.Year < 1 || d.Month < time.January || d.Month > time.December || d.Day < 1 {
		return false
	}
	return d.Day <= DaysInMonth(d.Year, d.Month)
}

// DaysInMonth is the length of a month, with February taken from the calendar
// rather than from a table, so that a leap year needs no special case here.
func DaysInMonth(year int, m time.Month) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// Time turns the date into midnight UTC, for arithmetic only.
func (d Date) Time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// AddDays returns the date that many days later, by the calendar, so that
// month lengths and leap days are handled by the calendar and not by us.
func (d Date) AddDays(n int) Date {
	t := d.Time().AddDate(0, 0, n)
	return Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

// Before reports whether d falls strictly before o.
func (d Date) Before(o Date) bool {
	if d.Year != o.Year {
		return d.Year < o.Year
	}
	if d.Month != o.Month {
		return d.Month < o.Month
	}
	return d.Day < o.Day
}

// After reports whether d falls strictly after o.
func (d Date) After(o Date) bool { return o.Before(d) }

// DaysUntil is the number of days from d to o; negative if o is in the past.
func (d Date) DaysUntil(o Date) int {
	return int(o.Time().Sub(d.Time()).Hours() / 24)
}

// Weekday is the day of the week the date falls on.
func (d Date) Weekday() time.Weekday { return d.Time().Weekday() }

// IsWeekend reports a Saturday or a Sunday.
func (d Date) IsWeekend() bool {
	w := d.Weekday()
	return w == time.Saturday || w == time.Sunday
}

// String writes the date as YYYY-MM-DD.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// Long writes the date the way a reminder should read it.
func (d Date) Long() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%d %s %d", d.Day, d.Month.String(), d.Year)
}

// MarshalJSON writes a date as "YYYY-MM-DD", or null when unset.
func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + d.String() + `"`), nil
}

// UnmarshalJSON reads "YYYY-MM-DD", null, or an empty string.
func (d *Date) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "null" || s == "" {
		*d = Date{}
		return nil
	}
	p, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = p
	return nil
}

// ParseDate reads a date written as YYYY-MM-DD, which is what a date input in
// a browser sends and what a spreadsheet exports when told to behave.
func ParseDate(s string) (Date, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return Date{}, errors.New("no date given")
	}
	parts := strings.Split(t, "-")
	if len(parts) != 3 {
		return Date{}, fmt.Errorf("write the date as YYYY-MM-DD, not %q", s)
	}
	var n [3]int
	for i, p := range parts {
		if !allDigits(p) {
			return Date{}, fmt.Errorf("write the date as YYYY-MM-DD, not %q", s)
		}
		v := 0
		for j := 0; j < len(p); j++ {
			v = v*10 + int(p[j]-'0')
		}
		n[i] = v
	}
	d := Date{Year: n[0], Month: time.Month(n[1]), Day: n[2]}
	if !d.Valid() {
		return Date{}, fmt.Errorf("%q is not a date in the calendar", s)
	}
	return d, nil
}

// ------------------------------------------------------------------- quarters

// QuartersPerYear is how many quarters a taxable year has. The income tax
// return is filed for the first three of them; the fourth is settled on the
// annual return, which is why there is no fourth quarterly income tax return.
const QuartersPerYear = 4

// QuarterlyReturns is how many quarterly income tax returns a year has.
const QuarterlyReturns = 3

// A Quarter is one taxable quarter of a calendar year.
//
// An individual's taxable year is the calendar year — NIRC sec. 43 allows a
// fiscal year only to a taxpayer that is not an individual — so the quarters
// are always January to March, April to June, July to September and October to
// December, and this tool offers no fiscal-year option.
type Quarter struct {
	Year   int    `json:"year"`
	N      int    `json:"n"` // 1 to 4
	Start  Date   `json:"start"`
	End    Date   `json:"end"`
	Days   int    `json:"days"`
	Months string `json:"months"`
}

// QuarterOf returns quarter n of a year.
func QuarterOf(year, n int) (Quarter, error) {
	if n < 1 || n > QuartersPerYear {
		return Quarter{}, fmt.Errorf("there are %d quarters in a year, not a quarter %d", QuartersPerYear, n)
	}
	if year < 1 {
		return Quarter{}, fmt.Errorf("%d is not a taxable year", year)
	}
	firstMonth := time.Month((n-1)*3 + 1)
	lastMonth := firstMonth + 2
	start := Date{Year: year, Month: firstMonth, Day: 1}
	end := Date{Year: year, Month: lastMonth, Day: DaysInMonth(year, lastMonth)}
	return Quarter{
		Year: year, N: n, Start: start, End: end,
		Days:   start.DaysUntil(end) + 1,
		Months: firstMonth.String()[:3] + "–" + lastMonth.String()[:3],
	}, nil
}

// Quarters returns all four quarters of a year.
func Quarters(year int) ([]Quarter, error) {
	out := make([]Quarter, 0, QuartersPerYear)
	for n := 1; n <= QuartersPerYear; n++ {
		q, err := QuarterOf(year, n)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, nil
}

// QuarterForDate says which quarter of its own year a date falls in.
func QuarterForDate(d Date) (int, error) {
	if !d.Valid() {
		return 0, fmt.Errorf("%q is not a date in the calendar", d.String())
	}
	return (int(d.Month)-1)/3 + 1, nil
}

// ------------------------------------------------------------------ deadlines

// Deadlines are the statutory filing dates. They are kept here as month and
// day rather than as computed dates so that they can be overridden if the law
// moves one, which it has: the TRAIN law moved the first quarterly return from
// the fifteenth of April to the fifteenth of May.
type Deadlines struct {
	// QuarterlyMonths and QuarterlyDay give the date each quarterly income
	// tax return is due, in the same calendar year as the quarter.
	QuarterlyMonths []time.Month `json:"quarterlyMonths"`
	QuarterlyDay    int          `json:"quarterlyDay"`

	// AnnualMonth and AnnualDay give the date the annual return is due, in
	// the year after the taxable year.
	AnnualMonth time.Month `json:"annualMonth"`
	AnnualDay   int        `json:"annualDay"`

	// SecondInstalmentMonth and SecondInstalmentDay give the date the
	// optional second instalment of the annual tax is due.
	SecondInstalmentMonth time.Month `json:"secondInstalmentMonth"`
	SecondInstalmentDay   int        `json:"secondInstalmentDay"`

	// PercentageTaxDays is how many days after the end of a quarter the
	// percentage tax return is due.
	PercentageTaxDays int `json:"percentageTaxDays"`
}

// DefaultDeadlines are the dates in force at the time of writing.
//
//   - The quarterly income tax return, BIR Form No. 1701Q, is due on the
//     fifteenth of May, August and November of the taxable year. NIRC sec.
//     74(A) as amended by the TRAIN law moved the first one to the fifteenth
//     of May; sec. 74(B) puts the second and third instalments on the
//     fifteenth of August and the fifteenth of November.
//   - The annual return, BIR Form No. 1701 or 1701A, is due on the fifteenth
//     of April of the following year. NIRC sec. 51(C)(1).
//   - Up to half the annual tax may be deferred to a second instalment due on
//     or before the fifteenth of October. NIRC sec. 56(A)(2) as amended by
//     the TRAIN law; BIR Form No. 1701 (January 2018 ENCS) item 25 names the
//     date on the face of the return.
//   - The percentage tax return, BIR Form No. 2551Q, is due within
//     twenty-five days after the end of each taxable quarter. NIRC sec.
//     128(A)(1).
func DefaultDeadlines() Deadlines {
	return Deadlines{
		QuarterlyMonths:       []time.Month{time.May, time.August, time.November},
		QuarterlyDay:          15,
		AnnualMonth:           time.April,
		AnnualDay:             15,
		SecondInstalmentMonth: time.October,
		SecondInstalmentDay:   15,
		PercentageTaxDays:     25,
	}
}

func (d Deadlines) normalise() Deadlines {
	def := DefaultDeadlines()
	if len(d.QuarterlyMonths) != QuarterlyReturns {
		d.QuarterlyMonths = def.QuarterlyMonths
	}
	if d.QuarterlyDay < 1 || d.QuarterlyDay > 31 {
		d.QuarterlyDay = def.QuarterlyDay
	}
	if d.AnnualMonth < time.January || d.AnnualMonth > time.December {
		d.AnnualMonth = def.AnnualMonth
	}
	if d.AnnualDay < 1 || d.AnnualDay > 31 {
		d.AnnualDay = def.AnnualDay
	}
	if d.SecondInstalmentMonth < time.January || d.SecondInstalmentMonth > time.December {
		d.SecondInstalmentMonth = def.SecondInstalmentMonth
	}
	if d.SecondInstalmentDay < 1 || d.SecondInstalmentDay > 31 {
		d.SecondInstalmentDay = def.SecondInstalmentDay
	}
	if d.PercentageTaxDays <= 0 {
		d.PercentageTaxDays = def.PercentageTaxDays
	}
	return d
}

// IncomeTaxDue is the date the income tax return covering quarter n of a year
// is due. Quarters one to three are the quarterly return; the fourth quarter
// has no quarterly return of its own and is settled on the annual return, so
// the annual date is returned for it.
func (d Deadlines) IncomeTaxDue(year, n int) (Date, error) {
	d = d.normalise()
	switch {
	case n >= 1 && n <= QuarterlyReturns:
		return Date{Year: year, Month: d.QuarterlyMonths[n-1], Day: d.QuarterlyDay}, nil
	case n == QuartersPerYear:
		return d.AnnualDue(year), nil
	}
	return Date{}, fmt.Errorf("there are %d quarters in a year, not a quarter %d", QuartersPerYear, n)
}

// AnnualDue is the date the annual return for a taxable year is due.
func (d Deadlines) AnnualDue(year int) Date {
	d = d.normalise()
	return Date{Year: year + 1, Month: d.AnnualMonth, Day: d.AnnualDay}
}

// SecondInstalmentDue is the date the optional second instalment of the annual
// tax for a taxable year is due.
func (d Deadlines) SecondInstalmentDue(year int) Date {
	d = d.normalise()
	return Date{Year: year + 1, Month: d.SecondInstalmentMonth, Day: d.SecondInstalmentDay}
}

// PercentageTaxDue is the date the percentage tax return for a quarter is due:
// a number of days after the quarter ends, counted on the calendar, so that
// the length of the following month never comes into it.
func (d Deadlines) PercentageTaxDue(year, n int) (Date, error) {
	q, err := QuarterOf(year, n)
	if err != nil {
		return Date{}, err
	}
	return q.End.AddDays(d.normalise().PercentageTaxDays), nil
}

func (d Deadlines) constants() []Constant {
	d = d.normalise()
	names := []string{"first", "second", "third"}
	out := make([]Constant, 0, 6)
	for i, m := range d.QuarterlyMonths {
		which := "quarter " + itoa(i+1)
		if i < len(names) {
			which = names[i] + " quarter"
		}
		ref := "NIRC sec. 74(B): \"the second and third shall be paid on August 15 and November 15 of the current year, respectively\""
		if i == 0 {
			ref = "NIRC sec. 74(A) as amended by the TRAIN law (R.A. No. 10963) sec. 20: the declaration is filed \"on or before May 15 of the same taxable year\", moved from 15 April"
		}
		out = append(out, Constant{
			Name:       "1701Q deadline, " + which,
			Value:      itoa(d.QuarterlyDay) + " " + m.String() + " of the taxable year",
			What:       "the quarterly income tax return for the " + which + ", reporting the year to date",
			Reference:  ref,
			Confidence: Sourced, Field: "deadlines",
		})
	}
	out = append(out,
		Constant{
			Name:       "1701/1701A deadline",
			Value:      itoa(d.AnnualDay) + " " + d.AnnualMonth.String() + " of the following year",
			What:       "the annual income tax return, which settles the fourth quarter and the year",
			Reference:  "NIRC sec. 51(C)(1); NIRC sec. 74(B) puts the fourth instalment \"on or before April 15 of the following calendar year when the final adjusted income tax return is due to be filed\"",
			Confidence: Sourced, Field: "deadlines",
		},
		Constant{
			Name:       "Second instalment of the annual tax",
			Value:      "on or before " + itoa(d.SecondInstalmentDay) + " " + d.SecondInstalmentMonth.String() + " of the following year",
			What:       "up to half the annual tax due may be deferred to this date; the rest is paid when the return is filed",
			Reference:  "NIRC sec. 56(A)(2) as amended by the TRAIN law; BIR Form No. 1701 (January 2018 ENCS) item 25 names the date on the face of the return",
			Confidence: Sourced, Field: "deadlines",
		},
		Constant{
			Name:       "2551Q deadline",
			Value:      "within " + itoa(d.PercentageTaxDays) + " days after the quarter ends",
			What:       "the quarterly percentage tax return, for a non-VAT person under the graduated rates; it is per quarter, not cumulative",
			Reference:  "NIRC sec. 128(A)(1)",
			Confidence: Sourced, Field: "deadlines",
		},
	)
	return out
}
