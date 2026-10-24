// Package revgeo는 좌표 → 주소 역지오코딩을 제공한다.
// 최근접 건물(KNN)로 도로명주소·거리를, 점을 포함하는 필지(ST_Contains)로 법정동을
// 판정한다. admin_areas.geom(구역 폴리곤)은 미적재라 필지 폴리곤 포함관계로 법정동을 얻는다.
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

// RadiusDeg는 반경 m를 그 반경을 덮는 경위도 상자의 반폭(도)으로 바꾼다. 경도 1도는
// 111.32km × cos(위도)라 한국 최북단(약 38.6도)에서 약 87km이므로 85km로 나눠 넉넉히 잡는다.
//
// KNN(ORDER BY <->)에 geography 캐스트 ST_DWithin만 걸면 인덱스가 반경 조건을 못 쓴다.
// 반경 안에 건물이 하나도 없으면(바다·산·접경지) LIMIT을 채울 때까지 전국 데이터를 거리순으로
// 끝까지 훑어 수십 초가 걸리고, 연결 풀이 막혀 다른 요청까지 멈춘다(2026-10-23 운영 장애).
// geom && 상자 조건을 함께 주면 GiST가 상자 밖을 처음부터 배제한다.
func RadiusDeg(m float64) float64 { return m / 85000.0 }

// ByPoint는 입력 좌표의 역지오코딩 결과를 만든다. 도로명주소·거리·건물좌표는
// 반경 maxM 이내 최근접 건물(KNN)에서, 법정동(bcode·명)은 그 점을 포함하는 필지
// (ST_Contains)에서 얻는다. 포함 필지가 없으면(하천·미등록 등) 건물 bcode로 폴백한다.
func ByPoint(ctx context.Context, pool *pgxpool.Pool, lon, lat, maxM float64) (*Result, error) {
	var r Result
	var bldgBcode, bldgName string
	err := pool.QueryRow(ctx, `
		SELECT b.road_address, b.bcode, COALESCE(a.name,''),
		       ST_DistanceSphere(b.geom, ST_SetSRID(ST_MakePoint($1,$2),4326)),
		       ST_X(b.geom), ST_Y(b.geom)
		FROM buildings b LEFT JOIN admin_areas a ON a.bcode = b.bcode
		WHERE b.geom && ST_Expand(ST_SetSRID(ST_MakePoint($1,$2),4326), $4)
		  AND ST_DWithin(b.geom::geography, ST_SetSRID(ST_MakePoint($1,$2),4326)::geography, $3)
		ORDER BY b.geom <-> ST_SetSRID(ST_MakePoint($1,$2),4326)
		LIMIT 1`, lon, lat, maxM, RadiusDeg(maxM)).
		Scan(&r.RoadAddress, &bldgBcode, &bldgName, &r.DistanceM, &r.Point[0], &r.Point[1])
	if err != nil {
		return nil, err
	}

	// 점을 포함하는 필지의 법정동이 경계에서 최근접 건물보다 정확하다. 포함 필지가
	// 없거나 조회 실패면 건물 bcode로 폴백한다(역지오코딩 자체는 건물이 좌우).
	var parcelBcode, parcelName string
	if err := pool.QueryRow(ctx, `
		SELECT p.bcode, COALESCE(a.name,'')
		FROM parcels p LEFT JOIN admin_areas a ON a.bcode = p.bcode
		WHERE ST_Contains(p.geom, ST_SetSRID(ST_MakePoint($1,$2),4326))
		LIMIT 1`, lon, lat).Scan(&parcelBcode, &parcelName); err == nil && parcelBcode != "" {
		r.Bcode, r.AddressName = parcelBcode, parcelName
	} else {
		r.Bcode, r.AddressName = bldgBcode, bldgName
	}
	return &r, nil
}
