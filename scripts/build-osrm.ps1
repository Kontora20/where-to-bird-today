# Builds the OSRM road graph for Lithuania, Latvia and Estonia into data/osm/.
# Each step is skipped when its output already exists; delete data/osm to
# rebuild everything. The extract step needs about 3.3 GB of free RAM.
$ErrorActionPreference = 'Stop'

# Keep in step with the image in deploy/docker-compose.yml: the graph files
# only load in the OSRM version that built them.
$osrmImage = 'ghcr.io/project-osrm/osrm-backend@sha256:8a1b1bc938412f15f9b5b32d794c4ec6bf4a85dfbbabfa0a014b70b187edb53b'

$dataDir = Join-Path $PSScriptRoot '..\data\osm'
New-Item -ItemType Directory -Force $dataDir | Out-Null
$dataDir = (Resolve-Path $dataDir).Path

function Invoke-Docker {
	docker @args
	if ($LASTEXITCODE -ne 0) { throw "docker $($args -join ' ') failed with exit code $LASTEXITCODE" }
}

$countries = 'lithuania', 'latvia', 'estonia'
$ProgressPreference = 'SilentlyContinue'
foreach ($country in $countries) {
	$file = Join-Path $dataDir "$country-latest.osm.pbf"
	if (-not (Test-Path $file)) {
		Write-Host "Downloading $country"
		Invoke-WebRequest "https://download.geofabrik.de/europe/$country-latest.osm.pbf" -OutFile $file
	}
}

if (-not (Test-Path (Join-Path $dataDir 'baltics.osm.pbf'))) {
	Write-Host 'Merging the three extracts'
	@'
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends osmium-tool && rm -rf /var/lib/apt/lists/*
ENTRYPOINT ["osmium"]
'@ | docker build -q -t wtbt-osmium -
	if ($LASTEXITCODE -ne 0) { throw 'building the osmium image failed' }

	$inputs = $countries | ForEach-Object { "/data/$_-latest.osm.pbf" }
	Invoke-Docker run --rm -v "${dataDir}:/data" wtbt-osmium merge @inputs -o /data/baltics.osm.pbf
}

if (-not (Test-Path (Join-Path $dataDir 'baltics.osrm'))) {
	Write-Host 'Extracting the road network'
	Invoke-Docker run --rm -v "${dataDir}:/data" $osrmImage osrm-extract -p /opt/car.lua /data/baltics.osm.pbf
}

if (-not (Test-Path (Join-Path $dataDir 'baltics.osrm.hsgr'))) {
	Write-Host 'Contracting the graph'
	Invoke-Docker run --rm -v "${dataDir}:/data" $osrmImage osrm-contract /data/baltics.osrm
}

Write-Host 'Done. Start the server: docker compose --env-file .env -f deploy/docker-compose.yml --profile routing up -d osrm'
