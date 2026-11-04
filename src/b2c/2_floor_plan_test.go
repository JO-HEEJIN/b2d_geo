package b2c

import "testing"

func sampleCatalog() *FloorPlanCatalog {
	c := NewFloorPlanCatalog()
	c.AddVariant(FloorPlanVariant{ComplexID: "c1", TypeName: "84B", ExclusiveAreaM2: 84.9, Rooms: 3, Bathrooms: 2, Bays: 3,
		BathroomPosition: "주방 옆", LivingRoom: "타워형 2면 개방", Orientation: "남동향"})
	c.AddVariant(FloorPlanVariant{ComplexID: "c1", TypeName: "84A", ExclusiveAreaM2: 84.9, Rooms: 3, Bathrooms: 2, Bays: 4,
		BathroomPosition: "현관 옆", LivingRoom: "판상형 맞통풍", Orientation: "남향", Units: []string{"101동 1203호", "103동 502호"}})
	c.AddVariant(FloorPlanVariant{ComplexID: "c1", TypeName: "59A", ExclusiveAreaM2: 59.9, Rooms: 3, Bathrooms: 2, Bays: 3})
	return c
}

func TestVariantsKeepsEveryType(t *testing.T) {
	got := sampleCatalog().Variants("c1")
	want := []string{"59A", "84A", "84B"}
	if len(got) != len(want) {
		t.Fatalf("len = %d", len(got))
	}
	for i := range want {
		if got[i].TypeName != want[i] {
			t.Errorf("[%d] = %s want %s", i, got[i].TypeName, want[i])
		}
	}
}

func TestCompare(t *testing.T) {
	diffs, err := sampleCatalog().Compare("c1", "84A", "84B")
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]bool{}
	for _, d := range diffs {
		fields[d.Field] = true
	}
	for _, f := range []string{"베이", "화장실 위치", "거실 구조", "향"} {
		if !fields[f] {
			t.Errorf("missing diff %s", f)
		}
	}
	if fields["전용면적"] || fields["방"] {
		t.Errorf("unexpected diff: %+v", diffs)
	}
	if _, err := sampleCatalog().Compare("c1", "84A", "99Z"); err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestTourPlanAndUnits(t *testing.T) {
	c := sampleCatalog()
	stops := c.TourPlan("c1")
	if len(stops) != 3 {
		t.Fatalf("stops = %d", len(stops))
	}
	if stops[1].Unit != "101동 1203호" || stops[0].Unit != "" {
		t.Errorf("units = %q, %q", stops[0].Unit, stops[1].Unit)
	}
	if u := c.UnitsWithLayout("c1", "84A"); len(u) != 2 {
		t.Errorf("units = %v", u)
	}
	if n := len(c.ViewingChecklist(stops[0].Variant)); n != 7 {
		t.Errorf("checklist = %d", n)
	}
}
