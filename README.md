# smoll-url

`smoll-url` is a high-performance, self-hosted URL shortener written in Go, with a redesigned Retro-Terminal web interface and a robust SQLite backend.



## Features

- **Blazing Fast**: Written in Go with a performance-tuned SQLite backend (WAL mode enabled).
- **Aesthetic UI**: A distinctive "Cyberpunk Brutalist" web interface embedded directly into the binary.
- **Secure**: Argon2id password hashing and API key authentication.
- **Flexible**: Supports custom slugs, automatic expiry (TTL), and hit counting.
- **Zero Dependencies**: Single binary deployment with everything (including frontend) bundled via `//go:embed`.

## Quick Start

### 1. Installation

Ensure you have Go 1.21+ installed, then clone and build:

```bash
git clone https://github.com/taf/smoll-url.git
cd smoll-url
go build ./cmd/smoll-url
```

### 2. Configuration

`smoll-url` uses environment variables only.

For local/dev usage, copy and edit `.env.example`:

```bash
cp .env.example .env
```

Example `.env`:

```dotenv
listen_address=0.0.0.0
port=4567
db_url=urls.sqlite
password=your-admin-password
api_key=your-secret-api-key
slug_length=6
use_wal_mode=true
```

### 3. Execution

Run the binary:

```bash
./smoll-url
```

The server will initialize the SQLite database and start listening on the configured port. Access the web UI at `http://localhost:<port>`.

## API Usage

You can interact with `smoll-url` via the web UI or directly through the REST API.

### Create a Short URL

**Endpoint**: `POST /api/new`
**Auth**: Pass `X-API-KEY` header or a valid session cookie.

```bash
curl -X POST http://localhost:8080/api/new \
  -H "X-API-KEY: your-secret-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "longlink": "https://github.com/taf/smoll-url",
    "shortlink": "repo",
    "expiry_delay": 3600
  }'
```

### Authentication

- **Login**: `POST /api/login` with the password in the request body.
- **Logout**: `DELETE /api/logout`.

## Development

The project structure is organized as follows:

- `cmd/smoll-url/`: Entry point and CLI logic.
- `internal/server/`: HTTP server, API routes, and embedded frontend.
- `internal/store/`: SQLite storage layer using `modernc.org/sqlite`.
- `internal/auth/`: Argon2id password hashing and session management.
- `internal/slug/`: Random slug generation logic.

To modify the frontend, edit the files in `internal/server/web/`. The Go compiler will automatically bundle them into the binary on the next build.

## License

MIT
