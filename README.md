# Where to Bird Today?

Tell it *when* you want to bird, *where* you are, *how far* you'll travel, and a few preferences —
get back a ranked list of places with a summary of what you can realistically expect to see.

**Primary goal: learning Go.** The app is the vehicle.

> Detailed planning notes and decision records are kept privately for now and are not part of this
> repository.

**Status: Phase 1** — wiring the database in.

---

## Stack

| Layer | Choice |
|---|---|
| Backend | Go 1.26, standard-library `net/http` (enhanced `ServeMux`) |
| DB access | sqlc (SQL → typed Go) + pgx; goose for migrations |
| Database | PostgreSQL 16+ with **PostGIS** |
| Logging | `log/slog` |
| Frontend | SvelteKit + `adapter-static` (SSR off), embedded into the Go binary via `embed` — Phase 4 |
| Data | eBird API 2.0 + eBird Basic Dataset + AviList v2025b + Wikidata/Commons + iNaturalist — Phase 2 |
| Languages | Lithuanian (default) and English — Phase 2 |
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
```

Start the database (see below), then put `.env` into your shell and run the server. **The Go process
does not read `.env`** — it reads real environment variables, and only Docker Compose reads the file
directly. `DATABASE_URL` is required; the server refuses to start without it.

```bash
set -a && . ./.env && set +a && go run ./cmd/server
```

In PowerShell:

```powershell
Get-Content .env | Where-Object { $_ -match '^\s*[^#].*=' } | ForEach-Object { $name, $value = $_ -split '=', 2; Set-Item "env:$($name.Trim())" $value.Trim() }
go run ./cmd/server
```

Or, on Windows, [scripts/dev.ps1](scripts/dev.ps1) wraps that PowerShell snippet into one command — it still only sets real environment variables before running the binary, same as above, just without retyping it:

```powershell
.\scripts\dev.ps1
```

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

`/health` is a **liveness** probe — it answers "is this process running?" and never touches the
database, so a database blip cannot cause the orchestrator to restart a healthy container.
`/ready` is a **readiness** probe: it pings Postgres and returns `503` when it cannot reach it, so
the instance is taken out of rotation instead of being killed.

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

## Loading eBird data

Request the Baltic countries from the [eBird Basic Dataset](https://ebird.org/data/download) with the
sampling event data option, and unzip each export. Load the hotspots and species first
(`cmd/ingest-ebird-hotspots`, `cmd/ingest-avilist`), then for each export load the main file and its
`_sampling.txt` file, then rebuild the seasonality table:

```bash
go run ./cmd/ingest-ebd path/to/ebd_LT_..._relJun-2026.txt
go run ./cmd/ingest-ebd path/to/ebd_LT_..._relJun-2026_sampling.txt
go run ./cmd/compute-seasonality
```

Both files are needed for exact frequencies: the sampling file lists every checklist, including the
ones with no observation rows, and each frequency is out of all complete checklists. Loading is
idempotent (rows are upserted), so overlapping date ranges are harmless. Expect about 1,000 rows
per second, one transaction per file.

`compute-seasonality` also stores an `adjusted_frequency` for each row: the frequency pulled toward the
country's frequency for the same species and week, more strongly the fewer checklists it rests on. Hotspots
are ranked by it, so one sighting in three checklists no longer outranks 25% over 200. Change
`priorChecklists` in `internal/ingest/seasonality/seasonality.go` and run it again to tune the pull.

**Run `compute-seasonality` only after the checklists are loaded.** It rebuilds the table from
`checklists`, so with none loaded it replaces the table with nothing.

## Travel time

Searches by drive time (`max_minutes`) ask a local OSRM routing server how long it takes to drive to
each hotspot. It is opt-in, because it needs about 1.3 GB of road data built first.

Build the road graph for Lithuania, Latvia and Estonia into `data/osm/` (git-ignored). It downloads
about 490 MB of OpenStreetMap extracts from Geofabrik, then needs about 3.3 GB of free RAM and around
10 minutes for the one-time build. Each step is skipped if its output exists.

```powershell
.\scripts\build-osrm.ps1
```

Start the routing server (512 MB memory limit, port 5000) and set `OSRM_URL` in `.env`:

```bash
docker compose --env-file .env -f deploy/docker-compose.yml --profile routing up -d osrm
```

Without `OSRM_URL`, a `max_minutes` request answers `503` and searches by `radius_km` still work.
Map data © OpenStreetMap contributors (ODbL).

## Configuration

All configuration is environment variables. Copy `.env.example` to `.env` and fill it in.

| Variable | Purpose |
|---|---|
| `PORT` | HTTP listen port (default `8080`) |
| `LOG_LEVEL` | `debug` \| `info` \| `warn` \| `error` |
| `DATABASE_URL` | **Required.** Postgres connection string — no default, startup fails without it |
| `EBIRD_API_KEY` | **Secret.** eBird API 2.0 token — Phase 2 |
| `OSRM_URL` | Optional. OSRM routing server, e.g. `http://127.0.0.1:5000`; enables `max_minutes` searches |

> **The eBird key is a secret.** It lives in `.env` (gitignored) locally and in GitHub Actions /
> Container Apps secrets in CI and production. It must never appear in source, compose files,
> workflow YAML, or logs. `.env.example` contains `PLACEHOLDER` values only.

## Layout

```
cmd/server/      main.go — wiring and startup only
internal/        app code; the compiler forbids imports from outside this module
  api/           handlers, middleware, routing
  config/        environment-variable loading
  store/         connection pool and, from Phase 1, the sqlc-generated queries
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

- **Species reference backbone — AviList v2025b.** One CC BY 4.0 spreadsheet supplies, per species,
  the eBird species code, AvibaseID, IUCN Red List status and canonical Birds of the World /
  BirdLife DataZone URLs. Loaded offline into `species`; **no feature resolves a species identifier
  over the network at request time.**

- **Photos — Wikidata/Wikimedia Commons first, iNaturalist for gaps.** The app stores image
  *references*, never bytes, and licence + attribution are required columns on `species_media`.
  Macaulay Library is a link target only: eBird's API exposes no media endpoints, and Macaulay
  grants no download rights.

- **Species detail is our own panel, never an iframe.** eBird sends `X-Frame-Options: DENY` and
  Avibase `SAMEORIGIN`, so embedding is not possible — and linking out is the better design anyway.
  The panel leads with our own week-of-year seasonality data, which is the part no one else has.

- **Languages — Lithuanian and English, Lithuanian the default.** The load-bearing rule: **the Go
  API never returns a user-facing sentence.** Explanations are structured codes plus params; the
  Svelte app renders the prose. Common names live in `species_names`, source-ranked so the
  `lt → en → scientific` fallback resolves in SQL. Language is a request parameter, never global
  state.

## Attribution

Third-party data carries licence obligations that the UI must honour, not just the source:

- **AviList** — CC BY 4.0. *AviList Core Team. 2026. AviList: The Global Avian Checklist, v2025b.*
  <https://doi.org/10.2173/avilist.v2025b>
- **Wikimedia Commons** — per-image licence and author, rendered next to the image.
- **iNaturalist** — per-photo `attribution` string, rendered next to the image. Photos default to
  CC BY-**NC**.
- **eBird / Cornell Lab** — taxonomy and observation data, per the eBird API terms.

## Roadmap

| Phase | Deliverable |
|---|---|
| **0** | `curl localhost:8080/health` works; CI green |
| **1** | Config, sqlc+pgx, first migration, an endpoint that reads/writes a row ← *here* |
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
- **Style follows the [Google Go Style Guide](https://google.github.io/styleguide/go/guide)**, which
  governs over personal preference. The rule with the most visible effect here: **no assertion
  libraries.** Tests use standard comparisons and [`go-cmp`](https://github.com/google/go-cmp), and a
  failure message names what was called — `Load().Port = 9000, want 8080`, never `port = 9000`.
