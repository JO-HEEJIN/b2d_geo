// Package places는 상호명 키워드 검색을 제공한다.
// v0: 부분일치 우선 + trigram 유사도 순. 자모 분해/초성 검색은 Phase 3.
package places

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Place는 검색 결과 1건이다.
type Place struct {
	Name         string     `json:"name"`
	CategoryCode string     `json:"category_code,omitempty"`
	RoadAddress  string     `json:"road_address,omitempty"`
	JibunAddress string     `json:"jibun_address,omitempty"`
	Point        [2]float64 `json:"point"`
	Similarity   float64    `json:"similarity"`
}

// Search는 상호명 키워드로 places를 찾는다 (부분일치 우선, 유사도 내림차순).
func Search(ctx context.Context, pool *pgxpool.Pool, q string, limit int) ([]Place, error) {
	norm := strings.Join(strings.Fields(q), " ")
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := pool.Query(ctx, `
		SELECT name, COALESCE(category_code,''), COALESCE(road_address,''),
		       COALESCE(jibun_address,''), ST_X(geom), ST_Y(geom),
		       similarity(norm_name, $1)
		FROM places
		WHERE norm_name % $1 OR norm_name ILIKE '%' || $1 || '%'
		ORDER BY (norm_name ILIKE '%' || $1 || '%') DESC,
		         similarity(norm_name, $1) DESC
		LIMIT $2`, norm, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Place
	for rows.Next() {
		var p Place
		if err := rows.Scan(&p.Name, &p.CategoryCode, &p.RoadAddress,
			&p.JibunAddress, &p.Point[0], &p.Point[1], &p.Similarity); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
