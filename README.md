# Where to Bird Today?

Tell it *when* you want to bird, *where* you are, *how far* you'll travel, and a few preferences —
get back a ranked list of places with a summary of what you can realistically expect to see.

**Primary goal: learning Go.** The app is the vehicle.

> Detailed planning notes and decision records are kept privately for now and are not part of this
> repository.

**Status: Phase 0** — foundations and a running skeleton.

---

## Stack

| Layer | Choice |
|---|---|
| Backend | Go 1.26, standard-library `net/http` (enhanced `ServeMux`) |
| DB access | sqlc (SQL → typed Go) + pgx; goose for migrations |
| Database | PostgreSQL 16+ with **PostGIS** |
| Logging | `log/slog` |
| Frontend | SvelteKit + `adapter-static` (SSR off), embedded into the Go binary via `embed` — Phase 4 |
| Data | eBird API 2.0 + eBird Basic Dataset + iNaturalist — Phase 2 |
| Hosting | Azure Container Apps + Postgres Flexible Server, Bicep, GitHub Actions — Phase 5 |

Deliberately **not** using Gin/Echo/Fiber or an ORM: the point is to learn the real `net/http` model
and to actually understand the SQL.

## Prerequisites

- **Go 1.26.x** — `go version`
- **Docker Desktop** — for local Postgres+PostGIS
- **Node LTS** — not needed until Phase 4
- An **eBird API key** ([keygen](https://ebird.org/api/keygen)) — not needed until Phase 2

## Quick start

```bash
git clone https://github.com/Kontora20/where-to-bird-today.git
cd where-to-bird-today
cp .env.example .env    # then replace every PLACEHOLDER with a real value
go build ./...
go run ./cmd/server
```

```bash
curl http://localhost:8080/health
```

## Local database

Postgres 17 + PostGIS 3.5, from the **repository root**:

```bash
docker compose --env-file .env -f deploy/docker-compose.yml up -d
```

`--env-file .env` is required: with `-f`, Compose otherwise looks for `.env` next to the compose
file in `deploy/` rather than in the directory you ran it from.

Stop it, keeping the data:

```bash
docker compose --env-file .env -f deploy/docker-compose.yml down
```

Destroy it, **including the data volume**:

```bash
docker compose --env-file .env -f deploy/docker-compose.yml down -v
```

Open a psql shell:

```bash
docker exec -it wtbt-postgres psql -U birder -d wheretobird
```

## Configuration

All configuration is environment variables. Copy `.env.example` to `.env` and fill it in.

| Variable | Purpose |
|---|---|
| `PORT` | HTTP listen port (default `8080`) |
| `LOG_LEVEL` | `debug` \| `info` \| `warn` \| `error` |
| `DATABASE_URL` | Postgres connection string |
| `EBIRD_API_KEY` | **Secret.** eBird API 2.0 token — Phase 2 |

> **The eBird key is a secret.** It lives in `.env` (gitignored) locally and in GitHub Actions /
> Container Apps secrets in CI and production. It must never appear in source, compose files,
> workflow YAML, or logs. `.env.example` contains `PLACEHOLDER` values only.

## Layout

```
cmd/server/      main.go — wiring and startup only
internal/        app code; the compiler forbids imports from outside this module
  config/        environment-variable loading
  http/          handlers, middleware, routing
migrations/      goose SQL migrations (Phase 1)
deploy/          docker-compose, Dockerfile, Bicep
web/             SvelteKit app (Phase 4)
```

Coming from .NET: `cmd/server` ≈ your `Program.cs` host project, and `internal/*` ≈ class libraries
— but wired by hand in `main` rather than through a DI container. `internal/` is enforced by the Go
compiler: no module outside this one can import anything beneath it.

## Decisions

Architecture decision records are kept privately alongside the planning notes. Decisions that
affect how the code is written are summarised here as they land.

- **EBD region scope** — the Baltics (Estonia, Latvia, Lithuania). The ingestion loader itself
  stays region-agnostic: the region is configuration, never baked into loader logic, schema, or
  queries.

## Roadmap

| Phase | Deliverable |
|---|---|
| **0** | `curl localhost:8080/health` works; CI green ← *here* |
| 1 | Config, sqlc+pgx, first migration, an endpoint that reads/writes a row |
| 2 | eBird + iNat clients, EBD bulk loader, `species_seasonality` |
| 3 | Spatial `ST_DWithin` query, ranking, parallel fetches via `errgroup` |
| 4 | SvelteKit UI with a Leaflet map, embedded in the binary |
| 5 | Dockerfile, Bicep, Container Apps, full CI/CD |
| 6 | Observability, pprof pass, write-up |

## Conventions

- **Line endings are LF**, enforced by `.gitattributes` rather than left to each machine's git
  config. Phase 5 copies shell scripts into a Linux container, where CRLF breaks the shebang.
- **`internal/`** is enforced by the Go compiler: no other module can import anything beneath it.
- **Dependencies are wired by hand in `main`** — no container, no service registration.
