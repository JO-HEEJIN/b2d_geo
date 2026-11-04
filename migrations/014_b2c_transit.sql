-- B2C 인프라맵(모듈 4)의 지하철역·버스정류장 위치. 공공데이터만 적재한다.
CREATE TABLE IF NOT EXISTS transit_stops (
    id             bigserial PRIMARY KEY,
    kind           text NOT NULL CHECK (kind IN ('subway_station', 'bus_stop')),
    stop_id        text NOT NULL,
    name           text NOT NULL,
    line           text,
    geom           geometry(Point, 4326) NOT NULL,
    source         text NOT NULL,
    source_version date NOT NULL,
    UNIQUE (kind, stop_id)
);

CREATE INDEX IF NOT EXISTS idx_transit_stops_geom ON transit_stops USING gist (geom);
