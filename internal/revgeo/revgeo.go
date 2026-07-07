// Package revgeo는 좌표 → 주소 역지오코딩을 제공한다.
// v0: 최근접 건물(KNN) + 법정동명. 법정동 폴리곤 ST_Contains는 구역의도형
// 확보 후 Phase 2에서 추가한다 (현재는 필지 bcode로 동일 효과).
package revgeo

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Result는 역지오코딩 결과다.
type Result struct {
	RoadAddress string     `json:"road_address"`
	Bcode       string     `json:"bcode"`
	AddressName string     `json:"address_name,omitempty"` // 법정동 전체명
	DistanceM   float64    `json:"distance_m"`             // 입력점 → 건물 좌표
	Point       [2]float64 `json:"point"`                  // 건물 좌표 [lon, lat]
}

// ByPoint는 입력 좌표에서 가장 가까운 건물 주소를 찾는다 (반경 maxM 이내).
func ByPoint(ctx context.Context, pool *pgxpool.Pool, lon, lat, maxM float64) (*Result, error) {
	var r Result
	err := pool.QueryRow(ctx, `
		SELECT b.road_address, b.bcode, COALESCE(a.name,''),
		       ST_DistanceSphere(b.geom, ST_SetSRID(ST_MakePoint($1,$2),4326)),
		       ST_X(b.geom), ST_Y(b.geom)
		FROM buildings b LEFT JOIN admin_areas a ON a.bcode = b.bcode
		WHERE ST_DWithin(b.geom::geography, ST_SetSRID(ST_MakePoint($1,$2),4326)::geography, $3)
		ORDER BY b.geom <-> ST_SetSRID(ST_MakePoint($1,$2),4326)
		LIMIT 1`, lon, lat, maxM).
		Scan(&r.RoadAddress, &r.Bcode, &r.AddressName, &r.DistanceM, &r.Point[0], &r.Point[1])
	if err != nil {
		return nil, err
	}
	return &r, nil
}
