CREATE TABLE b2c_market_sources (
    source_id text NOT NULL,
    scope text NOT NULL,
    name text NOT NULL,
    complex_name text NOT NULL,
    last_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_success_at timestamptz,
    observed_at timestamptz,
    snapshot_id text,
    last_error text NOT NULL DEFAULT '',
    next_attempt_at timestamptz,
    active_count integer CHECK (active_count >= 0),
    PRIMARY KEY(source_id,scope)
);
CREATE TABLE b2c_market_snapshots (
    source_id text NOT NULL,
    scope text NOT NULL,
    snapshot_id text NOT NULL,
    observed_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    digest text NOT NULL,
    item_count integer NOT NULL,
    PRIMARY KEY(source_id,scope,snapshot_id),
    FOREIGN KEY(source_id,scope) REFERENCES b2c_market_sources(source_id,scope)
);
CREATE TABLE b2c_market_changes (
    id bigserial PRIMARY KEY,
    source_id text NOT NULL,
    scope text NOT NULL,
    snapshot_id text NOT NULL,
    listing_id bigint NOT NULL REFERENCES b2c_listings(id),
    external_id text NOT NULL,
    complex_name text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('added','relisted','price_changed','sold','withdrawn','missing')),
    previous_manwon integer,
    current_manwon integer,
    observed_at timestamptz NOT NULL,
    detected_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX b2c_market_changes_recent ON b2c_market_changes(complex_name,detected_at DESC,id DESC);
CREATE FUNCTION b2c_notify_market_status() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('b2c_listings_changed','');
    RETURN NULL;
END;
$$;
CREATE TRIGGER b2c_market_source_live AFTER INSERT OR UPDATE ON b2c_market_sources
    FOR EACH ROW EXECUTE FUNCTION b2c_notify_market_status();
