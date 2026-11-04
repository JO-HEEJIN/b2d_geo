package b2c

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClassify(t *testing.T) {
	d := NewDistrictClassifier()
	cases := []struct {
		addr string
		want string
	}{
		{"인천광역시 연수구 송도동 23-1", "songdo"},
		{"경기도 화성시 반송동 88", "dongtan1"},
		{"경기도 화성시 동탄구 반송동 88", "dongtan1"},
		{"경기도 화성시 동탄구 여울동 964", "dongtan2"},
		{"경기도 화성시 동탄대로 100 (청계동, 동탄역시범아파트)", "dongtan2"},
		{"경기도 하남시 미사강변남로 99", "hanam"},
		{"경기도 용인시 수지구 풍덕천동 1", "suji"},
		{"경기도 수원시 영통구 매탄동 1", "suwon"},
		{"서울특별시 송파구 잠실2동 1", "jamsil"},
		{"서울특별시 양천구 목동 1", ""}, // 목동이지만 화성이 아니다
		{"서울특별시 송파구 가락동 1", ""},
		{"경기도 화성시 동탄대로 100", ""}, // 법정동 없는 도로명주소는 1/2동탄을 가를 수 없다
	}
	for _, c := range cases {
		z, ok := d.Classify(c.addr)
		if z.Key != c.want || ok != (c.want != "") {
			t.Errorf("Classify(%q) = %q,%v want %q", c.addr, z.Key, ok, c.want)
		}
	}
}

func TestNearestZone(t *testing.T) {
	z, m := NewDistrictClassifier().NearestZone(127.0857, 37.5133) // 잠실역 부근
	if z.Key != "jamsil" || m > 2000 {
		t.Fatalf("got %s %.0fm", z.Key, m)
	}
}

func TestCluster(t *testing.T) {
	pts := []Point{
		{"a1", 127.000, 37.000}, {"a2", 127.001, 37.001}, {"a3", 127.002, 37.000},
		{"b1", 127.100, 37.100}, {"b2", 127.101, 37.101},
	}
	groups := NewDistrictClassifier().Cluster(pts, 2)
	if len(groups) != 2 {
		t.Fatalf("groups = %d", len(groups))
	}
	if n0, n1 := len(groups[0]), len(groups[1]); n0+n1 != 5 || (n0 != 3 && n1 != 3) {
		t.Fatalf("sizes = %d,%d", n0, n1)
	}
}

func TestPNU(t *testing.T) {
	cases := map[string]string{
		"80":      "4159710200100800000",
		"722-3":   "4159710200107220003",
		"산12-1":   "4159710200200120001",
		" 1234 ":  "4159710200112340000",
		"":        "",
		"abc":     "",
		"12345":   "",
		"12-abcd": "",
	}
	for jibun, want := range cases {
		got, ok := PNU("4159710200", jibun)
		if got != want || ok != (want != "") {
			t.Errorf("PNU(%q) = %q,%v want %q", jibun, got, ok, want)
		}
	}
	if _, ok := PNU("41597", "80"); ok {
		t.Error("short bcode accepted")
	}
}

func TestZoneLookupAndInZone(t *testing.T) {
	d := NewDistrictClassifier()
	z, ok := d.Zone("dongtan2")
	if !ok || z.Lawd[0] != "41597" || !z.InZone("여울동") || z.InZone("반송동") {
		t.Fatalf("dongtan2 = %+v", z)
	}
	h, _ := d.Zone("hanam")
	if !h.InZone("망월동") {
		t.Error("hanam should include every dong")
	}
	if s, _ := d.Zone("suwon"); len(s.Lawd) != 4 {
		t.Errorf("suwon lawd = %v", s.Lawd)
	}
	if _, ok := d.Zone("busan"); ok {
		t.Error("unknown zone found")
	}
}

func TestLegalDongResolver(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("locatadd_nm") != "경기도 화성시 동탄구" || r.URL.Query().Get("flag") != "Y" {
			t.Errorf("query = %v", r.URL.Query())
		}
		fmt.Fprint(w, `{"StanReginCd":[{"head":[{"totalCount":3}]},{"row":[
			{"region_cd":"4159700000","locallow_nm":""},
			{"region_cd":"4159710200","locallow_nm":"반송동"},
			{"region_cd":"4159711500","locallow_nm":"여울동"},
			{"region_cd":"4159710221","locallow_nm":"어떤리"}]}]}`)
	}))
	defer srv.Close()
	r := &LegalDongResolver{Key: "k", BaseURL: srv.URL}
	for i := 0; i < 2; i++ {
		codes, err := r.Codes(context.Background(), "경기도 화성시 동탄구")
		if err != nil {
			t.Fatal(err)
		}
		if len(codes) != 2 || codes["41597|반송동"] != "4159710200" || codes["41597|여울동"] != "4159711500" {
			t.Fatalf("codes = %v", codes)
		}
	}
	if calls != 1 {
		t.Errorf("calls = %d (cache miss)", calls)
	}
	if _, err := (&LegalDongResolver{}).Codes(context.Background(), "x"); err == nil {
		t.Error("expected error without key")
	}
}
