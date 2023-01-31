CREATE TABLE export_payloads (
    id uuid PRIMARY KEY,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    completed_at timestamp with time zone,
    expires timestamp with time zone,
    request_id text,
    name text,
    format text,
    status text,
    s3_key text,
    account_id text,
    organization_id text,
    username text
);

CREATE TABLE export_sources (
    id uuid PRIMARY KEY,
    export_payload_id uuid REFERENCES export_payloads(id) ON DELETE CASCADE,
    application text NOT NULL,
    status text NOT NULL,
    resource text NOT NULL,
    filters jsonb,
    error text,
    message text
);
