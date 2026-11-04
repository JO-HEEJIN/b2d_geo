package b2c

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FloorPlanVariant는 한 단지 안의 평면 타입 하나다. 같은 단지, 같은 평형이어도
// 화장실 위치나 거실 구조가 다르므로 타입(84A/84B)마다 따로 둔다.
type FloorPlanVariant struct {
	ComplexID        string   `json:"complex_id"`
	TypeName         string   `json:"type_name"`         // 예: "84A"
	ExclusiveAreaM2  float64  `json:"exclusive_area_m2"` // 전용면적
	Rooms            int      `json:"rooms"`             // 방 수
	Bathrooms        int      `json:"bathrooms"`         // 화장실 수
	Bays             int      `json:"bays"`              // 베이 수 (전면 발코니 쪽 칸 수)
	BathroomPosition string   `json:"bathroom_position"` // 예: "현관 옆 공용, 안방 안쪽 부부욕실"
	LivingRoom       string   `json:"living_room"`       // 예: "판상형 맞통풍", "타워형 2면 개방"
	Orientation      string   `json:"orientation"`       // 향. 예: "남향"
	ImageURL         string   `json:"image_url"`         // 출처가 분명한 평면도 이미지만 (분양공고 등)
	Units            []string `json:"units"`             // 이 구조인 동·호. 예: "101동 1203호"
}

// LayoutDiff는 두 평면 타입이 다른 항목 하나다.
type LayoutDiff struct {
	Field string `json:"field"`
	A     string `json:"a"`
	B     string `json:"b"`
}

// TourStop은 집 보기 동선의 한 곳이다. 타입마다 하나씩, 직접 들어가 볼 세대를 붙인다.
type TourStop struct {
	Variant FloorPlanVariant `json:"variant"`
	Unit    string           `json:"unit"` // 비면 아직 볼 수 있는 세대가 등록되지 않았다
}

// FloorPlanCatalog는 단지별 평면 타입을 모아 비교하고, 직접 볼 세대를 찾아 준다.
type FloorPlanCatalog struct {
	variants map[string][]FloorPlanVariant
}

// NewFloorPlanCatalog는 빈 평면 카탈로그를 만든다.
func NewFloorPlanCatalog() *FloorPlanCatalog {
	return &FloorPlanCatalog{variants: map[string][]FloorPlanVariant{}}
}

// AddVariant는 평면 타입을 등록한다. 같은 단지에 같은 TypeName이 있으면 바꾼다.
func (c *FloorPlanCatalog) AddVariant(v FloorPlanVariant) {
	list := c.variants[v.ComplexID]
	for i := range list {
		if list[i].TypeName == v.TypeName {
			list[i] = v
			return
		}
	}
	c.variants[v.ComplexID] = append(list, v)
}

// Variants는 단지의 모든 평면 타입을 면적, 타입명 순으로 돌려준다.
// 추려서 줄이지 않는다. 사용자는 여러 구조를 구경하는 것 자체를 즐긴다.
func (c *FloorPlanCatalog) Variants(complexID string) []FloorPlanVariant {
	out := append([]FloorPlanVariant(nil), c.variants[complexID]...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].ExclusiveAreaM2 != out[j].ExclusiveAreaM2 {
			return out[i].ExclusiveAreaM2 < out[j].ExclusiveAreaM2
		}
		return out[i].TypeName < out[j].TypeName
	})
	return out
}

// Compare는 같은 단지의 두 타입이 어떻게 다른지 항목별로 돌려준다.
func (c *FloorPlanCatalog) Compare(complexID, typeA, typeB string) ([]LayoutDiff, error) {
	a, okA := c.find(complexID, typeA)
	b, okB := c.find(complexID, typeB)
	if !okA || !okB {
		return nil, fmt.Errorf("floor plan type not found: %s/%s, %s", complexID, typeA, typeB)
	}
	fields := []LayoutDiff{
		{"전용면적", fmt.Sprintf("%.2f", a.ExclusiveAreaM2), fmt.Sprintf("%.2f", b.ExclusiveAreaM2)},
		{"방", fmt.Sprint(a.Rooms), fmt.Sprint(b.Rooms)},
		{"화장실", fmt.Sprint(a.Bathrooms), fmt.Sprint(b.Bathrooms)},
		{"베이", fmt.Sprint(a.Bays), fmt.Sprint(b.Bays)},
		{"화장실 위치", a.BathroomPosition, b.BathroomPosition},
		{"거실 구조", a.LivingRoom, b.LivingRoom},
		{"향", a.Orientation, b.Orientation},
	}
	var diffs []LayoutDiff
	for _, f := range fields {
		if f.A != f.B {
			diffs = append(diffs, f)
		}
	}
	return diffs, nil
}

// UnitsWithLayout은 해당 구조로 지어진 동·호를 돌려준다. 평면도만으로는 알 수 없는
// 채광, 소음, 실제 동선은 이 세대에 직접 가서 확인한다.
func (c *FloorPlanCatalog) UnitsWithLayout(complexID, typeName string) []string {
	v, ok := c.find(complexID, typeName)
	if !ok {
		return nil
	}
	return append([]string(nil), v.Units...)
}

// TourPlan은 단지의 모든 타입을 하나씩 직접 볼 수 있도록 동선을 만든다.
// 타입마다 등록된 첫 세대를 붙인다. 볼 세대가 없는 타입도 빼지 않는다.
func (c *FloorPlanCatalog) TourPlan(complexID string) []TourStop {
	var stops []TourStop
	for _, v := range c.Variants(complexID) {
		stop := TourStop{Variant: v}
		if len(v.Units) > 0 {
			stop.Unit = v.Units[0]
		}
		stops = append(stops, stop)
	}
	return stops
}

// ViewingChecklist는 집을 직접 볼 때 확인할 항목이다. 평면도에서 이미 다른 점으로
// 드러난 항목을 앞에 두고, 계약 후 들어갈 리모델링·유지보수 항목을 뒤에 붙인다.
func (c *FloorPlanCatalog) ViewingChecklist(v FloorPlanVariant) []string {
	list := []string{
		fmt.Sprintf("화장실 위치 (%s): 현관·주방과의 거리, 환기창 유무", orUnknown(v.BathroomPosition)),
		fmt.Sprintf("거실 구조 (%s): 맞통풍, 소파·TV 벽 길이", orUnknown(v.LivingRoom)),
		fmt.Sprintf("향과 채광 (%s): 오전·오후 해 드는 방향", orUnknown(v.Orientation)),
		"층간·도로 소음: 창을 닫은 상태와 연 상태 모두",
		"수납: 붙박이장, 팬트리, 발코니 확장 여부",
	}
	return append(list,
		"리모델링 필요 범위: 욕실 타일, 주방 상판, 바닥재, 샷시",
		"유지보수 흔적: 천장 누수 자국, 결로·곰팡이, 배관 소음",
	)
}

func (c *FloorPlanCatalog) find(complexID, typeName string) (FloorPlanVariant, bool) {
	for _, v := range c.variants[complexID] {
		if v.TypeName == typeName {
			return v, true
		}
	}
	return FloorPlanVariant{}, false
}

func orUnknown(s string) string {
	if s == "" {
		return "정보 없음"
	}
	return s
}

// FloorPlanStore는 평면 타입을 b2c_floor_plans에 저장하고 단지별 카탈로그로 읽어 온다.
type FloorPlanStore struct {
	pool *pgxpool.Pool
}

// NewFloorPlanStore는 b2c_floor_plans 테이블을 쓰는 저장소를 만든다.
func NewFloorPlanStore(pool *pgxpool.Pool) *FloorPlanStore {
	return &FloorPlanStore{pool: pool}
}

// Save는 평면 타입을 저장한다. 같은 단지·타입이 있으면 새 내용으로 바꾼다.
func (s *FloorPlanStore) Save(ctx context.Context, v FloorPlanVariant, registeredBy string) error {
	if strings.TrimSpace(v.ComplexID) == "" || strings.TrimSpace(v.TypeName) == "" || v.ExclusiveAreaM2 <= 0 || registeredBy == "" {
		return fmt.Errorf("complex_id, type_name, exclusive_area_m2(>0), registered_by required")
	}
	units := v.Units
	if units == nil {
		units = []string{}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO b2c_floor_plans (complex_id, type_name, exclusive_area_m2, rooms, bathrooms, bays,
		    bathroom_position, living_room, orientation, image_url, units, registered_by)
		VALUES ($1, $2, $3, NULLIF($4, 0), NULLIF($5, 0), NULLIF($6, 0),
		    NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), $11, $12)
		ON CONFLICT (complex_id, type_name) DO UPDATE SET
		    exclusive_area_m2 = EXCLUDED.exclusive_area_m2, rooms = EXCLUDED.rooms,
		    bathrooms = EXCLUDED.bathrooms, bays = EXCLUDED.bays,
		    bathroom_position = EXCLUDED.bathroom_position, living_room = EXCLUDED.living_room,
		    orientation = EXCLUDED.orientation, image_url = EXCLUDED.image_url,
		    units = EXCLUDED.units, registered_by = EXCLUDED.registered_by, updated_at = now()`,
		strings.TrimSpace(v.ComplexID), strings.TrimSpace(v.TypeName), v.ExclusiveAreaM2, v.Rooms, v.Bathrooms, v.Bays,
		v.BathroomPosition, v.LivingRoom, v.Orientation, v.ImageURL, units, registeredBy)
	return err
}

// Catalog는 단지의 모든 평면 타입을 읽어 카탈로그로 돌려준다.
func (s *FloorPlanStore) Catalog(ctx context.Context, complexID string) (*FloorPlanCatalog, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT complex_id, type_name, exclusive_area_m2, COALESCE(rooms, 0), COALESCE(bathrooms, 0),
		       COALESCE(bays, 0), COALESCE(bathroom_position, ''), COALESCE(living_room, ''),
		       COALESCE(orientation, ''), COALESCE(image_url, ''), units
		FROM b2c_floor_plans WHERE complex_id = $1`, complexID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c := NewFloorPlanCatalog()
	for rows.Next() {
		var v FloorPlanVariant
		if err := rows.Scan(&v.ComplexID, &v.TypeName, &v.ExclusiveAreaM2, &v.Rooms, &v.Bathrooms, &v.Bays,
			&v.BathroomPosition, &v.LivingRoom, &v.Orientation, &v.ImageURL, &v.Units); err != nil {
			return nil, err
		}
		c.AddVariant(v)
	}
	return c, rows.Err()
}

// RegisteredUnit은 건축물대장 전유부의 아파트 세대 1개다.
type RegisteredUnit struct {
	BuildingName string  `json:"building_name"`
	Dong         string  `json:"dong"` // 예: "1622동"
	Ho           string  `json:"ho"`   // 예: "102"
	Floor        int     `json:"floor"`
	AreaM2       float64 `json:"area_m2"` // 전용면적
}

// buildingRegisterURL은 국토교통부 건축HUB 건축물대장 전유공용면적 API다.
const buildingRegisterURL = "https://apis.data.go.kr/1613000/BldRgstHubService/getBrExposPubuseAreaInfo"

// buildingRegisterPageRows는 이 API가 한 페이지에 주는 최대 행 수다(1000을 넣어도 100행만 온다, 2026-11-04 실측).
const buildingRegisterPageRows = 100

// BuildingRegisterSource는 건축물대장 전유부에서 단지의 세대 목록을 읽는다.
// Key는 data.go.kr 일반 인증키(Decoding)이며 MOLIT_API_KEY와 같은 계정 키다.
type BuildingRegisterSource struct {
	Key     string
	Client  *http.Client
	BaseURL string // 비면 buildingRegisterURL. 테스트에서 바꾼다
}

type bldItem struct {
	BldNm           string `xml:"bldNm"`
	DongNm          string `xml:"dongNm"`
	HoNm            string `xml:"hoNm"`
	FlrNo           string `xml:"flrNo"`
	ExposPubuseGbCd string `xml:"exposPubuseGbCd"` // 1 전유, 2 공용
	MainPurpsCdNm   string `xml:"mainPurpsCdNm"`
	Area            string `xml:"area"`
}

type bldResp struct {
	Header struct {
		ResultCode string `xml:"resultCode"`
		ResultMsg  string `xml:"resultMsg"`
	} `xml:"header"`
	Body struct {
		Items      []bldItem `xml:"items>item"`
		TotalCount int       `xml:"totalCount"`
	} `xml:"body"`
}

// Units는 필지(시군구·법정동 코드, 번·지)의 아파트 전유 세대를 모두 읽는다. 공용 면적 행과
// 아파트가 아닌 용도는 버린다. 한 단지가 수천 행이라 페이지를 끝까지 넘긴다.
func (s *BuildingRegisterSource) Units(ctx context.Context, sigunguCd, bjdongCd, bun, ji string) ([]RegisteredUnit, error) {
	if s.Key == "" {
		return nil, fmt.Errorf("data.go.kr key not set")
	}
	base := s.BaseURL
	if base == "" {
		base = buildingRegisterURL
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	seen := map[string]bool{}
	var out []RegisteredUnit
	for page := 1; ; page++ {
		u := fmt.Sprintf("%s?serviceKey=%s&sigunguCd=%s&bjdongCd=%s&bun=%s&ji=%s&numOfRows=%d&pageNo=%d",
			base, url.QueryEscape(s.Key), sigunguCd, bjdongCd, bun, ji, buildingRegisterPageRows, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			// url.Error는 요청 URL(키 포함)을 담으므로 그대로 올리지 않는다.
			return nil, fmt.Errorf("building register %s%s %s-%s: request failed", sigunguCd, bjdongCd, bun, ji)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		var br bldResp
		if err := xml.Unmarshal(body, &br); err != nil || br.Header.ResultCode != "00" {
			return nil, fmt.Errorf("building register %s%s %s-%s: http %d resultCode=%q msg=%q",
				sigunguCd, bjdongCd, bun, ji, resp.StatusCode, br.Header.ResultCode, br.Header.ResultMsg)
		}
		for _, it := range br.Body.Items {
			if it.ExposPubuseGbCd != "1" || it.MainPurpsCdNm != "아파트" {
				continue
			}
			area, err := strconv.ParseFloat(strings.TrimSpace(it.Area), 64)
			if err != nil || area <= 0 {
				continue
			}
			unit := RegisteredUnit{
				BuildingName: strings.TrimSpace(it.BldNm),
				Dong:         strings.TrimSpace(it.DongNm),
				Ho:           strings.TrimSpace(it.HoNm),
				AreaM2:       math.Round(area*100) / 100,
			}
			unit.Floor, _ = strconv.Atoi(strings.TrimSpace(it.FlrNo))
			key := unit.Dong + "|" + unit.Ho
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, unit)
		}
		if len(br.Body.Items) < buildingRegisterPageRows || page*buildingRegisterPageRows >= br.Body.TotalCount {
			return out, nil
		}
	}
}

// CandidatesFromUnits는 건축물대장 세대 목록에서 평면 타입 후보를 만든다.
// 같은 전용면적이어도 라인(호수 끝 두 자리)마다 구조가 다르므로 (전용면적, 라인)으로 묶는다.
// 실측(동탄 여울동 964): 538세대가 모두 84.9㎡라 면적만으로는 타입이 1개뿐이었고, 라인은 01~06이었다.
// 후보는 화장실 위치·거실 구조를 모른다. 사람이 직접 보고 채우거나 같은 구조의 라인을 합친다.
// 후보 이름은 "84.90㎡ 01라인"이고, 이름과 같은 순서(면적, 라인)로 정렬한다. 줄이지 않고 전부 돌려준다.
func (c *FloorPlanCatalog) CandidatesFromUnits(complexID string, units []RegisteredUnit) []FloorPlanVariant {
	type key struct {
		area float64
		line string
	}
	groups := map[key][]RegisteredUnit{}
	for _, u := range units {
		line := u.Ho
		if len(line) > 2 {
			line = line[len(line)-2:]
		}
		k := key{u.AreaM2, line}
		groups[k] = append(groups[k], u)
	}
	keys := make([]key, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].area != keys[j].area {
			return keys[i].area < keys[j].area
		}
		return keys[i].line < keys[j].line
	})
	out := make([]FloorPlanVariant, 0, len(keys))
	for _, k := range keys {
		g := groups[k]
		sort.Slice(g, func(i, j int) bool {
			if g[i].Dong != g[j].Dong {
				return g[i].Dong < g[j].Dong
			}
			return g[i].Floor < g[j].Floor
		})
		v := FloorPlanVariant{
			ComplexID:       complexID,
			TypeName:        CandidateTypeName(k.area, k.line),
			ExclusiveAreaM2: k.area,
		}
		for _, u := range g {
			v.Units = append(v.Units, u.Dong+" "+u.Ho+"호")
		}
		out = append(out, v)
	}
	return out
}

// BuildingRegisterRegistrant는 건축물대장에서 자동으로 만든 후보의 registered_by 값이다.
const BuildingRegisterRegistrant = "building_register"

// SaveCandidates는 후보를 저장하되, 같은 단지·타입이 이미 있으면 건드리지 않는다.
// 사람이 채운 화장실 위치·거실 구조를 자동 후보가 덮어쓰지 않게 하려는 것이다. 새로 넣은 수를 돌려준다.
func (s *FloorPlanStore) SaveCandidates(ctx context.Context, candidates []FloorPlanVariant) (int, error) {
	inserted := 0
	for _, v := range candidates {
		units := v.Units
		if units == nil {
			units = []string{}
		}
		ct, err := s.pool.Exec(ctx, `
			INSERT INTO b2c_floor_plans (complex_id, type_name, exclusive_area_m2, units, registered_by)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (complex_id, type_name) DO NOTHING`,
			v.ComplexID, v.TypeName, v.ExclusiveAreaM2, units, BuildingRegisterRegistrant)
		if err != nil {
			return inserted, err
		}
		inserted += int(ct.RowsAffected())
	}
	return inserted, nil
}

// ParcelComplexID는 필지 코드로 단지 ID를 만든다. 예: "4159711500-0964-0000".
func ParcelComplexID(sigunguCd, bjdongCd, bun, ji string) string {
	return sigunguCd + bjdongCd + "-" + bun + "-" + ji
}

// CandidateTypeName은 자동 후보 이름이다. 정렬 기준인 전용면적을 소수 둘째 자리까지 그대로 보여서,
// 59.95㎡와 59.98㎡처럼 정수 부분이 같은 타입이 이름 순서와 다르게 보이지 않게 한다.
func CandidateTypeName(areaM2 float64, line string) string {
	return fmt.Sprintf("%.2f㎡ %s라인", areaM2, line)
}

// ResetCandidates는 건축물대장에서 자동으로 만든 후보를 지운다(사람이 등록한 타입은 남긴다).
// complexID가 비면 전체다. 이름 규칙을 바꿨을 때 다시 만들게 하려는 것이다. 지운 행 수를 돌려준다.
func (s *FloorPlanStore) ResetCandidates(ctx context.Context, complexID string) (int64, error) {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM b2c_floor_plans
		WHERE registered_by = $1 AND ($2 = '' OR complex_id = $2)`, BuildingRegisterRegistrant, complexID)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}
