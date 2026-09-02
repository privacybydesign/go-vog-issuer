# Go VOG Issuer

The Go VOG Issuer turns a **Verklaring Omtrent het Gedrag** (VOG, the Dutch certificate of conduct issued by Justis) into a privacy-preserving [Yivi](https://yivi.app) credential. It follows the architecture of the [go-passport-issuer](https://github.com/privacybydesign/go-passport-issuer): a Go backend with a ReDoc-documented API, a React frontend, Redis or in-memory session storage and Docker packaging. Credentials are issued over the IRMA protocol in both the IRMA (Idemix) and SD-JWT VC formats.

## How it works

1. **Upload.** The holder uploads the digital VOG PDF received from Justis. The backend sends it to the GAAV validation service of the Justitiële Informatiedienst ([validatie.nl](https://validatie.nl)) which confirms that the PDF is authentic and unaltered. Only then is the PDF parsed: reference number, issue date, name, date and place of birth, purpose and the screening profile codes are read from the (AES encrypted) PDF with PDFium running in WebAssembly, so the backend is pure Go.
2. **Identity.** The holder proves who they are in the Yivi app. The disclosure request offers four alternatives, the app lets the user pick: BRP personal data (`gemeente.personalData`), passport, ID card or driving licence. The backend runs this session itself so it can read the result.
3. **Match.** The disclosed name and date of birth are compared with the person named on the VOG (case- and diacritic-insensitive, prefix with or without, first given name suffices). No match, no credential; the holder may disclose again with another credential.
4. **Issue.** On a match the backend signs an IRMA issuance request for the VOG credential and the frontend hands it to the Yivi app.

The uploaded PDF and the parsed data live in the session store for at most one hour and are removed as soon as the credential is issued.

### Screening profile codes

A VOG lists the codes of the function aspects (functieaspecten) of the general screening profile that were screened for, e.g. `84, 85`. The issuer maps them to meaningful attributes using the Justis publication *Screeningsprofielen VOG – Uitleg voor werkgevers* (see `backend/vog/profiles.go`):

| Code | Risk area | Description |
|------|-----------|-------------|
| 11 | Informatie | Bevoegdheid hebben tot het raadplegen en/of bewerken van systemen |
| 12 | Informatie | Met gevoelige/vertrouwelijke informatie omgaan |
| 13 | Informatie | Kennis dragen van veiligheidssystemen, controlemechanismen en verificatieprocessen |
| 21 | Geld | Met contante en/of girale gelden en/of (digitale) waardepapieren omgaan |
| 22 | Geld | Budgetbevoegdheid hebben |
| 36 | Goederen | Het bewaken van productieprocessen |
| 37 | Goederen | Het beschikken over goederen |
| 38 | Goederen | Het voorhanden hebben van stoffen, objecten en voorwerpen die een risico vormen voor mensen (en dier) |
| 41 | Diensten | Het verlenen van diensten (advies, beveiliging, schoonmaak, catering, onderhoud etc.) |
| 43 | Diensten | Het verlenen van diensten in de persoonlijke leefomgeving |
| 53 | Zakelijke transacties | Het beslissen over offertes en het doen van aanbestedingen |
| 61 | Proces | Het onderhouden/ombouwen/bedienen van (productie)machines, apparaten, voertuigen en/of luchtvaartuigen |
| 62 | Proces | (Rijdend) vervoer van goederen, producten, post en pakketten |
| 63 | Proces | (Rijdend) vervoer waarbij personen worden vervoerd |
| 71 | Aansturen organisatie | Personen die mensen en/of een organisatie aansturen |
| 84 | Personen | Belast zijn met de zorg voor minderjarigen |
| 85 | Personen | Belast zijn met de zorg voor (hulpbehoevende) personen, zoals ouderen en gehandicapten |
| 86 | Personen | Kinderopvang |
| 91 | Locatie | Haven |

The credential carries the codes (`profileCodes`), the full descriptions (`profileDescription`, e.g. `84: Belast zijn met de zorg voor minderjarigen; 85: ...`), the risk areas (`riskAreas`) and **one yes/no attribute per code** (`aspect11` … `aspect91`), so a verifier can ask for exactly the aspect it cares about — "screened for the care of minors?" — without learning the rest of the profile. The specific screening profiles (01 Politieke ambtsdragers … 97 Beveiliging burgerluchtvaart) are known as well and used to describe codes that are not a function aspect.

### Credential

The VOG credential type is not part of the Yivi scheme yet. A proposed `description.xml` with all attributes (bilingual names and descriptions) is in [`docs/scheme/vog/description.xml`](docs/scheme/vog/description.xml); the attribute names must stay in sync with `VogAttributes` in `backend/jwt_creator.go`.

| Attribute | Value |
|-----------|-------|
| `referenceNumber` | Kenmerk of the VOG |
| `issueDate` | YYYY-MM-DD |
| `surname`, `prefix`, `givenNames` | As printed on the VOG |
| `dateOfBirth` | YYYY-MM-DD |
| `placeOfBirth`, `countryOfBirth` | As printed on the VOG |
| `purpose` | Function or purpose the VOG was requested for |
| `profileCodes` | e.g. `84, 85` |
| `profileDescription` | e.g. `84: Belast zijn met de zorg voor minderjarigen; 85: …` |
| `riskAreas` | e.g. `Personen` |
| `identitySource` | `brp`, `passport`, `id_card` or `driving_licence` |
| `aspect11` … `aspect91` | `yes` / `no` per function aspect |

### validatie.nl response codes

The GAAV API (`POST https://validatie.nl/api/valideer/`, multipart field `file`) answers `{"response_code": n}`. The table in the published API specification is garbled; the mapping below follows the order of the descriptions in that table and was confirmed against the live service (a genuine VOG yields `0`, an arbitrary PDF `2`). Note that the multipart part **must** declare `Content-Type: application/pdf`; with `application/octet-stream` the service answers `2` for a genuine VOG.

| Code | Meaning | Issuer response |
|------|---------|-----------------|
| 0 | Document is authentiek en integer | continue |
| 1 | Document is bekend, maar niet integer | 422 `error:validation-failed` |
| 2 | Document is niet bekend | 422 `error:validation-failed` |
| 3 | Validatie niet mogelijk, probeer nogmaals | 503 `error:validation-failed` (retryable) |
| 4 | Foutmelding van provenance server | 503 `error:validation-failed` (retryable) |
| 5 | Foutmelding van signature validation server | 503 `error:validation-failed` (retryable) |
| 6 | Handtekening is ongeldig | 422 `error:validation-failed` |
| 7 | Provenance store gaf fouten terug | 503 `error:validation-failed` (retryable) |

## Getting started

### Prerequisites

- **Go** 1.27 or later (see `backend/go.mod`; the Go toolchain downloads it automatically)
- **Node.js** 24 or later (frontend)

No C toolchain or system libraries are needed: PDF parsing uses PDFium compiled to WebAssembly.

### Configuration

Create `local-secrets/config.json` (the folder is git-ignored); `config.sample.json` is a complete example:

```json
{
  "server_config": {
    "host": "0.0.0.0",
    "port": 8080,
    "enable_api_docs": true
  },
  "irma_server_url": "https://is.staging.yivi.app",
  "issuer_id": "vog_issuer",
  "jwt_private_key_path": "/secrets/priv.pem",
  "sd_jwt_batch_size": 25,
  "credential_validity_days": 365,
  "credentials": {
    "vog": { "full_credential": "pbdf-staging.pbdf.vog" }
  },
  "identity_credentials": {
    "brp": "pbdf-staging.gemeente.personalData",
    "passport": "pbdf-staging.pbdf.passport",
    "id_card": "pbdf-staging.pbdf.idcard",
    "driving_licence": "pbdf-staging.pbdf.drivinglicence"
  },
  "validation": {
    "url": "https://validatie.nl/api/valideer/",
    "timeout_seconds": 30
  },
  "max_upload_size_bytes": 5242880,
  "storage_type": "memory",
  "log_level": "info"
}
```

- `jwt_private_key_path` points to the RSA private key (PEM) that signs the session requests. The IRMA server must know the matching public key under the requestor name `issuer_id`, and that requestor must be allowed to **issue** the VOG credential and to **verify** the four identity credentials. The backend starts the disclosure session itself, so `irma_server_url` must be reachable from the backend as well as from the Yivi app.
- `identity_credentials` are the full credential type identifiers of the identity credentials; their attribute names (`firstnames`/`prefix`/`familyname`/`dateofbirth` for BRP, `firstName`/`lastName`/`dateOfBirth` for the documents) are fixed by the scheme. `passport`, `id_card` and `driving_licence` are required. `brp` is optional: leave the key out and the disclosure request only offers the three documents, and a disclosed BRP credential is not accepted as an identity.
- `storage_type` is `memory`, `redis` (with `redis_config`) or `redis_sentinel` (with `redis_sentinel_config`). Use Redis when running more than one instance.
- `sd_jwt_batch_size` is the number of SD-JWT VCs issued alongside the IRMA credential.

### Running the application

**Backend**

```bash
cd backend
go run . --config ../local-secrets/config.json
```

**Frontend** (development server on port 3000, proxies `/api` to the backend):

```bash
cd frontend
npm install
npm run dev
```

**Tests**

```bash
cd backend && go test ./...
cd frontend && npm test
```

The backend tests parse the two VOG PDFs in `backend/test-data` and run the HTTP flow against fake validation, PDF and IRMA server implementations; the real validatie.nl endpoint is not called by the tests.

### Docker

```bash
docker-compose up --build
```

This builds the frontend and backend into one image and starts it together with Redis. Mount `local-secrets` at `/secrets` (the compose file does).

### API documentation

The backend serves ReDoc at `/api/docs` when `enable_api_docs` is `true`. The OpenAPI specification is generated from the swag annotations in `backend/main.go`, `backend/server.go` and `backend/models`:

```bash
go install github.com/swaggo/swag/cmd/swag@latest
cd backend
go generate ./...
```

### API

| Endpoint | Purpose |
|----------|---------|
| `POST /api/vog/upload` (multipart `file`) | Validate with validatie.nl and parse the VOG; returns a `session_id`, the validation outcome and the parsed document. |
| `POST /api/vog/start-disclosure` `{session_id}` | Start the identity disclosure; returns the IRMA session package (`sessionPtr`, `frontendRequest`) for yivi-frontend. |
| `POST /api/vog/issue` `{session_id}` | Fetch the disclosure result, compare it with the VOG and return the signed issuance JWT plus `irma_server_url`. `403 error:identity-mismatch` explains which of date of birth, surname and given names differed. |
| `GET /api/health` | Health check. |

Errors are JSON: `{"error": "error:<key>", "message": "...", "validation": {...}, "identity": {...}}`.

## Repository layout

```
backend/            Go backend
  vog/              VOG PDF parser (PDFium/WebAssembly), validatie.nl client, screening profile table
  identity/         Name and date of birth comparison
  models/           API request/response models (swag annotated)
  docs/             Generated OpenAPI spec and ReDoc page
  test-data/        Two genuine VOG PDFs used by the parser tests
frontend/           React + Vite frontend (nl/en)
docs/scheme/vog/    Proposed credential type for the Yivi scheme
```

## Funding

This project builds on the Yivi issuers of the [Privacy by Design Foundation](https://privacybydesign.foundation).
