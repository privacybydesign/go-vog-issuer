# React frontend

This directory contains the React + TypeScript frontend of the VOG issuer, built with Vite.

The flow has three pages:

1. `/nl/upload` – upload the VOG PDF; the backend validates it with validatie.nl and shows what it read. When validatie.nl is unavailable (HTTP 503 from the backend) the page says so, counts down to an automatic retry (three retries after 5, 10 and 20 seconds, see `VALIDATION_RETRY_DELAYS_MS` in `src/api.ts`) and lets the user stop retrying; a rejected document is never retried.
2. `/nl/verify` – prove your identity in the Yivi app (BRP, passport, ID card or driving licence); the backend compares it with the VOG and, on a match, returns the issuance request that is handed to the Yivi app.
3. `/nl/done` – the VOG credential is in the app.

## Development

```
npm install
npm run dev
```

The dev server proxies `/api` to the backend on `http://localhost:8080`.

## Tests

```
npm test
```

## Build

```
npm run build
```
