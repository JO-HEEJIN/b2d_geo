package b2c

import "testing"

func TestClassify(t *testing.T) {
	d := NewDistrictClassifier()
	cases := []struct {
		addr string
		want string
	}{
		{"인천광역시 연수구 송도동 23-1", "songdo"},
		{"경기도 화성시 반송동 88", "dongtan1"},
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
