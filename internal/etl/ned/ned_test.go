package ned

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 2026-10-28 운영 서버에서 받은 실제 응답 모양(필드 일부)을 그대로 쓴다.
const pricePage = `{"indvdLandPrices" : {"field" : [%s], "pageNo" : "%s", "resultCode" : "", "totalCount" : "%d", "numOfRows" : "1000", "resultMsg" : ""}}`
const priceField = `{"mnnmSlno" : "1", "pblntfDe" : "2026-04-30", "stdLandAt" : "N", "stdrYear" : "2026", "pnu" : "%s", "lastUpdtDt" : "2026-05-21", "ldCode" : "4145010600", "pblntfPclnd" : "%s", "regstrSeCode" : "1"}`
const featField = `{"tpgrphHgCodeNm" : "평지", "stdrYear" : "2026", "roadSideCodeNm" : "세로한면(가)", "ladUseSittnNm" : "주거용", "tpgrphFrmCodeNm" : "가로장방형", "lastUpdtDt" : "2026-05-12", "pnu" : "4145010600100010000"}`

func TestFetchDongPagesUntilTotal(t *testing.T) {
	// 서버가 요청(1000)보다 작은 페이지(2건)를 줘도 totalCount(3)까지 넘겨 받는지 본다.
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		if q.Get("pnu") != "4145010600" || q.Get("stdrYear") != "2026" || q.Get("domain") == "" || q.Get("key") != "SECRET" {
			t.Errorf("bad query: %v", q)
		}
		var fields []string
		switch q.Get("pageNo") {
		case "1":
			fields = []string{fmt.Sprintf(priceField, "4145010600100010000", "121800"), fmt.Sprintf(priceField, "4145010600100020000", "1,234,000")}
		case "2":
			fields = []string{fmt.Sprintf(priceField, "41450106001000X", "5")} // pnu 길이 오류 -> 버림
		}
		fmt.Fprintf(w, pricePage, strings.Join(fields, ","), q.Get("pageNo"), 3)
	}))
	defer srv.Close()
	c := &Client{Key: "SECRET", Domain: "https://example.test", Base: srv.URL + "/"}
	rows, total, err := c.FetchDong(context.Background(), Price, "4145010600", 2026)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || calls != 2 || len(rows) != 2 {
		t.Fatalf("total=%d calls=%d rows=%d, want 3/2/2", total, calls, len(rows))
	}
	if rows[1].PricePerSqm != 1234000 || rows[0].BaseYear != 2026 || rows[0].SourceVersion.Format("2006-01-02") != "2026-05-21" {
		t.Fatalf("row parse: %+v", rows)
	}
}

func TestFeaturesRowMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/getLandCharacteristics") {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprintf(w, `{"landCharacteristicss" : {"field" : [%s], "pageNo" : "1", "resultCode" : "", "totalCount" : "1", "numOfRows" : "1000", "resultMsg" : ""}}`, featField)
	}))
	defer srv.Close()
	c := &Client{Key: "k", Domain: "d", Base: srv.URL + "/"}
	rows, _, err := c.FetchDong(context.Background(), Features, "4145010600", 2026)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	r := rows[0]
	if r.LandUse != "주거용" || r.TerrainHeight != "평지" || r.TerrainShape != "가로장방형" || r.RoadSide != "세로한면(가)" {
		t.Fatalf("labels: %+v", r)
	}
}

func TestEmptyDongAndAPIErrorStops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pnu") == "1111111111" {
			fmt.Fprintf(w, pricePage, "", "1", 0)
			return
		}
		// 키 오류 등은 다른 모양의 JSON으로 온다고 가정. 원문에 키가 섞여도 오류 메시지엔 남지 않아야 한다.
		fmt.Fprint(w, `{"response":{"status":"ERROR","error":{"code":"INCORRECT_KEY","text":"key SECRET is invalid"}}}`)
	}))
	defer srv.Close()
	c := &Client{Key: "SECRET", Domain: "d", Base: srv.URL + "/"}
	rows, total, err := c.FetchDong(context.Background(), Price, "1111111111", 2026)
	if err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("empty dong: rows=%v total=%d err=%v", rows, total, err)
	}
	_, _, err = c.FetchDong(context.Background(), Price, "2222222222", 2026)
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want APIError, got %v", err)
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("key leaked: %v", err)
	}
}

func TestServerErrorRetriesThenFailsWithoutKey(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := &Client{Key: "SECRET", Domain: "d", Base: srv.URL + "/", RetryWait: time.Millisecond}
	_, _, err := c.FetchDong(context.Background(), Price, "4145010600", 2026)
	if err == nil || calls != 3 {
		t.Fatalf("want 3 attempts and error, got calls=%d err=%v", calls, err)
	}
	var ae *APIError
	if errors.As(err, &ae) {
		t.Fatalf("5xx should be retryable error, not APIError: %v", err)
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("key leaked: %v", err)
	}
}

func TestDedupeKeepsLast(t *testing.T) {
	in := []Row{{PNU: "a", BaseYear: 2026, PricePerSqm: 1}, {PNU: "b", BaseYear: 2026}, {PNU: "a", BaseYear: 2026, PricePerSqm: 2}}
	out := dedupe(in)
	if len(out) != 2 || out[0].PricePerSqm != 2 {
		t.Fatalf("dedupe: %+v", out)
	}
}
