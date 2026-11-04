package b2c

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FloorPlanVariant는 한 단지 안의 평면 타입 하나다. 같은 단지, 같은 평형이어도
// 화장실 위치나 거실 구조가 다르므로 타입(84A/84B)마다 따로 둔다.
type FloorPlanVariant struct {
	ComplexID        string
	TypeName         string   // 예: "84A"
	ExclusiveAreaM2  float64  // 전용면적
	Rooms            int      // 방 수
	Bathrooms        int      // 화장실 수
	Bays             int      // 베이 수 (전면 발코니 쪽 칸 수)
	BathroomPosition string   // 예: "현관 옆 공용, 안방 안쪽 부부욕실"
	LivingRoom       string   // 예: "판상형 맞통풍", "타워형 2면 개방"
	Orientation      string   // 향. 예: "남향"
	ImageURL         string   // 출처가 분명한 평면도 이미지만 (분양공고 등)
	Units            []string // 이 구조인 동·호. 예: "101동 1203호"
}

// LayoutDiff는 두 평면 타입이 다른 항목 하나다.
type LayoutDiff struct {
	Field string
	A, B  string
}

// TourStop은 집 보기 동선의 한 곳이다. 타입마다 하나씩, 직접 들어가 볼 세대를 붙인다.
type TourStop struct {
	Variant FloorPlanVariant
	Unit    string // 비면 아직 볼 수 있는 세대가 등록되지 않았다
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
