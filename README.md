# Quarterly income tax tracker and 1701Q figure preparer (Philippines)

A single binary that follows a Philippine freelancer's or virtual assistant's taxable year: record the
fees as they come in, and it works out the **cumulative** tax due at the end of each quarter under the
regime you elected, credits what the earlier quarters already paid, and lays the figures out ready to
transcribe onto a quarterly or annual return.

It runs on your own computer, serves a small interface on `127.0.0.1`, and keeps everything in one JSON
file. It makes no network calls. **It prepares figures; it files nothing.**

## What it does

- Records receipts, refunds and credit notes, other non-operating income, expenses, and the creditable
  tax your clients withheld on BIR Form No. 2307.
- Works out, quarter by quarter, the tax due on the **year to date** under the elected regime, then the
  year-to-date credits — the prior year's excess, the tax the earlier quarters paid, the withholding —
  and what is left to pay.
- Reports an **overpayment** in brackets, as the return does, when the credits exceed the tax due. A loss
  quarter after a profitable one produces an overpayment carried forward, never a negative payment.
- Keeps the quarterly **percentage tax** separate, because BIR Form No. 2551Q covers one quarter only and
  credits nothing — unlike the income tax return beside it.
- Lays out the figures BIR Forms **1701Q**, **1701** and **1701A** ask for, named line by line, rounded to
  whole pesos as the return asks, with the exact centavos still shown.
- Gives the year's filing dates with the provision each comes from, and flags one that lands on a weekend.
- Warns when gross passes the ₱3,000,000 registration threshold — which ends the 8% election for the whole
  year — and works the year again under the graduated rates with what you already paid credited.

## What it does not do

**It does not choose your regime.** Whether the 8% flat rate or the graduated rates cost you less is a
different question, answered once a year before you elect, and a separate tool in this family already
answers it: <https://sreenivas-sadhu-prabhakara.github.io/ph-8pct-vs-graduated/>. Here the regime is an
**input** — you tell this tool what you elected, and it tracks the year under it.

It also does not file, submit or e-file anything, does not validate a return, does not reproduce the
Bureau's form, and does not compute penalties, surcharge or interest.

## Who it's for

Philippine self-employed individuals and professionals registered with the BIR who file their own returns:
virtual assistants, freelance writers, designers and marketers, home-based customer-support workers,
consultants and solo practitioners. A mixed-income earner is handled on the business side only, and the
tool says so.

## The regulation

The National Internal Revenue Code as amended by the **TRAIN law** (Republic Act No. 10963):

- **sec. 74(A)** — the declaration of estimated income is filed *on or before 15 May of the same taxable
  year*, moved by the TRAIN law from 15 April.
- **sec. 74(B)** — *"the second and third shall be paid on August 15 and November 15 of the current year,
  respectively"*, and the fourth *"on or before April 15 of the following calendar year when the final
  adjusted income tax return is due to be filed"*. There is no fourth quarterly return.
- **sec. 51(C)(1)** — the annual return, on 15 April of the following year.
- **sec. 56(A)(2)** — where the tax due exceeds ₱2,000, the option to pay in two equal instalments, the
  second on or before 15 October following the close of the calendar year.
- **sec. 24(A)(2)(a) and (b)** — the graduated rates, and the 8% flat option on gross in excess of
  ₱250,000, in lieu of the graduated income tax and the percentage tax. The election is irrevocable for the
  taxable year.
- **sec. 34(L)** — the 40% optional standard deduction, computed from sales net of returns, allowances and
  discounts.
- **secs. 116 and 128(A)(1)** — the percentage tax, 3% of the quarter's gross, due within 25 days after the
  quarter ends. It was 1% from 1 July 2020 to 30 June 2023 under sec. 13 of the CREATE law (R.A. No.
  11534), restored to 3% from 1 July 2023 by BIR RMC No. 69-2023.
- **secs. 109(1)(BB) and 236(G)** — the ₱3,000,000 VAT registration threshold; **R.R. No. 8-2018** for what
  happens when an 8% filer crosses it mid-year.

Every rate, threshold and date is a **named constant in one place** (`internal/phtax/rates.go` and
`quarters.go`), published with its reference in the interface and by `-constants`, and every one of them is
overridable from the interface if the law moves.

## Install and run

```sh
git clone https://github.com/sreenivas-sadhu-prabhakara/freelancer-va-tax-co-pilot.git
cd freelancer-va-tax-co-pilot

go build -o freelancer-va-tax-co-pilot .
./freelancer-va-tax-co-pilot
```

Then open <http://127.0.0.1:8080/>. Two flags change the defaults:

```sh
./freelancer-va-tax-co-pilot -addr 127.0.0.1:9000 -data ~/tax/2026.json
```

It also runs once from the command line:

```sh
./freelancer-va-tax-co-pilot -report -year 2026 -quarter 2   # the position and the figures
./freelancer-va-tax-co-pilot -constants                      # every figure, with its source
./freelancer-va-tax-co-pilot -deadlines                      # this year's filing dates
```

Entries can be typed one at a time or pasted in from a spreadsheet, one per line — date, amount, kind,
note, tax withheld, expense category. Only the date and the amount are needed. A line that cannot be read
is reported by its number and left out; nothing is guessed.

## Where the data is stored

One JSON file on your own computer, `./freelancer-va-tax-co-pilot-data.json` by default, holding your
entries, your election and any figures you have overridden, one record per taxable year. It is written
atomically — to a temporary file that is then renamed — so an interrupted write cannot corrupt it, and it
is created readable only by you.

Nothing leaves the machine. The tool binds to 127.0.0.1, makes no outbound network calls, has no accounts,
no analytics and no telemetry, and loads no fonts, scripts or styles from anywhere but itself.

## Scope and accuracy

The figures come from what you record. The tool does not check them against your books, your invoices or
your clients' certificates, and it is not tax, accounting or legal advice.

Known gaps, stated rather than papered over:

- **The item numbers of BIR Form No. 1701Q could not be sourced** from a published copy of the form, so the
  quarterly figures are named and not numbered — match them to your copy by their labels. The item numbers
  offered for BIR Form No. 1701 were read from the form itself (January 2018 ENCS). BIR Form No. 1701A was
  not obtained either.
- **The ₱3,000,000 threshold is subject to a three-yearly CPI adjustment** under the TRAIN law. No
  adjustment had been issued at the time of writing; confirm it, and override it here if it has moved.
- **A deadline on a weekend or a public holiday is flagged, not moved.** Philippine public holidays are not
  encoded.
- **Penalties are not computed.** The Ease of Paying Taxes Act (R.A. No. 11976) reduced the surcharge and
  the interest for micro and small taxpayers, but the compromise penalty is a schedule and the interest
  runs from facts this tool does not hold.
- **The percentage tax base** used here is the quarter's sales net of returns, allowances and discounts;
  other non-operating income is left out of it.

Confirm every rate, threshold and filing date with the Bureau of Internal Revenue or a qualified
practitioner before you file.

MIT licensed.
