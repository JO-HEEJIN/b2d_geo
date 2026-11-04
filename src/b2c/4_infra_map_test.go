package b2c

import (
	"context"
	"errors"
	"testing"
)

type fakeFacilities []Facility

func (f fakeFacilities) Nearby(_ context.Context, _, _, _ float64, limit int) ([]Facility, error) {
	out := append([]Facility(nil), f...)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type failingFacilities struct{}

func (failingFacilities) Nearby(context.Context, float64, float64, float64, int) ([]Facility, error) {
	return nil, errors.New("db down")
}

func TestWalkMinutes(t *testing.T) {
	m := NewInfraMap(nil)
	cases := map[float64]int{0: 1, 40: 1, 200: 4, 615: 10, 1500: 25}
	for dist, want := range cases {
		if got := m.WalkMinutes(dist); got != want {
			t.Errorf("WalkMinutes(%.0f) = %d want %d", dist, got, want)
		}
	}
}

func TestSummary(t *testing.T) {
	home := Point{Lon: 127.0700, Lat: 37.2010}
	m := NewInfraMap(map[InfraKind]FacilitySource{
		InfraConvenience: fakeFacilities{{Name: "near", Lon: 127.0710, Lat: 37.2010}},
		InfraHospital:    fakeFacilities{},
	})
	lines, err := m.Summary(context.Background(), home.Lon, home.Lat)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 4 || lines[0].Kind != InfraSubway || lines[3].Kind != InfraHospital {
		t.Fatalf("lines = %+v", lines)
	}
	if lines[0].Found || lines[0].Note != "데이터 준비 중" {
		t.Errorf("subway line = %+v", lines[0])
	}
	c := lines[2]
	if !c.Found || c.Facility.Name != "near" || c.Facility.WalkMinutes != 2 {
		t.Errorf("convenience line = %+v", c)
	}
	if lines[3].Found || lines[3].Note == "" {
		t.Errorf("hospital line = %+v", lines[3])
	}
}

func TestAroundSortsByDistance(t *testing.T) {
	m := NewInfraMap(map[InfraKind]FacilitySource{
		InfraBusStop: fakeFacilities{
			{Name: "far", Lon: 127.0750, Lat: 37.2010},
			{Name: "near", Lon: 127.0710, Lat: 37.2010},
		},
	})
	list, err := m.Around(context.Background(), InfraBusStop, 127.0700, 37.2010, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "near" || list[0].Kind != InfraBusStop {
		t.Fatalf("list = %+v", list)
	}
}

func TestSummaryPropagatesError(t *testing.T) {
	m := NewInfraMap(map[InfraKind]FacilitySource{InfraSubway: failingFacilities{}})
	if _, err := m.Summary(context.Background(), 127, 37); err == nil {
		t.Fatal("expected error")
	}
}
