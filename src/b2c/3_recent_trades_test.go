package b2c

import (
	"context"
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
	if v := a.Verdict(TradeActivity{Level: LivelinessDormant}, 0); len(v.Cautions) != 0 {
		t.Errorf("unexpected cautions %v", v.Cautions)
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
