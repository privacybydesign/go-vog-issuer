// Package identity compares the person named on a VOG with the identity a user
// disclosed through Yivi.
package identity

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Person is the identity to compare, as printed on the VOG or as disclosed.
type Person struct {
	// Given names / first names, space separated.
	GivenNames string
	// Surname without prefix (geslachtsnaam). For sources that do not split
	// prefix and surname (passport MRZ, driving licence) put the full last
	// name here and leave Prefix empty.
	Surname string
	// Surname prefix (tussenvoegsel), e.g. "van der". Optional.
	Prefix string
	// Date of birth in any of the formats accepted by ParseDate.
	DateOfBirth string
}

// Result explains the outcome of a comparison.
type Result struct {
	Matched          bool     `json:"matched"`
	DateOfBirthMatch bool     `json:"date_of_birth_match"`
	SurnameMatch     bool     `json:"surname_match"`
	GivenNamesMatch  bool     `json:"given_names_match"`
	Reasons          []string `json:"reasons,omitempty"`
}

// Match compares the person on the VOG with the disclosed person. All three of
// date of birth, surname and given names must match. The comparison is
// tolerant of the differences between identity sources:
//
//   - names are compared case-insensitively and without diacritics, because
//     passports and driving licences carry transliterated upper case names;
//   - the surname matches with or without prefix in either input, because the
//     BRP splits "van der Berg" into prefix and family name while travel
//     documents print it as one field;
//   - given names match when the first given name is equal, or when all given
//     names on the VOG appear among the disclosed ones, because documents may
//     abbreviate or omit later given names.
func Match(vog Person, disclosed Person) Result {
	result := Result{}

	vogDob, err := ParseDate(vog.DateOfBirth)
	if err != nil {
		result.Reasons = append(result.Reasons, fmt.Sprintf("VOG date of birth unreadable: %v", err))
	}
	disclosedDob, err := ParseDate(disclosed.DateOfBirth)
	if err != nil {
		result.Reasons = append(result.Reasons, fmt.Sprintf("disclosed date of birth unreadable: %v", err))
	}
	if !vogDob.IsZero() && !disclosedDob.IsZero() {
		result.DateOfBirthMatch = vogDob.Equal(disclosedDob)
		if !result.DateOfBirthMatch {
			result.Reasons = append(result.Reasons, "date of birth differs")
		}
	}

	result.SurnameMatch = surnamesMatch(vog, disclosed)
	if !result.SurnameMatch {
		result.Reasons = append(result.Reasons, "surname differs")
	}

	result.GivenNamesMatch = givenNamesMatch(vog.GivenNames, disclosed.GivenNames)
	if !result.GivenNamesMatch {
		result.Reasons = append(result.Reasons, "given names differ")
	}

	result.Matched = result.DateOfBirthMatch && result.SurnameMatch && result.GivenNamesMatch
	return result
}

func surnamesMatch(a, b Person) bool {
	for _, x := range surnameVariants(a) {
		for _, y := range surnameVariants(b) {
			if x != "" && x == y {
				return true
			}
		}
	}
	return false
}

// surnameVariants returns the normalised surname with and without prefix, and
// with the prefix trailing (as some MRZ transliterations do).
func surnameVariants(p Person) []string {
	surname := Normalize(p.Surname)
	prefix := Normalize(p.Prefix)
	variants := []string{surname}
	if prefix != "" {
		variants = append(variants, prefix+" "+surname, surname+" "+prefix)
	}
	return variants
}

func givenNamesMatch(vogNames, disclosedNames string) bool {
	vogTokens := strings.Fields(Normalize(vogNames))
	disclosedTokens := strings.Fields(Normalize(disclosedNames))
	if len(vogTokens) == 0 || len(disclosedTokens) == 0 {
		return false
	}
	if vogTokens[0] == disclosedTokens[0] {
		return true
	}
	disclosedSet := map[string]bool{}
	for _, t := range disclosedTokens {
		disclosedSet[t] = true
	}
	for _, t := range vogTokens {
		if !disclosedSet[t] {
			return false
		}
	}
	return true
}

// Normalize upper-cases, strips diacritics, turns punctuation into spaces and
// collapses whitespace so "Müller-Lüdenscheidt" and "MULLER LUDENSCHEIDT"
// compare equal.
func Normalize(s string) string {
	decomposed := norm.NFD.String(s)
	var b strings.Builder
	for _, r := range decomposed {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToUpper(r))
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

var dateLayouts = []string{
	"2006-01-02",
	"02-01-2006",
	"2-1-2006",
	"02/01/2006",
	"20060102",
	"2 January 2006",
	"02 January 2006",
	"2006-01-02T15:04:05Z07:00",
}

var dutchMonths = map[string]string{
	"januari": "January", "februari": "February", "maart": "March", "april": "April",
	"mei": "May", "juni": "June", "juli": "July", "augustus": "August",
	"september": "September", "oktober": "October", "november": "November", "december": "December",
}

// ParseDate accepts the date formats used by the VOG ("14 mei 1991"), the BRP
// credential ("14-05-1991") and the travel document credentials
// ("1991-05-14") and returns the calendar date in UTC.
func ParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	candidate := s
	parts := strings.Fields(strings.ToLower(s))
	if len(parts) == 3 {
		if month, ok := dutchMonths[parts[1]]; ok {
			candidate = parts[0] + " " + month + " " + parts[2]
		}
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, candidate); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised date %q", s)
}
