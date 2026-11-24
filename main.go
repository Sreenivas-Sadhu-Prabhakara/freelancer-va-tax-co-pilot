// Command freelancer-va-tax-co-pilot keeps a Philippine freelancer's or
// virtual assistant's income tax year in order: it records what came in and
// what went out, works out the cumulative tax position at the end of each
// quarter under the regime the taxpayer has elected, credits what the earlier
// quarters already paid, and lays the figures out ready to transcribe onto a
// quarterly or annual return.
//
// It serves a small interface on the loopback address and keeps everything in
// one JSON file on the same machine. It makes no network calls of its own: a
// year of client names and fees never leaves the computer it runs on.
//
// It does not file, submit or validate anything, it does not compute penalties,
// and it does not decide between the 8% rate and the graduated rates — that
// election is an input here.
//
// It also runs once from the command line, which is what somebody filling in a
// return at the kitchen table wants:
//
//	freelancer-va-tax-co-pilot -report -quarter 2
//	freelancer-va-tax-co-pilot -constants
//	freelancer-va-tax-co-pilot -deadlines
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/sreenivas-sadhu-prabhakara/freelancer-va-tax-co-pilot/internal/phtax"
)

//go:embed web
var webFiles embed.FS

const (
	defaultAddr = "127.0.0.1:8080"
	defaultData = "./freelancer-va-tax-co-pilot-data.json"

	// maxRequestBytes caps one request. A year of a freelancer's invoices is
	// a few tens of kilobytes.
	maxRequestBytes = 4 << 20
	// maxEntriesPerYear stops a runaway paste from filling the data file.
	maxEntriesPerYear = 5000
	// dataFileVersion is the shape of the data file, for future upgrades.
	dataFileVersion = 1

	// contentSecurityPolicy is sent with the interface as well as being set
	// in the page itself: everything the page loads comes from this binary.
	contentSecurityPolicy = "default-src 'self'; base-uri 'self'; img-src 'self' data:; " +
		"style-src 'self'; script-src 'self'; font-src 'self'; form-action 'self'; connect-src 'self'"
)

func main() {
	addr := flag.String("addr", defaultAddr, "address to serve the interface on; keep it on 127.0.0.1 unless you know you want it reachable")
	data := flag.String("data", defaultData, "file this tool keeps your year in")

	year := flag.Int("year", 0, "the taxable year to work on; the current calendar year by default")
	report := flag.Bool("report", false, "run once instead of serving: print the year's position and the figures for a return")
	quarter := flag.Int("quarter", 0, "with -report: which quarter's figures to print, 1 to 4; 4 is the annual return")
	showConstants := flag.Bool("constants", false, "print every rate, threshold and filing date this tool uses, with its statutory reference, and exit")
	showDeadlines := flag.Bool("deadlines", false, "print the year's filing dates and exit")
	flag.Parse()

	st, err := openStore(*data)
	if err != nil {
		log.Fatalf("freelancer-va-tax-co-pilot: %v", err)
	}
	y := *year
	if y == 0 {
		y = st.currentYear()
	}

	switch {
	case *showConstants:
		printConstants(os.Stdout, st.rates())
		return
	case *showDeadlines:
		pos, err := st.compute(y)
		if err != nil {
			log.Fatalf("freelancer-va-tax-co-pilot: %v", err)
		}
		printDeadlines(os.Stdout, pos)
		return
	case *report:
		pos, err := st.compute(y)
		if err != nil {
			log.Fatalf("freelancer-va-tax-co-pilot: %v", err)
		}
		printReport(os.Stdout, pos, *quarter)
		return
	}

	ui, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatalf("freelancer-va-tax-co-pilot: %v", err)
	}
	srv := &server{store: st, ui: http.FileServerFS(ui)}
	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.routes(),
		ReadHeaderTimeout: 20 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("quarterly tax tracker ready at http://%s/ (taxable year %d, data file: %s)", *addr, y, st.path)
	log.Printf("press Ctrl-C to stop")
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("freelancer-va-tax-co-pilot: %v", err)
	}
}

// ---------------------------------------------------------------- the data file

// yearData is one taxable year: what the taxpayer elected and what they
// recorded.
type yearData struct {
	Year     int            `json:"year"`
	Election phtax.Election `json:"election"`
	Entries  []phtax.Entry  `json:"entries"`
	Notes    string         `json:"notes,omitempty"`
}

type fileData struct {
	Version     int               `json:"version"`
	CurrentYear int               `json:"currentYear"`
	Years       map[int]*yearData `json:"years"`
	// Overrides, when set, replaces the published rates wholesale. It is
	// nil until the user changes a figure, so a change in the published
	// defaults reaches anyone who has not overridden them.
	Overrides *phtax.Rates `json:"rateOverrides,omitempty"`
}

type store struct {
	mu   sync.Mutex
	path string
	data fileData
}

func openStore(path string) (*store, error) {
	st := &store{path: path, data: fileData{
		Version:     dataFileVersion,
		CurrentYear: time.Now().Year(),
		Years:       map[int]*yearData{},
	}}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return st, nil
	case err != nil:
		return nil, fmt.Errorf("could not open the data file %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return st, nil
	}
	var loaded fileData
	if err := json.Unmarshal(b, &loaded); err != nil {
		return nil, fmt.Errorf("the data file %s is not readable (%v) — move it aside or point -data somewhere else", path, err)
	}
	if loaded.Years == nil {
		loaded.Years = map[int]*yearData{}
	}
	for y, yd := range loaded.Years {
		if yd == nil {
			delete(loaded.Years, y)
			continue
		}
		yd.Year = y
	}
	if loaded.CurrentYear == 0 {
		loaded.CurrentYear = time.Now().Year()
	}
	loaded.Version = dataFileVersion
	st.data = loaded
	return st, nil
}

// save writes the data file in one piece: a temporary file in the same
// directory, flushed to disk, then renamed over the old one, so an interrupted
// write cannot leave a half-written file behind.
func (s *store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, append(b, '\n'))
}

func writeFileAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func(e error) error {
		tmp.Close()
		os.Remove(name)
		return e
	}
	if _, err := tmp.Write(body); err != nil {
		return cleanup(err)
	}
	if err := tmp.Sync(); err != nil {
		return cleanup(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func (s *store) currentYear() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.CurrentYear
}

func (s *store) rates() phtax.Rates {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Overrides != nil {
		return *s.data.Overrides
	}
	return phtax.DefaultRates()
}

func (s *store) overridden() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Overrides != nil
}

// yearOf returns the stored year, creating an empty one if it is new. The
// caller holds the lock.
func (s *store) yearOf(year int) *yearData {
	yd, ok := s.data.Years[year]
	if !ok || yd == nil {
		yd = &yearData{Year: year, Election: phtax.Election{
			Regime:    phtax.Graduated,
			Deduction: phtax.OSD,
		}}
		s.data.Years[year] = yd
	}
	return yd
}

func (s *store) snapshot(year int) (yearData, phtax.Rates) {
	s.mu.Lock()
	defer s.mu.Unlock()
	yd := s.yearOf(year)
	out := yearData{Year: yd.Year, Election: yd.Election, Notes: yd.Notes,
		Entries: append([]phtax.Entry(nil), yd.Entries...)}
	if s.data.Overrides != nil {
		return out, *s.data.Overrides
	}
	return out, phtax.DefaultRates()
}

func (s *store) compute(year int) (phtax.YearPosition, error) {
	yd, rates := s.snapshot(year)
	return phtax.ComputeYear(phtax.YearInput{
		Year: year, Election: yd.Election, Entries: yd.Entries,
		Rates: rates, Today: phtax.Today(),
	})
}

func (s *store) years() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int, 0, len(s.data.Years))
	for y := range s.data.Years {
		out = append(out, y)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

func (s *store) setCurrentYear(year int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.CurrentYear = year
	s.yearOf(year)
	return s.save()
}

func (s *store) putElection(year int, el phtax.Election) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	yd := s.yearOf(year)
	// The payments the user has recorded are kept here, not in the form the
	// browser sends, so that saving the election cannot lose them.
	el.Payments = yd.Election.Payments
	yd.Election = el
	s.data.CurrentYear = year
	return s.save()
}

func (s *store) putPayment(year, quarter int, p phtax.QuarterPayment) error {
	if quarter < 1 || quarter > phtax.QuartersPerYear {
		return fmt.Errorf("there are %d quarters in a year, not a quarter %d", phtax.QuartersPerYear, quarter)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	yd := s.yearOf(year)
	yd.Election.Payments[quarter-1] = p
	return s.save()
}

func (s *store) addEntries(year int, entries []phtax.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	yd := s.yearOf(year)
	if len(yd.Entries)+len(entries) > maxEntriesPerYear {
		return fmt.Errorf("a year holds at most %d entries; this would make %d", maxEntriesPerYear, len(yd.Entries)+len(entries))
	}
	for _, e := range entries {
		if err := e.Validate(year); err != nil {
			return err
		}
		if e.ID == "" {
			e.ID = newID()
		}
		yd.Entries = append(yd.Entries, e)
	}
	phtax.SortEntries(yd.Entries)
	s.data.CurrentYear = year
	return s.save()
}

func (s *store) deleteEntry(year int, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	yd := s.yearOf(year)
	for i, e := range yd.Entries {
		if e.ID == id {
			yd.Entries = append(yd.Entries[:i:i], yd.Entries[i+1:]...)
			return true, s.save()
		}
	}
	return false, nil
}

func (s *store) putRates(r *phtax.Rates) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Overrides = r
	return s.save()
}

func newID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

// ------------------------------------------------------------------- the server

type server struct {
	store *store
	ui    http.Handler
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("POST /api/year", s.handleYear)
	mux.HandleFunc("POST /api/election", s.handleElection)
	mux.HandleFunc("POST /api/payment", s.handlePayment)
	mux.HandleFunc("POST /api/entry", s.handleAddEntry)
	mux.HandleFunc("POST /api/entries", s.handleAddEntries)
	mux.HandleFunc("DELETE /api/entry", s.handleDeleteEntry)
	mux.HandleFunc("POST /api/rates", s.handleRates)
	mux.HandleFunc("GET /api/export", s.handleExport)
	mux.Handle("/", s.ui)
	return headers(sameOriginOnly(mux))
}

// headers sends the same policy as the page's own meta tag, so the interface
// can only ever load what this binary serves.
func headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// sameOriginOnly refuses a request that another web page tried to make to this
// tool. The interface is served from this binary, so its own requests always
// carry a matching origin or none at all.
func sameOriginOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
				httpError(w, http.StatusForbidden, "this request did not come from the tool's own page")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					httpError(w, http.StatusForbidden, "this request did not come from the tool's own page")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("could not write the reply: %v", err)
	}
}

func httpError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, into any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("could not read the request: %w", err)
	}
	return nil
}

// yearParam reads the taxable year from a query string or a request body,
// falling back to the stored current year.
func (s *server) yearParam(raw string) int {
	if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n > 0 {
		return n
	}
	return s.store.currentYear()
}

// state is everything the interface needs in one reply.
func (s *server) state(year int) (map[string]any, error) {
	yd, rates := s.store.snapshot(year)
	pos, err := phtax.ComputeYear(phtax.YearInput{
		Year: year, Election: yd.Election, Entries: yd.Entries,
		Rates: rates, Today: phtax.Today(),
	})
	if err != nil {
		return nil, err
	}
	figures := make([]phtax.FormFigures, 0, phtax.QuartersPerYear)
	for q := 1; q <= phtax.QuartersPerYear; q++ {
		f, err := pos.Figures(q)
		if err != nil {
			return nil, err
		}
		figures = append(figures, f)
	}
	return map[string]any{
		"year":            year,
		"years":           s.store.years(),
		"today":           phtax.Today(),
		"election":        yd.Election,
		"entries":         yd.Entries,
		"position":        pos,
		"figures":         figures,
		"constants":       rates.Constants(),
		"rates":           rates,
		"ratesOverridden": s.store.overridden(),
		"decisionTool": map[string]string{
			"url":  "https://sreenivas-sadhu-prabhakara.github.io/ph-8pct-vs-graduated/",
			"what": "Choosing between the 8% flat rate and the graduated rates is a separate job, and a separate tool does it.",
		},
	}, nil
}

func (s *server) replyState(w http.ResponseWriter, year int) {
	st, err := s.state(year)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	s.replyState(w, s.yearParam(r.URL.Query().Get("year")))
}

func (s *server) handleYear(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Year int `json:"year"`
	}
	if err := decode(w, r, &in); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.Year < 1900 || in.Year > 2200 {
		httpError(w, http.StatusBadRequest, fmt.Sprintf("%d is not a taxable year this tool will work on", in.Year))
		return
	}
	if err := s.store.setCurrentYear(in.Year); err != nil {
		httpError(w, http.StatusInternalServerError, "could not save the data file: "+err.Error())
		return
	}
	s.replyState(w, in.Year)
}

func (s *server) handleElection(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Year            int    `json:"year"`
		Regime          string `json:"regime"`
		Deduction       string `json:"deduction"`
		HasCompensation bool   `json:"hasCompensation"`
		VATRegistered   bool   `json:"vatRegistered"`
		PriorYearExcess string `json:"priorYearExcess"`
		OtherCredits    string `json:"otherCredits"`
	}
	if err := decode(w, r, &in); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	year := s.yearParam(strconv.Itoa(in.Year))
	el := phtax.Election{
		Regime:          phtax.Regime(in.Regime),
		Deduction:       phtax.DeductionMethod(in.Deduction),
		HasCompensation: in.HasCompensation,
		VATRegistered:   in.VATRegistered,
	}
	var err error
	if el.PriorYearExcess, err = phtax.ParseAmount(in.PriorYearExcess); err != nil {
		httpError(w, http.StatusBadRequest, "the prior year's excess credits: "+err.Error())
		return
	}
	if el.OtherCredits, err = phtax.ParseAmount(in.OtherCredits); err != nil {
		httpError(w, http.StatusBadRequest, "the other credits: "+err.Error())
		return
	}
	if el.PriorYearExcess < 0 || el.OtherCredits < 0 {
		httpError(w, http.StatusBadRequest, "a credit cannot be negative")
		return
	}
	if err := s.store.putElection(year, el); err != nil {
		httpError(w, http.StatusInternalServerError, "could not save the data file: "+err.Error())
		return
	}
	s.replyState(w, year)
}

func (s *server) handlePayment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Year     int    `json:"year"`
		Quarter  int    `json:"quarter"`
		Recorded bool   `json:"recorded"`
		Amount   string `json:"amount"`
		FiledOn  string `json:"filedOn"`
	}
	if err := decode(w, r, &in); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	year := s.yearParam(strconv.Itoa(in.Year))
	p := phtax.QuarterPayment{Recorded: in.Recorded}
	amount, err := phtax.ParseAmount(in.Amount)
	if err != nil {
		httpError(w, http.StatusBadRequest, "the amount paid: "+err.Error())
		return
	}
	if amount < 0 {
		httpError(w, http.StatusBadRequest, "a payment cannot be negative")
		return
	}
	p.Amount = amount
	if strings.TrimSpace(in.FiledOn) != "" {
		d, err := phtax.ParseDate(in.FiledOn)
		if err != nil {
			httpError(w, http.StatusBadRequest, "the date filed: "+err.Error())
			return
		}
		p.FiledOn = d
	}
	if err := s.store.putPayment(year, in.Quarter, p); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.replyState(w, year)
}

func (s *server) handleAddEntry(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Year     int    `json:"year"`
		Date     string `json:"date"`
		Kind     string `json:"kind"`
		Amount   string `json:"amount"`
		Client   string `json:"client"`
		Note     string `json:"note"`
		Withheld string `json:"withheld"`
		Category string `json:"category"`
	}
	if err := decode(w, r, &in); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	year := s.yearParam(strconv.Itoa(in.Year))

	d, err := phtax.ParseDate(in.Date)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	kind, err := phtax.ParseKind(in.Kind)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	amount, err := phtax.ParseAmount(in.Amount)
	if err != nil {
		httpError(w, http.StatusBadRequest, "the amount: "+err.Error())
		return
	}
	withheld, err := phtax.ParseAmount(in.Withheld)
	if err != nil {
		httpError(w, http.StatusBadRequest, "the tax withheld: "+err.Error())
		return
	}
	e := phtax.Entry{
		Date: d, Kind: kind, Amount: amount,
		Client: strings.TrimSpace(in.Client), Note: strings.TrimSpace(in.Note),
		Withheld: withheld,
	}
	if kind == phtax.Expense {
		e.Category = phtax.ExpenseCategory(in.Category)
		if e.Category == "" {
			e.Category = phtax.OrdinaryDeduction
		}
	}
	if err := e.Validate(year); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.addEntries(year, []phtax.Entry{e}); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.replyState(w, year)
}

func (s *server) handleAddEntries(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Year int    `json:"year"`
		Text string `json:"text"`
	}
	if err := decode(w, r, &in); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	year := s.yearParam(strconv.Itoa(in.Year))
	parsed := phtax.ParseRows(in.Text, year)
	if len(parsed.Entries) > 0 {
		if err := s.store.addEntries(year, parsed.Entries); err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	st, err := s.state(year)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	st["added"] = len(parsed.Entries)
	st["problems"] = parsed.Problems
	writeJSON(w, http.StatusOK, st)
}

func (s *server) handleDeleteEntry(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	year := s.yearParam(q.Get("year"))
	removed, err := s.store.deleteEntry(year, q.Get("id"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not save the data file: "+err.Error())
		return
	}
	if !removed {
		httpError(w, http.StatusNotFound, "no entry with that reference")
		return
	}
	s.replyState(w, year)
}

func (s *server) handleRates(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Year  int          `json:"year"`
		Reset bool         `json:"reset"`
		Rates *phtax.Rates `json:"rates"`
	}
	if err := decode(w, r, &in); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	year := s.yearParam(strconv.Itoa(in.Year))
	switch {
	case in.Reset:
		if err := s.store.putRates(nil); err != nil {
			httpError(w, http.StatusInternalServerError, "could not save the data file: "+err.Error())
			return
		}
	case in.Rates != nil:
		if err := validRates(*in.Rates); err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.store.putRates(in.Rates); err != nil {
			httpError(w, http.StatusInternalServerError, "could not save the data file: "+err.Error())
			return
		}
	default:
		httpError(w, http.StatusBadRequest, "send the figures to use, or reset to go back to the published ones")
		return
	}
	s.replyState(w, year)
}

// validRates refuses an override that would make a computation meaningless.
// It is deliberately permissive about the figures themselves: the point of the
// override is that the law can change to something we did not expect.
func validRates(r phtax.Rates) error {
	if r.EightPercentRateBP < 0 || r.EightPercentRateBP > 10000 {
		return errors.New("a rate is a share, so it belongs between 0% and 100%")
	}
	if r.OSDRateBP < 0 || r.OSDRateBP > 10000 {
		return errors.New("the standard deduction is a share, so it belongs between 0% and 100%")
	}
	if r.EightPercentAllowance < 0 || r.VATThreshold < 0 || r.SecondInstalmentMinimum < 0 {
		return errors.New("a threshold cannot be negative")
	}
	if len(r.Graduated) == 0 {
		return errors.New("at least one graduated rate table is needed")
	}
	for _, s := range r.Graduated {
		if len(s.Brackets) == 0 {
			return errors.New("a graduated rate table with no brackets cannot tax anything")
		}
		last := phtax.Centavos(-1)
		for _, b := range s.Brackets {
			if b.Over < last {
				return errors.New("the brackets of a graduated rate table must run upwards")
			}
			if b.RateBP < 0 || b.RateBP > 10000 {
				return errors.New("a bracket rate belongs between 0% and 100%")
			}
			last = b.Over
		}
	}
	return nil
}

// handleExport writes the year out, either as the entries so they can be kept
// somewhere else, or as the figures for a return so they can be printed.
func (s *server) handleExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	year := s.yearParam(q.Get("year"))
	pos, err := s.store.compute(year)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	switch q.Get("what") {
	case "figures":
		quarter, _ := strconv.Atoi(q.Get("quarter"))
		if quarter < 1 || quarter > phtax.QuartersPerYear {
			quarter = phtax.QuartersPerYear
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=tax-figures-%d-q%d.txt", year, quarter))
		printReport(w, pos, quarter)
	default:
		yd, _ := s.store.snapshot(year)
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=entries-%d.csv", year))
		writeEntriesCSV(w, yd.Entries)
	}
}

func writeEntriesCSV(w io.Writer, entries []phtax.Entry) {
	fmt.Fprintln(w, "Date,Amount,Kind,Note,Withheld,Category")
	for _, e := range entries {
		note := strings.TrimSpace(e.Client + " " + e.Note)
		fmt.Fprintf(w, "%s,%s,%s,%s,%s,%s\n",
			e.Date, e.Amount.Plain(), e.Kind, csvField(note), e.Withheld.Plain(), e.Category)
	}
}

func csvField(s string) string {
	s = strings.NewReplacer("\n", " ", "\r", " ").Replace(s)
	if strings.ContainsAny(s, `,"`) {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// ------------------------------------------------------------ printed output

func printConstants(w io.Writer, rates phtax.Rates) {
	fmt.Fprintln(w, "Every rate, threshold and filing date this tool uses.")
	fmt.Fprintln(w, "All of them are indicative and all of them can be overridden.")
	fmt.Fprintln(w, "Confirm the current figures with the Bureau of Internal Revenue before you file.")
	fmt.Fprintln(w)
	for _, c := range rates.Constants() {
		fmt.Fprintf(w, "%s\n", c.Name)
		fmt.Fprintf(w, "    value       %s\n", c.Value)
		fmt.Fprintf(w, "    what it is  %s\n", wrap(c.What, 64, 16))
		fmt.Fprintf(w, "    source      %s\n", wrap(c.Reference, 64, 16))
		fmt.Fprintf(w, "    sourcing    %s\n", c.Confidence)
		if c.Field != "" {
			fmt.Fprintf(w, "    override    %s\n", c.Field)
		}
		fmt.Fprintln(w)
	}
}

func printDeadlines(w io.Writer, pos phtax.YearPosition) {
	fmt.Fprintf(w, "Filing dates for the %d taxable year.\n", pos.Year)
	fmt.Fprintln(w, "These are the statutory dates. Confirm them with the Bureau, and note that this")
	fmt.Fprintln(w, "tool does not move a date for a weekend or a public holiday.")
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	fmt.Fprintln(tw, "DUE\tFORM\tWHAT\tSTATUS")
	for _, d := range pos.Deadlines {
		status := d.Status
		switch {
		case d.Status == "overdue":
			status = fmt.Sprintf("overdue by %d day(s)", -d.DaysAway)
		case d.Status == "soon" || d.Status == "upcoming":
			status = fmt.Sprintf("in %d day(s)", d.DaysAway)
		}
		if d.IsWeekend {
			status += " (a " + d.Due.Weekday().String() + ")"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", d.Due, d.Form, d.What, status)
	}
	tw.Flush()
}

func printReport(w io.Writer, pos phtax.YearPosition, quarter int) {
	if quarter < 1 || quarter > phtax.QuartersPerYear {
		quarter = currentQuarter(pos)
	}
	p := pos.Quarters[quarter-1]

	fmt.Fprintf(w, "Taxable year %d — ", pos.Year)
	switch p.Regime {
	case phtax.EightPercent:
		fmt.Fprintln(w, "elected the 8% flat rate")
	default:
		method := "the optional standard deduction"
		if p.Deduction == phtax.Itemized {
			method = "itemised deductions"
		}
		fmt.Fprintf(w, "elected the graduated rates with %s\n", method)
	}
	fmt.Fprintln(w, strings.Repeat("=", 72))
	fmt.Fprintln(w)

	fmt.Fprintln(w, "THE YEAR, QUARTER BY QUARTER (each line is the year to date)")
	tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "\tGROSS\tTAXABLE\tTAX DUE\tCREDITS\tTO PAY\tOVERPAID\t")
	for _, q := range pos.Quarters {
		label := fmt.Sprintf("Q%d %s", q.Quarter, q.Period.Months)
		if q.IsAnnual {
			label = "Year (annual)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t\n", label,
			q.Gross, q.Taxable, q.TaxDue, q.TotalCredits, q.AmountToPay, q.Overpayment)
	}
	tw.Flush()
	fmt.Fprintln(w)

	f, err := pos.Figures(quarter)
	if err != nil {
		fmt.Fprintf(w, "could not lay out the figures: %v\n", err)
		return
	}
	fmt.Fprintf(w, "FIGURES FOR BIR FORM No. %s\n", f.Form)
	fmt.Fprintf(w, "covers %s\n", f.Covers)
	fmt.Fprintf(w, "due    %s", f.Deadline.Long())
	if f.Deadline.IsWeekend() {
		fmt.Fprintf(w, " (a %s — confirm the working date)", f.Deadline.Weekday())
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w)
	for _, b := range f.Blocks {
		fmt.Fprintf(w, "  %s\n", b.Title)
		printBlock(w, b)
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "  %s\n\n", wrap(f.Caveat, 68, 2))

	if len(p.Notes) > 0 {
		fmt.Fprintln(w, "NOTES ON THIS QUARTER")
		printNotes(w, p.Notes)
	}
	if pos.RecomputedWhy != "" {
		fmt.Fprintln(w, "THE ELECTION HAS COME APART")
		fmt.Fprintf(w, "  %s\n\n", wrap(pos.RecomputedWhy, 68, 2))
		tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', tabwriter.AlignRight)
		fmt.Fprintln(tw, "\tTAXABLE\tTAX DUE\tCREDITS\tTO PAY\tOVERPAID\t")
		for _, q := range pos.Recomputed {
			label := fmt.Sprintf("Q%d", q.Quarter)
			if q.IsAnnual {
				label = "Year"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t\n", label,
				q.Taxable, q.TaxDue, q.TotalCredits, q.AmountToPay, q.Overpayment)
		}
		tw.Flush()
		fmt.Fprintln(w)
	}
	if pos.Projection.Available {
		fmt.Fprintln(w, "AT THIS PACE")
		fmt.Fprintf(w, "  %s of gross over %d quarter(s) annualises to %s, on which the tax would be %s.\n",
			pos.Projection.GrossToDate, pos.Projection.QuartersCounted,
			pos.Projection.ProjectedGross, pos.Projection.ProjectedTax)
		if pos.Projection.CrossesThreshold {
			fmt.Fprintln(w, "  At that pace the year crosses the registration threshold.")
		}
		fmt.Fprintln(w, "  This is a straight line through what is recorded, not a forecast.")
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "NOTES ON THE YEAR")
	printNotes(w, pos.Notes)
	printDeadlines(w, pos)
}

// printBlock lays one block of figures out by hand rather than through a
// tabwriter, because a note under a line would otherwise widen the label
// column to the width of the note and push every figure off the screen.
const blockLabelWidth = 58

func printBlock(w io.Writer, b phtax.FigureBlock) {
	for _, l := range b.Lines {
		value := l.Text
		if l.Kind != phtax.Choice {
			value = l.Peso.String()
		}
		label := l.Label
		if len(label) > blockLabelWidth {
			label = label[:blockLabelWidth-1] + "…"
		}
		pad := strings.Repeat(" ", blockLabelWidth-len([]rune(label)))
		if len([]rune(label)) > blockLabelWidth {
			pad = " "
		}
		suffix := ""
		if l.Item != "" {
			suffix = "   item " + l.Item
		}
		fmt.Fprintf(w, "    %s%s  %s%s\n", label, pad, value, suffix)
		if l.Note != "" {
			fmt.Fprintf(w, "        — %s\n", wrap(l.Note, 62, 10))
		}
	}
}

func printNotes(w io.Writer, notes []phtax.Note) {
	for _, n := range notes {
		mark := " "
		switch n.Severity {
		case phtax.Warn:
			mark = "!"
		case phtax.Stop:
			mark = "*"
		}
		fmt.Fprintf(w, "  %s %s\n", mark, wrap(n.Text, 68, 4))
		if n.Reference != "" {
			fmt.Fprintf(w, "      %s\n", wrap(n.Reference, 64, 6))
		}
	}
	fmt.Fprintln(w)
}

// currentQuarter is the quarter whose return is next to be filed: the first
// one whose deadline has not passed, or the annual return.
func currentQuarter(pos phtax.YearPosition) int {
	today := phtax.Today()
	for _, q := range pos.Quarters {
		if !q.Deadline.Before(today) {
			return q.Quarter
		}
	}
	return phtax.QuartersPerYear
}

// wrap folds a long line so that a terminal reading of the report stays
// readable, indenting continuations to line up under the first word.
func wrap(s string, width, indent int) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var out strings.Builder
	col := 0
	pad := strings.Repeat(" ", indent)
	for i, word := range words {
		if i > 0 {
			if col+1+len(word) > width {
				out.WriteString("\n" + pad)
				col = 0
			} else {
				out.WriteByte(' ')
				col++
			}
		}
		out.WriteString(word)
		col += len(word)
	}
	return out.String()
}
