package b2c

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// NaverListingSource fetches a complete current snapshot. Invalid or partial
// responses return an error so RemainingListings preserves its previous state.
type NaverListingSource struct {
	ComplexNo            string
	TradeType            string
	PageSize             int
	HTTPClient           *http.Client
	UserAgent            string
	Referer              string
	HTTPTimeout          time.Duration // zero uses 40 seconds
	MaxRetries           int           // additional attempts; zero disables retries
	BaseDelay            time.Duration // zero uses two seconds
	EnableMobileFallback bool
	Logf                 func(string, ...any)
}

// NaverFetchError describes the observed failure without guessing its cause.
type NaverFetchError struct {
	StatusCode int
	Cause      string
	Host       string
	Page       int
	Attempt    int
	RetryAfter time.Duration
	Err        error
}

var naverURLCredentials = regexp.MustCompile(`(https?://)[^/\s@]+@`)

func (e *NaverFetchError) Error() string {
	detail := ""
	if e.Err != nil {
		detail = naverURLCredentials.ReplaceAllString(e.Err.Error(), "${1}[redacted]@")
	}
	return fmt.Sprintf("naver fetch page %d attempt %d host=%s: status %d cause=%s: %s", e.Page, e.Attempt, e.Host, e.StatusCode, e.Cause, detail)
}

func (e *NaverFetchError) Unwrap() error { return e.Err }

func (n *NaverListingSource) client() *http.Client {
	if n.HTTPClient != nil {
		return n.HTTPClient
	}
	tr, ok := http.DefaultTransport.(*http.Transport)
	if ok {
		tr = tr.Clone()
	} else {
		tr = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	tr.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = 25 * time.Second
	tr.ExpectContinueTimeout = 2 * time.Second
	tr.MaxIdleConnsPerHost = 4
	timeout := n.HTTPTimeout
	if timeout <= 0 {
		timeout = 40 * time.Second
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

func (n *NaverListingSource) Fetch(ctx context.Context) ([]Listing, error) {
	if strings.TrimSpace(n.ComplexNo) == "" {
		return nil, fmt.Errorf("naver: ComplexNo is required")
	}
	if n.MaxRetries < 0 || n.MaxRetries > 10 {
		return nil, fmt.Errorf("naver: MaxRetries must be between 0 and 10")
	}
	client := n.client()
	if n.HTTPClient == nil {
		defer client.CloseIdleConnections()
	}
	listings, err := n.fetchSnapshot(ctx, client, false)
	var failure *NaverFetchError
	if err == nil || !n.EnableMobileFallback || !errors.As(err, &failure) || failure.StatusCode != http.StatusTooManyRequests {
		return listings, err
	}
	if err := n.waitRetry(ctx, failure, 0); err != nil {
		return nil, err
	}
	if n.Logf != nil {
		n.Logf("naver fallback host=m.land.naver.com restart_page=1")
	}
	// Never mix pages from endpoints with potentially different page sizes.
	return n.fetchSnapshot(ctx, client, true)
}

func (n *NaverListingSource) fetchSnapshot(ctx context.Context, client *http.Client, mobile bool) ([]Listing, error) {
	var all []Listing
	seen := map[string]bool{}
	expected := -1
	for page := 1; page <= 1000; page++ {
		payload, err := n.fetchPage(ctx, client, page, mobile)
		if err != nil {
			return nil, err
		}
		batch := n.parseArticles(payload)
		total := -1
		if raw, ok := payload["totalCount"]; ok {
			value, err := strconv.Atoi(strings.ReplaceAll(strFromAny(raw), ",", ""))
			if err != nil || value < 0 {
				return nil, n.invalid(page, mobile, "invalid totalCount")
			}
			total = value
			if expected >= 0 && expected != total {
				return nil, n.invalid(page, mobile, "totalCount changed during snapshot")
			}
			expected = total
		}
		more, hasMore := payload["isMoreData"].(bool)
		if len(batch) == 0 {
			if more || (total >= 0 && len(all) < total) {
				return nil, n.invalid(page, mobile, "incomplete snapshot")
			}
			return all, nil
		}
		for _, listing := range batch {
			if seen[listing.ID] {
				return nil, n.invalid(page, mobile, "duplicate articleNo or repeated page")
			}
			seen[listing.ID] = true
		}
		all = append(all, batch...)
		if total >= 0 {
			if len(all) > total {
				return nil, n.invalid(page, mobile, "snapshot exceeds totalCount")
			}
			if len(all) == total {
				return all, nil
			}
		}
		if hasMore && !more {
			if total >= 0 && len(all) < total {
				return nil, n.invalid(page, mobile, "incomplete final page")
			}
			return all, nil
		}
		if err := naverWait(ctx, 500*time.Millisecond); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("naver: snapshot exceeds 1000 pages")
}

func (n *NaverListingSource) invalid(page int, mobile bool, detail string) error {
	return &NaverFetchError{StatusCode: http.StatusOK, Cause: "invalid_response", Host: naverHost(mobile), Page: page, Err: errors.New(detail)}
}

func naverHost(mobile bool) string {
	if mobile {
		return "m.land.naver.com"
	}
	return "new.land.naver.com"
}

func (n *NaverListingSource) fetchPage(ctx context.Context, client *http.Client, page int, mobile bool) (map[string]any, error) {
	var last *NaverFetchError
	for attempt := 0; attempt <= n.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			if last == nil {
				return nil, err
			}
			return nil, errors.Join(last, err)
		}
		if attempt > 0 {
			if err := n.waitRetry(ctx, last, attempt-1); err != nil {
				return nil, err
			}
		}
		payload, failure := n.fetchPageOnce(ctx, client, page, attempt+1, mobile)
		if failure == nil {
			return payload, nil
		}
		last = failure
		if ctx.Err() != nil {
			return nil, errors.Join(last, ctx.Err())
		}
		retryable := last.StatusCode == http.StatusTooManyRequests || (last.StatusCode >= 500 && last.StatusCode <= 599) || last.Cause == "timeout" || last.Cause == "transport"
		if !retryable {
			return nil, last
		}
		if !mobile && n.EnableMobileFallback && last.StatusCode == http.StatusTooManyRequests {
			return nil, last
		}
	}
	return nil, last
}

func (n *NaverListingSource) waitRetry(ctx context.Context, previous *NaverFetchError, attempt int) error {
	base := n.BaseDelay
	if base <= 0 {
		base = 2 * time.Second
	}
	delay := base * time.Duration(1<<min(attempt, 5))
	if delay > time.Minute {
		delay = time.Minute
	}
	delay += time.Duration(rand.Int64N(int64(base)))
	if previous != nil && previous.RetryAfter > delay {
		delay = previous.RetryAfter
	}
	if n.Logf != nil {
		n.Logf("naver retry wait=%s", delay.Round(time.Millisecond))
	}
	if err := naverWait(ctx, delay); err != nil {
		return errors.Join(previous, err)
	}
	return nil
}

func naverWait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func naverRetryAfter(raw string) time.Duration {
	if seconds, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && seconds > 0 {
		const maxSeconds = int64(^uint64(0)>>1) / int64(time.Second)
		if seconds > maxSeconds {
			return time.Duration(maxSeconds) * time.Second
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(raw); err == nil {
		if wait := time.Until(at); wait > 0 {
			return wait
		}
	}
	return 0
}

func (n *NaverListingSource) fetchPageOnce(ctx context.Context, client *http.Client, page, attempt int, mobile bool) (map[string]any, *NaverFetchError) {
	trade := n.TradeType
	if trade == "" {
		trade = "A1"
	}
	size := n.PageSize
	if size <= 0 {
		size = 100
	}
	host := naverHost(mobile)
	path := "/api/articles/complex/" + url.PathEscape(n.ComplexNo)
	q := url.Values{"realEstateType": {"APT"}, "tradeType": {trade}, "page": {strconv.Itoa(page)}, "pageSize": {strconv.Itoa(size)}, "order": {"articleConfirmYmd"}}
	if mobile {
		path = "/complex/getComplexArticleList"
		q = url.Values{"hscpNo": {n.ComplexNo}, "tradTpCd": {trade}, "page": {strconv.Itoa(page)}}
	}
	started := time.Now()
	if n.Logf != nil {
		event := func(phase string) {
			n.Logf("naver request host=%s page=%d attempt=%d phase=%s elapsed=%s", host, page, attempt, phase, time.Since(started).Round(time.Millisecond))
		}
		trace := &httptrace.ClientTrace{
			DNSDone:              func(httptrace.DNSDoneInfo) { event("dns_done") },
			ConnectDone:          func(_, _ string, _ error) { event("tcp_done") },
			TLSHandshakeDone:     func(tls.ConnectionState, error) { event("tls_done") },
			GotFirstResponseByte: func() { event("first_byte") },
		}
		ctx = httptrace.WithClientTrace(ctx, trace)
	}
	failure := func(status int, cause string, err error) *NaverFetchError {
		return &NaverFetchError{StatusCode: status, Cause: cause, Host: host, Page: page, Attempt: attempt, Err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+path+"?"+q.Encode(), nil)
	if err != nil {
		return nil, failure(0, "transport", err)
	}
	ua := n.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
	}
	referer := n.Referer
	if referer == "" {
		referer = "https://new.land.naver.com/"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "ko-KR,ko;q=0.9,en;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		cause := "transport"
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			cause = "timeout"
		}
		return nil, failure(0, cause, err)
	}
	defer resp.Body.Close()
	if n.Logf != nil {
		n.Logf("naver response host=%s page=%d status=%d elapsed=%s", host, page, resp.StatusCode, time.Since(started).Round(time.Millisecond))
	}
	if resp.StatusCode != http.StatusOK {
		cause := "http_status"
		switch {
		case resp.StatusCode == 429:
			cause = "rate_limited"
		case resp.StatusCode == 401:
			cause = "unauthorized"
		case resp.StatusCode == 403:
			cause = "forbidden"
		case resp.StatusCode >= 500:
			cause = "upstream"
		}
		e := failure(resp.StatusCode, cause, fmt.Errorf("HTTP %s", resp.Status))
		e.RetryAfter = naverRetryAfter(resp.Header.Get("Retry-After"))
		return nil, e
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return nil, failure(resp.StatusCode, "transport", err)
	}
	if len(raw) > 8<<20 {
		return nil, failure(resp.StatusCode, "invalid_response", errors.New("response exceeds 8 MiB"))
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, failure(resp.StatusCode, "invalid_response", fmt.Errorf("naver decode page %d: %w", page, err))
	}
	articles, ok := payload["articleList"].([]any)
	if !ok || payload["success"] == false || payload["error"] != nil {
		return nil, failure(resp.StatusCode, "invalid_response", errors.New("missing or invalid articleList; snapshot rejected"))
	}
	for _, item := range articles {
		article, ok := item.(map[string]any)
		if !ok || strings.TrimSpace(strFromAny(article["articleNo"])) == "" {
			return nil, failure(resp.StatusCode, "invalid_response", errors.New("invalid articleNo; snapshot rejected"))
		}
	}
	return payload, nil
}

func (n *NaverListingSource) parseArticles(payload map[string]any) []Listing {
	raw, _ := payload["articleList"].([]any)
	out := make([]Listing, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, Listing{
			ID:           strFromAny(m["articleNo"]),
			ComplexName:  firstNonEmpty(strFromAny(m["complexName"]), strFromAny(m["articleName"])),
			TypeName:     strFromAny(m["areaName"]),
			AskingManwon: parseManwon(firstNonEmpty(strFromAny(m["dealOrWarrantPrc"]), strFromAny(m["rentPrc"]))),
			Floor:        parseFloor(strFromAny(m["floorInfo"])),
			Lon:          floatFromAny(m["longitude"]), Lat: floatFromAny(m["latitude"]),
			Source: "naver", FirstSeen: time.Now(), LastSeen: time.Now(),
		})
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

// parseManwon converts Korean 억/만원 amounts without joining unit digits.
func parseManwon(s string) int {
	if len(s) > 64 {
		return 0
	}
	s = strings.NewReplacer(",", "", " ", "", "\u00a0", "").Replace(strings.TrimSpace(s))
	s = strings.TrimSuffix(strings.TrimSuffix(s, "원"), "만")
	major, minor, hasEok := strings.Cut(s, "억")
	if !hasEok {
		value, err := strconv.Atoi(s)
		if err != nil || value < 0 {
			return 0
		}
		return value
	}
	amount, ok := new(big.Rat).SetString(major)
	if !ok || amount.Sign() < 0 {
		return 0
	}
	amount.Mul(amount, big.NewRat(10000, 1))
	if minor != "" {
		remainder, err := strconv.Atoi(minor)
		if err != nil || remainder < 0 || remainder >= 10000 {
			return 0
		}
		amount.Add(amount, big.NewRat(int64(remainder), 1))
	}
	if !amount.IsInt() {
		return 0
	}
	value, err := strconv.Atoi(amount.Num().String())
	if err != nil {
		return 0
	}
	return value
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
