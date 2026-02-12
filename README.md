# OpenVoice

## Prerequisites
- Go 1.22+
- Node.js + npm

## Build Frontend Assets
```bash
cd web
npm install
npm run build
```

## Run Server
```bash
go run ./cmd/server
```

(Also supported: `go run main.go`.)

Server starts on `http://localhost:8080`.

## API Endpoints
- `GET /api/health`
- `POST /api/register`
- `POST /api/login`
- `POST /api/logout`
- `GET /api/me`
- `GET /api/channels` (auth required)
- `POST /api/channels` (auth required)
- `GET /api/ws` (auth required, WebSocket)
