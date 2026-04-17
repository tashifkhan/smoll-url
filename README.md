# smoll-url

`smoll-url` is a high-performance, self-hosted URL shortener written in Go, with a redesigned Retro-Terminal web interface, SQLite storage, Redis-backed redirect caching, and asynchronous click analytics.

## Features

- **Blazing Fast**: Written in Go with a performance-tuned SQLite backend (WAL mode enabled).
- **Aesthetic UI**: A distinctive "Cyberpunk Brutalist" web interface embedded directly into the binary.
- **Secure**: Argon2id password hashing and API key authentication.
- **Flexible**: Supports custom slugs, automatic expiry (TTL), and hit counting.
- **Production Caching**: Optional Redis cache for hot redirect lookups.
- **Click Analytics Pipeline**: Non-blocking, in-memory queue + background batch writes to SQLite `click_events`.
- **Geo Enrichment (Optional)**: MaxMind City DB lookup for country/city on click events.

## Redirect and Click Flow

The redirect path is intentionally optimized for low latency:

1. User requests `/<slug>`.
2. Server resolves URL (Redis cache first, SQLite fallback).
3. Server returns redirect immediately (`307` or `308` depending on config).
4. Click metadata is pushed to an in-memory queue (non-blocking).
5. Background worker batches events, does optional MaxMind lookup, and writes to SQLite (`click_events`) in WAL mode.

This keeps the user-facing redirect fast while preserving durable analytics.

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
redis_url=redis://localhost:6379/0
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
| `redis_url` | _(empty)_ | Redis connection URL for redirect caching (for example `redis://localhost:6379/0`). |
| `redis_cache_key_prefix` | `smoll-url:redirect:` | Prefix used for Redis cache keys. |
| `redis_cache_timeout_ms` | `200` | Redis operation timeout in milliseconds. |
| `click_queue_size` | `4096` | Buffered in-memory queue size for async click events. |
| `click_batch_size` | `200` | Number of click events written per SQLite batch transaction. |
| `click_flush_interval_ms` | `500` | Flush interval for draining queued click events. |
| `maxmind_db_path` | _(empty)_ | Optional MaxMind City DB (`.mmdb`) path for country/city lookup. |
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
- `redis_url` is optional; when unset, redirect caching is disabled.
- `maxmind_db_path` is optional; when unset, click events are still stored without geo fields.

Click pipeline tuning notes:
- Increase `click_queue_size` if you expect traffic bursts and want fewer dropped events under pressure.
- Increase `click_batch_size` to reduce SQLite write frequency at the cost of slightly slower analytics visibility.
- Lower `click_flush_interval_ms` to persist events sooner, or raise it to reduce write overhead.

### 2.2 Docker Compose

`docker-compose.yml` uses the same app variables from `.env`.

```bash
cp .env.example .env
docker compose up -d --build
```

With the current `docker-compose.yml`, these are the effective service URLs:

- Browser / API from host machine: `http://localhost:${HOST_PORT:-8080}` (default: `http://localhost:8080`)
- App service from another container on the same Compose network: `http://smoll-url:${port:-4567}` (default: `http://smoll-url:4567`)
- Redis service from another container on the same Compose network: `redis://redis:6379/0`

### 3. Execution

Run the binary:

```bash
./smoll-url
```

The server will initialize the SQLite database and start listening on the configured port. Access the web UI at `http://localhost:<port>`.

## Self-Hosting Guide

This section covers practical production deployment patterns for VPS/home-server setups.

### Option A: Run as a Binary + systemd (recommended for minimal setups)

1. Build and place the binary:

```bash
go build -o smoll-url ./cmd/smoll-url
sudo mkdir -p /opt/smoll-url
sudo mv smoll-url /opt/smoll-url/
```

2. Create `.env` at `/opt/smoll-url/.env`:

```dotenv
listen_address=0.0.0.0
port=8080
db_url=/opt/smoll-url/urls.db
password=change-me
api_key=change-me
use_wal_mode=true
```

3. Create `/etc/systemd/system/smoll-url.service`:

```ini
[Unit]
Description=smoll-url URL shortener
After=network.target

[Service]
User=www-data
Group=www-data
WorkingDirectory=/opt/smoll-url
ExecStart=/opt/smoll-url/smoll-url
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

4. Start and enable service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now smoll-url
sudo systemctl status smoll-url
```

### Option B: Run with Docker Compose

1. Copy env template and configure secrets:

```bash
cp .env.example .env
```

2. Use persistent DB path for container mode:

```dotenv
DOCKER_DB_URL=/data/urls.db
```

3. Start container:

```bash
docker compose up -d --build
docker compose logs -f smoll-url
```

`docker-compose.yml` includes a Redis service by default and wires `redis_url=redis://redis:6379/0` for the app container.

If you want geo enrichment in Docker, mount your MaxMind DB into the app container and set `maxmind_db_path` to that mounted path.

### Reverse Proxy (Nginx Proxy Manager)

If using NPM with Docker-network routing:

- Attach `smoll-url` and NPM to the same Docker network.
- In NPM Proxy Host:
  - `Scheme`: `http`
  - `Forward Hostname/IP`: `smoll-url`
  - `Forward Port`: `<port inside container>` (default `4567`, or your configured `port` value)
- Enable WebSocket support in NPM.
- Use Cloudflare SSL mode `Full` (or `Full (strict)` if certs are valid end-to-end).

### Security Checklist

- Change `password` and `api_key` from defaults.
- Keep `.env` out of source control.
- Run behind HTTPS (NPM/Caddy/Nginx/Traefik).
- Restrict server firewall to required ports only (`80/443` for proxy, app internal only).
- Back up SQLite DB file regularly.

### Health and Troubleshooting

- Check app logs:

```bash
docker compose logs -f smoll-url
```

- Confirm container network routing:

```bash
docker inspect smoll-url --format '{{json .NetworkSettings.Networks}}'
```

- Validate upstream from reverse-proxy container:

```bash
docker exec -it nginx-proxy-manager-app-1 sh -c "wget -S -O- http://smoll-url:<port>/"
```

- Common issue: `unable to open database file`
  - Cause: container DB path points to non-writable location.
  - Fix: set `DOCKER_DB_URL=/data/urls.db` and mount `/data` volume.

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

If you changed `HOST_PORT` in `.env`, replace `8080` with your host port.

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
