package b2c

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestCandidatesFromUnits(t *testing.T) {
	units := []RegisteredUnit{
		{Dong: "1622동", Ho: "1502", Floor: 15, AreaM2: 84.9},
		{Dong: "1621동", Ho: "102", Floor: 1, AreaM2: 84.9},
		{Dong: "1621동", Ho: "201", Floor: 2, AreaM2: 84.9},
		{Dong: "1621동", Ho: "101", Floor: 1, AreaM2: 84.9},
		{Dong: "1621동", Ho: "103", Floor: 1, AreaM2: 59.98},
	}
	got := NewFloorPlanCatalog().CandidatesFromUnits("c", units)
	names := []string{}
	for _, v := range got {
		names = append(names, v.TypeName)
	}
	want := []string{"59.98㎡ 03라인", "84.90㎡ 01라인", "84.90㎡ 02라인"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %v", names)
	}
	if u := got[1].Units; len(u) != 2 || u[0] != "1621동 101호" || u[1] != "1621동 201호" {
		t.Errorf("01 line units = %v", u)
	}
	if u := got[2].Units; len(u) != 2 || u[0] != "1621동 102호" || u[1] != "1622동 1502호" {
		t.Errorf("02 line units = %v", u)
	}
	if got[0].ComplexID != "c" || got[0].ExclusiveAreaM2 != 59.98 {
		t.Errorf("first = %+v", got[0])
	}
}

func TestBuildingRegisterSource(t *testing.T) {
	item := func(dong, ho, gb, purpose, area string) string {
		return `<item><bldNm>동탄역 신미주</bldNm><dongNm>` + dong + `</dongNm><hoNm>` + ho + `</hoNm><flrNo>1</flrNo>` +
			`<exposPubuseGbCd>` + gb + `</exposPubuseGbCd><mainPurpsCdNm>` + purpose + `</mainPurpsCdNm><area>` + area + `</area></item>`
	}
	pages := map[string]string{
		"1": item("1622동", "102", "1", "아파트", "84.896") + item("1622동", "102", "2", "아파트", "22.6765") + item("상가동", "101", "1", "제2종근린생활시설", "30"),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("serviceKey") != "k" || q.Get("sigunguCd") != "41597" || q.Get("bun") != "0964" || q.Get("numOfRows") != "100" {
			t.Errorf("query = %v", q)
		}
		// 행이 100개보다 적으면 totalCount와 상관없이 마지막 페이지로 본다.
		fmt.Fprintf(w, `<response><header><resultCode>00</resultCode></header><body><items>%s</items><totalCount>150</totalCount></body></response>`, pages[q.Get("pageNo")])
	}))
	defer srv.Close()
	src := &BuildingRegisterSource{Key: "k", BaseURL: srv.URL}
	units, err := src.Units(context.Background(), "41597", "11500", "0964", "0000")
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].Dong != "1622동" || units[0].AreaM2 != 84.9 || units[0].BuildingName != "동탄역 신미주" {
		t.Fatalf("units = %+v", units)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<OpenAPI_ServiceResponse><cmmMsgHeader><errMsg>SERVICE_KEY_IS_NOT_REGISTERED_ERROR</errMsg></cmmMsgHeader></OpenAPI_ServiceResponse>`)
	}))
	defer bad.Close()
	if _, err := (&BuildingRegisterSource{Key: "secret", BaseURL: bad.URL}).Units(context.Background(), "41597", "11500", "0964", "0000"); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildingRegisterSourcePaging(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var b strings.Builder
		if r.URL.Query().Get("pageNo") == "1" {
			for i := 0; i < buildingRegisterPageRows; i++ { // 꽉 찬 첫 페이지: 공용 행 99개 + 전유 1개
				gb := "2"
				if i == 0 {
					gb = "1"
				}
				fmt.Fprintf(&b, `<item><dongNm>101동</dongNm><hoNm>101</hoNm><exposPubuseGbCd>%s</exposPubuseGbCd><mainPurpsCdNm>아파트</mainPurpsCdNm><area>84.9</area></item>`, gb)
			}
		} else { // 둘째 페이지: 같은 세대 중복 1개 + 새 세대 1개
			b.WriteString(`<item><dongNm>101동</dongNm><hoNm>101</hoNm><exposPubuseGbCd>1</exposPubuseGbCd><mainPurpsCdNm>아파트</mainPurpsCdNm><area>84.9</area></item>`)
			b.WriteString(`<item><dongNm>101동</dongNm><hoNm>102</hoNm><exposPubuseGbCd>1</exposPubuseGbCd><mainPurpsCdNm>아파트</mainPurpsCdNm><area>59.9</area></item>`)
		}
		fmt.Fprintf(w, `<response><header><resultCode>00</resultCode></header><body><items>%s</items><totalCount>102</totalCount></body></response>`, b.String())
	}))
	defer srv.Close()
	units, err := (&BuildingRegisterSource{Key: "k", BaseURL: srv.URL}).Units(context.Background(), "41597", "11500", "0964", "0000")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(units) != 2 || units[1].Ho != "102" {
		t.Fatalf("calls=%d units=%+v", calls, units)
	}
}

func TestCandidateNamesFollowSortOrder(t *testing.T) {
	got := NewFloorPlanCatalog().CandidatesFromUnits("c", []RegisteredUnit{
		{Dong: "1동", Ho: "101", AreaM2: 59.98}, {Dong: "1동", Ho: "102", AreaM2: 59.95}, {Dong: "1동", Ho: "103", AreaM2: 59.95},
	})
	var names []string
	for _, v := range got {
		names = append(names, v.TypeName)
	}
	if strings.Join(names, ",") != "59.95㎡ 02라인,59.95㎡ 03라인,59.98㎡ 01라인" {
		t.Fatalf("names = %v", names)
	}
}
