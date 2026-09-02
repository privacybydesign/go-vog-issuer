# React frontend

This directory contains the React + TypeScript frontend of the VOG issuer, built with Vite.

The flow has three pages:

1. `/nl/upload` – upload the VOG PDF; the backend validates it with validatie.nl and shows what it read.
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
