package b2c

import (
	"context"
	"fmt"
	"time"
)

// AptTrade는 아파트 매매 실거래 1건이다 (국토교통부 실거래가 공개 자료 기준).
type AptTrade struct {
	ComplexName string
	DealDate    time.Time
	PriceManwon int // 거래금액 (만원)
	AreaM2      float64
	Floor       int
	BuildYear   int
	Canceled    bool // 해제된 거래
}

// TradeSource는 실거래 원천이다. lawdCode는 시군구 5자리 법정동코드다.
// 국토교통부 아파트 매매 실거래 API 연동은 별도 승인 후 구현한다.
type TradeSource interface {
	Trades(ctx context.Context, lawdCode string, from, to time.Time) ([]AptTrade, error)
}

// Liveliness는 거래 속도로 본 동네 상태다.
type Liveliness string

const (
	LivelinessHot     Liveliness = "hot"     // 매물이 나오는 족족 팔린다
	LivelinessActive  Liveliness = "active"  // 꾸준히 거래된다
	LivelinessSlow    Liveliness = "slow"    // 드물게 거래된다
	LivelinessDormant Liveliness = "dormant" // 6개월간 거래가 없다
)

// tradeWindow는 거래 빈도를 보는 기간이다. 6개월 동안 몇 번 거래됐는지가 핵심 지표다.
const tradeWindow = 6

// TradeActivity는 최근 6개월 거래 빈도 요약이다.
type TradeActivity struct {
	Count           int        // 해제 제외 거래 수
	Canceled        int        // 해제 거래 수
	AvgIntervalDays float64    // 거래 사이 평균 간격 (2건 이상일 때)
	DaysSinceLast   int        // 마지막 거래 후 지난 날 (-1이면 거래 없음)
	RecentHalf      int        // 최근 3개월 거래 수
	EarlierHalf     int        // 그 전 3개월 거래 수
	Level           Liveliness
}

// BuyerVerdict는 사람이 집을 고를 때 실제로 느끼는 감정을 따라 쓴 판단 문구다.
type BuyerVerdict struct {
	Level    Liveliness
	Message  string
	Cautions []string
}

// RecentTradeAnalyzer는 최근 실거래 빈도로 살아 있는 동네인지 판단한다.
// 실거래가 있다는 것은 수요와 공급이 만나 실제로 계약이 성사됐다는 뜻이고,
// 개인이 매매를 결정할 때 가장 믿을 만한 근거가 된다.
type RecentTradeAnalyzer struct {
	source TradeSource
	now    func() time.Time
}

// NewRecentTradeAnalyzer는 실거래 원천으로 분석기를 만든다. source가 nil이면
// Fetch는 쓸 수 없고, 이미 가진 거래 목록으로 Activity/Verdict만 쓸 수 있다.
func NewRecentTradeAnalyzer(source TradeSource) *RecentTradeAnalyzer {
	return &RecentTradeAnalyzer{source: source, now: time.Now}
}

// Fetch는 시군구의 최근 6개월 거래를 가져온다.
func (a *RecentTradeAnalyzer) Fetch(ctx context.Context, lawdCode string) ([]AptTrade, error) {
	if a.source == nil {
		return nil, fmt.Errorf("trade source not configured")
	}
	to := a.now()
	return a.source.Trades(ctx, lawdCode, to.AddDate(0, -tradeWindow, 0), to)
}

// Activity는 거래 목록에서 최근 6개월 빈도를 센다. complexName이 비지 않으면 그
// 단지만 센다. 예: 반도체 공장 직원들이 한 동네로 몰려 매물이 다 팔리는 곳은
// 거래 간격이 매우 짧게 나온다. 이것은 죽은 동네가 아니라는 반증이다.
func (a *RecentTradeAnalyzer) Activity(trades []AptTrade, complexName string) TradeActivity {
	now := a.now()
	start := now.AddDate(0, -tradeWindow, 0)
	mid := now.AddDate(0, -tradeWindow/2, 0)
	act := TradeActivity{DaysSinceLast: -1}
	var first, last time.Time
	for _, t := range trades {
		if complexName != "" && t.ComplexName != complexName {
			continue
		}
		if t.DealDate.Before(start) || t.DealDate.After(now) {
			continue
		}
		if t.Canceled {
			act.Canceled++
			continue
		}
		act.Count++
		if t.DealDate.Before(mid) {
			act.EarlierHalf++
		} else {
			act.RecentHalf++
		}
		if first.IsZero() || t.DealDate.Before(first) {
			first = t.DealDate
		}
		if t.DealDate.After(last) {
			last = t.DealDate
		}
	}
	if act.Count > 0 {
		act.DaysSinceLast = int(now.Sub(last).Hours() / 24)
	}
	if act.Count > 1 {
		act.AvgIntervalDays = last.Sub(first).Hours() / 24 / float64(act.Count-1)
	}
	switch {
	case act.Count >= 24: // 평균 일주일에 한 건 이상
		act.Level = LivelinessHot
	case act.Count >= tradeWindow: // 평균 한 달에 한 건 이상
		act.Level = LivelinessActive
	case act.Count >= 1:
		act.Level = LivelinessSlow
	default:
		act.Level = LivelinessDormant
	}
	return act
}

// Verdict는 거래 빈도와 준공연도로 판단 문구를 만든다. 숫자보다 사람이 느끼는
// 감정(놓칠까 하는 조급함, 남들도 샀다는 안도감, 계약 뒤 수리비 걱정)을 따른다.
// buildYear가 0이면 준공연도 경고를 생략한다.
func (a *RecentTradeAnalyzer) Verdict(act TradeActivity, buildYear int) BuyerVerdict {
	v := BuyerVerdict{Level: act.Level}
	switch act.Level {
	case LivelinessHot:
		v.Message = fmt.Sprintf("6개월 동안 %d건, 평균 %.0f일마다 거래됐습니다. 매물이 나오는 대로 팔리는 동네라 "+
			"마음에 든 집은 망설이는 사이 사라질 수 있습니다. 여러 집을 즐겁게 둘러보되, 첫눈에 든 집은 바로 다시 보는 일정을 잡으세요.",
			act.Count, act.AvgIntervalDays)
		v.Cautions = append(v.Cautions, "급하게 결정하면 구조와 하자 확인을 건너뛰기 쉽습니다. 집 보기 체크리스트를 꼭 채우세요.")
	case LivelinessActive:
		v.Message = fmt.Sprintf("6개월 동안 %d건 거래된 살아 있는 동네입니다. 다른 사람들도 여기서 사기로 결정했다는 뜻이라 "+
			"판단 근거로 믿을 만합니다. 서두르지 않고 여러 집을 비교해 볼 여유도 있습니다.", act.Count)
	case LivelinessSlow:
		v.Message = fmt.Sprintf("6개월 동안 %d건으로 거래가 드뭅니다. 천천히 구경하며 고르기 좋지만, "+
			"나중에 팔 때도 시간이 걸릴 수 있습니다.", act.Count)
	default:
		v.Message = "최근 6개월 거래가 없습니다. 기준이 될 실거래가가 없어 호가를 그대로 믿기 어렵습니다."
	}
	if act.RecentHalf > act.EarlierHalf && act.EarlierHalf > 0 {
		v.Cautions = append(v.Cautions, fmt.Sprintf("최근 3개월 거래가 그 전 3개월보다 늘었습니다 (%d건 -> %d건). 수요가 몰리는 중입니다.",
			act.EarlierHalf, act.RecentHalf))
	}
	if act.Canceled > 0 {
		v.Cautions = append(v.Cautions, fmt.Sprintf("해제된 거래가 %d건 있습니다. 거래 이력을 한 번 더 확인하세요.", act.Canceled))
	}
	if buildYear > 0 {
		age := a.now().Year() - buildYear
		switch {
		case age >= 25:
			v.Cautions = append(v.Cautions, fmt.Sprintf("준공 %d년차입니다. 욕실·주방·샷시에 배관 교체까지 리모델링 비용을 매매가에 더해 생각하세요.", age))
		case age >= 15:
			v.Cautions = append(v.Cautions, fmt.Sprintf("준공 %d년차입니다. 욕실·주방·샷시 리모델링 비용을 매매가에 더해 생각하세요.", age))
		}
	}
	return v
}
