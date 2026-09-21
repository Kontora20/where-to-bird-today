-- name: CreateHotspot :one
INSERT INTO hotspots (name, location, region, source, source_ref)
VALUES (
    sqlc.arg(name),
    ST_SetSRID(ST_MakePoint(sqlc.arg(longitude), sqlc.arg(latitude)), 4326)::geography,
    sqlc.arg(region),
    sqlc.arg(source),
    sqlc.arg(source_ref)
)
RETURNING
    id,
    name,
    ST_X(location::geometry) AS longitude,
    ST_Y(location::geometry) AS latitude,
    region,
    source,
    source_ref,
    metadata,
    created_at,
    updated_at;

-- name: UpsertHotspot :one
INSERT INTO hotspots (name, location, region, source, source_ref)
VALUES (
    sqlc.arg(name),
    ST_SetSRID(ST_MakePoint(sqlc.arg(longitude), sqlc.arg(latitude)), 4326)::geography,
    sqlc.arg(region),
    sqlc.arg(source),
    sqlc.arg(source_ref)
)
ON CONFLICT (source, source_ref) DO UPDATE SET
    name       = EXCLUDED.name,
    location   = EXCLUDED.location,
    region     = EXCLUDED.region,
    updated_at = now()
RETURNING
    id,
    name,
    ST_X(location::geometry) AS longitude,
    ST_Y(location::geometry) AS latitude,
    region,
    source,
    source_ref,
    metadata,
    created_at,
    updated_at;

-- name: GetHotspot :one
SELECT
    id,
    name,
    ST_X(location::geometry) AS longitude,
    ST_Y(location::geometry) AS latitude,
    region,
    source,
    source_ref,
    metadata,
    created_at,
    updated_at
FROM hotspots
WHERE id = sqlc.arg(id);

-- name: ListHotspotsWithin :many
-- The hotspots within radius_meters of a point, straight-line, with their
-- coordinates so the caller can narrow them by drive time.
SELECT
    h.id,
    ST_Y(h.location::geometry) AS latitude,
    ST_X(h.location::geometry) AS longitude
FROM hotspots h
WHERE ST_DWithin(
    h.location,
    ST_SetSRID(ST_MakePoint(sqlc.arg(longitude), sqlc.arg(latitude)), 4326)::geography,
    sqlc.arg(radius_meters)
);
