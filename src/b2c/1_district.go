package b2c

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Point는 지도 위 한 지점(아파트 단지, 매물 등)이다. 좌표는 WGS84 경위도.
type Point struct {
	Name string  `json:"name"`
	Lon  float64 `json:"lon"`
	Lat  float64 `json:"lat"`
}

// Zone은 B2C 1차 서비스 구역이다.
type Zone struct {
	Key    string     `json:"key"`        // 영문 식별자
	Name   string     `json:"name"`       // 표시명
	Within []string   `json:"within"`     // 주소에 모두 있어야 하는 시/구 (토큰 접두 일치)
	Dongs  []string   `json:"dongs"`      // 법정동 목록. 비면 Within 전체가 구역이다
	Center [2]float64 `json:"center"`     // 대략적인 구역 중심 (lon, lat). 주소가 없을 때 판정 보조용
	Lawd   []string   `json:"lawd_codes"` // 실거래 조회용 시군구 코드 (법정동코드 앞 5자리)
	Region string     `json:"region"`     // 법정동코드 API 조회용 시도·시군구 이름
}

// districtZones는 1차 구현 범위의 7개 구역이다. 동탄은 도로명주소만으로는 1/2를
// 가를 수 없어 법정동으로 나눈다(법정동이 없으면 NearestZone으로 보조 판정).
// 시군구 코드와 Region은 행정안전부 법정동코드 API로 확인했다(2026-11-05).
// 화성시는 2026-02-01 동탄구(41597)를 두었고, 2026-03-01 여울동(4159711500)이 새로 생겼다
// (행정안전부 법정동코드 API로 확인). 현재 목록에 없는 오산동은 예전 주소를 읽으려고 남긴다.
var districtZones = []Zone{
	{Key: "songdo", Name: "인천 송도동", Within: []string{"인천", "연수구"}, Dongs: []string{"송도동"}, Center: [2]float64{126.6560, 37.3830}, Lawd: []string{"28185"}, Region: "인천광역시 연수구"},
	{Key: "dongtan1", Name: "1동탄", Within: []string{"화성"}, Dongs: []string{"반송동", "석우동", "능동"}, Center: [2]float64{127.0700, 37.2010}, Lawd: []string{"41597"}, Region: "경기도 화성시 동탄구"},
	{Key: "dongtan2", Name: "2동탄", Within: []string{"화성"}, Dongs: []string{"영천동", "청계동", "오산동", "여울동", "목동", "산척동", "장지동", "송동", "방교동", "중동", "신동", "금곡동"}, Center: [2]float64{127.1000, 37.1990}, Lawd: []string{"41597"}, Region: "경기도 화성시 동탄구"},
	{Key: "hanam", Name: "하남", Within: []string{"하남"}, Center: [2]float64{127.2060, 37.5390}, Lawd: []string{"41450"}, Region: "경기도 하남시"},
	{Key: "suji", Name: "용인 수지", Within: []string{"용인", "수지구"}, Center: [2]float64{127.0960, 37.3220}, Lawd: []string{"41465"}, Region: "경기도 용인시 수지구"},
	{Key: "suwon", Name: "수원", Within: []string{"수원"}, Center: [2]float64{127.0290, 37.2640}, Lawd: []string{"41111", "41113", "41115", "41117"}, Region: "경기도 수원시"},
	{Key: "jamsil", Name: "잠실", Within: []string{"송파구"}, Dongs: []string{"잠실동", "신천동"}, Center: [2]float64{127.0900, 37.5110}, Lawd: []string{"11710"}, Region: "서울특별시 송파구"},
}

// DistrictClassifier는 주소/좌표를 구역으로 분류하고, 구역 안의 지점들을 묶는다.
type DistrictClassifier struct {
	zones []Zone
}

// NewDistrictClassifier는 1차 7개 구역으로 분류기를 만든다.
func NewDistrictClassifier() *DistrictClassifier {
	return &DistrictClassifier{zones: districtZones}
}

// Zone은 key로 구역을 찾는다.
func (d *DistrictClassifier) Zone(key string) (Zone, bool) {
	for _, z := range d.zones {
		if z.Key == key {
			return z, true
		}
	}
	return Zone{}, false
}

// InZone은 (법정동 이름)이 구역 안인지 본다. Dongs가 비면 구역의 시군구 전체다.
func (z Zone) InZone(umdNm string) bool {
	if len(z.Dongs) == 0 {
		return true
	}
	for _, d := range z.Dongs {
		if d == umdNm {
			return true
		}
	}
	return false
}

// Zones는 지원 구역 목록을 돌려준다.
func (d *DistrictClassifier) Zones() []Zone {
	return d.zones
}

// Classify는 주소 문자열이 속한 구역을 찾는다. 지번주소("... 반송동 123")와
// 도로명주소의 괄호 참고항목("(반송동, OO아파트)") 모두 읽는다. 행정동 표기
// ("잠실2동")는 숫자를 떼어 법정동("잠실동")으로 맞춘다.
func (d *DistrictClassifier) Classify(address string) (Zone, bool) {
	tokens := addressTokens(address)
	for _, z := range d.zones {
		if !hasAllPrefixes(tokens, z.Within) {
			continue
		}
		if len(z.Dongs) == 0 || hasAnyDong(tokens, z.Dongs) {
			return z, true
		}
	}
	return Zone{}, false
}

// NearestZone은 좌표에서 가장 가까운 구역 중심과 그 거리(m)를 돌려준다.
// 구역 경계 폴리곤이 아니라 중심점 기준이므로 Classify를 못 쓸 때만 보조로 쓴다.
func (d *DistrictClassifier) NearestZone(lon, lat float64) (Zone, float64) {
	best, bestDist := Zone{}, math.Inf(1)
	for _, z := range d.zones {
		if m := distanceM(lon, lat, z.Center[0], z.Center[1]); m < bestDist {
			best, bestDist = z, m
		}
	}
	return best, bestDist
}

// Cluster는 한 구역 안의 지점들을 k개 생활권 묶음으로 나눈다 (k-means).
// 초기 중심은 가장 먼 점을 차례로 고르는 방식이라 같은 입력이면 결과가 같다.
// 빈 묶음은 돌려주지 않는다.
func (d *DistrictClassifier) Cluster(points []Point, k int) [][]Point {
	if len(points) == 0 {
		return nil
	}
	if k <= 1 {
		return [][]Point{points}
	}
	if k > len(points) {
		k = len(points)
	}
	centers := farthestCenters(points, k)
	assign := make([]int, len(points))
	for iter := 0; iter < 50; iter++ {
		changed := false
		for i, p := range points {
			if c := nearestCenter(p, centers); c != assign[i] {
				assign[i] = c
				changed = true
			}
		}
		if !changed && iter > 0 {
			break
		}
		sum := make([][3]float64, k)
		for i, p := range points {
			sum[assign[i]][0] += p.Lon
			sum[assign[i]][1] += p.Lat
			sum[assign[i]][2]++
		}
		for c := range centers {
			if sum[c][2] > 0 {
				centers[c] = Point{Lon: sum[c][0] / sum[c][2], Lat: sum[c][1] / sum[c][2]}
			}
		}
	}
	groups := make([][]Point, k)
	for i, p := range points {
		groups[assign[i]] = append(groups[assign[i]], p)
	}
	out := groups[:0]
	for _, g := range groups {
		if len(g) > 0 {
			out = append(out, g)
		}
	}
	return out
}

func addressTokens(address string) []string {
	clean := strings.NewReplacer("(", " ", ")", " ", ",", " ").Replace(address)
	return strings.Fields(clean)
}

func hasAllPrefixes(tokens, prefixes []string) bool {
	for _, p := range prefixes {
		found := false
		for _, t := range tokens {
			if strings.HasPrefix(t, p) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func hasAnyDong(tokens, dongs []string) bool {
	for _, t := range tokens {
		legal := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return -1
			}
			return r
		}, t)
		for _, dong := range dongs {
			if legal == dong {
				return true
			}
		}
	}
	return false
}

func farthestCenters(points []Point, k int) []Point {
	centers := []Point{points[0]}
	for len(centers) < k {
		far, farDist := points[0], -1.0
		for _, p := range points {
			c := centers[nearestCenter(p, centers)]
			if m := distanceM(p.Lon, p.Lat, c.Lon, c.Lat); m > farDist {
				far, farDist = p, m
			}
		}
		centers = append(centers, far)
	}
	return centers
}

func nearestCenter(p Point, centers []Point) int {
	best, bestDist := 0, math.Inf(1)
	for i, c := range centers {
		if m := distanceM(p.Lon, p.Lat, c.Lon, c.Lat); m < bestDist {
			best, bestDist = i, m
		}
	}
	return best
}

// distanceM은 두 경위도 사이 대원거리(m)다 (haversine, 지구 반지름 6,371km).
func distanceM(lon1, lat1, lon2, lat2 float64) float64 {
	const r = 6371000.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

// legalDongURL은 행정안전부 법정동코드 API다 (data.go.kr, 계정 키는 MOLIT_API_KEY와 같다).
const legalDongURL = "https://apis.data.go.kr/1741000/StanReginCd/getStanReginCdList"

// LegalDongResolver는 (시군구 코드, 법정동 이름) -> 10자리 법정동코드를 찾는다.
// 구역 Region 단위로 한 번 받아 24시간 기억한다. 실거래 자료에는 법정동 이름만 있어서 필요하다.
type LegalDongResolver struct {
	Key     string
	Client  *http.Client
	BaseURL string

	mu    sync.Mutex
	cache map[string]legalDongEntry
}

type legalDongEntry struct {
	codes map[string]string // "41597|반송동" -> "4159710200"
	at    time.Time
}

// Codes는 region(예: "경기도 화성시 동탄구") 아래 현존 법정동의 코드표를 돌려준다.
func (r *LegalDongResolver) Codes(ctx context.Context, region string) (map[string]string, error) {
	r.mu.Lock()
	if e, ok := r.cache[region]; ok && time.Since(e.at) < 24*time.Hour {
		r.mu.Unlock()
		return e.codes, nil
	}
	r.mu.Unlock()
	if r.Key == "" {
		return nil, fmt.Errorf("data.go.kr key not set")
	}
	base := r.BaseURL
	if base == "" {
		base = legalDongURL
	}
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	u := fmt.Sprintf("%s?serviceKey=%s&type=json&pageNo=1&numOfRows=1000&flag=Y&locatadd_nm=%s",
		base, url.QueryEscape(r.Key), url.QueryEscape(region))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("legal dong codes %s: request failed", region)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	// 응답: {"StanReginCd":[{"head":[...]},{"row":[{"region_cd","sido_cd","sgg_cd","umd_cd","ri_cd","locallow_nm"}]}]}
	var parsed struct {
		StanReginCd []struct {
			Row []struct {
				RegionCd   string `json:"region_cd"`
				LocallowNm string `json:"locallow_nm"`
			} `json:"row"`
		} `json:"StanReginCd"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.StanReginCd) < 2 {
		return nil, fmt.Errorf("legal dong codes %s: http %d unexpected response", region, resp.StatusCode)
	}
	codes := map[string]string{}
	for _, row := range parsed.StanReginCd[1].Row {
		c := row.RegionCd
		// 읍면동 단위만 (시군구 행 xxxxx00000, 리 행 끝 두 자리 != 00 제외)
		if len(c) != 10 || c[5:] == "00000" || c[8:] != "00" {
			continue
		}
		codes[c[:5]+"|"+strings.TrimSpace(row.LocallowNm)] = c
	}
	r.mu.Lock()
	if r.cache == nil {
		r.cache = map[string]legalDongEntry{}
	}
	r.cache[region] = legalDongEntry{codes: codes, at: time.Now()}
	r.mu.Unlock()
	return codes, nil
}

// PNU는 10자리 법정동코드와 지번("80", "722-3", "산12-1")으로 19자리 필지코드를 만든다.
func PNU(bcode, jibun string) (string, bool) {
	j := strings.TrimSpace(jibun)
	land := "1"
	if strings.HasPrefix(j, "산") {
		land, j = "2", strings.TrimSpace(strings.TrimPrefix(j, "산"))
	}
	bun, ji, _ := strings.Cut(j, "-")
	b, errB := strconv.Atoi(bun)
	k := 0
	var errJ error
	if ji != "" {
		k, errJ = strconv.Atoi(ji)
	}
	if len(bcode) != 10 || errB != nil || errJ != nil || b <= 0 || b > 9999 || k < 0 || k > 9999 {
		return "", false
	}
	return fmt.Sprintf("%s%s%04d%04d", bcode, land, b, k), true
}

// ComplexPlace는 단지(필지)의 위치다. ComplexID는 평면 카탈로그 키(ParcelComplexID)와 같다.
type ComplexPlace struct {
	PNU       string  `json:"pnu"`
	ComplexID string  `json:"complex_id"`
	Lon       float64 `json:"lon"`
	Lat       float64 `json:"lat"`
}

// ComplexLocator는 PNU로 parcels(연속지적도)에서 필지 안쪽 대표점을 찾는다.
type ComplexLocator struct {
	pool *pgxpool.Pool
}

// NewComplexLocator는 parcels 테이블을 쓰는 위치 찾기를 만든다.
func NewComplexLocator(pool *pgxpool.Pool) *ComplexLocator {
	return &ComplexLocator{pool: pool}
}

// Locate는 여러 PNU의 위치를 한 번에 찾는다. parcels에 없는 PNU는 결과에서 빠진다.
func (l *ComplexLocator) Locate(ctx context.Context, pnus []string) (map[string]ComplexPlace, error) {
	out := map[string]ComplexPlace{}
	if len(pnus) == 0 {
		return out, nil
	}
	rows, err := l.pool.Query(ctx, `
		SELECT pnu, ST_X(ST_PointOnSurface(geom)), ST_Y(ST_PointOnSurface(geom))
		FROM parcels WHERE pnu = ANY($1) AND geom IS NOT NULL`, pnus)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p ComplexPlace
		if err := rows.Scan(&p.PNU, &p.Lon, &p.Lat); err != nil {
			return nil, err
		}
		p.ComplexID = ParcelComplexID(p.PNU[:5], p.PNU[5:10], p.PNU[11:15], p.PNU[15:19])
		out[p.PNU] = p
	}
	return out, rows.Err()
}
