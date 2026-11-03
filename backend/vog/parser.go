package vog

import (
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
)

// ErrNotAVog is returned when the PDF does not look like a VOG at all.
var ErrNotAVog = errors.New("document is not a Verklaring Omtrent het Gedrag")

// ErrParseTimeout is returned when extracting the text of a page takes
// longer than DefaultParseTimeout, e.g. a crafted PDF designed to stall
// PDFium. The worker handling it is killed rather than returned to the pool.
var ErrParseTimeout = errors.New("pdf parsing timed out")

// DefaultParseTimeout bounds how long a single PDF may occupy a PDFium
// worker. Without it, a slow or malicious PDF could tie up the small,
// fixed-size worker pool indefinitely.
const DefaultParseTimeout = 20 * time.Second

// Parser extracts the printed data from a VOG PDF.
type Parser interface {
	Parse(pdf []byte) (*Document, error)
}

// Word is a positioned piece of text on the first page of the VOG. Coordinates
// are PDF user space points: X grows to the right, Y grows upwards, so Top is
// larger than Bottom and the first line of the page has the largest Top.
type Word struct {
	Text   string
	Left   float64
	Top    float64
	Right  float64
	Bottom float64
}

// PdfiumParser parses VOG PDFs with PDFium running in a WebAssembly sandbox
// (pure Go, no cgo). PDFium handles the AES encryption Justis applies to the
// VOG PDFs and exposes the position of every text run, which the layout based
// extraction below relies on.
type PdfiumParser struct {
	pool         pdfium.Pool
	parseTimeout time.Duration
}

// NewPdfiumParser initialises the PDFium WebAssembly pool. Initialisation
// compiles the WebAssembly module and takes a few seconds; do it once at
// startup.
func NewPdfiumParser() (*PdfiumParser, error) {
	pool, err := webassembly.Init(webassembly.Config{
		MinIdle:  1,
		MaxIdle:  2,
		MaxTotal: 4,
		// Lets Kill() interrupt a worker that is stuck mid-call; see
		// extractWords.
		RuntimeConfig: wazero.NewRuntimeConfig().WithCloseOnContextDone(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialise pdfium: %w", err)
	}
	return &PdfiumParser{pool: pool, parseTimeout: DefaultParseTimeout}, nil
}

// Close releases the PDFium pool.
func (p *PdfiumParser) Close() error {
	return p.pool.Close()
}

// Parse extracts the VOG data from the PDF bytes.
func (p *PdfiumParser) Parse(pdf []byte) (*Document, error) {
	words, err := p.extractWords(pdf)
	if err != nil {
		return nil, err
	}
	return ExtractDocument(words)
}

// extractWords runs the actual PDFium calls on a goroutine and races them
// against parseTimeout. A crafted PDF that makes PDFium hang would otherwise
// hold a worker (and the request goroutine) forever; on timeout the worker is
// killed instead of returned, so the pool can replace it and keep serving
// other requests.
func (p *PdfiumParser) extractWords(pdf []byte) ([]Word, error) {
	instance, err := p.pool.GetInstance(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to get pdfium instance: %w", err)
	}

	type result struct {
		words []Word
		err   error
	}
	done := make(chan result, 1)
	go func() {
		words, err := readWords(instance, pdf)
		done <- result{words, err}
	}()

	select {
	case r := <-done:
		if err := instance.Close(); err != nil {
			slog.Warn("failed to return pdfium instance to pool", "error", err)
		}
		return r.words, r.err
	case <-time.After(p.parseTimeout):
		slog.Warn("pdf parsing exceeded deadline, killing pdfium worker", "timeout", p.parseTimeout)
		if err := instance.Kill(); err != nil {
			slog.Warn("failed to kill stalled pdfium instance", "error", err)
		}
		return nil, ErrParseTimeout
	}
}

// readWords does the actual PDFium calls to open the document and extract
// the positioned text of its first page.
func readWords(instance pdfium.Pdfium, pdf []byte) ([]Word, error) {
	doc, err := instance.OpenDocument(&requests.OpenDocument{File: &pdf})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to open pdf: %v", ErrNotAVog, err)
	}
	defer func() {
		_, err := instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
		if err != nil {
			slog.Warn("failed to close pdf document", "error", err)
		}
	}()

	pageCount, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, fmt.Errorf("failed to count pages: %w", err)
	}
	if pageCount.PageCount < 1 {
		return nil, fmt.Errorf("%w: pdf has no pages", ErrNotAVog)
	}

	text, err := instance.GetPageTextStructured(&requests.GetPageTextStructured{
		Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: 0}},
		Mode: requests.GetPageTextStructuredModeRects,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to extract text: %w", err)
	}

	words := make([]Word, 0, len(text.Rects))
	for _, rect := range text.Rects {
		t := strings.TrimSpace(rect.Text)
		if t == "" {
			continue
		}
		words = append(words, Word{
			Text:   t,
			Left:   rect.PointPosition.Left,
			Top:    rect.PointPosition.Top,
			Right:  rect.PointPosition.Right,
			Bottom: rect.PointPosition.Bottom,
		})
	}
	return words, nil
}

// line is a group of words that share (approximately) the same baseline.
type line struct {
	top   float64
	words []Word
}

func (l line) text() string {
	parts := make([]string, len(l.words))
	for i, w := range l.words {
		parts[i] = w.Text
	}
	return strings.Join(parts, " ")
}

// lineTolerance is the maximum vertical distance (points) between two words on
// the same line. Word and its converters render the VOG at ~10pt so words on
// one line differ by a few points at most, while lines are ~10pt apart.
const lineTolerance = 4.0

func groupLines(words []Word) []line {
	sorted := make([]Word, len(words))
	copy(sorted, words)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Top != sorted[j].Top {
			return sorted[i].Top > sorted[j].Top
		}
		return sorted[i].Left < sorted[j].Left
	})

	var lines []line
	for _, w := range sorted {
		if len(lines) > 0 {
			current := &lines[len(lines)-1]
			// Compare against the running maximum of the line so a slightly
			// lower word (descenders) still joins the line.
			if current.top-w.Top <= lineTolerance {
				current.words = append(current.words, w)
				continue
			}
		}
		lines = append(lines, line{top: w.Top, words: []Word{w}})
	}
	for i := range lines {
		sort.Slice(lines[i].words, func(a, b int) bool {
			return lines[i].words[a].Left < lines[i].words[b].Left
		})
	}
	return lines
}

// fieldLabels maps the Dutch label printed on the VOG to the field it
// introduces. The English label precedes the Dutch one on the same line and the
// value follows the Dutch label, so the Dutch label is the anchor.
var fieldLabels = map[string]string{
	"Datum":          "issue_date",
	"kenmerk":        "reference_number",
	"Geslachtsnaam":  "surname",
	"Tussenvoegsels": "prefix",
	"Voorna(a)m(en)": "given_names",
	"Geboortedatum":  "date_of_birth",
	"Geboorteplaats": "place_of_birth",
	"Geboorteland":   "country_of_birth",
}

var codePattern = regexp.MustCompile(`\b\d{2}\b`)

// ExtractDocument interprets the positioned words of the first page of a VOG.
// It is layout based: labelled fields are read from the value column right of
// the Dutch label, the purpose is the paragraph after "voor:" and the profile
// codes are the line after "profiel:".
func ExtractDocument(words []Word) (*Document, error) {
	lines := groupLines(words)
	if len(lines) == 0 {
		return nil, fmt.Errorf("%w: no text found", ErrNotAVog)
	}

	fullText := make([]string, len(lines))
	for i, l := range lines {
		fullText[i] = l.text()
	}
	pageText := strings.Join(fullText, "\n")
	if !strings.Contains(pageText, "Verklaring Omtrent het Gedrag") {
		return nil, fmt.Errorf("%w: title not found", ErrNotAVog)
	}

	fields := map[string]string{}
	for _, l := range lines {
		for i, w := range l.words {
			field, ok := fieldLabels[w.Text]
			if !ok {
				continue
			}
			if _, seen := fields[field]; seen {
				continue
			}
			var value []string
			for _, v := range l.words[i+1:] {
				if v.Left > w.Right+2 {
					value = append(value, v.Text)
				}
			}
			fields[field] = strings.Join(value, " ")
		}
	}

	doc := &Document{
		ReferenceNumber: fields["reference_number"],
		Surname:         fields["surname"],
		Prefix:          fields["prefix"],
		GivenNames:      fields["given_names"],
		PlaceOfBirth:    fields["place_of_birth"],
		CountryOfBirth:  fields["country_of_birth"],
	}

	var err error
	if doc.IssueDate, err = ParseDutchDate(fields["issue_date"]); err != nil {
		return nil, fmt.Errorf("%w: issue date: %v", ErrNotAVog, err)
	}
	if doc.DateOfBirth, err = ParseDutchDate(fields["date_of_birth"]); err != nil {
		return nil, fmt.Errorf("%w: date of birth: %v", ErrNotAVog, err)
	}
	if doc.ReferenceNumber == "" || doc.Surname == "" || doc.GivenNames == "" {
		return nil, fmt.Errorf("%w: reference number, surname or given names missing", ErrNotAVog)
	}

	doc.Purpose = extractPurpose(fullText)
	doc.ProfileCodes = extractProfileCodes(fullText)
	if len(doc.ProfileCodes) == 0 {
		return nil, fmt.Errorf("%w: screening profile codes not found", ErrNotAVog)
	}

	return doc, nil
}

// extractPurpose returns the paragraph following "Hierbij geef ik u de VOG die
// u nodig heeft voor:" up to (not including) the screening result paragraph.
func extractPurpose(lines []string) string {
	start := -1
	for i, l := range lines {
		if strings.HasSuffix(strings.TrimSpace(l), "voor:") && strings.Contains(l, "nodig heeft") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}
	var parts []string
	for _, l := range lines[start:] {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "Uit de screening") {
			break
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, " ")
}

// extractProfileCodes returns the two digit codes on the line(s) following
// "...volgende profiel:" up to the next paragraph.
func extractProfileCodes(lines []string) []string {
	start := -1
	for i, l := range lines {
		if strings.Contains(l, "profiel:") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	var codes []string
	for _, l := range lines[start:] {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "Op de volgende") {
			break
		}
		codes = append(codes, codePattern.FindAllString(trimmed, -1)...)
	}
	return codes
}

var dutchMonths = map[string]time.Month{
	"januari": time.January, "februari": time.February, "maart": time.March,
	"april": time.April, "mei": time.May, "juni": time.June, "juli": time.July,
	"augustus": time.August, "september": time.September, "oktober": time.October,
	"november": time.November, "december": time.December,
	"jan": time.January, "feb": time.February, "mrt": time.March, "apr": time.April,
	"jun": time.June, "jul": time.July, "aug": time.August, "sep": time.September,
	"okt": time.October, "nov": time.November, "dec": time.December,
}

// ParseDutchDate parses dates as printed on the VOG ("25 maart 2026"); the
// numeric forms "25-03-2026" and "2026-03-25" are accepted as well.
func ParseDutchDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("empty date")
	}
	for _, layout := range []string{"02-01-2006", "2006-01-02", "2-1-2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	parts := strings.Fields(s)
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("unrecognised date %q", s)
	}
	var day, year int
	if _, err := fmt.Sscanf(parts[0], "%d", &day); err != nil {
		return time.Time{}, fmt.Errorf("unrecognised day in %q", s)
	}
	month, ok := dutchMonths[strings.ToLower(parts[1])]
	if !ok {
		return time.Time{}, fmt.Errorf("unrecognised month in %q", s)
	}
	if _, err := fmt.Sscanf(parts[2], "%d", &year); err != nil {
		return time.Time{}, fmt.Errorf("unrecognised year in %q", s)
	}
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if t.Day() != day {
		return time.Time{}, fmt.Errorf("invalid day in %q", s)
	}
	return t, nil
}
