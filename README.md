# OpenVoice Walking Skeleton

## Prerequisites
- Go 1.22+
- Node.js + npm

## Build Frontend Assets
```bash
cd web
npm install
npm run build
```

This writes production files to `cmd/server/dist` for Go embedding.

## Run Server
```bash
go run ./cmd/server
```

Server starts on `http://localhost:8080`.

## Verify
- Frontend: `http://localhost:8080`
- Health API: `http://localhost:8080/api/health`
