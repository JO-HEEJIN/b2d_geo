package b2c

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestFloorPlanStoreAgainstPostgres는 저장 -> 덮어쓰기 -> 카탈로그 읽기를 실제 DB로 본다.
// B2D_TEST_DATABASE_URL이 없으면 건너뛴다(운영 DB에 쓰지 말 것: 테스트 행을 넣고 지운다).
func TestFloorPlanStoreAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("B2D_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("B2D_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	cleanup := func() { pool.Exec(ctx, `DELETE FROM b2c_floor_plans WHERE complex_id = 'b2c_test_complex'`) }
	cleanup()
	defer cleanup()

	store := NewFloorPlanStore(pool)
	for _, v := range []FloorPlanVariant{
		{ComplexID: "b2c_test_complex", TypeName: "84A", ExclusiveAreaM2: 84.9, Rooms: 3, Bathrooms: 2, Bays: 4, BathroomPosition: "현관 옆", Units: []string{"101동 1203호"}},
		{ComplexID: "b2c_test_complex", TypeName: "84B", ExclusiveAreaM2: 84.9, Rooms: 3, Bathrooms: 2, Bays: 3},
		{ComplexID: "b2c_test_complex", TypeName: "84A", ExclusiveAreaM2: 84.9, Rooms: 3, Bathrooms: 2, Bays: 4, BathroomPosition: "주방 옆", Units: []string{"101동 1203호", "102동 701호"}},
	} {
		if err := store.Save(ctx, v, "b2c_test"); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Save(ctx, FloorPlanVariant{ComplexID: "b2c_test_complex"}, "b2c_test"); err == nil {
		t.Error("expected validation error")
	}
	c, err := store.Catalog(ctx, "b2c_test_complex")
	if err != nil {
		t.Fatal(err)
	}
	vs := c.Variants("b2c_test_complex")
	if len(vs) != 2 || vs[0].TypeName != "84A" || vs[0].BathroomPosition != "주방 옆" || len(vs[0].Units) != 2 {
		t.Fatalf("variants = %+v", vs)
	}
	if vs[1].Units == nil || len(vs[1].Units) != 0 {
		t.Errorf("84B units = %#v", vs[1].Units)
	}

	// 자동 후보는 사람이 등록한 같은 이름의 타입을 덮어쓰지 않는다.
	if err := store.Save(ctx, FloorPlanVariant{ComplexID: "b2c_test_complex", TypeName: "84.90㎡ 01라인", ExclusiveAreaM2: 84.9, BathroomPosition: "현관 옆"}, "b2c_test"); err != nil {
		t.Fatal(err)
	}
	cands := NewFloorPlanCatalog().CandidatesFromUnits("b2c_test_complex", []RegisteredUnit{
		{Dong: "101동", Ho: "101", AreaM2: 84.9}, {Dong: "101동", Ho: "102", AreaM2: 84.9},
	})
	n, err := store.SaveCandidates(ctx, cands)
	if err != nil || n != 1 {
		t.Fatalf("inserted=%d err=%v", n, err)
	}
	c, err = store.Catalog(ctx, "b2c_test_complex")
	if err != nil {
		t.Fatal(err)
	}
	keep, _ := c.find("b2c_test_complex", "84.90㎡ 01라인")
	added, _ := c.find("b2c_test_complex", "84.90㎡ 02라인")
	if keep.BathroomPosition != "현관 옆" || len(keep.Units) != 0 || len(added.Units) != 1 || added.Units[0] != "101동 102호" {
		t.Fatalf("keep=%+v added=%+v", keep, added)
	}
}
