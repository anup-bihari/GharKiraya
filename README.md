# GharKiraya

A working rental marketplace for students, interns, and landlords. A Go HTTP server serves both the API and an embedded, responsive JavaScript frontend. No npm installation or external Go modules are required.

## Run on Windows

Requires Go 1.25 or later. A project-local compiler, if present in `.tools/go/`, is used automatically.

```powershell
# Explore sample homes and switch between tenant/landlord test accounts.
powershell -ExecutionPolicy Bypass -File .\run.ps1 -Demo

# Start with empty, real-user data; demo sign-in is disabled.
powershell -ExecutionPolicy Bypass -File .\run.ps1
```

Open http://localhost:8080. Stop the server with Ctrl+C. Demo data uses `data/demo.json`; normal data uses `data/state.json`. The sample home descriptions and photos are illustrative advertisements, not real available inventory.

On macOS/Linux:

```sh
DEMO_MODE=true DATA_FILE=data/demo.json go run .
# Or start with an empty production database:
go run .
```

## Try the full journey

1. Sign in → **Explore as tenant**. Open a home, click **I’m interested**, and send an introduction.
2. Sign out from the avatar menu. Sign in → **Explore as landlord**. Open **My interest requests**, and accept the request.
3. Open the conversation and send a message. Switch back to the tenant to see the reply. Separate browser profiles can also run both accounts simultaneously. Chats refresh every four seconds while open.
4. Either participant can open **Agreement**, set dates, rent, deposit, and terms, then create a shared draft. Both participants can acknowledge review and download a text copy. A draft is immutable after creation, so agree on details in chat first.
5. Landlords can **List a property**. Tenants can **Post a requirement**, visible on the tenant board. A landlord responds to a requirement by publishing a matching home; the tenant sends an interest request to start the same acceptance flow.

The normal registration and email/password login flows work in both modes. Test accounts have random passwords and can only be accessed through the explicitly enabled demo endpoint.

## Included

- Tenant and landlord registration, login, logout, and persistent sessions.
- Search by city, maximum rent, room type, and minimum stay, plus price/newest sorting.
- Property advertisement forms with price, deposit, dates, furnishing, amenities, and photo URL.
- Public short-term accommodation requirements with budget, dates, and stay length.
- Account-persisted saved homes.
- Tenant interest requests; only the owning landlord can accept or decline.
- Private chat history, gated by acceptance and participant authorization on every request.
- Shared rental agreement drafts, bilateral review acknowledgements, and text downloads.
- Responsive desktop/mobile layouts, accessible dialogs, keyboard focus trapping, escaped user content, and form error messages.
- Atomic file persistence and rollback if a write fails, along with a health endpoint.

## Architecture and data

`main.go` contains the HTTP server, API, session/auth logic, authorization, validation, and JSON store. `web/` contains the frontend, embedded into the Go binary at build time. `main_test.go` exercises complete workflows and authorization boundaries.

The store serializes writes with a mutex, writes to a temporary file, syncs it, and renames it atomically. Requests and chats survive process restarts. This is deliberately a **single-process MVP**, suitable for running and demonstrating the complete product. Do not run multiple replicas against the same data file. Move to PostgreSQL and migrations for multi-instance deployment, larger datasets, and operational backups. Back up the data file while the process is stopped, or use a filesystem snapshot.

Passwords use PBKDF2-HMAC-SHA256 with random salts and 600,000 iterations. Session tokens are cryptographically random, stored hashed, expire after seven days, and are delivered through HttpOnly/SameSite cookies. The API rejects cross-origin mutations, rate limits login and registration by client IP, constrains input sizes, and checks request/chat/agreement ownership. CSP restricts scripts and external content.

The app uses Google Fonts and Unsplash photos. System fonts work if Google Fonts is unavailable. Property photos are optional HTTPS `images.unsplash.com` URLs; uploads are not included.

## Configuration

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `ADDR` | `127.0.0.1:8080` | HTTP listening address |
| `DATA_FILE` | `data/state.json` | Persistent JSON state file |
| `DEMO_MODE` | `false` | Enable test-account sign-in and sample seed data on first creation |
| `COOKIE_SECURE` | `false` | Set `true` when serving over HTTPS |

`DEMO_MODE` only seeds a new data file. Do not reuse a demo file for production. The PowerShell launcher separates them automatically. Restart the server to change these settings.

## Test and build

```sh
go test ./...
go test -race ./...   # Requires a supported C compiler for the race runtime.
go vet ./...
go build -o gharkiraya .
node --check web/app.js
```

On this Windows workspace, the portable compiler is `.tools/go/bin/go.exe`. Set `GOCACHE` to `.tools/cache` and `GOPATH` to `.tools/gopath` if using it directly. Files under `web/` are compiled into the binary; restart `go run .` after frontend changes.

## Docker and deployment

```sh
docker compose up --build -d
```

The container runs as an unprivileged user and persists data in a named volume. The default port mapping binds to the local machine. To serve real users, put the Go application behind an HTTPS reverse proxy, set `COOKIE_SECURE=true`, disable demo mode, use a fresh persistent data file, and configure backups. Configure the proxy’s request limits and per-client rate limiting; the application intentionally does not trust arbitrary forwarded IP headers.

No production deployment is included. This project keeps the requested native Go backend; Sites hosting runs a different backend runtime.

## API

All request/response bodies are JSON. Writes expect `Content-Type: application/json`. Authentication uses the `gk_session` cookie.

| Method | Endpoint | Access |
| --- | --- | --- |
| GET | `/api/health` | Public |
| GET | `/api/bootstrap` | Public data and current session |
| POST | `/api/auth/register`, `/api/auth/login`, `/api/auth/logout` | Registration/session |
| POST | `/api/auth/demo` | Explicit demo mode only |
| GET | `/api/me` | Signed in |
| GET, POST | `/api/listings` | Read public; write landlord |
| POST | `/api/listings/{id}/save` | Signed in; toggles saved state |
| GET, POST | `/api/needs` | Read public; write tenant |
| GET, POST | `/api/requests` | Own requests; write tenant |
| PATCH | `/api/requests/{id}` | Owning landlord; pending requests only |
| GET, POST | `/api/requests/{id}/messages` | Accepted request participants only |
| GET, POST, PATCH | `/api/requests/{id}/agreement` | Accepted request participants only |

## Product boundaries

This is a functional first version, not a completed public rental service. It does not yet include email verification/password recovery, photo uploads, listing editing or removal, moderation, identity/property verification, payments, notifications, or formal electronic signatures. Agreement acknowledgements record in-app review and do not represent a legally executed rental contract. Complete formal signing and applicable registration separately. Public launch needs these product decisions and appropriate operational hardening.
