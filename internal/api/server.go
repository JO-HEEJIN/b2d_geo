// Package api는 HTTP 핸들러를 제공한다. 이 파일은 부동산 특화 최소 서버로,
// 인증/rate limit/사용량 계측 미들웨어는 Phase 4에서 추가한다.
// 절대 경계(Phase 7 지시서): 사실 반환만 — 보정/평가/추천 로직 금지.
package api

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JO-HEEJIN/b2d_geo/internal/auth"
	"github.com/JO-HEEJIN/b2d_geo/internal/geocode"
	"github.com/JO-HEEJIN/b2d_geo/internal/meter"
	"github.com/JO-HEEJIN/b2d_geo/internal/parcel"
	"github.com/JO-HEEJIN/b2d_geo/internal/places"
	"github.com/JO-HEEJIN/b2d_geo/internal/revgeo"
)

// Server는 최소 API 서버다.
type Server struct {
	Pool     *pgxpool.Pool
	MolitKey string // 토지매매 실거래 (data.go.kr)
	Client   *http.Client
}

// New는 라우팅이 구성된 http.Handler와, 종료 시 계측기를 드레인하는
// close 함수를 돌려준다.
func New(pool *pgxpool.Pool, molitKey string) (http.Handler, func()) {
	s := &Server{Pool: pool, MolitKey: molitKey,
		Client: &http.Client{Timeout: 15 * time.Second}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.health)
	mux.HandleFunc("GET /v1/geocode", s.geocodeHandler)
	mux.HandleFunc("GET /v1/reverse", s.reverseHandler)
	mux.HandleFunc("GET /v1/places/search", s.placesSearchHandler)
	mux.HandleFunc("GET /v1/parcel/by-point", s.parcelByPoint)
	mux.HandleFunc("GET /v1/parcel/by-jibun", s.parcelByJibun)
	mux.HandleFunc("GET /v1/parcel/{pnu}", s.parcelByPNU)
	mux.HandleFunc("GET /v1/appraisal/sales", s.appraisalSales)
	mux.HandleFunc("GET /v1/appraisal/price-index", s.appraisalPriceIndex)
	mux.HandleFunc("GET /v1/appraisal/standard-lots", s.appraisalStandardLots)
	mux.HandleFunc("GET /v1/appraisal/official-price", s.appraisalOfficialPrice)

	// Phase 4 미들웨어 체인: IP rate limit → auth → key rate limit → meter
	// → handler. IP 제한이 최외곽이라 미인증 플러드도 막는다 (QA MED-1).
	// (/v1/health는 auth·rate limit 미들웨어 내부에서 제외한다.)
	authr := auth.NewAuthenticator(pool)
	limiter := auth.NewRateLimiter()
	m := meter.New(pool)
	h := limiter.IPMiddleware(authr.Middleware(limiter.Middleware(m.Middleware(mux))))
	return h, m.Close
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.Pool.Ping(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "db unreachable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func wantGeom(r *http.Request) bool { return r.URL.Query().Get("geometry") == "true" }

func (s *Server) respondParcel(w http.ResponseWriter, p *parcel.Parcel, err error) {
	switch {
	case err == pgx.ErrNoRows:
		writeErr(w, http.StatusNotFound, "parcel not found")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "query failed")
	default:
		writeJSON(w, http.StatusOK, p)
	}
}

func (s *Server) parcelByPNU(w http.ResponseWriter, r *http.Request) {
	pnu := r.PathValue("pnu")
	if len(pnu) != 19 {
		writeErr(w, http.StatusBadRequest, "pnu must be 19 digits")
		return
	}
	p, err := parcel.ByPNU(r.Context(), s.Pool, pnu, wantGeom(r))
	s.respondParcel(w, p, err)
}

func (s *Server) parcelByPoint(w http.ResponseWriter, r *http.Request) {
	lon, err1 := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	lat, err2 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	if err1 != nil || err2 != nil {
		writeErr(w, http.StatusBadRequest, "lon, lat required")
		return
	}
	p, err := parcel.ByPoint(r.Context(), s.Pool, lon, lat, wantGeom(r))
	s.respondParcel(w, p, err)
}

func (s *Server) parcelByJibun(w http.ResponseWriter, r *http.Request) {
	bcode, jibun := r.URL.Query().Get("bcode"), r.URL.Query().Get("jibun")
	if len(bcode) != 10 || jibun == "" {
		writeErr(w, http.StatusBadRequest, "bcode(10 digits), jibun required")
		return
	}
	p, err := parcel.ByJibun(r.Context(), s.Pool, bcode, jibun,
		r.URL.Query().Get("san") == "true", wantGeom(r))
	s.respondParcel(w, p, err)
}

// geocodeHandler는 도로명주소 정확 매칭 v0 (Phase 2에서 파서/폴백 확장).
func (s *Server) geocodeHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) < 5 {
		writeErr(w, http.StatusBadRequest, "q (road address) required")
		return
	}
	res, err := geocode.Search(r.Context(), s.Pool, q)
	switch {
	case err == geocode.ErrAmbiguous:
		// point 필드 제외: 후보 미확정 상태에 [0,0] 좌표를 내보내지 않는다
		// (환각 좌표 금지 — QA MED-2)
		writeJSON(w, http.StatusOK, map[string]any{
			"match": "ambiguous", "candidates": res.Candidates, "warnings": res.Warnings,
		})
	case err == pgx.ErrNoRows:
		writeErr(w, http.StatusNotFound, "no match")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "query failed")
	default:
		writeJSON(w, http.StatusOK, res)
	}
}

// reverseHandler는 좌표 → 최근접 건물 주소 (v0: 반경 200m KNN).
func (s *Server) reverseHandler(w http.ResponseWriter, r *http.Request) {
	lon, err1 := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	lat, err2 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	if err1 != nil || err2 != nil {
		writeErr(w, http.StatusBadRequest, "lon, lat required")
		return
	}
	res, err := revgeo.ByPoint(r.Context(), s.Pool, lon, lat, 200)
	switch {
	case err == pgx.ErrNoRows:
		writeErr(w, http.StatusNotFound, "no building within 200m")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "query failed")
	default:
		writeJSON(w, http.StatusOK, res)
	}
}

// placesSearchHandler는 상호명 키워드 검색 (v0: trigram).
func (s *Server) placesSearchHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		writeErr(w, http.StatusBadRequest, "q required (2+ chars)")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	out, err := places.Search(r.Context(), s.Pool, q, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(out), "items": out})
}

// ---- 실거래가 (MOLIT RTMSDataSvcLandTrade 사실 반환) ----

type molitItem struct {
	DealYear   string `xml:"dealYear" json:"deal_year"`
	DealMonth  string `xml:"dealMonth" json:"deal_month"`
	DealDay    string `xml:"dealDay" json:"deal_day"`
	DealAmount string `xml:"dealAmount" json:"deal_amount_manwon"`
	DealArea   string `xml:"dealArea" json:"deal_area_m2"`
	Jimok      string `xml:"jimok" json:"jimok"`
	LandUse    string `xml:"landUse" json:"land_use"`
	SggCd      string `xml:"sggCd" json:"sgg_cd"`
	SggNm      string `xml:"sggNm" json:"sgg_nm"`
	UmdNm      string `xml:"umdNm" json:"umd_nm"`
	Jibun      string `xml:"jibun" json:"jibun_masked"`
	DealingGbn string `xml:"dealingGbn" json:"dealing_gbn"`
	ShareGbn   string `xml:"shareDealingType" json:"share_dealing_type"`
}

type molitResp struct {
	Header struct {
		ResultCode string `xml:"resultCode"`
		ResultMsg  string `xml:"resultMsg"`
	} `xml:"header"`
	Body struct {
		Items      []molitItem `xml:"items>item"`
		TotalCount int         `xml:"totalCount"`
	} `xml:"body"`
}

func (s *Server) appraisalSales(w http.ResponseWriter, r *http.Request) {
	lawd, ymd := r.URL.Query().Get("lawd_cd"), r.URL.Query().Get("deal_ymd")
	if !allDigits(lawd) || !allDigits(ymd) || len(lawd) != 5 || len(ymd) != 6 {
		writeErr(w, http.StatusBadRequest, "lawd_cd(5), deal_ymd(YYYYMM) required")
		return
	}
	u := fmt.Sprintf("https://apis.data.go.kr/1613000/RTMSDataSvcLandTrade/getRTMSDataSvcLandTrade?serviceKey=%s&LAWD_CD=%s&DEAL_YMD=%s&numOfRows=1000",
		url.QueryEscape(s.MolitKey), lawd, ymd)
	body, err := s.fetch(r.Context(), u)
	if err != nil {
		log.Printf("appraisal/sales upstream error: %v", redactKey(err, s.MolitKey))
		writeErr(w, http.StatusBadGateway, "upstream unavailable")
		return
	}
	var mr molitResp
	if err := xml.Unmarshal(body, &mr); err != nil || mr.Header.ResultCode != "000" {
		writeErr(w, http.StatusBadGateway, "upstream molit response invalid")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source": "rt.molit.go.kr (RTMSDataSvcLandTrade)", "lawd_cd": lawd,
		"deal_ymd": ymd, "total_count": mr.Body.TotalCount, "items": mr.Body.Items,
	})
}

// ---- 지가변동률 (land_price_index = R-ONE 용도지역별 월, 사실 반환) ----

// appraisalPriceIndex는 land_price_index에서 (시군구×용도지역) 월별 변동률을 그대로
// 돌려준다. cumulative=true면 시점수정용 누적률((1+r/100)의 곱)을 산식·기간과 함께 병기한다
// (지시서 허용 계산). 평가·판단·보정 없음.
func (s *Server) appraisalPriceIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sigungu, useZone := q.Get("sigungu"), q.Get("use_zone_class")
	from, to := q.Get("from"), q.Get("to")
	if len(sigungu) != 5 || !allDigits(sigungu) {
		writeErr(w, http.StatusBadRequest, "sigungu(5자리) required")
		return
	}
	if useZone == "" {
		writeErr(w, http.StatusBadRequest, "use_zone_class required")
		return
	}
	if (from != "" && (!allDigits(from) || len(from) != 6)) || (to != "" && (!allDigits(to) || len(to) != 6)) {
		writeErr(w, http.StatusBadRequest, "from/to must be YYYYMM")
		return
	}
	rows, err := s.Pool.Query(r.Context(),
		`SELECT month, rate_pct::float8 FROM land_price_index
		 WHERE sigungu_code=$1 AND use_zone_class=$2
		   AND ($3='' OR month >= $3) AND ($4='' OR month <= $4)
		 ORDER BY month`, sigungu, useZone, from, to)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	type point struct {
		Month string  `json:"month"`
		Rate  float64 `json:"rate_pct"`
	}
	series := []point{}
	for rows.Next() {
		var p point
		if err := rows.Scan(&p.Month, &p.Rate); err != nil {
			writeErr(w, http.StatusInternalServerError, "scan failed")
			return
		}
		series = append(series, p)
	}
	if rows.Err() != nil {
		writeErr(w, http.StatusInternalServerError, "read failed")
		return
	}
	resp := map[string]any{
		"source":         "R-ONE 용도지역별 지가변동률 (월)",
		"sigungu_code":   sigungu,
		"use_zone_class": useZone,
		"count":          len(series),
		"series":         series,
	}
	if q.Get("cumulative") == "true" && len(series) > 0 {
		prod := 1.0
		for _, p := range series {
			prod *= 1 + p.Rate/100
		}
		resp["cumulative_pct"] = (prod - 1) * 100
		resp["cumulative_formula"] = "∏(1 + rate_pct/100) - 1, 단위 %"
		resp["period"] = map[string]string{"from": series[0].Month, "to": series[len(series)-1].Month}
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- 비교표준지 검색 (standard_lots, 앵커 반경 내 거리순 사실 반환) ----

// appraisalStandardLots는 앵커(pnu 또는 lat/lng) 반경 내 표준지를 거리 오름차순으로
// 돌려준다. 정렬·필터는 객관 지표(거리)만 — 유사도/추천/격차 점수 없음(지시서 절대경계).
func (s *Server) appraisalStandardLots(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pnu := q.Get("pnu")
	var alon, alat float64
	if pnu != "" {
		if len(pnu) != 19 || !allDigits(pnu) {
			writeErr(w, http.StatusBadRequest, "pnu must be 19 digits")
			return
		}
		err := s.Pool.QueryRow(r.Context(),
			`SELECT ST_X(p), ST_Y(p) FROM (SELECT ST_PointOnSurface(geom) p FROM parcels WHERE pnu=$1) t`,
			pnu).Scan(&alon, &alat)
		if err == pgx.ErrNoRows {
			writeErr(w, http.StatusNotFound, "pnu not found in parcels")
			return
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "anchor lookup failed")
			return
		}
	} else {
		var e1, e2 error
		alat, e1 = strconv.ParseFloat(q.Get("lat"), 64)
		alon, e2 = strconv.ParseFloat(q.Get("lng"), 64)
		if e1 != nil || e2 != nil {
			writeErr(w, http.StatusBadRequest, "pnu, or lat+lng required")
			return
		}
	}
	radius := clampInt(atoiDefault(q.Get("radius"), 2000), 1, 10000)
	limit := clampInt(atoiDefault(q.Get("limit"), 20), 1, 50)
	baseYear := atoiDefault(q.Get("base_year"), 0)

	rows, err := s.Pool.Query(r.Context(), `
		SELECT pnu,
		       round(ST_Distance(geom::geography, ST_SetSRID(ST_MakePoint($1,$2),4326)::geography)::numeric, 1)::float8 AS dist,
		       price_per_sqm, land_category, use_zone, land_use, road_side, terrain_shape,
		       ST_Y(geom), ST_X(geom)
		FROM standard_lots
		WHERE geom && ST_Expand(ST_SetSRID(ST_MakePoint($1,$2),4326), $3/88000.0)
		  AND ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($1,$2),4326)::geography, $3)
		  AND ($4 = 0 OR base_year = $4)
		  AND ($5 = '' OR use_zone = $5)
		  AND ($6 = '' OR land_category = $6)
		  AND ($7 = '' OR land_use = $7)
		ORDER BY dist ASC
		LIMIT $8`,
		alon, alat, radius, baseYear, q.Get("use_zone"), q.Get("land_category"), q.Get("land_use"), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	type lot struct {
		PNU      string  `json:"pnu"`
		DistM    float64 `json:"distance_m"`
		Price    int64   `json:"price_per_sqm"`
		LandCat  *string `json:"land_category"`
		UseZone  *string `json:"use_zone"`
		LandUse  *string `json:"land_use"`
		RoadSide *string `json:"road_side"`
		Shape    *string `json:"terrain_shape"`
		Lat      float64 `json:"lat"`
		Lng      float64 `json:"lng"`
	}
	out := []lot{}
	for rows.Next() {
		var l lot
		if err := rows.Scan(&l.PNU, &l.DistM, &l.Price, &l.LandCat, &l.UseZone,
			&l.LandUse, &l.RoadSide, &l.Shape, &l.Lat, &l.Lng); err != nil {
			writeErr(w, http.StatusInternalServerError, "scan failed")
			return
		}
		out = append(out, l)
	}
	if rows.Err() != nil {
		writeErr(w, http.StatusInternalServerError, "read failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source":        "표준지공시지가 (MOLIT/NSDI)",
		"anchor":        map[string]float64{"lat": alat, "lng": alon},
		"radius_m":      radius,
		"count":         len(out),
		"standard_lots": out,
	})
}

// ---- 공시지가 (개별 우선, 없으면 표준지, 사실 반환) ----

// appraisalOfficialPrice는 PNU의 공시지가를 돌려준다. 개별공시지가(official_land_prices)가
// 있으면 그것(INDIVIDUAL), 없으면 표준지공시지가(standard_lots, STANDARD)를 반환한다.
// base_year 미지정 시 최신 연도. 사실만 — 보정·계산 없음.
func (s *Server) appraisalOfficialPrice(w http.ResponseWriter, r *http.Request) {
	pnu := r.URL.Query().Get("pnu")
	if len(pnu) != 19 || !allDigits(pnu) {
		writeErr(w, http.StatusBadRequest, "pnu must be 19 digits")
		return
	}
	byr := atoiDefault(r.URL.Query().Get("base_year"), 0) // 0 = 최신

	var price int64
	var year int
	var ptype string
	// 개별공시지가 우선
	err := s.Pool.QueryRow(r.Context(),
		`SELECT price_per_sqm, base_year FROM official_land_prices
		 WHERE pnu=$1 AND ($2=0 OR base_year=$2) ORDER BY base_year DESC LIMIT 1`,
		pnu, byr).Scan(&price, &year)
	switch {
	case err == nil:
		ptype = "INDIVIDUAL"
	case err == pgx.ErrNoRows:
		// 표준지공시지가 폴백
		err = s.Pool.QueryRow(r.Context(),
			`SELECT price_per_sqm, base_year FROM standard_lots
			 WHERE pnu=$1 AND ($2=0 OR base_year=$2) ORDER BY base_year DESC LIMIT 1`,
			pnu, byr).Scan(&price, &year)
		switch {
		case err == nil:
			ptype = "STANDARD"
		case err == pgx.ErrNoRows:
			writeErr(w, http.StatusNotFound, "no official price for this pnu")
			return
		default:
			writeErr(w, http.StatusInternalServerError, "query failed")
			return
		}
	default:
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pnu":           pnu,
		"price_per_sqm": price,
		"price_type":    ptype, // STANDARD(표준지) | INDIVIDUAL(개별)
		"base_year":     year,
	})
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return def
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

// redactKey는 에러 문자열에서 API 키(원문/URL인코딩)를 가린다.
func redactKey(err error, key string) string {
	msg := err.Error()
	if key == "" {
		return msg // 빈 키면 ReplaceAll이 모든 문자 사이에 삽입되므로 원문 반환
	}
	msg = strings.ReplaceAll(msg, key, "***")
	msg = strings.ReplaceAll(msg, url.QueryEscape(key), "***")
	return msg
}

func (s *Server) fetch(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	// data.go.kr 게이트웨이가 기본 UA를 차단함 (실측) — 명시 UA 필수
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; b2d-geo/0.1)")
	res, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", res.StatusCode)
	}
	return io.ReadAll(res.Body)
}
