package b2c

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func marketTestSnapshot(id string, at time.Time, items ...MarketListing) MarketSnapshot {
	complete, total := true, len(items)
	if items == nil {
		items = []MarketListing{}
	}
	return MarketSnapshot{ID: id, Scope: "scope_1", TradeType: "sale", ObservedAt: at, Complete: &complete, Total: &total, Items: items}
}

func TestMarketFeedCompletePaginationAndFailures(t *testing.T) {
	source := MarketSource{ID: "supplier", Name: "Test supplier", Scope: "scope_1", ComplexName: "test complex"}
	stamp := time.Now().UTC().Truncate(time.Microsecond)
	mode := "valid"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer test-only-secret" || r.URL.Query().Get("scope") != "scope_1" {
			t.Error("supplier header or scope missing")
		}
		switch mode {
		case "null":
			w.Write([]byte("null"))
			return
		case "error200":
			w.Write([]byte(`{"error":"unavailable"}`))
			return
		case "rate":
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(429)
			return
		case "redirect":
			w.Header().Set("Location", serverURLForTest(r))
			w.WriteHeader(302)
			return
		}
		item := MarketListing{ExternalID: "one", AskingManwon: 80000, Status: "active"}
		batch := marketTestSnapshot("snapshot", stamp, item)
		total := 2
		batch.Total = &total
		if r.URL.Query().Get("cursor") == "" {
			batch.NextCursor = "next"
		} else {
			batch.Items[0].ExternalID = "two"
		}
		if mode == "mixed" && r.URL.Query().Get("cursor") != "" {
			batch.ID = "different"
		}
		if mode == "duplicate" {
			batch.Items[0].ExternalID = "one"
		}
		if mode == "partial" {
			no := false
			batch.Complete = &no
		}
		json.NewEncoder(w).Encode(batch)
	}))
	defer server.Close()
	feed := HTTPMarketFeed{Source: source, URL: server.URL, Token: "test-only-secret", Client: server.Client()}
	got, err := feed.Fetch(context.Background())
	if err != nil || len(got.Items) != 2 || requests != 2 {
		t.Fatalf("full snapshot %+v %v requests=%d", got, err, requests)
	}
	for _, testMode := range []string{"null", "error200", "rate", "redirect", "mixed", "duplicate", "partial"} {
		t.Run(testMode, func(t *testing.T) {
			mode = testMode
			requests = 0
			got, err := feed.Fetch(context.Background())
			if err == nil || got.Items != nil {
				t.Fatalf("invalid response accepted %+v %v", got, err)
			}
			if strings.Contains(err.Error(), feed.Token) {
				t.Fatal("credential leaked")
			}
			if testMode == "rate" {
				var status *MarketHTTPError
				if !errors.As(err, &status) || status.RetryAfter != time.Hour || requests != 1 {
					t.Fatalf("rate limit not retained: %v", err)
				}
			}
			if testMode == "redirect" && requests != 1 {
				t.Fatal("redirect followed")
			}
		})
	}
	valid := marketTestSnapshot("empty", stamp)
	if err := valid.Validate(source, time.Now()); err != nil {
		t.Fatal("explicit complete empty snapshot rejected:", err)
	}
	valid.Items = nil
	if err := valid.Validate(source, time.Now()); err == nil {
		t.Fatal("null items accepted as empty inventory")
	}
}

func serverURLForTest(r *http.Request) string { return "http://" + r.Host + "/redirected" }
