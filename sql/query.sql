-- name: GetExports :one
SELECT * FROM exports
WHERE id = $1 AND account_id = $2 AND organization_id = $3 and username =  $4
LIMIT 1;

-- name: ListExports :many
SELECT * FROM exports
WHERE account_id = $1 AND organization_id = $2 and username = $3
ORDER BY name, application;

-- name: CreateExport :one
INSERT INTO exports (
  name, application, format, resources, account_id, organization_id, username
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: DeleteExport :exec
DELETE FROM exports
WHERE id = $1 AND account_id = $2 AND organization_id = $3 and username =  $4;
