CREATE INDEX CONCURRENTLY IF NOT EXISTS export_payloads_account_id_org_id_username_index ON export_payloads (account_id, organization_id, username);

CREATE INDEX CONCURRENTLY IF NOT EXISTS export_payloads_account_id_org_id_username_created_at_index ON export_payloads (account_id, organization_id, username);

# explain analyze SELECT count(*) FROM "export_payloads" WHERE "export_payloads"."account_id" = '1' AND "export_payloads"."organization_id" = '2' AND "export_payloads"."username" = 'fred';

# explain analyze SELECT "export_payloads"."id","export_payloads"."name","export_payloads"."created_at","export_payloads"."completed_at","export_payloads"."expires","export_payloads"."format","export_payloads"."status" FROM "export_payloads" WHERE "export_payloads"."account_id" = '1' AND "export_payloads"."organization_id" = '2' AND "export_payloads"."username" = 'fred' ORDER BY created_at asc LIMIT 100;
