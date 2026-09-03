# React frontend

This directory contains the React + TypeScript frontend of the VOG issuer, built with Vite.

The flow has four pages:

1. `/nl/upload` – upload the VOG PDF; the backend validates it with validatie.nl and shows what it read. When validatie.nl is unavailable (HTTP 503 from the backend) the page says so, counts down to an automatic retry (three retries after 5, 10 and 20 seconds, see `VALIDATION_RETRY_DELAYS_MS` in `src/api.ts`) and lets the user stop retrying; a rejected document is never retried.
2. `/nl/verify` – prove your identity in the Yivi app (BRP, passport, ID card or driving licence); the backend compares it with the VOG.
3. `/nl/result` – says whether the identity matched. On a match it explains that the VOG can now be added to the Yivi app as a card and hands the signed issuance request (kept in the React context, the backend has already consumed the session) to the app on request. On a mismatch it lists what differed and offers another attempt with a different credential.
4. `/nl/done` – the VOG credential is in the app.

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
