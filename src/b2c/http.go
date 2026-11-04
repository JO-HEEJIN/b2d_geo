package b2c

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
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
	register  *BuildingRegisterSource
	dongs     *LegalDongResolver
	locator   *ComplexLocator
	autoPlans *dailyCap
	building  inflight
	listings  *ListingStore
	live      *listingLive
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
		register:  &BuildingRegisterSource{Key: molitKey},
		dongs:     &LegalDongResolver{Key: molitKey},
		locator:   NewComplexLocator(pool),
		autoPlans: &dailyCap{max: autoFloorPlansPerDay},
		listings:  NewListingStore(pool),
		live:      &listingLive{pool: pool},
		mux:       http.NewServeMux(),
	}
	h.mux.HandleFunc("GET /v1/b2c/keywords", h.keywords)
	h.mux.HandleFunc("GET /v1/b2c/districts", h.zones)
	h.mux.HandleFunc("GET /v1/b2c/district", h.district)
	h.mux.HandleFunc("GET /v1/b2c/trades", h.recentTrades)
	h.mux.HandleFunc("GET /v1/b2c/infra", h.infraSummary)
	h.mux.HandleFunc("GET /v1/b2c/zones/{key}/complexes", h.zoneComplexes)
	h.mux.HandleFunc("GET /v1/b2c/complex", h.complexDetail)
	h.mux.HandleFunc("GET /v1/b2c/floor-plans", h.floorPlans)
	h.mux.HandleFunc("GET /v1/b2c/listings", h.remainingListings)
	h.mux.HandleFunc("GET /v1/b2c/listings/stream", h.listingStream)
	h.mux.HandleFunc("POST /v1/b2c/write/floor-plans", h.saveFloorPlan)
	h.mux.HandleFunc("POST /v1/b2c/write/floor-plans/from-register", h.floorPlansFromRegister)
	h.mux.HandleFunc("POST /v1/b2c/write/listings", h.registerListing)
	h.mux.HandleFunc("POST /v1/b2c/write/listings/import", h.importListings)
	h.mux.HandleFunc("GET /v1/b2c/write/listings/mine", h.myListings)
	h.mux.HandleFunc("POST /v1/b2c/write/listings/{id}/close", h.closeListing)
	h.mux.HandleFunc("PATCH /v1/b2c/write/listings/{id}/price", h.updateListingPrice)
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
	status := "ready"
	if len(variants) == 0 {
		status = "none"
		if h.building.running(id) {
			status = "building"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"complex_id": id, "status": status, "variants": variants, "tour": tour, "checklists": checklists})
}

// remainingListings: ?complex=&limit=&offset=  남은 매물(페이지)과 최근 7일 팔린 매물.
func (h *Handler) remainingListings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	complexName := q.Get("complex")
	limit, offset, ok := pageParams(q.Get("limit"), q.Get("offset"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "limit(1-200) and offset(>=0) must be integers")
		return
	}
	page, err := h.listings.Page(r.Context(), complexName, limit, offset)
	if err != nil {
		log.Printf("b2c listings: %v", err)
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	sold, err := h.listings.Sold(r.Context(), complexName, time.Now().Add(-goneKeep))
	if err != nil {
		log.Printf("b2c listings sold: %v", err)
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	if sold == nil {
		sold = []Listing{}
	}
	statuses, err := h.listings.NaverSyncStatuses(r.Context(), complexName)
	if err != nil {
		log.Printf("b2c Naver status: %v", err)
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	changes, err := h.listings.NaverRecentChanges(r.Context(), complexName)
	if err != nil {
		log.Printf("b2c Naver changes: %v", err)
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"complex": complexName, "remaining": page, "recently_sold": sold, "naver_sync": statuses, "naver_changes": changes})
}

// pageParams는 limit, offset 쿼리를 읽는다. 비어 있으면 0(기본값 적용)이다.
func pageParams(limitStr, offsetStr string) (int, int, bool) {
	limit, offset := 0, 0
	var err error
	if limitStr != "" {
		if limit, err = strconv.Atoi(limitStr); err != nil || limit < 1 || limit > listingPageMax {
			return 0, 0, false
		}
	}
	if offsetStr != "" {
		if offset, err = strconv.Atoi(offsetStr); err != nil || offset < 0 {
			return 0, 0, false
		}
	}
	return limit, offset, true
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

// floorPlansFromRegister는 건축물대장 전유부로 단지의 평면 후보를 만들어 저장한다.
// 단지 하나가 data.go.kr 수십 번 호출이라 등록 권한(b2c_write)이 있는 키만 부른다.
func (h *Handler) floorPlansFromRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SigunguCd string `json:"sigungu_cd"`
		BjdongCd  string `json:"bjdong_cd"`
		Bun       string `json:"bun"`
		Ji        string `json:"ji"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if len(body.SigunguCd) != 5 || len(body.BjdongCd) != 5 || len(body.Bun) != 4 || len(body.Ji) != 4 ||
		!digits(body.SigunguCd+body.BjdongCd+body.Bun+body.Ji) {
		writeErr(w, http.StatusBadRequest, "sigungu_cd(5), bjdong_cd(5), bun(4), ji(4) digits required")
		return
	}
	if h.register.Key == "" {
		writeErr(w, http.StatusServiceUnavailable, "data.go.kr key not set: building register disabled")
		return
	}
	units, err := h.register.Units(r.Context(), body.SigunguCd, body.BjdongCd, body.Bun, body.Ji)
	if err != nil {
		log.Printf("b2c building register: %v", err)
		writeErr(w, http.StatusBadGateway, "upstream unavailable")
		return
	}
	id := ParcelComplexID(body.SigunguCd, body.BjdongCd, body.Bun, body.Ji)
	candidates := NewFloorPlanCatalog().CandidatesFromUnits(id, units)
	inserted, err := h.plans.SaveCandidates(r.Context(), candidates)
	if err != nil {
		log.Printf("b2c save candidates: %v", err)
		writeErr(w, http.StatusInternalServerError, "save failed")
		return
	}
	name := ""
	if len(units) > 0 {
		name = units[0].BuildingName
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"complex_id": id, "building_name": name, "units": len(units),
		"candidates": len(candidates), "inserted": inserted,
		"source": "건축HUB 건축물대장 전유공용면적 (BldRgstHubService)",
	})
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

// importListings는 사람이 정리한 매물 CSV(본문, 1MB 이하)를 한 번에 등록한다.
// 잘못된 행이 하나라도 있으면 아무것도 넣지 않고 줄 번호와 이유를 돌려준다.
func (h *Handler) importListings(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "CSV body must be 1MB or less")
		return
	}
	rows, bad, err := ParseListingsCSV(data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(bad) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid rows", "rows": bad, "inserted": 0})
		return
	}
	n, err := h.listings.RegisterMany(r.Context(), rows, registrant(r.Context()))
	if err != nil {
		log.Printf("b2c import listings: %v", err)
		writeErr(w, http.StatusInternalServerError, "save failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"inserted": n})
}

// myListings는 등록 화면의 "내 매물" 목록이다. b2c_write 키의 등록자 본인 것만 준다.
func (h *Handler) myListings(w http.ResponseWriter, r *http.Request) {
	list, err := h.listings.Mine(r.Context(), registrant(r.Context()))
	if err != nil {
		log.Printf("b2c my listings: %v", err)
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"listings": list})
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

// autoFloorPlansPerDay는 단지 상세를 열 때 건축물대장에서 평면 후보를 새로 만드는 하루 최대 단지 수다.
// 한 단지가 data.go.kr 수십 번 호출(실측 49번)이라 공개 데모 키로 일일 한도를 다 쓰지 않게 막는다.
const autoFloorPlansPerDay = 40

// dailyCap은 KST 하루 단위로 횟수를 센다.
type dailyCap struct {
	mu  sync.Mutex
	max int
	day string
	n   int
}

func (c *dailyCap) take() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	today := time.Now().In(kstZone).Format("2006-01-02")
	if c.day != today {
		c.day, c.n = today, 0
	}
	if c.n >= c.max {
		return false
	}
	c.n++
	return true
}

// zoneComplexes: 구역의 6개월 실거래를 단지별로 묶어 거래 수 순으로 전부 돌려준다. 위치를 찾은 단지는 좌표를 붙인다.
func (h *Handler) zoneComplexes(w http.ResponseWriter, r *http.Request) {
	z, ok := h.districts.Zone(r.PathValue("key"))
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown zone")
		return
	}
	if h.trades.source == nil {
		writeErr(w, http.StatusServiceUnavailable, "MOLIT_API_KEY not set: recent trades disabled")
		return
	}
	var trades []AptTrade
	for _, lawd := range z.Lawd {
		t, err := h.trades.Fetch(r.Context(), lawd)
		if err != nil {
			log.Printf("b2c zone %s trades %s: %v", z.Key, lawd, err)
			writeErr(w, http.StatusBadGateway, "upstream unavailable")
			return
		}
		trades = append(trades, t...)
	}
	complexes := h.trades.Complexes(trades, func(t AptTrade) bool { return z.InZone(t.UmdNm) })
	h.attachPlaces(r.Context(), z, complexes)
	writeJSON(w, http.StatusOK, map[string]any{
		"zone": z, "complexes": complexes, "disclaimer": reportingLagDisclaimer,
		"source": "rt.molit.go.kr (RTMSDataSvcAptTrade)",
	})
}

// attachPlaces는 단지마다 법정동코드 -> PNU -> parcels 대표점을 찾아 붙인다. 못 찾으면 비워 둔다.
func (h *Handler) attachPlaces(ctx context.Context, z Zone, complexes []ComplexSummary) {
	codes, err := h.dongs.Codes(ctx, z.Region)
	if err != nil {
		log.Printf("b2c legal dong codes %s: %v", z.Region, err)
		return
	}
	pnus := make([]string, len(complexes))
	var all []string
	for i, c := range complexes {
		if bcode, ok := codes[c.SggCd+"|"+c.UmdNm]; ok {
			if pnu, ok := PNU(bcode, c.Jibun); ok {
				pnus[i] = pnu
				all = append(all, pnu)
			}
		}
	}
	places, err := h.locator.Locate(ctx, all)
	if err != nil {
		log.Printf("b2c locate complexes: %v", err)
		return
	}
	for i := range complexes {
		if p, ok := places[pnus[i]]; ok {
			complexes[i].Place = &p
		}
	}
}

// complexDetail: ?zone=&sgg=&umd=&jibun=&name=  한 단지의 판단, 최근 거래, 도보시간, 평면, 매물을 한 번에 준다.
// 평면이 비어 있으면 건축물대장에서 후보를 만든다(하루 autoFloorPlansPerDay 단지까지).
func (h *Handler) complexDetail(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	z, ok := h.districts.Zone(q.Get("zone"))
	sgg, umd, jibun, name := q.Get("sgg"), q.Get("umd"), q.Get("jibun"), q.Get("name")
	if !ok || len(sgg) != 5 || !digits(sgg) || umd == "" || name == "" {
		writeErr(w, http.StatusBadRequest, "zone, sgg(5), umd, jibun, name required")
		return
	}
	if h.trades.source == nil {
		writeErr(w, http.StatusServiceUnavailable, "MOLIT_API_KEY not set: recent trades disabled")
		return
	}
	all, err := h.trades.Fetch(r.Context(), sgg)
	if err != nil {
		log.Printf("b2c complex trades %s: %v", sgg, err)
		writeErr(w, http.StatusBadGateway, "upstream unavailable")
		return
	}
	var mine []AptTrade
	for _, t := range all {
		if t.ComplexName == name && t.UmdNm == umd && t.Jibun == jibun {
			mine = append(mine, t)
		}
	}
	sort.Slice(mine, func(i, j int) bool { return mine[i].DealDate.After(mine[j].DealDate) })
	summary := ComplexSummary{Name: name, SggCd: sgg, UmdNm: umd, Jibun: jibun}
	if cs := h.trades.Complexes(mine, nil); len(cs) == 1 {
		summary = cs[0]
	}
	one := []ComplexSummary{summary}
	h.attachPlaces(r.Context(), z, one)
	summary = one[0]
	resp := map[string]any{
		"complex": summary,
		"verdict": h.trades.Verdict(summary.Activity, summary.BuildYear),
		"trades":  firstN(mine, 30),
	}

	if p := summary.Place; p != nil {
		if lines, err := h.infra.Summary(r.Context(), p.Lon, p.Lat); err == nil {
			resp["infra"] = lines
		} else {
			log.Printf("b2c complex infra: %v", err)
		}
		catalog, err := h.plans.Catalog(r.Context(), p.ComplexID)
		if err == nil {
			variants := catalog.Variants(p.ComplexID)
			status := "ready"
			if len(variants) == 0 {
				status = h.startFloorPlans(*p)
				variants = []FloorPlanVariant{}
			}
			checklists := map[string][]string{}
			for _, v := range variants {
				checklists[v.TypeName] = catalog.ViewingChecklist(v)
			}
			resp["floor_plans"] = map[string]any{"complex_id": p.ComplexID, "status": status, "variants": variants, "checklists": checklists}
		} else {
			log.Printf("b2c complex floor plans: %v", err)
		}
	}

	if page, err := h.listings.Page(r.Context(), name, listingPageDefault, 0); err == nil {
		resp["listings"] = page
	}
	if sold, err := h.listings.Sold(r.Context(), name, time.Now().Add(-goneKeep)); err == nil {
		if sold == nil {
			sold = []Listing{}
		}
		resp["recently_sold"] = sold
	}
	writeJSON(w, http.StatusOK, resp)
}

// inflight는 지금 백그라운드에서 만들고 있는 단지 ID 집합이다.
type inflight struct {
	mu  sync.Mutex
	ids map[string]bool
}

func (f *inflight) start(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ids == nil {
		f.ids = map[string]bool{}
	}
	if f.ids[id] {
		return false
	}
	f.ids[id] = true
	return true
}

func (f *inflight) done(id string) {
	f.mu.Lock()
	delete(f.ids, id)
	f.mu.Unlock()
}

func (f *inflight) running(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ids[id]
}

// startFloorPlans는 평면 후보가 없는 단지의 건축물대장 조회를 백그라운드로 시작하고 상태를 돌려준다.
// "building": 만드는 중(화면이 /v1/b2c/floor-plans를 다시 부른다), "none": 키가 없거나 오늘 한도를 다 썼다.
// 건축물대장은 한 단지에 수십 페이지라 첫 요청이 30초 걸렸다(실측). 응답을 붙잡지 않으려고 분리한다.
func (h *Handler) startFloorPlans(p ComplexPlace) string {
	if h.building.running(p.ComplexID) {
		return "building"
	}
	if h.register.Key == "" || !h.autoPlans.take() {
		return "none"
	}
	if !h.building.start(p.ComplexID) {
		return "building"
	}
	go func() {
		defer h.building.done(p.ComplexID)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		units, err := h.register.Units(ctx, p.PNU[:5], p.PNU[5:10], p.PNU[11:15], p.PNU[15:19])
		if err != nil {
			log.Printf("b2c auto floor plans %s: %v", p.ComplexID, err)
			return
		}
		if _, err := h.plans.SaveCandidates(ctx, NewFloorPlanCatalog().CandidatesFromUnits(p.ComplexID, units)); err != nil {
			log.Printf("b2c auto floor plans save %s: %v", p.ComplexID, err)
		}
	}()
	return "building"
}

func firstN(t []AptTrade, n int) []AptTrade {
	if len(t) > n {
		return t[:n]
	}
	if t == nil {
		return []AptTrade{}
	}
	return t
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
