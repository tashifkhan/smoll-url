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

### 2.1 Configuration Reference

The app loads `.env` from the current directory by default. You can point to a different env file with `env_file`.

| Variable | Default | Description |
|---|---|---|
| `listen_address` | `0.0.0.0` | Interface/address to bind the HTTP server. |
| `port` | `4567` | HTTP port for the server. |
| `db_url` | `urls.sqlite` | SQLite database file path. |
| `database` | _(unset)_ | Alias for `db_url` (supported for compatibility). |
| `db_path` | _(unset)_ | Alias for `db_url` (supported for compatibility). |
| `password` | _(empty)_ | Admin password for web/session login. |
| `api_key` | _(empty)_ | API key for header auth via `X-API-Key`. |
| `hash_algorithm` | _(empty)_ | Set to `Argon2` to treat `password` and/or `api_key` as Argon2 hashes. |
| `slug_style` | `Pair` | Slug generation style: `Pair` or `UID`. |
| `slug_length` | `8` | Length used for `UID` slug generation (minimum enforced: `4`). |
| `allow_capital_letters` | `false` | Allow uppercase characters in generated/validated slugs. |
| `try_longer_slug` | `false` | If `slug_style=UID`, retry conflicts with a longer slug. |
| `public_mode` | `false` | Allow unauthenticated short-link creation. |
| `public_mode_expiry_delay` | `0` | Max/forced expiry for public mode (seconds). `0` means unlimited. |
| `use_temp_redirect` | `false` | Use temporary redirects (`307`) instead of permanent (`308`). |
| `redirect_method` | _(unset)_ | Legacy override: `TEMPORARY` forces temporary redirect mode. |
| `use_wal_mode` | `false` | Enable SQLite WAL mode. |
| `ensure_acid` | `true` | Keep strict SQLite durability/sync settings. |
| `disable_frontend` | `false` | Disable embedded admin frontend. |
| `site_url` | _(empty)_ | Canonical base URL for generated absolute short URLs. |
| `cache_control_header` | _(empty)_ | Optional `Cache-Control` response header value. |
| `custom_landing_directory` | _(empty)_ | Serve files from a custom landing directory. |
| `frontend_page_size` | `10` | Default page size for frontend list pagination APIs. |
| `env_file` | `.env` | Path to env file loaded at startup. |

Notes:
- Boolean values accept `true/false`, `1/0`, `yes/no`, `on/off`, `enable/disable`.
- Values already present in the process environment take precedence over `.env` values.
- If `password` and `api_key` are both empty and `public_mode=false`, protected endpoints require login and will reject unauthenticated access.
- For Docker Compose, set `DOCKER_DB_URL=/data/urls.db` (included in `.env.example`) so SQLite writes to the mounted volume.

### 2.2 Docker Compose

`docker-compose.yml` uses the same app variables from `.env`.

```bash
cp .env.example .env
docker compose up -d --build
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
