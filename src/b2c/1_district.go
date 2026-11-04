package b2c

import (
	"math"
	"strings"
)

// Point는 지도 위 한 지점(아파트 단지, 매물 등)이다. 좌표는 WGS84 경위도.
type Point struct {
	Name string
	Lon  float64
	Lat  float64
}

// Zone은 B2C 1차 서비스 구역이다.
type Zone struct {
	Key    string     // 영문 식별자
	Name   string     // 표시명
	Within []string   // 주소에 모두 있어야 하는 시/구 (토큰 접두 일치)
	Dongs  []string   // 법정동 목록. 비면 Within 전체가 구역이다
	Center [2]float64 // 대략적인 구역 중심 (lon, lat). 주소가 없을 때 판정 보조용
}

// districtZones는 1차 구현 범위의 7개 구역이다. 동탄은 도로명주소만으로는 1/2를
// 가를 수 없어 법정동으로 나눈다(법정동이 없으면 NearestZone으로 보조 판정).
var districtZones = []Zone{
	{Key: "songdo", Name: "인천 송도동", Within: []string{"인천", "연수구"}, Dongs: []string{"송도동"}, Center: [2]float64{126.6560, 37.3830}},
	{Key: "dongtan1", Name: "1동탄", Within: []string{"화성"}, Dongs: []string{"반송동", "석우동", "능동"}, Center: [2]float64{127.0700, 37.2010}},
	{Key: "dongtan2", Name: "2동탄", Within: []string{"화성"}, Dongs: []string{"영천동", "청계동", "오산동", "목동", "산척동", "장지동", "송동", "방교동", "중동", "신동", "금곡동"}, Center: [2]float64{127.1000, 37.1990}},
	{Key: "hanam", Name: "하남", Within: []string{"하남"}, Center: [2]float64{127.2060, 37.5390}},
	{Key: "suji", Name: "용인 수지", Within: []string{"용인", "수지구"}, Center: [2]float64{127.0960, 37.3220}},
	{Key: "suwon", Name: "수원", Within: []string{"수원"}, Center: [2]float64{127.0290, 37.2640}},
	{Key: "jamsil", Name: "잠실", Within: []string{"송파구"}, Dongs: []string{"잠실동", "신천동"}, Center: [2]float64{127.0900, 37.5110}},
}

// DistrictClassifier는 주소/좌표를 구역으로 분류하고, 구역 안의 지점들을 묶는다.
type DistrictClassifier struct {
	zones []Zone
}

// NewDistrictClassifier는 1차 7개 구역으로 분류기를 만든다.
func NewDistrictClassifier() *DistrictClassifier {
	return &DistrictClassifier{zones: districtZones}
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
