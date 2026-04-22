-- name: CreateAuditEntry :exec
INSERT INTO audit_log (
    organization_id, actor_user_id, actor_api_key_id,
    action, resource_type, resource_id,
    metadata, ip_address, user_agent
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
