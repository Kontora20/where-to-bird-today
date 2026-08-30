-- name: CreateIngestionRun :one
INSERT INTO ingestion_runs (source)
VALUES ($1)
RETURNING *;

-- name: GetIngestionRun :one
SELECT * FROM ingestion_runs
WHERE id = $1;
