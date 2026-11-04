ALTER TABLE b2c_listings ADD COLUMN external_source text;
ALTER TABLE b2c_listings ADD COLUMN external_key text;
ALTER TABLE b2c_listings ADD CONSTRAINT b2c_external_key_pair
    CHECK ((external_source IS NULL) = (external_key IS NULL));
CREATE UNIQUE INDEX b2c_listings_external_key
    ON b2c_listings (external_source, external_key) WHERE external_source IS NOT NULL;

CREATE TABLE b2c_naver_sync (
    complex_no text PRIMARY KEY,
    complex_name text NOT NULL,
    last_success_at timestamptz NOT NULL,
    active_count integer NOT NULL CHECK (active_count >= 0)
);

CREATE TABLE b2c_naver_changes (
    id bigserial PRIMARY KEY,
    complex_no text NOT NULL,
    complex_name text NOT NULL,
    article_no text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('added', 'gone', 'price_changed')),
    previous_manwon integer,
    current_manwon integer,
    detected_at timestamptz NOT NULL
);
CREATE INDEX b2c_naver_changes_recent ON b2c_naver_changes (complex_name, detected_at DESC);
