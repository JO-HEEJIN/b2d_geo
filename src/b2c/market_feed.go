package b2c

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MarketSource is an actual supplier connection, not a generated inventory.
// Scope identifies one complex in that supplier's system.
type MarketSource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Scope       string `json:"scope"`
	ComplexName string `json:"complex_name"`
}

type MarketListing struct {
	ExternalID   string `json:"external_id"`
	TypeName     string `json:"type_name"`
	AskingManwon int    `json:"asking_manwon"`
	Floor        int    `json:"floor"`
	Status       string `json:"status"` // active, sold, withdrawn; never inferred sold
}

// MarketSnapshot is the supplier integration contract. It is not a claim that
// any specific vendor implements this format; vendor adapters must map into it.
type MarketSnapshot struct {
	ID         string          `json:"snapshot_id"`
	Scope      string          `json:"scope"`
	TradeType  string          `json:"trade_type"` // module 5 currently collects sale listings
	ObservedAt time.Time       `json:"observed_at"`
	Complete   *bool           `json:"complete"`
	Total      *int            `json:"total"`
	Items      []MarketListing `json:"items"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

var marketSourceID = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)
var marketScopeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

func (s MarketSource) Validate() error {
	if !marketSourceID.MatchString(s.ID) || !marketScopeID.MatchString(s.Scope) || strings.TrimSpace(s.Name) == "" || strings.TrimSpace(s.ComplexName) == "" || len(s.Name) > 200 || len(s.ComplexName) > 300 {
		return fmt.Errorf("market supplier id, name, scope and complex_name required")
	}
	return nil
}

func (s MarketSnapshot) Validate(source MarketSource, now time.Time) error {
	if err := source.Validate(); err != nil {
		return err
	}
	if s.ID == "" || len(s.ID) > 128 || s.Scope != source.Scope || s.TradeType != "sale" || s.Complete == nil || !*s.Complete || s.Total == nil || *s.Total != len(s.Items) || len(s.Items) > 10000 || s.Items == nil || s.NextCursor != "" {
		return fmt.Errorf("incomplete or invalid market snapshot; inventory preserved")
	}
	if s.ObservedAt.IsZero() || s.ObservedAt.After(now.Add(time.Minute)) || s.ObservedAt.Before(now.Add(-15*time.Minute)) {
		return fmt.Errorf("market snapshot timestamp outside accepted freshness window")
	}
	seen := map[string]bool{}
	for _, item := range s.Items {
		if strings.TrimSpace(item.ExternalID) == "" || len(item.ExternalID) > 200 || seen[item.ExternalID] || item.AskingManwon <= 0 || item.AskingManwon > 2147483647 || len(item.TypeName) > 100 || (item.Status != "active" && item.Status != "sold" && item.Status != "withdrawn") {
			return fmt.Errorf("invalid or duplicate market listing; inventory preserved")
		}
		seen[item.ExternalID] = true
	}
	return nil
}

// HTTPMarketFeed reads the explicit supplier contract above. An opaque cursor is
// added to the configured origin; remote next URLs and cross-origin redirects
// are never followed with the supplier credential.
type HTTPMarketFeed struct {
	Source MarketSource
	URL    string
	Token  string
	Client *http.Client
}

type MarketHTTPError struct {
	Status     int
	RetryAfter time.Duration
}

func (e *MarketHTTPError) Error() string {
	return fmt.Sprintf("market supplier returned HTTP %d; inventory preserved", e.Status)
}

func marketRetryAfter(raw string) time.Duration {
	if seconds, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && seconds > 0 {
		const limit = int64(^uint64(0)>>1) / int64(time.Second)
		if seconds > limit {
			seconds = limit
		}
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(raw); err == nil && date.After(time.Now()) {
		return time.Until(date)
	}
	return 0
}

func (f *HTTPMarketFeed) Fetch(ctx context.Context) (MarketSnapshot, error) {
	var result MarketSnapshot
	if err := f.Source.Validate(); err != nil {
		return result, err
	}
	base, err := url.Parse(f.URL)
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Scheme != "https" && !(base.Scheme == "http" && f.Client != nil)) {
		return result, fmt.Errorf("market feed requires a configured HTTPS endpoint without embedded credentials")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	if f.Client != nil {
		copy := *f.Client
		client = &copy
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	cursor := ""
	cursors := map[string]bool{}
	for page := 0; page < 100; page++ {
		endpoint := *base
		q := endpoint.Query()
		q.Set("scope", f.Source.Scope)
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		endpoint.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return MarketSnapshot{}, fmt.Errorf("invalid market request")
		}
		req.Header.Set("Accept", "application/json")
		if f.Token != "" {
			req.Header.Set("Authorization", "Bearer "+f.Token)
		}
		response, err := client.Do(req)
		if err != nil {
			return MarketSnapshot{}, fmt.Errorf("market supplier request failed; inventory preserved")
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return MarketSnapshot{}, &MarketHTTPError{Status: response.StatusCode, RetryAfter: marketRetryAfter(response.Header.Get("Retry-After"))}
		}
		if readErr != nil || len(raw) > 2<<20 {
			return MarketSnapshot{}, fmt.Errorf("market supplier response exceeds limit or is incomplete")
		}
		var batch MarketSnapshot
		if err := json.Unmarshal(raw, &batch); err != nil || batch.Items == nil || batch.Total == nil || batch.Complete == nil || !*batch.Complete {
			return MarketSnapshot{}, fmt.Errorf("invalid market supplier JSON; inventory preserved")
		}
		if page == 0 {
			result = batch
			result.Items = []MarketListing{}
		} else if batch.ID != result.ID || batch.Scope != result.Scope || batch.TradeType != result.TradeType || !batch.ObservedAt.Equal(result.ObservedAt) || *batch.Total != *result.Total {
			return MarketSnapshot{}, fmt.Errorf("market snapshot changed during pagination")
		}
		result.Items = append(result.Items, batch.Items...)
		if len(result.Items) > 10000 {
			return MarketSnapshot{}, fmt.Errorf("market snapshot exceeds listing limit")
		}
		cursor = batch.NextCursor
		if cursor == "" {
			result.NextCursor = ""
			if err := result.Validate(f.Source, time.Now()); err != nil {
				return MarketSnapshot{}, err
			}
			return result, nil
		}
		if len(cursor) > 2048 || cursors[cursor] || len(batch.Items) == 0 {
			return MarketSnapshot{}, fmt.Errorf("repeated or invalid market cursor")
		}
		cursors[cursor] = true
	}
	return MarketSnapshot{}, fmt.Errorf("market pagination exceeds limit")
}
