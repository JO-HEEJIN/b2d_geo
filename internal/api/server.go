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
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JO-HEEJIN/b2d_geo/internal/parcel"
)

// Server는 최소 API 서버다.
type Server struct {
	Pool     *pgxpool.Pool
	MolitKey string // 토지매매 실거래 (data.go.kr)
	RebKey   string // R-ONE 부동산통계
	Client   *http.Client
}

// New는 라우팅이 구성된 http.Handler를 돌려준다.
func New(pool *pgxpool.Pool, molitKey, rebKey string) http.Handler {
	s := &Server{Pool: pool, MolitKey: molitKey, RebKey: rebKey,
		Client: &http.Client{Timeout: 15 * time.Second}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.health)
	mux.HandleFunc("GET /v1/parcel/by-point", s.parcelByPoint)
	mux.HandleFunc("GET /v1/parcel/by-jibun", s.parcelByJibun)
	mux.HandleFunc("GET /v1/parcel/{pnu}", s.parcelByPNU)
	mux.HandleFunc("GET /v1/appraisal/sales", s.appraisalSales)
	mux.HandleFunc("GET /v1/appraisal/price-index", s.appraisalPriceIndex)
	return mux
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
	p, err := parcel.ByJibun(r.Context(), s.Pool, bcode, jibun, wantGeom(r))
	s.respondParcel(w, p, err)
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
	if len(lawd) != 5 || len(ymd) != 6 {
		writeErr(w, http.StatusBadRequest, "lawd_cd(5), deal_ymd(YYYYMM) required")
		return
	}
	u := fmt.Sprintf("https://apis.data.go.kr/1613000/RTMSDataSvcLandTrade/getRTMSDataSvcLandTrade?serviceKey=%s&LAWD_CD=%s&DEAL_YMD=%s&numOfRows=1000",
		url.QueryEscape(s.MolitKey), lawd, ymd)
	body, err := s.fetch(r.Context(), u)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream molit: "+err.Error())
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

// ---- 지가변동률 (R-ONE A_2024_00903 사실 반환) ----

func (s *Server) appraisalPriceIndex(w http.ResponseWriter, r *http.Request) {
	ym := r.URL.Query().Get("ym")
	if len(ym) != 6 {
		writeErr(w, http.StatusBadRequest, "ym(YYYYMM) required")
		return
	}
	u := fmt.Sprintf("https://www.reb.or.kr/r-one/openapi/SttsApiTblData.do?KEY=%s&Type=json&pIndex=1&pSize=1000&STATBL_ID=A_2024_00903&DTACYCLE_CD=MM&WRTTIME_IDTFR_ID=%s",
		url.QueryEscape(s.RebKey), ym)
	body, err := s.fetch(r.Context(), u)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream r-one: "+err.Error())
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		writeErr(w, http.StatusBadGateway, "upstream r-one response invalid")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source": "R-ONE 전국지가변동률조사 (A_2024_00903, 월)", "ym": ym,
		"data": raw["SttsApiTblData"],
	})
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
