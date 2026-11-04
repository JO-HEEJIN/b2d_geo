package b2c

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var tradeNow = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

func fixedAnalyzer(src TradeSource) *RecentTradeAnalyzer {
	a := NewRecentTradeAnalyzer(src)
	a.now = func() time.Time { return tradeNow }
	return a
}

// tradesEvery는 now 이전 days일 간격으로 n건의 거래를 만든다.
func tradesEvery(name string, n, days int) []AptTrade {
	var out []AptTrade
	for i := 0; i < n; i++ {
		out = append(out, AptTrade{ComplexName: name, DealDate: tradeNow.AddDate(0, 0, -1-i*days)})
	}
	return out
}

func TestActivityLevels(t *testing.T) {
	a := fixedAnalyzer(nil)
	cases := []struct {
		trades []AptTrade
		want   Liveliness
	}{
		{tradesEvery("x", 30, 6), LivelinessHot},
		{tradesEvery("x", 8, 20), LivelinessActive},
		{tradesEvery("x", 2, 30), LivelinessSlow},
		{nil, LivelinessDormant},
	}
	for _, c := range cases {
		if got := a.Activity(c.trades, "").Level; got != c.want {
			t.Errorf("got %s want %s (n=%d)", got, c.want, len(c.trades))
		}
	}
}

func TestActivityFiltersWindowComplexAndCanceled(t *testing.T) {
	a := fixedAnalyzer(nil)
	trades := append(tradesEvery("a", 3, 10), tradesEvery("b", 5, 10)...)
	trades = append(trades,
		AptTrade{ComplexName: "a", DealDate: tradeNow.AddDate(0, -7, 0)},                 // 6개월 밖
		AptTrade{ComplexName: "a", DealDate: tradeNow.AddDate(0, 0, -3), Canceled: true}, // 해제
	)
	act := a.Activity(trades, "a")
	if act.Count != 3 || act.Canceled != 1 {
		t.Fatalf("count=%d canceled=%d", act.Count, act.Canceled)
	}
	if act.AvgIntervalDays != 10 || act.DaysSinceLast != 1 {
		t.Errorf("interval=%.1f since=%d", act.AvgIntervalDays, act.DaysSinceLast)
	}
}

func TestVerdict(t *testing.T) {
	a := fixedAnalyzer(nil)
	act := TradeActivity{Count: 30, AvgIntervalDays: 6, RecentHalf: 20, EarlierHalf: 10, Canceled: 1, Level: LivelinessHot}
	v := a.Verdict(act, 2008)
	if !strings.Contains(v.Message, "30건") {
		t.Errorf("message = %s", v.Message)
	}
	joined := strings.Join(v.Cautions, "\n")
	for _, want := range []string{"체크리스트", "수요가 몰리는", "해제", "18년차"} {
		if !strings.Contains(joined, want) {
			t.Errorf("cautions missing %q: %s", want, joined)
		}
	}
	if v := a.Verdict(TradeActivity{Level: LivelinessDormant}, 0); len(v.Cautions) != 0 || !strings.Contains(v.Disclaimer, "30일") {
		t.Errorf("unexpected cautions %v / disclaimer %q", v.Cautions, v.Disclaimer)
	}
}

type stubSource struct{ from, to time.Time }

func (s *stubSource) Trades(_ context.Context, _ string, from, to time.Time) ([]AptTrade, error) {
	s.from, s.to = from, to
	return nil, nil
}

func TestFetchWindow(t *testing.T) {
	src := &stubSource{}
	if _, err := fixedAnalyzer(src).Fetch(context.Background(), "41590"); err != nil {
		t.Fatal(err)
	}
	if !src.to.Equal(tradeNow) || !src.from.Equal(tradeNow.AddDate(0, -6, 0)) {
		t.Errorf("window %v ~ %v", src.from, src.to)
	}
	if _, err := fixedAnalyzer(nil).Fetch(context.Background(), "41590"); err == nil {
		t.Error("expected error without source")
	}
}

func TestMolitAptTradeSource(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		calls = append(calls, q.Get("DEAL_YMD")+"/"+q.Get("pageNo"))
		if q.Get("serviceKey") != "k+y" || q.Get("LAWD_CD") != "41597" {
			t.Errorf("query = %v", q)
		}
		if q.Get("DEAL_YMD") != "202610" {
			fmt.Fprint(w, `<response><header><resultCode>000</resultCode><resultMsg>OK</resultMsg></header><body><items></items><totalCount>0</totalCount></body></response>`)
			return
		}
		fmt.Fprint(w, `<response><header><resultCode>000</resultCode><resultMsg>OK</resultMsg></header><body><items>
<item><aptNm>동탄역시범</aptNm><dealAmount>    98,500</dealAmount><dealYear>2026</dealYear><dealMonth>10</dealMonth><dealDay>7</dealDay><excluUseAr>84.97</excluUseAr><floor>12</floor><buildYear>2015</buildYear><cdealType> </cdealType></item>
<item><aptNm>동탄역시범</aptNm><dealAmount>97,000</dealAmount><dealYear>2026</dealYear><dealMonth>10</dealMonth><dealDay>9</dealDay><excluUseAr>84.97</excluUseAr><floor>3</floor><buildYear>2015</buildYear><cdealType>O</cdealType></item>
<item><aptNm>broken</aptNm><dealYear>2026</dealYear><dealMonth></dealMonth><dealDay>1</dealDay></item>
</items><totalCount>3</totalCount></body></response>`)
	}))
	defer srv.Close()
	src := &MolitAptTradeSource{Key: "k+y", BaseURL: srv.URL}
	from := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	got, err := src.Trades(context.Background(), "41597", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, ",") != "202609/1,202610/1" {
		t.Errorf("calls = %v", calls)
	}
	if len(got) != 2 {
		t.Fatalf("trades = %+v", got)
	}
	a := got[0]
	if a.ComplexName != "동탄역시범" || a.PriceManwon != 98500 || a.AreaM2 != 84.97 || a.Floor != 12 || a.BuildYear != 2015 || a.Canceled {
		t.Errorf("first = %+v", a)
	}
	if a.DealDate.Day() != 7 || !got[1].Canceled {
		t.Errorf("date/canceled = %v %v", a.DealDate, got[1].Canceled)
	}
}

func TestMolitAptTradeSourceErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<OpenAPI_ServiceResponse><cmmMsgHeader><errMsg>SERVICE_KEY_IS_NOT_REGISTERED_ERROR</errMsg></cmmMsgHeader></OpenAPI_ServiceResponse>`)
	}))
	defer srv.Close()
	src := &MolitAptTradeSource{Key: "secret-key", BaseURL: srv.URL}
	_, err := src.Trades(context.Background(), "41597", tradeNow, tradeNow)
	if err == nil || strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("err = %v", err)
	}
	if _, err := (&MolitAptTradeSource{}).Trades(context.Background(), "41597", tradeNow, tradeNow); err == nil {
		t.Error("expected error without key")
	}
}

func TestMolitAptTradeSourcePaging(t *testing.T) {
	pages := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		n := molitAptPageRows
		if r.URL.Query().Get("pageNo") == "2" {
			n = 5
		}
		var b strings.Builder
		b.WriteString(`<response><header><resultCode>000</resultCode></header><body><items>`)
		for i := 0; i < n; i++ {
			b.WriteString(`<item><aptNm>a</aptNm><dealAmount>1</dealAmount><dealYear>2026</dealYear><dealMonth>10</dealMonth><dealDay>1</dealDay></item>`)
		}
		fmt.Fprintf(&b, `</items><totalCount>%d</totalCount></body></response>`, molitAptPageRows+5)
		fmt.Fprint(w, b.String())
	}))
	defer srv.Close()
	got, err := (&MolitAptTradeSource{Key: "k", BaseURL: srv.URL}).Trades(context.Background(), "41597", tradeNow.AddDate(0, -1, 0), tradeNow.AddDate(0, -1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 || len(got) != molitAptPageRows+5 {
		t.Fatalf("pages=%d trades=%d", pages, len(got))
	}
}
