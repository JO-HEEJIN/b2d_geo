package rone

import (
	"reflect"
	"testing"
)

func TestParseRegion(t *testing.T) {
	// 지역 이름은 운영 서버 프로브 출력(2026-10-28)에 나온 값 그대로다.
	cases := map[string]place{
		"부산>중구":       {Name: "부산광역시 중구"},
		"경기>화성시":      {Name: "경기도 화성시"},
		"경기>화성시>만세구":  {Name: "경기도 화성시 만세구"},
		"세종":          {Code: "36110"},
		"인천>(구)동구":    {Name: "인천광역시 동구", Legacy: true},
		"전남광주>광주>광산구": {Name: "광주광역시 광산구"},
		"전남광주>광주":     {Aggregate: true},
		"전남광주>순천시":    {Name: "전라남도 순천시"},
		"서울":          {Aggregate: true},
		"전국":          {Aggregate: true},
		"":            {Aggregate: true},
		"경기>시지역":      {Aggregate: true},
		"강원>군지역":      {Aggregate: true},
		"없는도>어딘가":     {},
		" 대구 > 군위군 ":  {Name: "대구광역시 군위군"},
		"강원>춘천시":      {Name: "강원특별자치도 춘천시"},
	}
	for in, want := range cases {
		if got := parseRegion(in); !reflect.DeepEqual(got, want) {
			t.Errorf("parseRegion(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestBuild(t *testing.T) {
	xw := map[string]string{
		"부산광역시 중구":    "26110",
		"경기도 수원시 장안구": "41111",
		"경기도 화성시":     "41590", // 주소 DB가 구 신설 전이면 시 단위만 있다
		"인천광역시 서구":    "28260",
		"인천광역시 동구":    "28140",
		"광주광역시 광산구":   "29200",
		"전라남도 순천시":    "46150",
		// "경기도 수원시"는 구가 여럿이라 크로스워크에서 빠져 있다
	}
	r := func(grp, zone string, v float64) Row { return Row{Region: grp, Zone: zone, Month: "202606", Rate: v} }
	rows := []Row{
		r("부산>중구", "주거지역", 0.016),
		r("인천>(구)서구", "주거지역", 9), // 같은 코드에 현재 이름 행이 뒤에 온다 -> 버림
		r("인천>서구", "주거지역", 0.2),
		r("인천>(구)동구", "주거지역", 0.3), // 현재 이름 행이 없다 -> 그대로 씀
		r("경기>수원시", "주거지역", 5),     // 하위 구가 있는 시 집계 -> 집계로 셈
		r("경기>수원시>장안구", "주거지역", 0.4),
		r("경기>화성시", "주거지역", 0.5),
		r("경기>화성시>만세구", "주거지역", 7), // 주소 DB에 없는 신설 구 -> 미매핑
		r("세종", "주거지역", 0.6),
		r("전남광주>광주>광산구", "녹지지역", 0.7),
		r("전남광주>순천시", "관리(통합)지역", 0.8),
		r("서울", "주거지역", 1),
		r("부산>중구", "주거지역", 99), // 같은 키 두 번째 -> 첫 행만
	}
	st := &Stats{}
	unknown := map[string]bool{}
	got := build(rows, xw, st, unknown)
	want := []rec{
		{"26110", "주거지역", "202606", 0.016, false},
		{"28260", "주거지역", "202606", 0.2, false},
		{"28140", "주거지역", "202606", 0.3, true},
		{"41111", "주거지역", "202606", 0.4, false},
		{"41590", "주거지역", "202606", 0.5, false},
		{"36110", "주거지역", "202606", 0.6, false},
		{"29200", "녹지지역", "202606", 0.7, false},
		{"46150", "관리(통합)지역", "202606", 0.8, false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("build:\n got %+v\nwant %+v", got, want)
	}
	if st.Aggregate != 2 || st.Unmapped != 1 || st.Superseded != 1 || st.Dup != 1 {
		t.Fatalf("stats %+v", st)
	}
	if !unknown["경기>화성시>만세구"] || len(unknown) != 1 {
		t.Fatalf("unknown %v", unknown)
	}
}

func TestPickAndMonthRange(t *testing.T) {
	ms, err := monthRange("202511", "202602")
	if err != nil || !reflect.DeepEqual(ms, []string{"202511", "202512", "202601", "202602"}) {
		t.Fatalf("monthRange %v %v", ms, err)
	}
	for _, bad := range [][2]string{{"202602", "202511"}, {"2025", "202602"}, {"202513", "202602"}} {
		if _, err := monthRange(bad[0], bad[1]); err == nil {
			t.Errorf("%v should fail", bad)
		}
	}
	all := []string{"202601", "202602", "202603", "202604", "202605", "202606"}
	// 받은 달 202601~202604, 마지막 2개월은 다시 -> 202603~202606
	got := pick(all, []string{"202601", "202602", "202603", "202604"}, 2)
	if !reflect.DeepEqual(got, []string{"202603", "202604", "202605", "202606"}) {
		t.Fatalf("pick %v", got)
	}
	if got := pick(all, nil, 3); !reflect.DeepEqual(got, all) {
		t.Fatalf("pick first run %v", got)
	}
	if got := pick(all, all, 0); got != nil {
		t.Fatalf("pick all loaded, refetch 0: %v", got)
	}
}
