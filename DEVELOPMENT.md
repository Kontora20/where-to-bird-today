# Development

How to run Where to Bird Today on your own machine. The overview and the architecture are in the
[README](README.md).

## Prerequisites

- **Go 1.27**: `go version`
- **Docker Desktop**, for the local Postgres + PostGIS and the optional routing server
- **Node LTS**, to build the web app in `web/`
- An **eBird API key** ([keygen](https://ebird.org/api/keygen)), for the loaders that call eBird

## Quick start

```bash
git clone https://github.com/Kontora20/where-to-bird-today.git
cd where-to-bird-today
cp .env.example .env    # then replace every PLACEHOLDER with a real value
go build ./...
```

Start the database (see below), then put `.env` into your shell and run the server. **The Go process
does not read `.env`**: it reads real environment variables, and only Docker Compose reads the file
directly. `DATABASE_URL` is required; the server refuses to start without it.

```bash
set -a && . ./.env && set +a && go run ./cmd/server
```

In PowerShell:

```powershell
Get-Content .env | Where-Object { $_ -match '^\s*[^#].*=' } | ForEach-Object { $name, $value = $_ -split '=', 2; Set-Item "env:$($name.Trim())" $value.Trim() }
go run ./cmd/server
```

Or, on Windows, [scripts/dev.ps1](scripts/dev.ps1) wraps that PowerShell snippet into one command. It still
only sets real environment variables before running the binary:

```powershell
.\scripts\dev.ps1
```

The web app runs separately during development, with a proxy to the Go server on port 8080:

```bash
npm run dev --prefix web
```

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

`/health` is a **liveness** probe: it answers "is this process running?" and never touches the
database, so a database blip cannot cause the orchestrator to restart a healthy container.
`/ready` is a **readiness** probe: it pings Postgres and returns `503` when it cannot reach it, so
the instance is taken out of rotation instead of being killed.

## Local database

Postgres 18 + PostGIS 3.6, from the **repository root**:

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
`_sampling.txt` file, then rebuild the tables derived from them:

```bash
go run ./cmd/ingest-ebd path/to/ebd_LT_..._relJun-2026.txt
go run ./cmd/ingest-ebd path/to/ebd_LT_..._relJun-2026_sampling.txt
go run ./cmd/compute-seasonality
```

Both files are needed for exact frequencies: the sampling file lists every checklist, including the
ones with no observation rows, and each frequency is out of all complete checklists. Loading is
idempotent (rows are upserted), so overlapping date ranges are harmless. Expect about 1,000 rows
per second, one transaction per file.

**Run `compute-seasonality` only after the checklists are loaded.** It rebuilds its tables from
`checklists`, so with none loaded it replaces them with nothing.

In production the export is not needed to stay current: see "Data pipeline" in the README.
`ingest-ebird-checklists` and `ingest-ebird-recent` can also be run by hand, for example
`go run ./cmd/ingest-ebird-checklists`; set `EBIRD_REGIONS` (for example `LT`) to load one country.

## Travel time

Searches by drive time (`max_minutes`) ask an OSRM routing server how long it takes to drive to each
hotspot. Locally it is opt-in, because it needs about 1.3 GB of road data built first.

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
| `DATABASE_URL` | **Required.** Postgres connection string, no default; startup fails without it |
| `EBIRD_API_KEY` | **Secret.** eBird API 2.0 token, used by the loaders |
| `EBIRD_REGIONS` | Optional. Comma-separated eBird country codes the loaders cover (default `LT,LV,EE`) |
| `OSRM_URL` | Optional. OSRM routing server, e.g. `http://127.0.0.1:5000`; enables `max_minutes` searches |

> **The eBird key is a secret.** It lives in `.env` (gitignored) locally and in Key Vault, behind Container
> Apps secrets, in production. It must never appear in source, compose files, workflow YAML, or logs.
> `.env.example` contains `PLACEHOLDER` values only.

## Tests and checks

```bash
go test ./...          # integration tests start a Postgres + PostGIS container, so Docker must be running
go test -short ./...   # skips the tests that need Docker
golangci-lint run ./...
npm run check --prefix web
```

After changing a file under `internal/store/queries/`, regenerate the Go code with `sqlc generate`
(it needs `DATABASE_URL` in the environment).

## Deploying

See [deploy/README.md](deploy/README.md): the Bicep template, the GitHub Actions sign-in, and the jobs.
