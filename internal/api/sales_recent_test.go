package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecentMonths(t *testing.T) {
	now := time.Date(2026, 1, 31, 23, 0, 0, 0, kst)
	got := recentMonths(now, 3)
	want := []string{"202601", "202512", "202511"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("recentMonths = %v, want %v", got, want)
	}
	// 3월 31일에서 한 달 빼기가 3월 3일로 넘어가는 AddDate 함정 회피 확인.
	got = recentMonths(time.Date(2026, 3, 31, 0, 0, 0, 0, kst), 2)
	if strings.Join(got, ",") != "202603,202602" {
		t.Fatalf("month-end rollover: %v", got)
	}
}

func TestMolitCacheTTLAndCap(t *testing.T) {
	c := &molitCache{m: map[string]molitCacheEntry{}}
	t0 := time.Date(2026, 10, 24, 12, 0, 0, 0, kst)
	c.put("11680:202610", []molitItem{{Jimok: "대"}}, t0)
	if _, ok := c.get("11680:202610", t0.Add(molitCacheTTL-time.Minute)); !ok {
		t.Fatal("entry should be fresh before TTL")
	}
	if _, ok := c.get("11680:202610", t0.Add(molitCacheTTL+time.Minute)); ok {
		t.Fatal("entry should expire after TTL")
	}
	for i := 0; i < molitCacheMax+5; i++ {
		c.put(strings.Repeat("k", i+1), nil, t0)
	}
	if len(c.m) > molitCacheMax {
		t.Fatalf("cache grew past cap: %d", len(c.m))
	}

	// 가득 찼을 때 만료 항목부터 지우고, 살아 있는 항목은 통째로 날리지 않는다.
	c = &molitCache{m: map[string]molitCacheEntry{}}
	c.put("old", nil, t0)
	t1 := t0.Add(molitCacheTTL + time.Minute)
	for i := 0; i < molitCacheMax-1; i++ {
		c.put(strings.Repeat("f", i+1), nil, t1)
	}
	c.put("new", nil, t1)
	if _, ok := c.m["old"]; ok {
		t.Fatal("expired entry should be evicted first")
	}
	if len(c.m) != molitCacheMax {
		t.Fatalf("fresh entries should survive, len=%d want %d", len(c.m), molitCacheMax)
	}
}

// stubTransport는 data.go.kr 대신 응답한다. failYM에 해당하는 달은 에러 XML을 준다.
type stubTransport struct {
	calls  atomic.Int32
	failYM string
	fullYM string // 이 달은 한 페이지(molitPageRows건)를 꽉 채워 준다
}

func (s *stubTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	s.calls.Add(1)
	ym := r.URL.Query().Get("DEAL_YMD")
	item := `<item><dealYear>` + ym[:4] + `</dealYear><dealMonth>` + ym[4:] + `</dealMonth>` +
		`<dealDay>5</dealDay><dealAmount>45,000</dealAmount><dealArea>4</dealArea><jimok>도로</jimok>` +
		`<umdNm>청담동</umdNm><jibun>1**</jibun></item>`
	n := 1
	if ym == s.fullYM {
		n = molitPageRows
	}
	body := `<response><header><resultCode>000</resultCode><resultMsg>OK</resultMsg></header>` +
		`<body><items>` + strings.Repeat(item, n) + `</items><totalCount>` + strconv.Itoa(n) + `</totalCount></body></response>`
	if ym == s.failYM {
		body = `<response><header><resultCode>30</resultCode><resultMsg>SERVICE KEY IS NOT REGISTERED</resultMsg></header></response>`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)),
		Header: http.Header{"Content-Type": []string{"application/xml"}}, Request: r}, nil
}

func newStubServer(st *stubTransport, key string) *Server {
	return &Server{MolitKey: key, Client: &http.Client{Transport: st}}
}

func TestSalesRecentValidation(t *testing.T) {
	s := newStubServer(&stubTransport{}, "k")
	for _, q := range []string{"", "lawd_cd=1168", "lawd_cd=11a80", "lawd_cd=11680&months=0", "lawd_cd=11680&months=7", "lawd_cd=11680&months=x"} {
		rec := httptest.NewRecorder()
		s.appraisalSalesRecent(rec, httptest.NewRequest("GET", "/v1/appraisal/sales/recent?"+q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%q: code %d, want 400", q, rec.Code)
		}
	}
}

func TestSalesRecentNoKey(t *testing.T) {
	s := newStubServer(&stubTransport{}, "")
	rec := httptest.NewRecorder()
	s.appraisalSalesRecent(rec, httptest.NewRequest("GET", "/v1/appraisal/sales/recent?lawd_cd=11680", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code %d, want 503", rec.Code)
	}
}

func TestSalesRecentAggregatesAndCaches(t *testing.T) {
	salesCache = &molitCache{m: map[string]molitCacheEntry{}}
	st := &stubTransport{}
	s := newStubServer(st, "secret")
	get := func() map[string]any {
		rec := httptest.NewRecorder()
		s.appraisalSalesRecent(rec, httptest.NewRequest("GET", "/v1/appraisal/sales/recent?lawd_cd=11680&months=3", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := get()
	if n := out["total_count"].(float64); n != 3 {
		t.Fatalf("total_count %v, want 3 (one per month)", n)
	}
	if len(out["months"].([]any)) != 3 || len(out["failed_months"].([]any)) != 0 {
		t.Fatalf("months/failed_months: %v / %v", out["months"], out["failed_months"])
	}
	if pts := out["umd_points"].([]any); len(pts) != 0 {
		t.Fatalf("nil pool should give empty umd_points, got %v", pts)
	}
	if st.calls.Load() != 3 {
		t.Fatalf("upstream calls %d, want 3", st.calls.Load())
	}
	get() // 두 번째 호출은 전부 캐시
	if st.calls.Load() != 3 {
		t.Fatalf("second call should be cached, upstream calls now %d", st.calls.Load())
	}
}

func TestSalesRecentPartialAndTotalFailure(t *testing.T) {
	salesCache = &molitCache{m: map[string]molitCacheEntry{}}
	months := recentMonths(time.Now().In(kst), 2)
	st := &stubTransport{failYM: months[1]}
	s := newStubServer(st, "secret")
	rec := httptest.NewRecorder()
	s.appraisalSalesRecent(rec, httptest.NewRequest("GET", "/v1/appraisal/sales/recent?lawd_cd=11680&months=2", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("partial failure should still be 200, got %d", rec.Code)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	f := out["failed_months"].([]any)
	if len(f) != 1 || f[0].(string) != months[1] {
		t.Fatalf("failed_months %v, want [%s]", f, months[1])
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatal("response leaked the MOLIT key")
	}

	salesCache = &molitCache{m: map[string]molitCacheEntry{}}
	one := recentMonths(time.Now().In(kst), 1)
	s = newStubServer(&stubTransport{failYM: one[0]}, "secret")
	rec = httptest.NewRecorder()
	s.appraisalSalesRecent(rec, httptest.NewRequest("GET", "/v1/appraisal/sales/recent?lawd_cd=11680&months=1", nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("all months failed should be 502, got %d", rec.Code)
	}
}

func TestSalesRecentFlagsTruncatedMonth(t *testing.T) {
	salesCache = &molitCache{m: map[string]molitCacheEntry{}}
	months := recentMonths(time.Now().In(kst), 2)
	s := newStubServer(&stubTransport{fullYM: months[0]}, "secret")
	rec := httptest.NewRecorder()
	s.appraisalSalesRecent(rec, httptest.NewRequest("GET", "/v1/appraisal/sales/recent?lawd_cd=11680&months=2", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	tr := out["truncated_months"].([]any)
	if len(tr) != 1 || tr[0].(string) != months[0] {
		t.Fatalf("truncated_months %v, want [%s]", tr, months[0])
	}
}
