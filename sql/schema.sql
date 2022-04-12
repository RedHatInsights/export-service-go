CREATE TABLE exports (
  id                uuid                      PRIMARY KEY DEFAULT uuid_generate_v4(),
  created_timestamp timestamp with time zone  NOT NULL DEFAULT NOW(),
  updated_timestamp timestamp with time zone,
  expires           timestamp with time zone,
  name              text                      NOT NULL,
  application       character varying(150)    NOT NULL,
  format            character varying(10)     NOT NULL,
  resources         jsonb,
  account_id        character varying(150)    NOT NULL,
  organization_id   character varying(150)    NOT NULL,
  username          character varying(150)    NOT NULL
);
