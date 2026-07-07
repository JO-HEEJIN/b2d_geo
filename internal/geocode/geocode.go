// Package geocode는 도로명주소 → 좌표 조회를 제공한다.
// v0: 공백 제거 정확 매칭 1단만 (buildings.norm_road). 정규화/trigram 폴백과
// 본격 주소 파서는 Phase 2 (docs/design/address_parser.md 설계 참조).
package geocode

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Result는 지오코딩 결과 1건이다.
type Result struct {
	RoadAddress string     `json:"road_address"`
	Bcode       string     `json:"bcode"`
	Point       [2]float64 `json:"point"` // [lon, lat]
	Match       string     `json:"match"` // v0: "exact"
}

// Exact는 질의를 어절 단일 공백 키로 정규화해 정확 매칭한다 (juso 관례 동일).
// 시도부터 시작하는 전체 도로명주소를 기대한다 (v0 제약).
func Exact(ctx context.Context, pool *pgxpool.Pool, q string) (*Result, error) {
	key := strings.Join(strings.Fields(q), " ")
	var r Result
	err := pool.QueryRow(ctx, `
		SELECT road_address, bcode, ST_X(geom), ST_Y(geom)
		FROM buildings WHERE norm_road = $1 AND geom IS NOT NULL LIMIT 1`, key).
		Scan(&r.RoadAddress, &r.Bcode, &r.Point[0], &r.Point[1])
	if err != nil {
		return nil, err
	}
	r.Match = "exact"
	return &r, nil
}
