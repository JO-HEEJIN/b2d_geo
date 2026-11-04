-- B2C 남은 매물(모듈 5)의 직접 등록 원천. 중개사·집주인이 등록한 매물만 담는다.
-- 포털 스크레이핑 매물은 넣지 않는다 (ADR 0002).
CREATE TABLE IF NOT EXISTS b2c_listings (
    id            bigserial PRIMARY KEY,
    complex_name  text NOT NULL,
    type_name     text,
    asking_manwon integer NOT NULL CHECK (asking_manwon > 0),
    floor         integer,
    lon           double precision,
    lat           double precision,
    registered_by text NOT NULL,
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'sold', 'withdrawn')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_b2c_listings_active ON b2c_listings (complex_name) WHERE status = 'active';
