package api

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// ---- 실거래 최근 N개월 묶음 + 법정동 중심점 (지도 화면용 집계, 사실 반환) ----
//
// 브라우저가 월별로 /v1/appraisal/sales를 여러 번 부르면 demo 티어(2 rps, burst 4)에서
// 429가 난다. 그래서 서버가 월별 MOLIT 호출을 모아 한 번에 응답한다. MOLIT 토지 실거래는
// 지번이 마스킹되어 필지 단위 위치를 알 수 없으므로, 지도 표시는 법정동 대표점까지만
// 제공한다. admin_areas.geom(법정동 폴리곤)은 미적재라(learning_log 2-4), 전국 적재된
// 표준지(standard_lots) 좌표의 법정동별 평균을 대표점으로 쓴다. 평가·판단·보정 없음.

const (
	salesRecentDefaultMonths = 3
	salesRecentMaxMonths     = 6
	molitCacheTTL            = 6 * time.Hour
	molitCacheMax            = 512  // 넘치면 만료 항목부터, 그래도 차 있으면 한 개만 비운다
	molitPageRows            = 1000 // MOLIT 한 페이지 행 수. 이만큼 오면 잘렸다고 표시한다
	umdPointsLimit           = 600
)

// kst는 거래월 계산 기준 시간대다. distroless 이미지에 tzdata가 없어도 동작하도록
// LoadLocation 대신 고정 오프셋을 쓴다(한국은 서머타임 없음).
var kst = time.FixedZone("KST", 9*60*60)

type molitCacheEntry struct {
	items []molitItem
	at    time.Time
}

// molitCache는 (시군구, 거래월) -> MOLIT 응답 항목 캐시다. data.go.kr 일일 호출 한도를
// 보호하고, 같은 지역을 연달아 조회할 때 응답을 빠르게 한다.
type molitCache struct {
	mu sync.Mutex
	m  map[string]molitCacheEntry
}

var salesCache = &molitCache{m: map[string]molitCacheEntry{}}

func (c *molitCache) get(key string, now time.Time) ([]molitItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || now.Sub(e.at) > molitCacheTTL {
		return nil, false
	}
	return e.items, true
}

func (c *molitCache) put(key string, items []molitItem, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= molitCacheMax {
		// 통째로 비우면 남의 지역 조회 몇십 번으로 캐시 전체를 날릴 수 있다(QA MED-2).
		// 만료된 것부터 지우고, 그래도 가득이면 임의의 한 개만 지운다(정확한 LRU는 불필요).
		for k, e := range c.m {
			if now.Sub(e.at) > molitCacheTTL {
				delete(c.m, k)
			}
		}
		if len(c.m) >= molitCacheMax {
			for k := range c.m {
				delete(c.m, k)
				break
			}
		}
	}
	c.m[key] = molitCacheEntry{items: items, at: now}
}

// recentMonths는 now가 속한 달을 포함해 최근 n개월을 YYYYMM 문자열로, 최신순으로 돌려준다.
func recentMonths(now time.Time, n int) []string {
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, first.AddDate(0, -i, 0).Format("200601"))
	}
	return out
}

type umdPoint struct {
	Name string  `json:"name"`
	Lng  float64 `json:"lng"`
	Lat  float64 `json:"lat"`
}

func (s *Server) appraisalSalesRecent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lawd := q.Get("lawd_cd")
	if len(lawd) != 5 || !allDigits(lawd) {
		writeErr(w, http.StatusBadRequest, "lawd_cd(5) required")
		return
	}
	n := salesRecentDefaultMonths
	if v := q.Get("months"); v != "" {
		m, err := strconv.Atoi(v)
		if err != nil || m < 1 || m > salesRecentMaxMonths {
			writeErr(w, http.StatusBadRequest, "months must be 1-6")
			return
		}
		n = m
	}
	if s.MolitKey == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": map[string]string{
				"code":    "SALES_DISABLED",
				"message": "실거래 조회가 현재 비활성 상태입니다",
			},
		})
		return
	}
	// 실존 시군구만 상류(MOLIT)로 보낸다. 임의 코드로 상류 호출을 늘리거나 캐시를 밀어내는
	// 것을 막는다(QA MED-2). 필지(화면이 시군구 코드를 얻는 곳) 또는 표준지 어느 쪽에든
	// 있으면 통과시킨다. 두 테이블의 코드 체계 시점이 다를 수 있어서다(화성 일반구 신설,
	// 광주·전남 통합 등 — QA NEW-2).
	if s.Pool != nil {
		ok, err := s.hasRegion(r.Context(), "parcels", lawd)
		if err == nil && !ok {
			ok, err = s.hasRegion(r.Context(), "standard_lots", lawd)
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "query failed")
			return
		}
		if !ok {
			writeErr(w, http.StatusNotFound, "unknown lawd_cd")
			return
		}
	}

	months := recentMonths(time.Now().In(kst), n)
	type monthResult struct {
		items []molitItem
		err   error
	}
	results := make([]monthResult, len(months))
	var wg sync.WaitGroup
	for i, ym := range months {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := s.molitMonth(r.Context(), lawd, ym)
			results[i] = monthResult{items: items, err: err}
		}()
	}
	wg.Wait()

	items := []molitItem{}
	failed := []string{}
	truncated := []string{}
	for i, res := range results {
		if res.err != nil {
			log.Printf("req %s: appraisal/sales/recent month %s failed: %s",
				RequestID(r.Context()), months[i], redactKey(res.err, s.MolitKey))
			failed = append(failed, months[i])
			continue
		}
		if len(res.items) >= molitPageRows {
			truncated = append(truncated, months[i])
		}
		items = append(items, res.items...)
	}
	if len(failed) == len(months) {
		writeErr(w, http.StatusBadGateway, "upstream unavailable")
		return
	}

	// 중심점은 보조 정보다. 실패해도 거래 목록은 돌려준다.
	points, err := s.umdPoints(r.Context(), lawd)
	if err != nil {
		log.Printf("req %s: appraisal/sales/recent umd points failed: %v", RequestID(r.Context()), err)
		points = []umdPoint{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"source":        "rt.molit.go.kr (RTMSDataSvcLandTrade)",
		"lawd_cd":       lawd,
		"months":        months,
		"failed_months": failed,
		// 한 달 거래가 molitPageRows건을 넘으면 첫 페이지만 담긴다. 숨기지 않고 알린다.
		"truncated_months": truncated,
		"total_count":      len(items),
		"items":            items,
		"umd_points":       points,
	})
}

// molitMonth는 (시군구, 거래월) 한 달치 토지 실거래를 캐시 우선으로 가져온다.
func (s *Server) molitMonth(ctx context.Context, lawd, ym string) ([]molitItem, error) {
	key := lawd + ":" + ym
	if items, ok := salesCache.get(key, time.Now()); ok {
		return items, nil
	}
	u := fmt.Sprintf("https://apis.data.go.kr/1613000/RTMSDataSvcLandTrade/getRTMSDataSvcLandTrade?serviceKey=%s&LAWD_CD=%s&DEAL_YMD=%s&numOfRows=1000",
		url.QueryEscape(s.MolitKey), lawd, ym)
	body, err := s.fetch(ctx, u)
	if err != nil {
		return nil, err
	}
	var mr molitResp
	if err := xml.Unmarshal(body, &mr); err != nil {
		return nil, fmt.Errorf("molit xml: %w", err)
	}
	if mr.Header.ResultCode != "000" {
		return nil, fmt.Errorf("molit resultCode=%q msg=%q", mr.Header.ResultCode, mr.Header.ResultMsg)
	}
	items := mr.Body.Items
	if items == nil {
		items = []molitItem{}
	}
	salesCache.put(key, items, time.Now())
	return items, nil
}

// umdPoints는 시군구(5자리) 아래 법정동마다 대표점 하나를 돌려준다. 대표점은 그 법정동
// 표준지 좌표의 평균이다(법정동 폴리곤 미적재). 같은 표준지가 여러 해 있으면 최신 해 하나만
// 세어 특정 필지에 가중이 걸리지 않게 한다. standard_lots PK(pnu, base_year)의 pnu
// 범위 조건으로 인덱스를 탄다(hasRegion과 같은 이유). 시군구 자체 행(xxxxx00000)은 표준지가
// 없어 자연히 빠진다. 이름은 admin_areas 원문("시도 시군구 읍면동")이고, 프론트가 MOLIT
// umdNm과 접미 일치로 맞춘다.
func (s *Server) umdPoints(ctx context.Context, lawd string) ([]umdPoint, error) {
	if s.Pool == nil {
		return []umdPoint{}, nil
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT a.name, s.lng, s.lat
		   FROM (SELECT substr(pnu, 1, 10) AS bcode,
		                avg(ST_X(geom)) AS lng, avg(ST_Y(geom)) AS lat
		           FROM (SELECT DISTINCT ON (pnu) pnu, geom
		                   FROM standard_lots
		                  WHERE pnu >= $1 AND pnu <= $2
		                  ORDER BY pnu, base_year DESC) latest
		          GROUP BY 1) s
		   JOIN admin_areas a ON a.bcode = s.bcode
		  ORDER BY s.bcode
		  LIMIT $3`,
		lawd+"00000000000000", lawd+"99999999999999", umdPointsLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []umdPoint{}
	for rows.Next() {
		var p umdPoint
		if err := rows.Scan(&p.Name, &p.Lng, &p.Lat); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
