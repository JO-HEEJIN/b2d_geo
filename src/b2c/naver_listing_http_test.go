package b2c

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestNaverHTTPRetriesTransientStatus(t *testing.T) {
	calls := 0
	var logs []string
	src := &NaverListingSource{
		ComplexNo: "11410", MaxRetries: 1, BaseDelay: time.Millisecond,
		Logf: func(format string, args ...any) { logs = append(logs, format) },
		HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Query().Get("page") != "1" {
				t.Errorf("retry changed page: %s", r.URL)
			}
			if calls == 1 {
				return naverResponse(http.StatusServiceUnavailable, "temporarily unavailable"), nil
			}
			return naverResponse(http.StatusOK, `{"totalCount":1,"articleList":[{"articleNo":"101"}]}`), nil
		})},
	}
	got, err := src.Fetch(context.Background())
	if err != nil || calls != 2 || len(got) != 1 || got[0].ID != "101" {
		t.Fatalf("Fetch = %+v, %v; calls=%d", got, err, calls)
	}
	if len(logs) == 0 {
		t.Error("retry was not logged")
	}
}

func TestNaverHTTPAuthorizationFailureStopsImmediately(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			src := &NaverListingSource{
				ComplexNo: "11410", MaxRetries: 3, BaseDelay: time.Millisecond, EnableMobileFallback: true,
				HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.URL.Host != "new.land.naver.com" {
						t.Errorf("authorization failure triggered fallback to %s", r.URL.Host)
					}
					return naverResponse(status, "authorization required"), nil
				})},
			}
			got, err := src.Fetch(context.Background())
			if err == nil || calls != 1 || len(got) != 0 {
				t.Fatalf("Fetch = %+v, %v; calls=%d", got, err, calls)
			}
		})
	}
}

func TestNaverHTTPRateLimitHonorsRetryAfter(t *testing.T) {
	for _, retryAfter := range []string{"30", time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)} {
		t.Run(retryAfter, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
			defer cancel()
			calls := 0
			src := &NaverListingSource{
				ComplexNo: "11410", MaxRetries: 1, BaseDelay: time.Millisecond, EnableMobileFallback: true,
				HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++
					resp := naverResponse(http.StatusTooManyRequests, "rate limited")
					resp.Header.Set("Retry-After", retryAfter)
					return resp, nil
				})},
			}
			started := time.Now()
			_, err := src.Fetch(ctx)
			if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
				t.Fatalf("Fetch error = %v; calls=%d (Retry-After=%q)", err, calls, retryAfter)
			}
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Errorf("Retry-After wait ignored context: %v", elapsed)
			}
		})
	}
}

func TestNaverHTTPCancellationDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requested := make(chan struct{})
	done := make(chan error, 1)
	calls := 0
	src := &NaverListingSource{
		ComplexNo: "11410", MaxRetries: 3, BaseDelay: 30 * time.Second, EnableMobileFallback: true,
		HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				close(requested)
			}
			return naverResponse(http.StatusServiceUnavailable, "temporarily unavailable"), nil
		})},
	}
	go func() {
		_, err := src.Fetch(ctx)
		done <- err
	}()
	select {
	case <-requested:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("first request was not sent")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("Fetch error = %v; calls=%d", err, calls)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt backoff")
	}
}

func TestNaverHTTPUnexpectedPayloadPreservesSavedState(t *testing.T) {
	cases := map[string]string{
		"null payload":     "null",
		"html":             "<html>please sign in</html>",
		"missing list":     `{"error":"unexpected response"}`,
		"null list":        `{"articleList":null}`,
		"object list":      `{"articleList":{}}`,
		"missing id":       `{"articleList":[{"articleName":"단지"}]}`,
		"empty id":         `{"articleList":[{"articleNo":""}]}`,
		"whitespace id":    `{"articleList":[{"articleNo":"  "}]}`,
		"null id":          `{"articleList":[{"articleNo":null}]}`,
		"nonobject record": `{"articleList":[null]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			statePath := filepath.Join(t.TempDir(), "listings.json")
			seed := NewRemainingListings(&scriptedSource{batches: [][]Listing{{listing("saved", "기존단지", 90000)}}})
			if _, err := seed.RefreshOnce(context.Background(), statePath); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}
			src := &NaverListingSource{ComplexNo: "11410", HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return naverResponse(http.StatusOK, body), nil
			})}}
			remaining := NewRemainingListings(src)
			change, err := remaining.RefreshOnce(context.Background(), statePath)
			if err == nil {
				t.Fatalf("unexpected payload was accepted: %s", body)
			}
			after, readErr := os.ReadFile(statePath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			// JSON round trips discard time.Time's monotonic component.
			wantMemory, _ := json.Marshal(seed.Remaining(""))
			gotMemory, _ := json.Marshal(remaining.Remaining(""))
			if !bytes.Equal(before, after) || !bytes.Equal(wantMemory, gotMemory) || len(change.Added) != 0 || len(change.Gone) != 0 {
				t.Fatalf("unexpected payload changed saved state: change=%+v, remaining=%+v", change, remaining.Remaining(""))
			}
		})
	}
}

func TestNaverHTTPExplicitEmptyListIsAValidSnapshot(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "listings.json")
	seed := NewRemainingListings(&scriptedSource{batches: [][]Listing{{listing("saved", "기존단지", 90000)}}})
	if _, err := seed.RefreshOnce(context.Background(), statePath); err != nil {
		t.Fatal(err)
	}
	src := &NaverListingSource{ComplexNo: "11410", HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return naverResponse(http.StatusOK, `{"totalCount":0,"articleList":[]}`), nil
	})}}
	remaining := NewRemainingListings(src)
	change, err := remaining.RefreshOnce(context.Background(), statePath)
	if err != nil || len(change.Gone) != 1 || change.Gone[0].ID != "saved" || len(remaining.Remaining("")) != 0 {
		t.Fatalf("empty snapshot: change=%+v, error=%v", change, err)
	}
}

func TestNaverHTTPRejectsDuplicateAndRepeatedPages(t *testing.T) {
	for _, name := range []string{"duplicate within page", "repeated page"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			calls := 0
			src := &NaverListingSource{ComplexNo: "11410", HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if name == "duplicate within page" {
					return naverResponse(http.StatusOK, `{"totalCount":2,"articleList":[{"articleNo":"101"},{"articleNo":"101"}]}`), nil
				}
				return naverResponse(http.StatusOK, `{"totalCount":3,"articleList":[{"articleNo":"101"}]}`), nil
			})}}
			got, err := src.Fetch(ctx)
			if err == nil || errors.Is(err, context.DeadlineExceeded) || len(got) != 0 {
				t.Fatalf("duplicate snapshot returned %+v, %v; calls=%d", got, err, calls)
			}
			if calls > 2 {
				t.Errorf("duplicate page was not rejected promptly: %d requests", calls)
			}
		})
	}
}

func TestNaverHTTPMobileFallbackRestartsSnapshotAtPageOne(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var requests []string
	src := &NaverListingSource{
		ComplexNo: "11410", TradeType: "B1", PageSize: 1,
		MaxRetries: 0, BaseDelay: time.Millisecond, EnableMobileFallback: true,
		HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			page := r.URL.Query().Get("page")
			requests = append(requests, r.URL.Host+":"+page)
			switch r.URL.Host + ":" + page {
			case "new.land.naver.com:1":
				return naverResponse(http.StatusOK, `{"totalCount":2,"articleList":[{"articleNo":"desktop-partial"}]}`), nil
			case "new.land.naver.com:2":
				return naverResponse(http.StatusTooManyRequests, "rate limited"), nil
			case "m.land.naver.com:1", "m.land.naver.com:2":
				if r.URL.Path != "/complex/getComplexArticleList" || r.URL.Query().Get("hscpNo") != "11410" || r.URL.Query().Get("tradTpCd") != "B1" {
					t.Errorf("mobile request = %s", r.URL)
				}
				// This fixture deliberately uses only the observed desktop schema;
				// it makes no claim about the live mobile response contract.
				if page == "1" {
					return naverResponse(http.StatusOK, `{"totalCount":2,"articleList":[{"articleNo":"201"}]}`), nil
				}
				return naverResponse(http.StatusOK, `{"totalCount":2,"articleList":[{"articleNo":"202"}]}`), nil
			default:
				t.Errorf("unexpected request = %s", r.URL)
				return naverResponse(http.StatusBadRequest, "unexpected request"), nil
			}
		})},
	}
	got, err := src.Fetch(ctx)
	if err != nil || len(got) != 2 || got[0].ID != "201" || got[1].ID != "202" {
		t.Fatalf("fallback snapshot = %+v, %v", got, err)
	}
	wantRequests := []string{"new.land.naver.com:1", "new.land.naver.com:2", "m.land.naver.com:1", "m.land.naver.com:2"}
	if !reflect.DeepEqual(requests, wantRequests) {
		t.Errorf("requests = %v, want %v", requests, wantRequests)
	}
}

func TestNaverHTTPZeroRetriesWithoutFallbackStopsAfterRateLimit(t *testing.T) {
	calls := 0
	src := &NaverListingSource{
		ComplexNo: "11410", MaxRetries: 0,
		HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return naverResponse(http.StatusTooManyRequests, "rate limited"), nil
		})},
	}
	_, err := src.Fetch(context.Background())
	if err == nil || calls != 1 {
		t.Fatalf("Fetch error = %v; calls=%d", err, calls)
	}
}
