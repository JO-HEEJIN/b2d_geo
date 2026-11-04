package b2c

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// AptTrade는 아파트 매매 실거래 1건이다 (국토교통부 실거래가 공개 자료 기준).
type AptTrade struct {
	ComplexName string    `json:"complex_name"`
	DealDate    time.Time `json:"deal_date"`
	PriceManwon int       `json:"price_manwon"` // 거래금액 (만원)
	AreaM2      float64   `json:"area_m2"`
	Floor       int       `json:"floor"`
	BuildYear   int       `json:"build_year"`
	Canceled    bool      `json:"canceled"` // 해제된 거래
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
	Count           int        `json:"count"`             // 해제 제외 거래 수
	Canceled        int        `json:"canceled"`          // 해제 거래 수
	AvgIntervalDays float64    `json:"avg_interval_days"` // 거래 사이 평균 간격 (2건 이상일 때)
	DaysSinceLast   int        `json:"days_since_last"`   // 마지막 거래 후 지난 날 (-1이면 거래 없음)
	RecentHalf      int        `json:"recent_half"`       // 최근 3개월 거래 수
	EarlierHalf     int        `json:"earlier_half"`      // 그 전 3개월 거래 수
	Level           Liveliness `json:"level"`
}

// BuyerVerdict는 사람이 집을 고를 때 실제로 느끼는 감정을 따라 쓴 판단 문구다.
type BuyerVerdict struct {
	Level      Liveliness `json:"level"`
	Message    string     `json:"message"`
	Cautions   []string   `json:"cautions"`
	Disclaimer string     `json:"disclaimer"`
}

// reportingLagDisclaimer는 모든 판단에 붙는 안내다. 실거래는 계약 후 30일 안에 신고하면 되므로
// 최근 한 달 거래는 아직 덜 잡혀 있고, 최근 3개월이 그 전 3개월보다 적어 보일 수 있다.
const reportingLagDisclaimer = "실거래는 계약 후 30일 안에 신고됩니다. 최근 한 달 거래는 아직 덜 집계되어 있어, " +
	"최근 3개월 거래 수가 실제보다 적게 보일 수 있습니다."

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
	v := BuyerVerdict{Level: act.Level, Disclaimer: reportingLagDisclaimer}
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

// molitAptTradeURL은 국토교통부 아파트 매매 실거래가 자료 API다 (data.go.kr 15126469).
const molitAptTradeURL = "https://apis.data.go.kr/1613000/RTMSDataSvcAptTrade/getRTMSDataSvcAptTrade"

// molitAptPageRows는 한 페이지 행 수다. 거래가 많은 시군구는 한 달에 1,000건을 넘으므로
// totalCount까지 페이지를 넘긴다.
const molitAptPageRows = 1000

// MolitAptTradeSource는 국토교통부 아파트 매매 실거래 API로 TradeSource를 구현한다.
// Key는 data.go.kr 일반 인증키(Decoding)이며 운영 env MOLIT_API_KEY와 같은 키다.
type MolitAptTradeSource struct {
	Key     string
	Client  *http.Client
	BaseURL string // 비면 molitAptTradeURL. 테스트에서 바꾼다
}

type molitAptItem struct {
	AptNm      string `xml:"aptNm"`
	DealAmount string `xml:"dealAmount"`
	DealYear   string `xml:"dealYear"`
	DealMonth  string `xml:"dealMonth"`
	DealDay    string `xml:"dealDay"`
	ExcluUseAr string `xml:"excluUseAr"`
	Floor      string `xml:"floor"`
	BuildYear  string `xml:"buildYear"`
	CdealType  string `xml:"cdealType"` // 해제여부. 해제된 거래는 "O"
}

type molitAptResp struct {
	Header struct {
		ResultCode string `xml:"resultCode"`
		ResultMsg  string `xml:"resultMsg"`
	} `xml:"header"`
	Body struct {
		Items      []molitAptItem `xml:"items>item"`
		TotalCount int            `xml:"totalCount"`
	} `xml:"body"`
}

// Trades는 from이 속한 달부터 to가 속한 달까지 월별로 불러 합친다.
func (s *MolitAptTradeSource) Trades(ctx context.Context, lawdCode string, from, to time.Time) ([]AptTrade, error) {
	if s.Key == "" {
		return nil, fmt.Errorf("MOLIT_API_KEY not set")
	}
	var out []AptTrade
	for m := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, from.Location()); !m.After(to); m = m.AddDate(0, 1, 0) {
		month, err := s.month(ctx, lawdCode, m.Format("200601"))
		if err != nil {
			return nil, err
		}
		out = append(out, month...)
	}
	return out, nil
}

func (s *MolitAptTradeSource) month(ctx context.Context, lawdCode, ym string) ([]AptTrade, error) {
	base := s.BaseURL
	if base == "" {
		base = molitAptTradeURL
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	var out []AptTrade
	for page := 1; ; page++ {
		u := fmt.Sprintf("%s?serviceKey=%s&LAWD_CD=%s&DEAL_YMD=%s&numOfRows=%d&pageNo=%d",
			base, url.QueryEscape(s.Key), lawdCode, ym, molitAptPageRows, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			// url.Error는 요청 URL(키 포함)을 담으므로 그대로 올리지 않는다.
			return nil, fmt.Errorf("molit apt trade %s %s: request failed", lawdCode, ym)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		var mr molitAptResp
		// data.go.kr은 키 오류도 200 + 에러 XML로 준다. resultCode로 판정한다.
		if err := xml.Unmarshal(body, &mr); err != nil || mr.Header.ResultCode != "000" {
			return nil, fmt.Errorf("molit apt trade %s %s: http %d resultCode=%q msg=%q",
				lawdCode, ym, resp.StatusCode, mr.Header.ResultCode, mr.Header.ResultMsg)
		}
		for _, it := range mr.Body.Items {
			t, ok := it.trade()
			if ok {
				out = append(out, t)
			}
		}
		if len(mr.Body.Items) < molitAptPageRows || page*molitAptPageRows >= mr.Body.TotalCount {
			return out, nil
		}
	}
}

// trade는 응답 항목을 AptTrade로 바꾼다. 날짜를 읽을 수 없는 항목은 버린다.
func (it molitAptItem) trade() (AptTrade, bool) {
	y, errY := strconv.Atoi(strings.TrimSpace(it.DealYear))
	m, errM := strconv.Atoi(strings.TrimSpace(it.DealMonth))
	d, errD := strconv.Atoi(strings.TrimSpace(it.DealDay))
	if errY != nil || errM != nil || errD != nil {
		return AptTrade{}, false
	}
	price, _ := strconv.Atoi(strings.NewReplacer(",", "", " ", "").Replace(it.DealAmount))
	area, _ := strconv.ParseFloat(strings.TrimSpace(it.ExcluUseAr), 64)
	floor, _ := strconv.Atoi(strings.TrimSpace(it.Floor))
	built, _ := strconv.Atoi(strings.TrimSpace(it.BuildYear))
	return AptTrade{
		ComplexName: strings.TrimSpace(it.AptNm),
		DealDate:    time.Date(y, time.Month(m), d, 0, 0, 0, 0, kstZone),
		PriceManwon: price,
		AreaM2:      area,
		Floor:       floor,
		BuildYear:   built,
		Canceled:    strings.TrimSpace(it.CdealType) == "O",
	}, true
}

// kstZone은 거래일 기준 시간대다. distroless 이미지에 tzdata가 없어도 되도록 고정 오프셋을 쓴다.
var kstZone = time.FixedZone("KST", 9*60*60)
