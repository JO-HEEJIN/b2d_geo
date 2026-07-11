// Package places는 상호명 키워드 검색을 제공한다.
// v0: 부분일치 우선 + trigram 유사도 순. 자모 분해/초성 검색은 Phase 3.
package places

import (
	"context"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// choseongRe: 질의가 초성(+공백/숫자)만인지 — 초성 검색 경로 판정.
var choseongRe = regexp.MustCompile(`^[ㄱ-ㅎ0-9 ]+$`)

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
// 질의가 초성만이면("ㅇㄴㄹㅇㄱ") choseong 컬럼 경로를 탄다 (Phase 3).
func Search(ctx context.Context, pool *pgxpool.Pool, q string, limit int) ([]Place, error) {
	norm := strings.Join(strings.Fields(q), " ")
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	if choseongRe.MatchString(norm) {
		return searchChoseong(ctx, pool, strings.ReplaceAll(norm, " ", ""), limit)
	}
	// 전역 similarity 정렬 금지: "약국"류 초단문은 매치 수십만 행이라
	// 정렬이 10초를 태움 (실측). 동일 → 접두 → 포함 3단 UNION으로 작업량을
	// LIMIT에 묶고, similarity는 반환분에만 계산한다.
	rows, err := pool.Query(ctx, `
		WITH hits AS (
			(SELECT id, 0 AS tier FROM places WHERE norm_name = $1 LIMIT $2)
			UNION ALL
			(SELECT id, 1 FROM places WHERE norm_name LIKE $1 || '%' AND norm_name <> $1 LIMIT $2)
			UNION ALL
			(SELECT id, 2 FROM places WHERE norm_name ILIKE '%' || $1 || '%'
			   AND norm_name NOT LIKE $1 || '%' LIMIT $2)
		)
		SELECT p.name, COALESCE(p.category_code,''), COALESCE(p.road_address,''),
		       COALESCE(p.jibun_address,''), ST_X(p.geom), ST_Y(p.geom),
		       similarity(p.norm_name, $1)
		FROM (SELECT DISTINCT ON (id) id, tier FROM hits ORDER BY id, tier) h
		JOIN places p ON p.id = h.id
		ORDER BY h.tier, length(p.norm_name)
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

// searchChoseong은 초성 질의를 choseong 컬럼에 부분일치+유사도로 매칭한다.
func searchChoseong(ctx context.Context, pool *pgxpool.Pool, q string, limit int) ([]Place, error) {
	rows, err := pool.Query(ctx, `
		SELECT name, COALESCE(category_code,''), COALESCE(road_address,''),
		       COALESCE(jibun_address,''), ST_X(geom), ST_Y(geom),
		       similarity(choseong, $1)
		FROM places
		WHERE choseong LIKE '%' || $1 || '%'
		ORDER BY (choseong = $1) DESC, length(choseong) ASC
		LIMIT $2`, q, limit)
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
