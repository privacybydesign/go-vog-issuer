package vog

import (
	"slices"
	"strings"
	"time"
)

// Document holds the data printed on a Verklaring Omtrent het Gedrag (VOG,
// certificate of conduct) issued by Justis.
type Document struct {
	// "Ons kenmerk": the unique reference number of the VOG.
	ReferenceNumber string `json:"reference_number"`
	// "Datum": the date the VOG was issued.
	IssueDate time.Time `json:"issue_date"`
	// "Geslachtsnaam": surname without prefix.
	Surname string `json:"surname"`
	// "Tussenvoegsels": surname prefix such as "van der"; may be empty.
	Prefix string `json:"prefix"`
	// "Voorna(a)m(en)": all given names, space separated.
	GivenNames string `json:"given_names"`
	// "Geboortedatum".
	DateOfBirth time.Time `json:"date_of_birth"`
	// "Geboorteplaats".
	PlaceOfBirth string `json:"place_of_birth"`
	// "Geboorteland".
	CountryOfBirth string `json:"country_of_birth"`
	// The function or purpose the VOG was requested for ("Hierbij geef ik u
	// de VOG die u nodig heeft voor: ...").
	Purpose string `json:"purpose"`
	// The screening profile codes ("Er is bij deze screening uitgegaan van het
	// volgende profiel: 84, 85"), in the order printed.
	ProfileCodes []string `json:"profile_codes"`
}

// FullSurname is the surname including its prefix, e.g. "van der Berg".
func (d *Document) FullSurname() string {
	if d.Prefix == "" {
		return d.Surname
	}
	return d.Prefix + " " + d.Surname
}

// HasProfileCode reports whether the VOG was screened for the given code.
func (d *Document) HasProfileCode(code string) bool {
	return slices.Contains(d.ProfileCodes, strings.TrimSpace(code))
}

// ProfileCodesString renders the codes as printed on the VOG: "84, 85".
func (d *Document) ProfileCodesString() string {
	return strings.Join(d.ProfileCodes, ", ")
}
