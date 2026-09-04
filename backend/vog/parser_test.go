package vog

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// sharedParser is initialised once: compiling the PDFium WebAssembly module
// takes seconds.
var sharedParser *PdfiumParser

func TestMain(m *testing.M) {
	parser, err := NewPdfiumParser()
	if err != nil {
		panic(err)
	}
	sharedParser = parser
	code := m.Run()
	_ = parser.Close()
	os.Exit(code)
}

func readTestPdf(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "test-data", name))
	require.NoError(t, err)
	return data
}

func TestParseRealVog(t *testing.T) {
	testCases := []struct {
		file      string
		reference string
		issued    time.Time
		codes     []string
	}{
		{"vog-9999012026032500922.pdf", "9999012026032500922", time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC), []string{"84", "85"}},
		{"vog-9999012026050801510.pdf", "9999012026050801510", time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC), []string{"11", "12", "84", "85"}},
	}

	for _, tc := range testCases {
		t.Run(tc.file, func(t *testing.T) {
			doc, err := sharedParser.Parse(readTestPdf(t, tc.file))
			require.NoError(t, err)

			require.Equal(t, tc.reference, doc.ReferenceNumber)
			require.Equal(t, tc.issued, doc.IssueDate)
			require.Equal(t, "Mulder", doc.Surname)
			require.Equal(t, "", doc.Prefix)
			require.Equal(t, "Dibran", doc.GivenNames)
			require.Equal(t, time.Date(1991, 5, 14, 0, 0, 0, 0, time.UTC), doc.DateOfBirth)
			require.Equal(t, "Barneveld", doc.PlaceOfBirth)
			require.Equal(t, "Nederland", doc.CountryOfBirth)
			require.Equal(t, "Vrijwilliger bij Hervormde Gemeente te Barneveld", doc.Purpose)
			require.Equal(t, tc.codes, doc.ProfileCodes)
		})
	}
}

func TestParseRejectsNonPdf(t *testing.T) {
	_, err := sharedParser.Parse([]byte("this is not a pdf"))
	require.ErrorIs(t, err, ErrNotAVog)
}

// TestParseTimeoutKillsWorkerAndPoolRecovers guards against a slow/malicious
// PDF tying up a PDFium worker forever: with the deadline forced well below
// what parsing takes, Parse must give up with ErrParseTimeout, and the pool
// must still be able to serve a normal request afterwards (i.e. the stalled
// worker was killed and replaced, not left wedged).
func TestParseTimeoutKillsWorkerAndPoolRecovers(t *testing.T) {
	original := sharedParser.parseTimeout
	t.Cleanup(func() { sharedParser.parseTimeout = original })

	pdf := readTestPdf(t, "vog-9999012026032500922.pdf")

	sharedParser.parseTimeout = time.Nanosecond
	_, err := sharedParser.Parse(pdf)
	require.ErrorIs(t, err, ErrParseTimeout)

	sharedParser.parseTimeout = original
	doc, err := sharedParser.Parse(pdf)
	require.NoError(t, err)
	require.Equal(t, "9999012026032500922", doc.ReferenceNumber)
}

// word builds a Word at the given position with a nominal 10pt height.
func word(text string, left, top float64) Word {
	return Word{Text: text, Left: left, Top: top, Right: left + float64(len(text))*5, Bottom: top - 8}
}

// syntheticVog mimics the layout of a real VOG: English label, Dutch label and
// value on one line, value column starting at x=227.
func syntheticVog(prefix string) []Word {
	words := []Word{
		word("Verklaring", 67, 771), word("Omtrent", 139, 771), word("het", 197, 771), word("Gedrag", 222, 771),
		word("Date", 125, 600), word("Datum", 150, 600), word("1", 228, 600), word("oktober", 236, 600), word("2025", 270, 600),
		word("Our", 86, 590), word("reference", 105, 590), word("Ons", 149, 590), word("kenmerk", 169, 590), word("1234567890123456789", 227, 590),
		word("Surname", 106, 580), word("Geslachtsnaam", 149, 580), word("Berg", 228, 580),
		word("Prefix", 69, 571), word("to", 97, 570), word("surname", 108, 569), word("Tussenvoegsels", 149, 571),
		word("Given", 90, 560), word("names", 117, 559), word("Voorna(a)m(en)", 149, 561), word("Anna", 228, 561), word("Maria", 250, 561),
		word("Date", 91, 550), word("of", 114, 551), word("birth", 125, 551), word("Geboortedatum", 149, 551), word("3", 228, 550), word("februari", 236, 551), word("1980", 275, 551),
		word("Place", 89, 541), word("of", 114, 541), word("birth", 125, 541), word("Geboorteplaats", 149, 541), word("Den", 228, 541), word("Haag", 248, 541),
		word("Country", 77, 531), word("of", 114, 531), word("birth", 125, 531), word("Geboorteland", 149, 531), word("Nederland", 228, 531),
		word("Hierbij", 150, 507), word("geef", 178, 507), word("ik", 199, 507), word("u", 209, 505), word("de", 216, 507), word("VOG", 228, 507), word("die", 250, 507), word("u", 265, 505), word("nodig", 272, 507), word("heeft", 297, 507), word("voor:", 320, 505),
		word("Medewerker", 149, 495), word("kinderopvang", 200, 495),
		word("bij", 149, 483), word("De", 165, 483), word("Kleine", 180, 483), word("Beer", 210, 483),
		word("Uit", 150, 471), word("de", 163, 471), word("screening", 176, 471),
		word("Er", 150, 435), word("is", 161, 435), word("bij", 170, 435), word("deze", 182, 435), word("screening", 204, 435), word("uitgegaan", 246, 435), word("van", 288, 433), word("het", 306, 435), word("volgende", 321, 435), word("profiel:", 361, 435),
		word("21,", 149, 423), word("84,", 166, 423), word("86", 181, 423),
		word("Op", 149, 399), word("de", 163, 399), word("volgende", 176, 399), word("pagina", 215, 399),
	}
	if prefix != "" {
		words = append(words, word(prefix, 228, 571))
	}
	return words
}

func TestExtractDocumentSynthetic(t *testing.T) {
	doc, err := ExtractDocument(syntheticVog("van den"))
	require.NoError(t, err)

	require.Equal(t, "1234567890123456789", doc.ReferenceNumber)
	require.Equal(t, time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC), doc.IssueDate)
	require.Equal(t, "Berg", doc.Surname)
	require.Equal(t, "van den", doc.Prefix)
	require.Equal(t, "van den Berg", doc.FullSurname())
	require.Equal(t, "Anna Maria", doc.GivenNames)
	require.Equal(t, time.Date(1980, 2, 3, 0, 0, 0, 0, time.UTC), doc.DateOfBirth)
	require.Equal(t, "Den Haag", doc.PlaceOfBirth)
	require.Equal(t, "Nederland", doc.CountryOfBirth)
	// The purpose spans two lines and stops before the screening paragraph.
	require.Equal(t, "Medewerker kinderopvang bij De Kleine Beer", doc.Purpose)
	require.Equal(t, []string{"21", "84", "86"}, doc.ProfileCodes)
	require.Equal(t, "21, 84, 86", doc.ProfileCodesString())
	require.True(t, doc.HasProfileCode("84"))
	require.False(t, doc.HasProfileCode("85"))
}

func TestExtractDocumentWithoutPrefix(t *testing.T) {
	doc, err := ExtractDocument(syntheticVog(""))
	require.NoError(t, err)
	require.Equal(t, "", doc.Prefix)
	require.Equal(t, "Berg", doc.FullSurname())
}

func TestExtractDocumentRejectsOtherDocuments(t *testing.T) {
	_, err := ExtractDocument(nil)
	require.ErrorIs(t, err, ErrNotAVog)

	_, err = ExtractDocument([]Word{word("Some", 10, 700), word("letter", 40, 700)})
	require.ErrorIs(t, err, ErrNotAVog)

	// Title present but no fields.
	_, err = ExtractDocument([]Word{word("Verklaring", 67, 771), word("Omtrent", 139, 771), word("het", 197, 771), word("Gedrag", 222, 771)})
	require.ErrorIs(t, err, ErrNotAVog)
}

func TestExtractDocumentRequiresProfileCodes(t *testing.T) {
	var words []Word
	for _, w := range syntheticVog("") {
		if w.Text == "21," || w.Text == "84," || w.Text == "86" {
			continue
		}
		words = append(words, w)
	}
	_, err := ExtractDocument(words)
	require.ErrorIs(t, err, ErrNotAVog)
}

func TestParseDutchDate(t *testing.T) {
	testCases := map[string]time.Time{
		"25 maart 2026":   time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
		"8 mei 2026":      time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC),
		"14 Mei 1991":     time.Date(1991, 5, 14, 0, 0, 0, 0, time.UTC),
		"01-02-2003":      time.Date(2003, 2, 1, 0, 0, 0, 0, time.UTC),
		"2003-02-01":      time.Date(2003, 2, 1, 0, 0, 0, 0, time.UTC),
		"1 augustus 1999": time.Date(1999, 8, 1, 0, 0, 0, 0, time.UTC),
	}
	for input, expected := range testCases {
		got, err := ParseDutchDate(input)
		require.NoError(t, err, input)
		require.Equal(t, expected, got, input)
	}

	for _, invalid := range []string{"", "maart 2026", "31 februari 2026", "25 march 2026", "foo"} {
		_, err := ParseDutchDate(invalid)
		require.Error(t, err, invalid)
	}
}
