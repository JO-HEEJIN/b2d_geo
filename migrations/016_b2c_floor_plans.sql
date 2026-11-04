-- B2C 평면 타입(모듈 2). 같은 단지·같은 평형이어도 화장실 위치·거실 구조가 다르므로 타입마다 한 행이다.
-- 평면도 이미지는 출처가 분명한 것(분양공고 등)의 URL만 둔다.
CREATE TABLE IF NOT EXISTS b2c_floor_plans (
    complex_id        text NOT NULL,
    type_name         text NOT NULL,
    exclusive_area_m2 double precision NOT NULL CHECK (exclusive_area_m2 > 0),
    rooms             integer,
    bathrooms         integer,
    bays              integer,
    bathroom_position text,
    living_room       text,
    orientation       text,
    image_url         text,
    units             text[] NOT NULL DEFAULT '{}',
    registered_by     text NOT NULL,
    updated_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (complex_id, type_name)
);
