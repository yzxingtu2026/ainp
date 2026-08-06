CREATE TABLE downstream_orders (
    id varchar(64) PRIMARY KEY,
    tenant_id varchar(64) NOT NULL,
    created_at timestamptz NOT NULL
);
