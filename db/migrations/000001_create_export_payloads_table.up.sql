CREATE TABLE export_payloads (
    id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    completed_at timestamp with time zone,
    expires timestamp with time zone,
    request_id text,
    name text,
    format text,
    status text,
    sources jsonb,
    s3_key text,
    account_id text,
    organization_id text,
    username text
);
