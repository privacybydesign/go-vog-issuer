package vog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDescribeCode(t *testing.T) {
	require.Equal(t, "84: Belast zijn met de zorg voor minderjarigen", DescribeCode("84"))
	require.Equal(t, "85: Belast zijn met de zorg voor (hulpbehoevende) personen, zoals ouderen en gehandicapten", DescribeCode(" 85 "))
	require.Equal(t, "84: Being responsible for the care of minors", DescribeCodeEN("84"))
	// A code that is only a specific screening profile.
	require.Equal(t, "60: Onderwijs (specifiek screeningsprofiel)", DescribeCode("60"))
	require.Equal(t, "60: Onderwijs (specific screening profile)", DescribeCodeEN("60"))
	// Unknown codes are kept, marked unknown.
	require.Equal(t, "99: Onbekend functieaspect", DescribeCode("99"))
	require.Equal(t, "99: Unknown job feature", DescribeCodeEN("99"))
}

func TestDescribeCodes(t *testing.T) {
	require.Equal(t,
		"84: Belast zijn met de zorg voor minderjarigen; 85: Belast zijn met de zorg voor (hulpbehoevende) personen, zoals ouderen en gehandicapten",
		DescribeCodes([]string{"84", "85"}))
	require.Equal(t, "", DescribeCodes(nil))
}

func TestRiskAreas(t *testing.T) {
	require.Equal(t, []string{"Informatie", "Personen"}, RiskAreas([]string{"11", "12", "84", "85"}))
	require.Equal(t, []string{"Personen"}, RiskAreas([]string{"84", "99"}))
	require.Empty(t, RiskAreas(nil))
}

func TestFunctionAspectCodesSortedAndComplete(t *testing.T) {
	require.Equal(t, []string{"11", "12", "13", "21", "22", "36", "37", "38", "41", "43", "53", "61", "62", "63", "71", "84", "85", "86", "91"}, FunctionAspectCodes)
	for _, code := range FunctionAspectCodes {
		aspect, ok := LookupFunctionAspect(code)
		require.True(t, ok)
		require.Equal(t, code, aspect.Code)
		require.NotEmpty(t, aspect.RiskArea)
		require.NotEmpty(t, aspect.Description)
		require.NotEmpty(t, aspect.DescriptionEN)
	}
}
