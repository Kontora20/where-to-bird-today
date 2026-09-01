-- name: CreateIngestionRun :one
INSERT INTO ingestion_runs (source)
VALUES ($1)
RETURNING *;

-- name: GetIngestionRun :one
SELECT * FROM ingestion_runs
WHERE id = $1;

-- name: FinishIngestionRun :one
UPDATE ingestion_runs
SET finished_at = now(),
    row_count   = sqlc.arg(row_count),
    status      = sqlc.arg(status),
    error       = sqlc.narg(error)
WHERE id = sqlc.arg(id)
RETURNING *;
