package vog

import (
	"fmt"
	"sort"
	"strings"
)

// FunctionAspect is one "functieaspect" of the general VOG screening profile
// (algemeen screeningsprofiel) as published by Justis. A VOG lists the codes
// of the function aspects that were screened for; this table turns those
// codes into meaningful, human readable attributes.
type FunctionAspect struct {
	// Two digit code as printed on the VOG, e.g. "84".
	Code string
	// Risk area (risicogebied) the aspect belongs to, Dutch.
	RiskArea string
	// Risk area, English.
	RiskAreaEN string
	// Description, Dutch (as printed on the VOG).
	Description string
	// Description, English (as printed on the VOG).
	DescriptionEN string
}

// FunctionAspects contains all function aspects of the general screening
// profile, indexed by code. Source: "Screeningsprofielen VOG - Uitleg voor
// werkgevers" (Justis) and the job feature list printed on every VOG.
var FunctionAspects = map[string]FunctionAspect{
	"11": {"11", "Informatie", "Information", "Bevoegdheid hebben tot het raadplegen en/of bewerken van systemen", "Being authorised to consult and or process data in computer systems"},
	"12": {"12", "Informatie", "Information", "Met gevoelige/vertrouwelijke informatie omgaan", "Handling sensitive/confidential information"},
	"13": {"13", "Informatie", "Information", "Kennis dragen van veiligheidssystemen, controlemechanismen en verificatieprocessen", "Having knowledge of security systems, control mechanisms and verification processes"},
	"21": {"21", "Geld", "Money", "Met contante en/of girale gelden en/of (digitale) waardepapieren omgaan", "Handling cash, transferable money and/or (digital) securities"},
	"22": {"22", "Geld", "Money", "Budgetbevoegdheid hebben", "Having budgetary authority"},
	"36": {"36", "Goederen", "Goods", "Het bewaken van productieprocessen", "Monitoring production processes"},
	"37": {"37", "Goederen", "Goods", "Het beschikken over goederen", "Having access to goods"},
	"38": {"38", "Goederen", "Goods", "Het voorhanden hebben van stoffen, objecten en voorwerpen e.d., die bij oneigenlijk of onjuist gebruik een risico vormen voor mensen (en dier)", "Having access to materials, property, objects etc. that, if used inappropriately or incorrectly, pose a risk to people and/or animals"},
	"41": {"41", "Diensten", "Services", "Het verlenen van diensten (advies, beveiliging, schoonmaak, catering, onderhoud etc.)", "Providing services (advice, security, cleaning, catering, maintenance etc.)"},
	"43": {"43", "Diensten", "Services", "Het verlenen van diensten in de persoonlijke leefomgeving", "Services in individual living environment"},
	"53": {"53", "Zakelijke transacties", "Business transactions", "Het beslissen over offertes (het voeren van onderhandelingen en het afsluiten van contracten) en het doen van aanbestedingen", "Making decisions on offers (conducting negotiations and concluding contracts) and awarding contracts"},
	"61": {"61", "Proces", "Processes", "Het onderhouden/ombouwen/bedienen van (productie)machines en/of apparaten, voertuigen en/of luchtvaartuigen", "Maintaining/converting/operating production or other machinery and/or devices, vehicles and/or aircrafts"},
	"62": {"62", "Proces", "Processes", "(Rijdend) vervoer waarbij goederen, producten, post en pakketten worden getransporteerd en/of bezorgd, anders dan het intern transport binnen een bedrijf", "Transporting and/or delivering goods, post and packages otherwise than via an in-company transport"},
	"63": {"63", "Proces", "Processes", "(Rijdend) vervoer waarbij personen worden vervoerd", "Transporting passengers"},
	"71": {"71", "Aansturen organisatie", "Management", "Personen die vanuit hun functie mensen en/of een organisatie (of een deel daarvan) aansturen", "Managing people and/or (part of) an organisation"},
	"84": {"84", "Personen", "Persons", "Belast zijn met de zorg voor minderjarigen", "Being responsible for the care of minors"},
	"85": {"85", "Personen", "Persons", "Belast zijn met de zorg voor (hulpbehoevende) personen, zoals ouderen en gehandicapten", "Being responsible for the care of persons requiring assistance such as the aged and disabled"},
	"86": {"86", "Personen", "Persons", "Kinderopvang", "Childcare"},
	"91": {"91", "Locatie", "Location", "Haven", "Port area"},
}

// FunctionAspectCodes lists every known function aspect code in ascending
// order. The issued credential carries one yes/no attribute per code, so this
// order also fixes the attribute order.
var FunctionAspectCodes = sortedCodes()

func sortedCodes() []string {
	codes := make([]string, 0, len(FunctionAspects))
	for code := range FunctionAspects {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// SpecificProfile is one of the specific screening profiles (specifieke
// screeningsprofielen) Justis defines for particular professions or purposes.
// They share the two digit code space with the function aspects (e.g. 85 is
// both "zorg voor hulpbehoevende personen" and "Lidmaatschap
// schietvereniging"). The codes printed on a VOG under "profiel" refer to the
// function aspects listed on the back of the VOG; the specific profiles are
// kept here for completeness and for describing codes that are not a function
// aspect.
type SpecificProfile struct {
	Code string
	Name string
}

// SpecificProfiles indexed by code.
var SpecificProfiles = map[string]SpecificProfile{
	"01": {"01", "Politieke ambtsdragers"},
	"06": {"06", "Visum en emigratie"},
	"18": {"18", "Huisvestingsvergunning Wbmgp"},
	"25": {"25", "(Buitengewoon) opsporingsambtenaar"},
	"40": {"40", "Vakantiegezinnen en adoptie"},
	"45": {"45", "Gezondheidszorg en welzijn van mens en dier"},
	"50": {"50", "Exploitatievergunning"},
	"55": {"55", "Juridische dienstverlening"},
	"60": {"60", "Onderwijs"},
	"65": {"65", "Taxibranche; chauffeurskaart"},
	"70": {"70", "Taxibranche; ondernemersvergunning"},
	"75": {"75", "(Gezins)voogd bij voogdij-instellingen, reclasseringswerker, raadsonderzoeker en maatschappelijk werker"},
	"80": {"80", "Beëdigd tolken en vertalers"},
	"85": {"85", "Lidmaatschap schietvereniging"},
	"95": {"95", "Financiële dienstverlening"},
	"97": {"97", "Beveiliging burgerluchtvaart"},
}

// LookupFunctionAspect returns the function aspect for a code.
func LookupFunctionAspect(code string) (FunctionAspect, bool) {
	aspect, ok := FunctionAspects[strings.TrimSpace(code)]
	return aspect, ok
}

// DescribeCode renders a code as "84: Belast zijn met de zorg voor
// minderjarigen". Codes that are not a function aspect but a specific
// screening profile are described as such; unknown codes are marked unknown
// so they are never silently dropped from the credential.
func DescribeCode(code string) string {
	code = strings.TrimSpace(code)
	if aspect, ok := FunctionAspects[code]; ok {
		return fmt.Sprintf("%s: %s", code, aspect.Description)
	}
	if profile, ok := SpecificProfiles[code]; ok {
		return fmt.Sprintf("%s: %s (specifiek screeningsprofiel)", code, profile.Name)
	}
	return fmt.Sprintf("%s: Onbekend functieaspect", code)
}

// DescribeCodeEN is the English counterpart of DescribeCode.
func DescribeCodeEN(code string) string {
	code = strings.TrimSpace(code)
	if aspect, ok := FunctionAspects[code]; ok {
		return fmt.Sprintf("%s: %s", code, aspect.DescriptionEN)
	}
	if profile, ok := SpecificProfiles[code]; ok {
		return fmt.Sprintf("%s: %s (specific screening profile)", code, profile.Name)
	}
	return fmt.Sprintf("%s: Unknown job feature", code)
}

// DescribeCodes joins the descriptions of all codes with "; ".
func DescribeCodes(codes []string) string {
	parts := make([]string, 0, len(codes))
	for _, code := range codes {
		parts = append(parts, DescribeCode(code))
	}
	return strings.Join(parts, "; ")
}

// RiskAreas returns the distinct Dutch risk areas covered by the codes, in the
// order of the codes on the VOG. Codes that are not a function aspect are
// skipped.
func RiskAreas(codes []string) []string {
	seen := map[string]bool{}
	var areas []string
	for _, code := range codes {
		aspect, ok := FunctionAspects[strings.TrimSpace(code)]
		if !ok || seen[aspect.RiskArea] {
			continue
		}
		seen[aspect.RiskArea] = true
		areas = append(areas, aspect.RiskArea)
	}
	return areas
}
