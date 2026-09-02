package main

import (
	"os"
	"testing"
	"time"

	"go-vog-issuer/vog"

	"github.com/golang-jwt/jwt/v4"
	"github.com/privacybydesign/irmago/irma"
	"github.com/stretchr/testify/require"
)

var testIdentityCredentials = IdentityCredentials{
	Brp:            "irma-demo.gemeente.personalData",
	Passport:       "irma-demo.pbdf.passport",
	IdCard:         "irma-demo.pbdf.idcard",
	DrivingLicence: "irma-demo.pbdf.drivinglicence",
}

func testVogDocument() *vog.Document {
	return &vog.Document{
		ReferenceNumber: "9999012026032500922",
		IssueDate:       time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
		Surname:         "Berg",
		Prefix:          "van der",
		GivenNames:      "Anna Maria",
		DateOfBirth:     time.Date(1980, 2, 3, 0, 0, 0, 0, time.UTC),
		PlaceOfBirth:    "Den Haag",
		CountryOfBirth:  "Nederland",
		Purpose:         "Vrijwilliger bij Sportvereniging",
		ProfileCodes:    []string{"84", "85"},
	}
}

func newTestJwtCreator(t *testing.T) *DefaultJwtCreator {
	t.Helper()
	creator, err := NewIrmaJwtCreator("test-secrets/priv.pem", "vog_issuer", "irma-demo.pbdf.vog", 25, 365*24*time.Hour, testIdentityCredentials)
	require.NoError(t, err)
	return creator
}

func publicKey(t *testing.T) any {
	t.Helper()
	keyBytes, err := os.ReadFile("test-secrets/pub.pem")
	require.NoError(t, err)
	key, err := jwt.ParseRSAPublicKeyFromPEM(keyBytes)
	require.NoError(t, err)
	return key
}

func TestNewIrmaJwtCreatorValidation(t *testing.T) {
	_, err := NewIrmaJwtCreator("test-secrets/does-not-exist.pem", "vog_issuer", "irma-demo.pbdf.vog", 25, time.Hour, testIdentityCredentials)
	require.Error(t, err)

	_, err = NewIrmaJwtCreator("test-secrets/priv.pem", "vog_issuer", "vog", 25, time.Hour, testIdentityCredentials)
	require.Error(t, err)

	broken := testIdentityCredentials
	broken.IdCard = ""
	_, err = NewIrmaJwtCreator("test-secrets/priv.pem", "vog_issuer", "irma-demo.pbdf.vog", 25, time.Hour, broken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "id_card")
}

func TestCreateDisclosureJwt(t *testing.T) {
	creator := newTestJwtCreator(t)
	signed, err := creator.CreateDisclosureJwt()
	require.NoError(t, err)

	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(signed, claims, func(token *jwt.Token) (any, error) { return publicKey(t), nil })
	require.NoError(t, err)
	require.Equal(t, "vog_issuer", claims["iss"])
	require.Equal(t, "verification_request", claims["sub"])

	parsed, err := irma.ParseRequestorJwt("verification_request", signed)
	require.NoError(t, err)
	request := parsed.SessionRequest().(*irma.DisclosureRequest)

	require.Len(t, request.Disclose, 1, "one condiscon entry with four alternatives")
	require.Len(t, request.Disclose[0], 4)
	require.Equal(t, "irma-demo.gemeente.personalData.firstnames", request.Disclose[0][0][0].Type.String())
	require.Equal(t, "irma-demo.gemeente.personalData.dateofbirth", request.Disclose[0][0][3].Type.String())
	require.Equal(t, "irma-demo.pbdf.passport.firstName", request.Disclose[0][1][0].Type.String())
	require.Equal(t, "irma-demo.pbdf.idcard.lastName", request.Disclose[0][2][1].Type.String())
	require.Equal(t, "irma-demo.pbdf.drivinglicence.dateOfBirth", request.Disclose[0][3][2].Type.String())
	require.Equal(t, "Je identiteit", request.Labels[0]["nl"])
}

func TestCreateIssuanceJwt(t *testing.T) {
	creator := newTestJwtCreator(t)
	signed, err := creator.CreateIssuanceJwt(testVogDocument(), SourcePassport)
	require.NoError(t, err)

	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(signed, claims, func(token *jwt.Token) (any, error) { return publicKey(t), nil })
	require.NoError(t, err)
	require.Equal(t, "vog_issuer", claims["iss"])
	require.Equal(t, "issue_request", claims["sub"])

	parsed, err := irma.ParseRequestorJwt("issue_request", signed)
	require.NoError(t, err)
	request := parsed.SessionRequest().(*irma.IssuanceRequest)
	require.Len(t, request.Credentials, 1)

	credential := request.Credentials[0]
	require.Equal(t, "irma-demo.pbdf.vog", credential.CredentialTypeID.String())
	require.Equal(t, uint(25), credential.SdJwtBatchSize)
	require.NotNil(t, credential.Validity)
	validity := time.Time(*credential.Validity)
	require.WithinDuration(t, time.Now().Add(365*24*time.Hour), validity, time.Minute)

	attributes := credential.Attributes
	require.Equal(t, "9999012026032500922", attributes["referenceNumber"])
	require.Equal(t, "2026-03-25", attributes["issueDate"])
	require.Equal(t, "Berg", attributes["surname"])
	require.Equal(t, "van der", attributes["prefix"])
	require.Equal(t, "Anna Maria", attributes["givenNames"])
	require.Equal(t, "1980-02-03", attributes["dateOfBirth"])
	require.Equal(t, "Den Haag", attributes["placeOfBirth"])
	require.Equal(t, "Nederland", attributes["countryOfBirth"])
	require.Equal(t, "Vrijwilliger bij Sportvereniging", attributes["purpose"])
	require.Equal(t, "84, 85", attributes["profileCodes"])
	require.Equal(t, "84: Belast zijn met de zorg voor minderjarigen; 85: Belast zijn met de zorg voor (hulpbehoevende) personen, zoals ouderen en gehandicapten", attributes["profileDescription"])
	require.Equal(t, "Personen", attributes["riskAreas"])
	require.Equal(t, "passport", attributes["identitySource"])
	require.Equal(t, "yes", attributes["aspect84"])
	require.Equal(t, "yes", attributes["aspect85"])
	require.Equal(t, "no", attributes["aspect11"])
	require.Equal(t, "no", attributes["aspect91"])
	require.Len(t, attributes, 13+len(vog.FunctionAspectCodes))
}

func TestCreateIssuanceJwtRequiresDocument(t *testing.T) {
	creator := newTestJwtCreator(t)
	_, err := creator.CreateIssuanceJwt(nil, SourceBrp)
	require.Error(t, err)
}

func TestIdentityCredentialsSourceOf(t *testing.T) {
	require.Equal(t, SourceBrp, testIdentityCredentials.SourceOf("irma-demo.gemeente.personalData"))
	require.Equal(t, SourcePassport, testIdentityCredentials.SourceOf("irma-demo.pbdf.passport"))
	require.Equal(t, SourceIdCard, testIdentityCredentials.SourceOf("irma-demo.pbdf.idcard"))
	require.Equal(t, SourceDrivingLicence, testIdentityCredentials.SourceOf("irma-demo.pbdf.drivinglicence"))
	require.Equal(t, "", testIdentityCredentials.SourceOf("irma-demo.pbdf.email"))
}
