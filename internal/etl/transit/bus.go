// Package transit는 B2C 인프라맵용 대중교통 정류장 위치를 transit_stops에 적재한다.
//
// 버스: 국토교통부_전국 버스정류장 위치정보 (data.go.kr 15067528). CSV, CP949, 콤마 구분,
// 따옴표로 감싼 정류장명에 콤마가 들어갈 수 있다. 좌표는 WGS84 위도·경도 그대로다.
// 2026-10-20판 실물 확인: 213,985행, 정류장번호 중복 없음, 좌표 이상치 없음.
package transit

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

// BusSourceName은 transit_stops.source 값이다.
const BusSourceName = "molit_bus_stops"

// busHeader는 2026-10-20판 실물 헤더다. 다르면 적재를 멈춘다.
var busHeader = []string{"정류장번호", "정류장명", "위도", "경도", "정보수집일", "모바일단축번호", "도시코드", "도시명", "관리도시명"}

// Stop은 정류장 1곳이다.
type Stop struct {
	ID   string
	Name string
	City string
	Lon  float64
	Lat  float64
}

// ParseBusCSV는 CP949 버스정류장 CSV를 읽는다. 좌표가 한국 범위를 벗어나거나 숫자가
// 아닌 행은 건너뛰고 그 수를 돌려준다.
func ParseBusCSV(r io.Reader, fn func(Stop) error) (skipped int, err error) {
	cr := csv.NewReader(bufio.NewReaderSize(transform.NewReader(r, korean.EUCKR.NewDecoder()), 1<<20))
	header, err := cr.Read()
	if err != nil {
		return 0, fmt.Errorf("헤더 읽기: %w", err)
	}
	if len(header) != len(busHeader) {
		return 0, fmt.Errorf("컬럼 수 불일치: got %d want %d", len(header), len(busHeader))
	}
	for i, h := range header {
		if strings.TrimPrefix(strings.TrimSpace(h), "\ufeff") != busHeader[i] {
			return 0, fmt.Errorf("컬럼 %d 불일치: got %q want %q", i, h, busHeader[i])
		}
	}
	cr.FieldsPerRecord = len(busHeader)
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			return skipped, nil
		}
		if err != nil {
			return skipped, err
		}
		lat, errLat := strconv.ParseFloat(strings.TrimSpace(rec[2]), 64)
		lon, errLon := strconv.ParseFloat(strings.TrimSpace(rec[3]), 64)
		id, name := strings.TrimSpace(rec[0]), strings.TrimSpace(rec[1])
		if errLat != nil || errLon != nil || id == "" || name == "" ||
			lat < 33 || lat > 39.5 || lon < 124 || lon > 132 {
			skipped++
			continue
		}
		if err := fn(Stop{ID: id, Name: name, City: strings.TrimSpace(rec[7]), Lon: lon, Lat: lat}); err != nil {
			return skipped, err
		}
	}
}

// LoadBus는 버스정류장 CSV를 transit_stops에 적재한다. 한 트랜잭션에서 기존 버스 행을
// 지우고 다시 넣으므로 재실행해도 결과가 같다. sourceVersion은 "2026-10-20" 형식이다.
func LoadBus(ctx context.Context, pool *pgxpool.Pool, r io.Reader, sourceVersion string) (inserted int64, skipped int, err error) {
	var rows [][]any
	skipped, err = ParseBusCSV(r, func(s Stop) error {
		rows = append(rows, []any{s.ID, s.Name, s.City, s.Lon, s.Lat})
		return nil
	})
	if err != nil {
		return 0, skipped, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, skipped, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM transit_stops WHERE source = $1`, BusSourceName); err != nil {
		return 0, skipped, err
	}
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE bus_staging (stop_id text, name text, city text, lon float8, lat float8) ON COMMIT DROP`); err != nil {
		return 0, skipped, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"bus_staging"}, []string{"stop_id", "name", "city", "lon", "lat"}, pgx.CopyFromRows(rows)); err != nil {
		return 0, skipped, fmt.Errorf("CopyFrom: %w", err)
	}
	ct, err := tx.Exec(ctx, `INSERT INTO transit_stops (kind, stop_id, name, line, geom, source, source_version)
		SELECT 'bus_stop', stop_id, name, NULLIF(city, ''), ST_SetSRID(ST_MakePoint(lon, lat), 4326), $1, $2::date
		FROM bus_staging
		ON CONFLICT (kind, stop_id) DO NOTHING`, BusSourceName, sourceVersion)
	if err != nil {
		return 0, skipped, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, skipped, err
	}
	return ct.RowsAffected(), skipped, nil
}
