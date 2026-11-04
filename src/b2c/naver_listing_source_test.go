package b2c

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

type naverRoundTripFunc func(*http.Request) (*http.Response, error)

func (f naverRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func naverResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestNaverListingSourceFetchPagesAndMapsListings(t *testing.T) {
	calls := 0
	before := time.Now()
	src := &NaverListingSource{
		ComplexNo: "11410", TradeType: "B1", PageSize: 1,
		UserAgent: "listing-test", Referer: "https://new.land.naver.com/complexes/11410",
		HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != http.MethodGet || r.URL.Scheme != "https" || r.URL.Host != "new.land.naver.com" || r.URL.Path != "/api/articles/complex/11410" {
				t.Errorf("request = %s %s", r.Method, r.URL)
			}
			for name, want := range map[string]string{
				"realEstateType": "APT", "tradeType": "B1", "pageSize": "1",
				"page": strconv.Itoa(calls), "order": "articleConfirmYmd",
			} {
				if got := r.URL.Query().Get(name); got != want {
					t.Errorf("query %s = %q, want %q", name, got, want)
				}
			}
			for name, want := range map[string]string{
				"User-Agent": "listing-test", "Referer": "https://new.land.naver.com/complexes/11410",
				"Accept": "application/json, text/plain, */*", "Accept-Language": "ko-KR,ko;q=0.9,en;q=0.8",
			} {
				if got := r.Header.Get(name); got != want {
					t.Errorf("header %s = %q, want %q", name, got, want)
				}
			}
			switch calls {
			case 1:
				return naverResponse(http.StatusOK, `{"totalCount":"2","articleList":[{"articleNo":"101","articleName":"동탄역시범","areaName":"127A","tradeTypeName":"전세","dealOrWarrantPrc":"85,000","floorInfo":"12/25","longitude":"127.0961","latitude":37.2003}]}`), nil
			case 2:
				return naverResponse(http.StatusOK, `{"totalCount":2,"articleList":[{"articleNo":102,"complexName":"잠실아파트","rentPrc":"3,000","floorInfo":"중/고","longitude":127.09,"latitude":"37.51"}]}`), nil
			default:
				t.Fatalf("unexpected page %d after totalCount was reached", calls)
				return nil, errors.New("unexpected page")
			}
		})},
	}
	got, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(got) != 2 {
		t.Fatalf("calls=%d listings=%+v", calls, got)
	}
	want := []Listing{
		{ID: "101", ComplexName: "동탄역시범", TypeName: "127A", AskingManwon: 85000, Floor: 12, Lon: 127.0961, Lat: 37.2003, Source: "naver"},
		{ID: "102", ComplexName: "잠실아파트", AskingManwon: 3000, Lon: 127.09, Lat: 37.51, Source: "naver"},
	}
	for i, l := range got {
		if l.FirstSeen.Before(before) || l.LastSeen.Before(before) || l.FirstSeen.After(time.Now()) || l.LastSeen.After(time.Now()) {
			t.Errorf("listing %d timestamps = %v, %v", i, l.FirstSeen, l.LastSeen)
		}
		l.FirstSeen, l.LastSeen = time.Time{}, time.Time{}
		if !reflect.DeepEqual(l, want[i]) {
			t.Errorf("listing %d = %+v, want %+v", i, l, want[i])
		}
	}
}

func TestNaverListingSourceDefaultsAndEmptyPage(t *testing.T) {
	calls := 0
	src := &NaverListingSource{ComplexNo: "11410", HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if q := r.URL.Query(); q.Get("tradeType") != "A1" || q.Get("pageSize") != "100" || q.Get("page") != strconv.Itoa(calls) {
			t.Errorf("default query = %v", q)
		}
		if r.UserAgent() == "" || r.Referer() != "https://new.land.naver.com/" {
			t.Errorf("default headers = %v", r.Header)
		}
		if calls == 1 {
			return naverResponse(http.StatusOK, `{"articleList":[{"articleNo":"101"}]}`), nil
		}
		return naverResponse(http.StatusOK, `{"articleList":[]}`), nil
	})}}
	got, err := src.Fetch(context.Background())
	if err != nil || calls != 2 || len(got) != 1 || got[0].ID != "101" {
		t.Fatalf("Fetch = %+v, %v; calls=%d", got, err, calls)
	}
}

func TestNaverListingSourceRequiresComplexNo(t *testing.T) {
	src := &NaverListingSource{HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("request sent without ComplexNo")
		return nil, errors.New("unexpected request")
	})}}
	if _, err := src.Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "ComplexNo is required") {
		t.Fatalf("Fetch error = %v", err)
	}
}

func TestNaverListingSourceErrorsPreserveRemainingState(t *testing.T) {
	transportErr := errors.New("upstream unavailable")
	cases := []struct {
		name string
		fail naverRoundTripFunc
		want string
	}{
		{"http", func(*http.Request) (*http.Response, error) {
			return naverResponse(http.StatusTooManyRequests, "rate limited"), nil
		}, "status 429"},
		{"json", func(*http.Request) (*http.Response, error) {
			return naverResponse(http.StatusOK, "<html>unavailable</html>"), nil
		}, "naver decode page 2"},
		{"transport", func(*http.Request) (*http.Response, error) { return nil, transportErr }, "upstream unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			remaining := NewRemainingListings(&scriptedSource{batches: [][]Listing{{listing("saved", "기존단지", 90000)}}})
			if _, err := remaining.Refresh(context.Background()); err != nil {
				t.Fatal(err)
			}
			before := remaining.Remaining("")
			calls := 0
			remaining.source = &NaverListingSource{ComplexNo: "11410", HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return naverResponse(http.StatusOK, `{"totalCount":2,"articleList":[{"articleNo":"new"}]}`), nil
				}
				return tc.fail(r)
			})}}
			change, err := remaining.Refresh(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Refresh error = %v, want %q", err, tc.want)
			}
			if tc.name == "transport" && !errors.Is(err, transportErr) {
				t.Errorf("transport error was not preserved: %v", err)
			}
			if len(change.Added) != 0 || len(change.Gone) != 0 || !reflect.DeepEqual(before, remaining.Remaining("")) || len(remaining.RecentlyGone(time.Time{})) != 0 {
				t.Fatalf("failed page changed state: change=%+v remaining=%+v", change, remaining.Remaining(""))
			}
		})
	}
}

func TestNaverListingSourceCancellationBetweenPages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	src := &NaverListingSource{ComplexNo: "11410", HTTPClient: &http.Client{Transport: naverRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Context() != ctx {
			t.Error("request did not carry Fetch context")
		}
		cancel()
		return naverResponse(http.StatusOK, `{"totalCount":2,"articleList":[{"articleNo":"101"}]}`), nil
	})}}
	_, err := src.Fetch(ctx)
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("Fetch error = %v, calls=%d", err, calls)
	}
}
