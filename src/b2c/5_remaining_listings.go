package b2c

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/encoding/korean"
)

// Listing은 지금 팔려고 내놓은 매물 1건이다.
type Listing struct {
	ID           string    `json:"id"`
	ComplexName  string    `json:"complex_name"`
	TypeName     string    `json:"type_name,omitempty"` // 평면 타입 (모듈 2와 같은 이름)
	AskingManwon int       `json:"asking_manwon"`       // 호가 (만원)
	Floor        int       `json:"floor,omitempty"`
	Lon          float64   `json:"lon,omitempty"`
	Lat          float64   `json:"lat,omitempty"`
	Source       string    `json:"source"` // 원천 이름. 출처를 항상 남긴다
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
}

// ListingSource는 현재 나와 있는 매물 전체를 돌려준다. 이용 계약이 있는 매물 피드나
// 중개사·집주인이 직접 등록한 매물만 원천이 될 수 있다. 포털 스크레이핑은 금지다.
type ListingSource interface {
	Fetch(ctx context.Context) ([]Listing, error)
}

// ListingChange는 Refresh 한 번에 생긴 변화다.
type ListingChange struct {
	Added []Listing // 새로 나온 매물
	Gone  []Listing // 사라진 매물 (팔렸거나 내려감)
}

// listingsOnCalendar는 systemd 타이머 주기다. 매분 0초에 돈다.
const listingsOnCalendar = "*:*:00"

// goneKeep은 사라진 매물을 "방금 팔림"으로 보여주는 기간이다.
const goneKeep = 7 * 24 * time.Hour

// RemainingListings는 남은 매물을 계속 새로 고쳐 보여준다. 매물이 워낙 빠르게
// 거래되므로 systemd 타이머(Persistent=true)가 Refresh를 매분 부르고, 상태는
// JSON 파일로 실행 사이에 이어진다.
type RemainingListings struct {
	source ListingSource
	now    func() time.Time

	mu     sync.Mutex
	active map[string]Listing
	gone   map[string]Listing
}

// NewRemainingListings는 매물 원천으로 남은 매물 목록을 만든다.
func NewRemainingListings(source ListingSource) *RemainingListings {
	return &RemainingListings{
		source: source,
		now:    time.Now,
		active: map[string]Listing{},
		gone:   map[string]Listing{},
	}
}

// Refresh는 원천에서 현재 매물을 받아 직전 상태와 비교한다. 새 매물은 Added,
// 이번에 안 보이는 매물은 Gone으로 옮긴다. 원천이 실패하면 상태를 건드리지 않는다.
func (r *RemainingListings) Refresh(ctx context.Context) (ListingChange, error) {
	current, err := r.source.Fetch(ctx)
	if err != nil {
		return ListingChange{}, fmt.Errorf("fetch listings: %w", err)
	}
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()

	var change ListingChange
	seen := make(map[string]bool, len(current))
	for _, l := range current {
		seen[l.ID] = true
		if prev, ok := r.active[l.ID]; ok {
			l.FirstSeen = prev.FirstSeen
		} else {
			l.FirstSeen = now
			delete(r.gone, l.ID) // 다시 올라온 매물
			change.Added = append(change.Added, l)
		}
		l.LastSeen = now
		r.active[l.ID] = l
	}
	for id, l := range r.active {
		if !seen[id] {
			delete(r.active, id)
			r.gone[id] = l
			change.Gone = append(change.Gone, l)
		}
	}
	for id, l := range r.gone {
		if now.Sub(l.LastSeen) > goneKeep {
			delete(r.gone, id)
		}
	}
	sortListings(change.Added)
	sortListings(change.Gone)
	return change, nil
}

// Remaining은 지금 남아 있는 매물 전부를 돌려준다. complexName이 비지 않으면 그
// 단지만. 추려서 줄이지 않는다. 여러 집을 둘러보는 것 자체가 즐거움이다.
func (r *RemainingListings) Remaining(complexName string) []Listing {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Listing
	for _, l := range r.active {
		if complexName == "" || l.ComplexName == complexName {
			out = append(out, l)
		}
	}
	sortListings(out)
	return out
}

// AddedSince는 since 이후 새로 나온 매물이다 ("새 매물" 알림용).
func (r *RemainingListings) AddedSince(since time.Time) []Listing {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Listing
	for _, l := range r.active {
		if !l.FirstSeen.Before(since) {
			out = append(out, l)
		}
	}
	sortListings(out)
	return out
}

// RecentlyGone은 since 이후 사라진 매물이다. "방금 팔림"을 보여주면 이 동네가
// 실제로 거래되고 있다는 것을 사용자가 눈으로 느낀다.
func (r *RemainingListings) RecentlyGone(since time.Time) []Listing {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Listing
	for _, l := range r.gone {
		if !l.LastSeen.Before(since) {
			out = append(out, l)
		}
	}
	sortListings(out)
	return out
}

type listingsState struct {
	Active []Listing `json:"active"`
	Gone   []Listing `json:"gone"`
}

// LoadState는 직전 실행이 저장한 상태를 읽는다. 파일이 없으면 빈 상태로 시작한다.
func (r *RemainingListings) LoadState(path string) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var st listingsState
	if err := json.Unmarshal(b, &st); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active, r.gone = map[string]Listing{}, map[string]Listing{}
	for _, l := range st.Active {
		r.active[l.ID] = l
	}
	for _, l := range st.Gone {
		r.gone[l.ID] = l
	}
	return nil
}

// SaveState는 상태를 임시 파일에 쓴 뒤 이름을 바꿔 저장한다. 타이머가 실행 도중
// 끊겨도 반쯤 쓴 파일이 남지 않는다.
func (r *RemainingListings) SaveState(path string) error {
	r.mu.Lock()
	st := listingsState{}
	for _, l := range r.active {
		st.Active = append(st.Active, l)
	}
	for _, l := range r.gone {
		st.Gone = append(st.Gone, l)
	}
	r.mu.Unlock()
	sortListings(st.Active)
	sortListings(st.Gone)
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".listings-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// SystemdUnits는 서버에 둘 systemd 서비스와 타이머 파일 내용을 돌려준다.
// cron이 아니라 타이머를 쓰는 이유: Persistent=true라서 서버가 꺼져 있던 사이
// 놓친 실행을 부팅 직후 바로 따라잡고, 실행 기록이 journalctl에 남는다.
// execStart는 Refresh 한 번을 돌리고 끝나는 명령이다.
func (r *RemainingListings) SystemdUnits(execStart string) map[string]string {
	service := strings.Join([]string{
		"[Unit]",
		"Description=b2c remaining listings refresh",
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=oneshot",
		"ExecStart=" + execStart,
		"TimeoutStartSec=50",
		"",
	}, "\n")
	timer := strings.Join([]string{
		"[Unit]",
		"Description=Refresh b2c remaining listings every minute",
		"",
		"[Timer]",
		"OnCalendar=" + listingsOnCalendar,
		"AccuracySec=1s",
		"Persistent=true",
		"Unit=b2c-listings.service",
		"",
		"[Install]",
		"WantedBy=timers.target",
		"",
	}, "\n")
	return map[string]string{
		"b2c-listings.service": service,
		"b2c-listings.timer":   timer,
	}
}

// sortListings는 단지, 호가, ID 순으로 정렬한다.
func sortListings(list []Listing) {
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.ComplexName != b.ComplexName {
			return a.ComplexName < b.ComplexName
		}
		if a.AskingManwon != b.AskingManwon {
			return a.AskingManwon < b.AskingManwon
		}
		return a.ID < b.ID
	})
}

// DirectListingSourceName은 직접 등록 매물의 Listing.Source 값이다.
const DirectListingSourceName = "direct_registration"

// ListingStore는 중개사·집주인이 직접 등록한 매물을 b2c_listings에 두고,
// RemainingListings가 읽는 ListingSource 역할도 한다.
type ListingStore struct {
	pool *pgxpool.Pool
}

// NewListingStore는 b2c_listings 테이블을 쓰는 매물 저장소를 만든다.
func NewListingStore(pool *pgxpool.Pool) *ListingStore {
	return &ListingStore{pool: pool}
}

// Register는 매물을 등록하고 새 ID를 돌려준다. registeredBy는 등록한 사람(API 키 이름 등)이다.
func (s *ListingStore) Register(ctx context.Context, l Listing, registeredBy string) (string, error) {
	if strings.TrimSpace(l.ComplexName) == "" || l.AskingManwon <= 0 || registeredBy == "" {
		return "", fmt.Errorf("complex_name, asking_manwon(>0), registered_by required")
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO b2c_listings (complex_name, type_name, asking_manwon, floor, lon, lat, registered_by)
		VALUES ($1, NULLIF($2, ''), $3, NULLIF($4, 0), NULLIF($5, 0), NULLIF($6, 0), $7)
		RETURNING id`,
		strings.TrimSpace(l.ComplexName), l.TypeName, l.AskingManwon, l.Floor, l.Lon, l.Lat, registeredBy).Scan(&id)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(id, 10), nil
}

// Close는 매물을 팔림(sold) 또는 내림(withdrawn)으로 바꾼다. 다음 Refresh에서 Gone으로 잡힌다.
// 등록한 사람만 닫을 수 있다.
func (s *ListingStore) Close(ctx context.Context, id, status, registeredBy string) error {
	if status != "sold" && status != "withdrawn" {
		return fmt.Errorf("status must be sold or withdrawn")
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid listing id %q", id)
	}
	ct, err := s.pool.Exec(ctx, `
		UPDATE b2c_listings SET status = $2, updated_at = now()
		WHERE id = $1 AND status = 'active' AND registered_by = $3`, n, status, registeredBy)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrListingNotFound
	}
	return nil
}

// ErrListingNotFound는 닫을 매물이 없거나(이미 닫힘 포함) 등록자가 다를 때다.
var ErrListingNotFound = errors.New("active listing not found for this registrant")

// Fetch는 지금 active인 직접 등록 매물 전부를 돌려준다 (ListingSource 구현).
func (s *ListingStore) Fetch(ctx context.Context) ([]Listing, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, complex_name, COALESCE(type_name, ''), asking_manwon, COALESCE(floor, 0),
		       COALESCE(lon, 0), COALESCE(lat, 0), created_at, updated_at, registered_by
		FROM b2c_listings WHERE status = 'active'`)
	if err != nil {
		return nil, err
	}
	return scanListings(rows)
}

// ListingPage는 남은 매물 목록의 한 페이지다. 전부를 보여주되 한 번에 내려보내는 양만 나눈다.
type ListingPage struct {
	Listings   []Listing `json:"listings"`
	Total      int       `json:"total"`
	Limit      int       `json:"limit"`
	Offset     int       `json:"offset"`
	NextOffset *int      `json:"next_offset"` // 마지막 페이지면 null
}

const (
	listingPageDefault = 50
	listingPageMax     = 200
)

// Page는 active 직접 등록 매물을 단지, 호가, ID 순으로 limit개씩 돌려준다.
// limit이 0 이하면 50, 200을 넘으면 200으로 맞춘다. 정렬이 고정이라 offset이 안정적이다.
func (s *ListingStore) Page(ctx context.Context, complexName string, limit, offset int) (ListingPage, error) {
	if limit <= 0 {
		limit = listingPageDefault
	}
	if limit > listingPageMax {
		limit = listingPageMax
	}
	if offset < 0 {
		offset = 0
	}
	page := ListingPage{Listings: []Listing{}, Limit: limit, Offset: offset}
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM b2c_listings
		WHERE status = 'active' AND ($1 = '' OR complex_name = $1)`, complexName).Scan(&page.Total); err != nil {
		return ListingPage{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, complex_name, COALESCE(type_name, ''), asking_manwon, COALESCE(floor, 0),
		       COALESCE(lon, 0), COALESCE(lat, 0), created_at, updated_at, registered_by
		FROM b2c_listings
		WHERE status = 'active' AND ($1 = '' OR complex_name = $1)
		ORDER BY complex_name, asking_manwon, id
		LIMIT $2 OFFSET $3`, complexName, limit, offset)
	if err != nil {
		return ListingPage{}, err
	}
	list, err := scanListings(rows)
	if err != nil {
		return ListingPage{}, err
	}
	if list != nil {
		page.Listings = list
	}
	if next := offset + len(list); next < page.Total {
		page.NextOffset = &next
	}
	return page, nil
}

// Sold는 since 이후 팔린 직접 등록 매물을 최근 순으로 최대 50건 돌려준다.
// complexName이 비지 않으면 그 단지만. LastSeen은 팔림으로 바뀐 시각이다.
func (s *ListingStore) Sold(ctx context.Context, complexName string, since time.Time) ([]Listing, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, complex_name, COALESCE(type_name, ''), asking_manwon, COALESCE(floor, 0),
		       COALESCE(lon, 0), COALESCE(lat, 0), created_at, updated_at, registered_by
		FROM b2c_listings
		WHERE status = 'sold' AND updated_at >= $1 AND ($2 = '' OR complex_name = $2)
		ORDER BY updated_at DESC
		LIMIT $3`, since, complexName, listingPageDefault)
	if err != nil {
		return nil, err
	}
	return scanListings(rows)
}

func scanListings(rows pgx.Rows) ([]Listing, error) {
	defer rows.Close()
	var out []Listing
	for rows.Next() {
		var id int64
		var by string
		l := Listing{Source: DirectListingSourceName}
		if err := rows.Scan(&id, &l.ComplexName, &l.TypeName, &l.AskingManwon, &l.Floor, &l.Lon, &l.Lat,
			&l.FirstSeen, &l.LastSeen, &by); err != nil {
			return nil, err
		}
		if strings.HasPrefix(by, SampleRegistrantPrefix) {
			l.Source = SampleListingSourceName
		}
		l.ID = strconv.FormatInt(id, 10)
		out = append(out, l)
	}
	return out, rows.Err()
}

// RefreshOnce는 systemd 타이머가 한 번 부르는 작업이다: 상태 읽기 -> Refresh -> 상태 저장.
func (r *RemainingListings) RefreshOnce(ctx context.Context, statePath string) (ListingChange, error) {
	if err := r.LoadState(statePath); err != nil {
		return ListingChange{}, err
	}
	change, err := r.Refresh(ctx)
	if err != nil {
		return ListingChange{}, err
	}
	return change, r.SaveState(statePath)
}

// listingCSVMaxRows는 CSV 한 번에 올릴 수 있는 최대 매물 수다.
const listingCSVMaxRows = 2000

// listingCSVColumns는 CSV 열 이름이다. 영문과 한글 헤더를 모두 받는다. complex_name과 asking_manwon은 필수.
var listingCSVColumns = map[string]string{
	"complex_name": "complex_name", "단지명": "complex_name",
	"type_name": "type_name", "타입": "type_name",
	"asking_manwon": "asking_manwon", "호가(만원)": "asking_manwon", "호가": "asking_manwon",
	"floor": "floor", "층": "floor",
	"lon": "lon", "경도": "lon",
	"lat": "lat", "위도": "lat",
}

// ListingCSVError는 CSV의 잘못된 행 하나다. Line은 헤더를 1로 센 줄 번호다.
type ListingCSVError struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// ParseListingsCSV는 사람이 정리한 매물 CSV를 읽는다. 엑셀이 저장한 CP949 파일도 받는다.
// 잘못된 행은 건너뛰고 줄 번호와 이유를 돌려준다. 헤더가 틀리거나 행이 너무 많으면 전체 오류다.
func ParseListingsCSV(data []byte) ([]Listing, []ListingCSVError, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		decoded, err := korean.EUCKR.NewDecoder().Bytes(data)
		if err != nil {
			return nil, nil, fmt.Errorf("CSV 인코딩을 읽을 수 없습니다 (UTF-8 또는 CP949)")
		}
		data = decoded
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	header, err := r.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("헤더 읽기: %w", err)
	}
	idx := map[string]int{}
	for i, h := range header {
		if col, ok := listingCSVColumns[strings.TrimSpace(h)]; ok {
			idx[col] = i
		}
	}
	if _, ok := idx["complex_name"]; !ok {
		return nil, nil, fmt.Errorf("헤더에 complex_name(단지명) 열이 없습니다")
	}
	if _, ok := idx["asking_manwon"]; !ok {
		return nil, nil, fmt.Errorf("헤더에 asking_manwon(호가(만원)) 열이 없습니다")
	}
	get := func(rec []string, col string) string {
		if i, ok := idx[col]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	var out []Listing
	var bad []ListingCSVError
	for line := 2; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			bad = append(bad, ListingCSVError{Line: line, Message: err.Error()})
			continue
		}
		if len(out)+len(bad) >= listingCSVMaxRows {
			return nil, nil, fmt.Errorf("한 번에 %d행까지 올릴 수 있습니다", listingCSVMaxRows)
		}
		l := Listing{ComplexName: get(rec, "complex_name"), TypeName: get(rec, "type_name")}
		price, err := strconv.Atoi(strings.ReplaceAll(get(rec, "asking_manwon"), ",", ""))
		if l.ComplexName == "" || err != nil || price <= 0 {
			bad = append(bad, ListingCSVError{Line: line, Message: "단지명과 호가(만원, 0보다 큰 정수)가 필요합니다"})
			continue
		}
		l.AskingManwon = price
		if f := get(rec, "floor"); f != "" {
			if l.Floor, err = strconv.Atoi(f); err != nil {
				bad = append(bad, ListingCSVError{Line: line, Message: "층은 정수여야 합니다"})
				continue
			}
		}
		lonStr, latStr := get(rec, "lon"), get(rec, "lat")
		if lonStr != "" || latStr != "" {
			lon, errLon := strconv.ParseFloat(lonStr, 64)
			lat, errLat := strconv.ParseFloat(latStr, 64)
			if errLon != nil || errLat != nil || lon < 124 || lon > 132 || lat < 33 || lat > 39.5 {
				bad = append(bad, ListingCSVError{Line: line, Message: "경도·위도가 한국 범위가 아닙니다"})
				continue
			}
			l.Lon, l.Lat = lon, lat
		}
		out = append(out, l)
	}
	return out, bad, nil
}

// RegisterMany는 여러 매물을 한 트랜잭션으로 등록한다. 하나라도 실패하면 아무것도 넣지 않는다.
func (s *ListingStore) RegisterMany(ctx context.Context, listings []Listing, registeredBy string) (int, error) {
	if registeredBy == "" {
		return 0, fmt.Errorf("registered_by required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	for _, l := range listings {
		if _, err := tx.Exec(ctx, `
			INSERT INTO b2c_listings (complex_name, type_name, asking_manwon, floor, lon, lat, registered_by)
			VALUES ($1, NULLIF($2, ''), $3, NULLIF($4, 0), NULLIF($5, 0), NULLIF($6, 0), $7)`,
			l.ComplexName, l.TypeName, l.AskingManwon, l.Floor, l.Lon, l.Lat, registeredBy); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(listings), nil
}

// 예시 매물: 매물 피드 계약 전 유저 테스트 화면을 채우려고 최근 실거래로 만든 가짜 매물이다.
// 실제 매물이 아니므로 Source를 "sample"로 내보내고 화면에 "예시"라고 표시한다. PurgeSamples로 한 번에 지운다.
const (
	SampleListingSourceName = "sample"
	SampleRegistrantPrefix  = "sample:"
	sampleMarkup            = 1.03 // 호가는 보통 실거래가보다 조금 높다
)

// SampleListings는 단지마다 최근 (해제 아닌) 거래 최대 perComplex건으로 예시 매물을 만든다.
// 호가 = 실거래가 x 1.03을 100만원 단위로 올림, 층은 그 거래의 층, 타입 칸에는 전용면적을 적는다.
// 같은 입력이면 같은 결과다(무작위 없음).
func SampleListings(trades []AptTrade, keep func(AptTrade) bool, perComplex int) []Listing {
	byComplex := map[string][]AptTrade{}
	var order []string
	for _, t := range trades {
		if t.Canceled || (keep != nil && !keep(t)) {
			continue
		}
		if _, ok := byComplex[t.ComplexName]; !ok {
			order = append(order, t.ComplexName)
		}
		byComplex[t.ComplexName] = append(byComplex[t.ComplexName], t)
	}
	sort.Strings(order)
	var out []Listing
	for _, name := range order {
		g := byComplex[name]
		sort.Slice(g, func(i, j int) bool { return g[i].DealDate.After(g[j].DealDate) })
		for i := 0; i < len(g) && i < perComplex; i++ {
			price := int(math.Ceil(float64(g[i].PriceManwon)*sampleMarkup/100) * 100)
			out = append(out, Listing{
				ComplexName:  name,
				TypeName:     fmt.Sprintf("전용 %.2f㎡", g[i].AreaM2),
				AskingManwon: price,
				Floor:        g[i].Floor,
			})
		}
	}
	return out
}

// PurgeSamples는 예시 매물을 모두 지운다. zone이 비지 않으면 그 구역 것만. 지운 행 수를 돌려준다.
func (s *ListingStore) PurgeSamples(ctx context.Context, zone string) (int64, error) {
	pattern := SampleRegistrantPrefix + "%"
	if zone != "" {
		pattern = SampleRegistrantPrefix + zone
	}
	ct, err := s.pool.Exec(ctx, `DELETE FROM b2c_listings WHERE registered_by LIKE $1`, pattern)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}
