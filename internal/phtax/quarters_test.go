package phtax

import (
	"testing"
	"time"
)

func TestQuarterOf(t *testing.T) {
	cases := []struct {
		year, n    int
		start, end Date
		days       int
		months     string
	}{
		// 2026 is a common year.
		{2026, 1, NewDate(2026, time.January, 1), NewDate(2026, time.March, 31), 90, "Jan–Mar"},
		{2026, 2, NewDate(2026, time.April, 1), NewDate(2026, time.June, 30), 91, "Apr–Jun"},
		{2026, 3, NewDate(2026, time.July, 1), NewDate(2026, time.September, 30), 92, "Jul–Sep"},
		{2026, 4, NewDate(2026, time.October, 1), NewDate(2026, time.December, 31), 92, "Oct–Dec"},
		// 2024 is a leap year, so its first quarter is a day longer.
		{2024, 1, NewDate(2024, time.January, 1), NewDate(2024, time.March, 31), 91, "Jan–Mar"},
		{2024, 2, NewDate(2024, time.April, 1), NewDate(2024, time.June, 30), 91, "Apr–Jun"},
		// 1900 was not a leap year and 2000 was: the century rule matters.
		{1900, 1, NewDate(1900, time.January, 1), NewDate(1900, time.March, 31), 90, "Jan–Mar"},
		{2000, 1, NewDate(2000, time.January, 1), NewDate(2000, time.March, 31), 91, "Jan–Mar"},
	}
	for _, c := range cases {
		q, err := QuarterOf(c.year, c.n)
		if err != nil {
			t.Fatalf("QuarterOf(%d, %d): %v", c.year, c.n, err)
		}
		if q.Start != c.start || q.End != c.end {
			t.Errorf("QuarterOf(%d, %d) runs %s to %s, want %s to %s", c.year, c.n, q.Start, q.End, c.start, c.end)
		}
		if q.Days != c.days {
			t.Errorf("QuarterOf(%d, %d) is %d days, want %d", c.year, c.n, q.Days, c.days)
		}
		if q.Months != c.months {
			t.Errorf("QuarterOf(%d, %d) is labelled %q, want %q", c.year, c.n, q.Months, c.months)
		}
	}
	for _, n := range []int{-1, 0, 5, 99} {
		if _, err := QuarterOf(2026, n); err == nil {
			t.Errorf("QuarterOf(2026, %d) should be refused", n)
		}
	}
	if _, err := QuarterOf(0, 1); err == nil {
		t.Error("QuarterOf(0, 1) should be refused")
	}
}

func TestQuartersCoverTheYearExactly(t *testing.T) {
	for _, year := range []int{2023, 2024, 2025, 2026} {
		qs, err := Quarters(year)
		if err != nil {
			t.Fatalf("Quarters(%d): %v", year, err)
		}
		total := 0
		for i, q := range qs {
			total += q.Days
			if i > 0 && qs[i-1].End.AddDays(1) != q.Start {
				t.Errorf("%d: quarter %d starts %s but quarter %d ended %s", year, q.N, q.Start, qs[i-1].N, qs[i-1].End)
			}
		}
		want := 365
		if DaysInMonth(year, time.February) == 29 {
			want = 366
		}
		if total != want {
			t.Errorf("the four quarters of %d are %d days, want %d", year, total, want)
		}
	}
}

func TestQuarterForDate(t *testing.T) {
	cases := []struct {
		d    Date
		want int
	}{
		{NewDate(2026, time.January, 1), 1},
		{NewDate(2026, time.March, 31), 1},
		{NewDate(2026, time.April, 1), 2},
		{NewDate(2026, time.June, 30), 2},
		{NewDate(2026, time.July, 1), 3},
		{NewDate(2026, time.September, 30), 3},
		{NewDate(2026, time.October, 1), 4},
		{NewDate(2026, time.December, 31), 4},
		{NewDate(2024, time.February, 29), 1},
	}
	for _, c := range cases {
		got, err := QuarterForDate(c.d)
		if err != nil {
			t.Fatalf("QuarterForDate(%s): %v", c.d, err)
		}
		if got != c.want {
			t.Errorf("QuarterForDate(%s) = %d, want %d", c.d, got, c.want)
		}
	}
	if _, err := QuarterForDate(NewDate(2026, time.February, 30)); err == nil {
		t.Error("the thirtieth of February should be refused")
	}
}

func TestDateValidAndLeapYears(t *testing.T) {
	cases := []struct {
		d    Date
		want bool
	}{
		{NewDate(2024, time.February, 29), true},  // a leap year
		{NewDate(2026, time.February, 29), false}, // not a leap year
		{NewDate(2000, time.February, 29), true},  // divisible by 400
		{NewDate(1900, time.February, 29), false}, // divisible by 100 but not 400
		{NewDate(2026, time.April, 30), true},
		{NewDate(2026, time.April, 31), false},
		{NewDate(2026, time.December, 31), true},
		{NewDate(2026, time.Month(13), 1), false},
		{NewDate(2026, time.Month(0), 1), false},
		{NewDate(2026, time.January, 0), false},
		{NewDate(0, time.January, 1), false},
	}
	for _, c := range cases {
		if got := c.d.Valid(); got != c.want {
			t.Errorf("Date{%d, %d, %d}.Valid() = %v, want %v", c.d.Year, c.d.Month, c.d.Day, got, c.want)
		}
	}
}

func TestDateArithmetic(t *testing.T) {
	cases := []struct {
		from Date
		add  int
		want Date
	}{
		{NewDate(2026, time.March, 31), 25, NewDate(2026, time.April, 25)},
		{NewDate(2026, time.December, 31), 25, NewDate(2027, time.January, 25)},
		{NewDate(2024, time.February, 28), 1, NewDate(2024, time.February, 29)},
		{NewDate(2026, time.February, 28), 1, NewDate(2026, time.March, 1)},
		{NewDate(2024, time.February, 29), 1, NewDate(2024, time.March, 1)},
		{NewDate(2026, time.January, 31), -1, NewDate(2026, time.January, 30)},
	}
	for _, c := range cases {
		if got := c.from.AddDays(c.add); got != c.want {
			t.Errorf("%s.AddDays(%d) = %s, want %s", c.from, c.add, got, c.want)
		}
	}
	if d := NewDate(2026, time.January, 1).DaysUntil(NewDate(2026, time.December, 31)); d != 364 {
		t.Errorf("1 January to 31 December 2026 is %d days, want 364", d)
	}
	if d := NewDate(2024, time.January, 1).DaysUntil(NewDate(2024, time.December, 31)); d != 365 {
		t.Errorf("1 January to 31 December 2024 is %d days, want 365 in a leap year", d)
	}
	if d := NewDate(2026, time.May, 20).DaysUntil(NewDate(2026, time.May, 15)); d != -5 {
		t.Errorf("a date in the past should be negative, got %d", d)
	}
	if !NewDate(2026, time.May, 15).Before(NewDate(2026, time.August, 15)) {
		t.Error("May should come before August")
	}
	if NewDate(2026, time.May, 15).Before(NewDate(2026, time.May, 15)) {
		t.Error("a date is not before itself")
	}
	if !NewDate(2027, time.January, 1).After(NewDate(2026, time.December, 31)) {
		t.Error("the new year comes after the old one")
	}
}

func TestParseDate(t *testing.T) {
	cases := []struct {
		in   string
		want Date
		bad  bool
	}{
		{in: "2026-05-15", want: NewDate(2026, time.May, 15)},
		{in: " 2026-01-01 ", want: NewDate(2026, time.January, 1)},
		{in: "2024-02-29", want: NewDate(2024, time.February, 29)},
		{in: "2026-02-29", bad: true},
		{in: "2026-13-01", bad: true},
		{in: "15/05/2026", bad: true},
		{in: "2026-5", bad: true},
		{in: "", bad: true},
		{in: "two thousand", bad: true},
		{in: "2026-0a-01", bad: true},
	}
	for _, c := range cases {
		got, err := ParseDate(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("ParseDate(%q) = %s, want an error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDate(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseDate(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestDateJSONRoundTrip(t *testing.T) {
	d := NewDate(2026, time.November, 15)
	b, err := d.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"2026-11-15"` {
		t.Fatalf("marshalled to %s, want \"2026-11-15\"", b)
	}
	var back Date
	if err := back.UnmarshalJSON(b); err != nil {
		t.Fatal(err)
	}
	if back != d {
		t.Fatalf("round-tripped to %s, want %s", back, d)
	}
	var zero Date
	nb, err := Date{}.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(nb) != "null" {
		t.Fatalf("an unset date marshalled to %s, want null", nb)
	}
	if err := zero.UnmarshalJSON([]byte("null")); err != nil || !zero.IsZero() {
		t.Fatalf("null should read back as unset, got %s (%v)", zero, err)
	}
	if err := zero.UnmarshalJSON([]byte(`"not a date"`)); err == nil {
		t.Fatal("a bad date in JSON should be refused")
	}
}

func TestIncomeTaxDeadlines(t *testing.T) {
	d := DefaultDeadlines()
	cases := []struct {
		year, n int
		want    Date
	}{
		{2026, 1, NewDate(2026, time.May, 15)},
		{2026, 2, NewDate(2026, time.August, 15)},
		{2026, 3, NewDate(2026, time.November, 15)},
		{2026, 4, NewDate(2027, time.April, 15)},
		{2024, 1, NewDate(2024, time.May, 15)},
	}
	for _, c := range cases {
		got, err := d.IncomeTaxDue(c.year, c.n)
		if err != nil {
			t.Fatalf("IncomeTaxDue(%d, %d): %v", c.year, c.n, err)
		}
		if got != c.want {
			t.Errorf("IncomeTaxDue(%d, %d) = %s, want %s", c.year, c.n, got, c.want)
		}
	}
	if _, err := d.IncomeTaxDue(2026, 5); err == nil {
		t.Error("there is no fifth quarter")
	}
	if got := d.AnnualDue(2026); got != NewDate(2027, time.April, 15) {
		t.Errorf("AnnualDue(2026) = %s, want 2027-04-15", got)
	}
	if got := d.SecondInstalmentDue(2026); got != NewDate(2027, time.October, 15) {
		t.Errorf("SecondInstalmentDue(2026) = %s, want 2027-10-15", got)
	}
}

func TestPercentageTaxDeadlines(t *testing.T) {
	d := DefaultDeadlines()
	cases := []struct {
		year, n int
		want    Date
	}{
		{2026, 1, NewDate(2026, time.April, 25)},
		{2026, 2, NewDate(2026, time.July, 25)},
		{2026, 3, NewDate(2026, time.October, 25)},
		{2026, 4, NewDate(2027, time.January, 25)},
		// A leap year does not move the quarter ends, because none of them
		// is in February.
		{2024, 1, NewDate(2024, time.April, 25)},
	}
	for _, c := range cases {
		got, err := d.PercentageTaxDue(c.year, c.n)
		if err != nil {
			t.Fatalf("PercentageTaxDue(%d, %d): %v", c.year, c.n, err)
		}
		if got != c.want {
			t.Errorf("PercentageTaxDue(%d, %d) = %s, want %s", c.year, c.n, got, c.want)
		}
	}
	if _, err := d.PercentageTaxDue(2026, 0); err == nil {
		t.Error("there is no quarter nil")
	}
}

func TestDeadlinesNormalise(t *testing.T) {
	got := Deadlines{}.normalise()
	want := DefaultDeadlines()
	if len(got.QuarterlyMonths) != len(want.QuarterlyMonths) || got.QuarterlyDay != want.QuarterlyDay ||
		got.AnnualMonth != want.AnnualMonth || got.AnnualDay != want.AnnualDay ||
		got.SecondInstalmentMonth != want.SecondInstalmentMonth || got.SecondInstalmentDay != want.SecondInstalmentDay ||
		got.PercentageTaxDays != want.PercentageTaxDays {
		t.Fatalf("empty deadlines did not normalise to the defaults: %+v", got)
	}
	// An override of the quarterly day survives, so a change in the law can
	// be applied without a new binary.
	over := Deadlines{QuarterlyDay: 20}.normalise()
	if over.QuarterlyDay != 20 {
		t.Fatalf("an overridden quarterly day was lost: %d", over.QuarterlyDay)
	}
	moved := Deadlines{
		QuarterlyMonths: []time.Month{time.April, time.August, time.November},
		QuarterlyDay:    15,
	}.normalise()
	q1, err := moved.IncomeTaxDue(2017, 1)
	if err != nil {
		t.Fatal(err)
	}
	if q1 != NewDate(2017, time.April, 15) {
		t.Fatalf("the pre-TRAIN first quarter deadline should be overridable, got %s", q1)
	}
	// A nonsense month count falls back rather than panicking.
	if bad := (Deadlines{QuarterlyMonths: []time.Month{time.May}}).normalise(); len(bad.QuarterlyMonths) != QuarterlyReturns {
		t.Fatalf("a short month list should fall back, got %v", bad.QuarterlyMonths)
	}
}

func TestWeekendDetection(t *testing.T) {
	// 15 August 2026 is a Saturday; 15 May 2026 is a Friday.
	if !NewDate(2026, time.August, 15).IsWeekend() {
		t.Error("15 August 2026 is a Saturday")
	}
	if NewDate(2026, time.May, 15).IsWeekend() {
		t.Error("15 May 2026 is a Friday")
	}
	if NewDate(2026, time.August, 15).Weekday() != time.Saturday {
		t.Errorf("15 August 2026 is a %s", NewDate(2026, time.August, 15).Weekday())
	}
}

func TestDateStrings(t *testing.T) {
	d := NewDate(2026, time.May, 15)
	if d.String() != "2026-05-15" {
		t.Errorf("String() = %q", d.String())
	}
	if d.Long() != "15 May 2026" {
		t.Errorf("Long() = %q", d.Long())
	}
	if (Date{}).String() != "" || (Date{}).Long() != "" {
		t.Error("an unset date should print as nothing")
	}
}
