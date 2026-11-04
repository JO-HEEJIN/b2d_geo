package b2c

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JO-HEEJIN/b2d_geo/internal/places"
)

// InfraKind는 집 주변에서 걸어서 가는 시간을 보여줄 시설 종류다.
type InfraKind string

const (
	InfraSubway      InfraKind = "subway_station"
	InfraBusStop     InfraKind = "bus_stop"
	InfraConvenience InfraKind = "convenience_store"
	InfraHospital    InfraKind = "hospital"
)

// infraKinds는 요약에 항상 나오는 시설 순서다.
var infraKinds = []InfraKind{InfraSubway, InfraBusStop, InfraConvenience, InfraHospital}

const (
	walkMetersPerMinute = 80.0   // 부동산 광고 관행: 도보 1분 = 80m
	walkDetourFactor    = 1.3    // 직선거리 -> 실제 걷는 거리 보정 (도로망 미반영 근사)
	infraDefaultRadiusM = 1500.0 // 도보 약 25분
)

// Facility는 주변 시설 1곳이다.
type Facility struct {
	Kind        InfraKind
	Name        string
	Address     string
	Lon, Lat    float64
	DistanceM   float64 // 직선거리
	WalkMinutes int
}

// InfraLine은 요약의 한 줄이다. Found가 false면 반경 안에 없거나 데이터가 아직 없다.
type InfraLine struct {
	Kind     InfraKind
	Found    bool
	Facility Facility
	Note     string
}

// FacilitySource는 한 종류의 시설을 가까운 순으로 찾는다.
type FacilitySource interface {
	Nearby(ctx context.Context, lon, lat, radiusM float64, limit int) ([]Facility, error)
}

// InfraMap은 지하철역, 버스정류장, 편의점, 병원까지의 도보시간을 보여준다.
type InfraMap struct {
	sources map[InfraKind]FacilitySource
	radiusM float64
}

// NewInfraMap은 시설 종류별 원천으로 인프라맵을 만든다. 원천이 없는 종류는
// 요약에 "데이터 준비 중"으로 나온다.
func NewInfraMap(sources map[InfraKind]FacilitySource) *InfraMap {
	return &InfraMap{sources: sources, radiusM: infraDefaultRadiusM}
}

// WalkMinutes는 직선거리(m)를 도보 분으로 바꾼다. 1분 미만은 1분으로 올린다.
func (m *InfraMap) WalkMinutes(distanceM float64) int {
	minutes := int(math.Ceil(distanceM * walkDetourFactor / walkMetersPerMinute))
	if minutes < 1 {
		return 1
	}
	return minutes
}

// Nearest는 해당 종류에서 가장 가까운 시설을 찾는다.
func (m *InfraMap) Nearest(ctx context.Context, kind InfraKind, lon, lat float64) (Facility, bool, error) {
	list, err := m.Around(ctx, kind, lon, lat, 1)
	if err != nil || len(list) == 0 {
		return Facility{}, false, err
	}
	return list[0], true, nil
}

// Around는 해당 종류의 시설을 가까운 순으로 limit개까지 도보시간과 함께 돌려준다.
func (m *InfraMap) Around(ctx context.Context, kind InfraKind, lon, lat float64, limit int) ([]Facility, error) {
	src, ok := m.sources[kind]
	if !ok {
		return nil, nil
	}
	list, err := src.Nearby(ctx, lon, lat, m.radiusM, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", kind, err)
	}
	for i := range list {
		list[i].Kind = kind
		list[i].DistanceM = distanceM(lon, lat, list[i].Lon, list[i].Lat)
		list[i].WalkMinutes = m.WalkMinutes(list[i].DistanceM)
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].DistanceM < list[j].DistanceM })
	return list, nil
}

// Summary는 네 종류 시설 각각의 가장 가까운 곳과 도보시간을 정해진 순서로 돌려준다.
func (m *InfraMap) Summary(ctx context.Context, lon, lat float64) ([]InfraLine, error) {
	lines := make([]InfraLine, 0, len(infraKinds))
	for _, kind := range infraKinds {
		line := InfraLine{Kind: kind}
		if _, ok := m.sources[kind]; !ok {
			line.Note = "데이터 준비 중"
			lines = append(lines, line)
			continue
		}
		f, found, err := m.Nearest(ctx, kind, lon, lat)
		if err != nil {
			return nil, err
		}
		if found {
			line.Found, line.Facility = true, f
		} else {
			line.Note = fmt.Sprintf("도보 %d분 거리 안에 없음", m.WalkMinutes(m.radiusM))
		}
		lines = append(lines, line)
	}
	return lines, nil
}

// PlacesSource는 기존 b2d_geo 상가(places) 테이블을 시설 원천으로 쓴다.
// 편의점, 병원처럼 소상공인 상가정보에 있는 업종에 쓴다. categoryCodes는 상가정보
// 업종 소분류 코드이며, 실제 적재 데이터에서 코드를 확인한 뒤 넣는다.
type PlacesSource struct {
	pool          *pgxpool.Pool
	categoryCodes []string
}

// NewPlacesSource는 places 테이블에서 categoryCodes 업종을 찾는 원천을 만든다.
func NewPlacesSource(pool *pgxpool.Pool, categoryCodes ...string) *PlacesSource {
	return &PlacesSource{pool: pool, categoryCodes: categoryCodes}
}

// Nearby는 업종 코드별로 places.Nearby를 불러 합친다. 정렬과 자르기는 InfraMap.Around가 한다.
func (s *PlacesSource) Nearby(ctx context.Context, lon, lat, radiusM float64, limit int) ([]Facility, error) {
	var out []Facility
	for _, code := range s.categoryCodes {
		found, err := places.Nearby(ctx, s.pool, code, lon, lat, radiusM, limit)
		if err != nil {
			return nil, err
		}
		for _, p := range found {
			addr := p.RoadAddress
			if addr == "" {
				addr = p.JibunAddress
			}
			out = append(out, Facility{Name: p.Name, Address: addr, Lon: p.Point[0], Lat: p.Point[1]})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return distanceM(lon, lat, out[i].Lon, out[i].Lat) < distanceM(lon, lat, out[j].Lon, out[j].Lat)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
