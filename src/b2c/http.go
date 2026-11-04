package b2c

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JO-HEEJIN/b2d_geo/internal/auth"
)

// Handler는 B2C 모듈을 /v1/b2c/* HTTP API로 연다. internal/api는 사실만 돌려주는 경계
// (Phase 7)라서, 판단 문구가 들어가는 B2C 응답은 이 패키지에서 만든다. 인증·rate limit·
// 계측은 internal/api의 /v1 체인이 앞에서 처리한다. 쓰기 경로(/v1/b2c/write/*)는
// b2c_write scope가 있는 키만 통과한다(공개 데모 키로 매물을 올리지 못하게).
type Handler struct {
	districts *DistrictClassifier
	trades    *RecentTradeAnalyzer
	infra     *InfraMap
	plans     *FloorPlanStore
	listings  *ListingStore
	mux       *http.ServeMux
}

// NewHandler는 DB와 MOLIT 키로 B2C API 핸들러를 만든다. molitKey가 비면 실거래
// 경로는 503을 준다.
func NewHandler(pool *pgxpool.Pool, molitKey string) *Handler {
	var src TradeSource
	if molitKey != "" {
		src = &cachedTradeSource{next: &MolitAptTradeSource{Key: molitKey}, ttl: 6 * time.Hour, m: map[string]tradeCacheEntry{}}
	}
	h := &Handler{
		districts: NewDistrictClassifier(),
		trades:    NewRecentTradeAnalyzer(src),
		infra:     NewInfraMapFromDB(pool),
		plans:     NewFloorPlanStore(pool),
		listings:  NewListingStore(pool),
		mux:       http.NewServeMux(),
	}
	h.mux.HandleFunc("GET /v1/b2c/keywords", h.keywords)
	h.mux.HandleFunc("GET /v1/b2c/districts", h.zones)
	h.mux.HandleFunc("GET /v1/b2c/district", h.district)
	h.mux.HandleFunc("GET /v1/b2c/trades", h.recentTrades)
	h.mux.HandleFunc("GET /v1/b2c/infra", h.infraSummary)
	h.mux.HandleFunc("GET /v1/b2c/floor-plans", h.floorPlans)
	h.mux.HandleFunc("GET /v1/b2c/listings", h.remainingListings)
	h.mux.HandleFunc("POST /v1/b2c/write/floor-plans", h.saveFloorPlan)
	h.mux.HandleFunc("POST /v1/b2c/write/listings", h.registerListing)
	h.mux.HandleFunc("POST /v1/b2c/write/listings/{id}/close", h.closeListing)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) keywords(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"keywords": Keywords})
}

func (h *Handler) zones(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"zones": h.districts.Zones()})
}

// district: ?address=... 를 먼저 보고, 못 가르면 ?lon=&lat= 으로 가장 가까운 구역을 붙인다.
func (h *Handler) district(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if addr := q.Get("address"); addr != "" {
		if z, ok := h.districts.Classify(addr); ok {
			writeJSON(w, http.StatusOK, map[string]any{"zone": z, "method": "address"})
			return
		}
	}
	lon, lat, ok := lonLat(r)
	if !ok {
		if q.Get("address") != "" {
			writeJSON(w, http.StatusOK, map[string]any{"zone": nil, "method": "address", "note": "지원 구역이 아니거나 주소에 동이 없습니다"})
			return
		}
		writeErr(w, http.StatusBadRequest, "address or lon,lat required")
		return
	}
	z, dist := h.districts.NearestZone(lon, lat)
	writeJSON(w, http.StatusOK, map[string]any{"zone": z, "method": "nearest_center", "distance_m": int(dist)})
}

// recentTrades: ?lawd_cd=41597&complex=동탄역시범&build_year=2015
func (h *Handler) recentTrades(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lawd := q.Get("lawd_cd")
	if len(lawd) != 5 || !digits(lawd) {
		writeErr(w, http.StatusBadRequest, "lawd_cd(5 digits) required")
		return
	}
	if h.trades.source == nil {
		writeErr(w, http.StatusServiceUnavailable, "MOLIT_API_KEY not set: recent trades disabled")
		return
	}
	trades, err := h.trades.Fetch(r.Context(), lawd)
	if err != nil {
		log.Printf("b2c trades %s: %v", lawd, err)
		writeErr(w, http.StatusBadGateway, "upstream unavailable")
		return
	}
	buildYear, _ := strconv.Atoi(q.Get("build_year"))
	act := h.trades.Activity(trades, q.Get("complex"))
	writeJSON(w, http.StatusOK, map[string]any{
		"source":   "rt.molit.go.kr (RTMSDataSvcAptTrade)",
		"lawd_cd":  lawd,
		"complex":  q.Get("complex"),
		"activity": act,
		"verdict":  h.trades.Verdict(act, buildYear),
	})
}

// infraSummary: ?lon=&lat=
func (h *Handler) infraSummary(w http.ResponseWriter, r *http.Request) {
	lon, lat, ok := lonLat(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "lon,lat required")
		return
	}
	lines, err := h.infra.Summary(r.Context(), lon, lat)
	if err != nil {
		log.Printf("b2c infra: %v", err)
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lon": lon, "lat": lat, "walk_basis": "직선거리 x 1.3 / 분당 80m", "lines": lines})
}

// floorPlans: ?complex_id=  모든 타입, 직접 볼 동선, 타입별 체크리스트.
func (h *Handler) floorPlans(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("complex_id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "complex_id required")
		return
	}
	c, err := h.plans.Catalog(r.Context(), id)
	if err != nil {
		log.Printf("b2c floor plans: %v", err)
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	variants, tour := c.Variants(id), c.TourPlan(id)
	if variants == nil {
		variants, tour = []FloorPlanVariant{}, []TourStop{}
	}
	checklists := map[string][]string{}
	for _, v := range variants {
		checklists[v.TypeName] = c.ViewingChecklist(v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"complex_id": id, "variants": variants, "tour": tour, "checklists": checklists})
}

// remainingListings: ?complex=  남은 매물 전부와 최근 7일 팔린 매물.
func (h *Handler) remainingListings(w http.ResponseWriter, r *http.Request) {
	complexName := r.URL.Query().Get("complex")
	all, err := h.listings.Fetch(r.Context())
	if err != nil {
		log.Printf("b2c listings: %v", err)
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	remaining := []Listing{}
	for _, l := range all {
		if complexName == "" || l.ComplexName == complexName {
			remaining = append(remaining, l)
		}
	}
	sortListings(remaining)
	sold, err := h.listings.Sold(r.Context(), complexName, time.Now().Add(-goneKeep))
	if err != nil {
		log.Printf("b2c listings sold: %v", err)
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"complex": complexName, "remaining": remaining, "recently_sold": sold})
}

func (h *Handler) saveFloorPlan(w http.ResponseWriter, r *http.Request) {
	var v FloorPlanVariant
	if !readJSON(w, r, &v) {
		return
	}
	if err := h.plans.Save(r.Context(), v, registrant(r.Context())); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "complex_id": v.ComplexID, "type_name": v.TypeName})
}

func (h *Handler) registerListing(w http.ResponseWriter, r *http.Request) {
	var l Listing
	if !readJSON(w, r, &l) {
		return
	}
	id, err := h.listings.Register(r.Context(), l, registrant(r.Context()))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handler) closeListing(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status string `json:"status"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	err := h.listings.Close(r.Context(), r.PathValue("id"), body.Status, registrant(r.Context()))
	switch {
	case errors.Is(err, ErrListingNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case err != nil:
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id"), "status": body.Status})
	}
}

// registrant는 등록자 식별자다. 인증된 API 키 ID를 쓴다.
func registrant(ctx context.Context) string {
	if id, ok := auth.APIKeyID(ctx); ok {
		return "apikey:" + strconv.FormatInt(id, 10)
	}
	return ""
}

func lonLat(r *http.Request) (float64, float64, bool) {
	lon, errLon := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	lat, errLat := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	if errLon != nil || errLat != nil || lon < 124 || lon > 132 || lat < 33 || lat > 39.5 {
		return 0, 0, false
	}
	return lon, lat, true
}

func digits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

type tradeCacheEntry struct {
	trades []AptTrade
	at     time.Time
}

// cachedTradeSource는 (시군구, 기간) 응답을 ttl 동안 기억한다. data.go.kr 일일 호출 한도를
// 지키고, 같은 동네를 연달아 볼 때 응답을 빠르게 한다. 항목이 256개를 넘으면 비운다.
type cachedTradeSource struct {
	next TradeSource
	ttl  time.Duration
	mu   sync.Mutex
	m    map[string]tradeCacheEntry
}

func (c *cachedTradeSource) Trades(ctx context.Context, lawdCode string, from, to time.Time) ([]AptTrade, error) {
	key := lawdCode + "|" + from.Format("2006-01") + "|" + to.Format("2006-01")
	now := time.Now()
	c.mu.Lock()
	e, ok := c.m[key]
	c.mu.Unlock()
	if ok && now.Sub(e.at) < c.ttl {
		return e.trades, nil
	}
	trades, err := c.next.Trades(ctx, lawdCode, from, to)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if len(c.m) >= 256 {
		c.m = map[string]tradeCacheEntry{}
	}
	c.m[key] = tradeCacheEntry{trades: trades, at: now}
	c.mu.Unlock()
	return trades, nil
}
