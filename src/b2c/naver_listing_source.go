package b2c

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// NaverListingSource는 네이버 부동산 단지 매물 API를 호출해 Listing으로 변환한다.
// 기존 ListingSource 인터페이스를 구현하므로 RemainingListings에 그대로 주입할 수 있다.
type NaverListingSource struct {
	ComplexNo  string       // 네이버 단지 번호 (예: "11410")
	TradeType  string       // "A1" 매매, "B1" 전세, "B2" 월세
	PageSize   int          // 페이지당 매물 수 (기본 100)
	HTTPClient *http.Client // nil이면 기본 타임아웃 15초 클라이언트 사용
	UserAgent  string       // 비워두면 기본 브라우저 UA
	Referer    string       // 비워두면 https://new.land.naver.com/
}

// Fetch는 네이버 부동산 API를 페이지네이션하며 모든 매물을 가져온다.
func (n *NaverListingSource) Fetch(ctx context.Context) ([]Listing, error) {
	if n.ComplexNo == "" {
		return nil, fmt.Errorf("naver: ComplexNo is required")
	}
	trade := n.TradeType
	if trade == "" {
		trade = "A1"
	}
	pageSize := n.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}
	client := n.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	ua := n.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
	}
	referer := n.Referer
	if referer == "" {
		referer = "https://new.land.naver.com/"
	}

	var all []Listing
	for page := 1; ; page++ {
		payload, err := n.fetchPage(ctx, client, trade, page, pageSize, ua, referer)
		if err != nil {
			return nil, err
		}
		batch := n.parseArticles(payload)
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		total := intFromAny(payload["totalCount"])
		if total > 0 && len(all) >= total {
			break
		}
		// 과도한 연속 호출을 피하기 위한 최소한의 간격
		select {
		case <-ctx.Done():
			return all, ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
	return all, nil
}

func (n *NaverListingSource) fetchPage(
	ctx context.Context,
	client *http.Client,
	trade string,
	page, pageSize int,
	ua, referer string,
) (map[string]any, error) {
	endpoint := fmt.Sprintf("https://new.land.naver.com/api/articles/complex/%s", url.PathEscape(n.ComplexNo))
	q := url.Values{}
	q.Set("realEstateType", "APT")
	q.Set("tradeType", trade)
	q.Set("page", strconv.Itoa(page))
	q.Set("pageSize", strconv.Itoa(pageSize))
	q.Set("order", "articleConfirmYmd")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "ko-KR,ko;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("naver fetch page %d: %w", page, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("naver fetch page %d: status %d", page, resp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("naver decode page %d: %w", page, err)
	}
	return payload, nil
}

func (n *NaverListingSource) parseArticles(payload map[string]any) []Listing {
	raw, _ := payload["articleList"].([]any)
	if raw == nil {
		return nil
	}
	out := make([]Listing, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		listing := Listing{
			ID:          strFromAny(m["articleNo"]),
			ComplexName: firstNonEmpty(strFromAny(m["articleName"]), strFromAny(m["complexName"])),
			TypeName:    firstNonEmpty(strFromAny(m["tradeTypeName"]), strFromAny(m["articleName"])),
			AskingManwon: parseManwon(firstNonEmpty(
				strFromAny(m["dealOrWarrantPrc"]),
				strFromAny(m["rentPrc"]),
			)),
			Floor:     parseFloor(strFromAny(m["floorInfo"])),
			Lon:       floatFromAny(m["longitude"]),
			Lat:       floatFromAny(m["latitude"]),
			Source:    "naver",
			FirstSeen: time.Now(),
			LastSeen:  time.Now(),
		}
		out = append(out, listing)
	}
	return out
}

func strFromAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	default:
		return ""
	}
}

func intFromAny(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case string:
		n, _ := strconv.Atoi(strings.ReplaceAll(x, ",", ""))
		return n
	default:
		return 0
	}
}

func floatFromAny(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(strings.ReplaceAll(x, ",", ""), 64)
		return f
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// "8억 5,000" 같은 문자열에서 만원 단위 숫자만 뽑는다.
func parseManwon(s string) int {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return 0
	}
	n, _ := strconv.Atoi(b.String())
	return n
}

// "12/25" 또는 "중/고" 같은 층 정보에서 숫자만 취한다.
func parseFloor(s string) int {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if b.Len() > 0 {
			break
		}
	}
	if b.Len() == 0 {
		return 0
	}
	n, _ := strconv.Atoi(b.String())
	return n
}
