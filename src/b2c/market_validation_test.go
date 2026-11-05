package b2c

import (
	"testing"
	"time"
)

func TestMarketSnapshotBoundary(t *testing.T) {
	source := MarketSource{ID: "supplier", Name: "supplier", Scope: "scope_1", ComplexName: "complex"}
	now := time.Now()
	item := MarketListing{ExternalID: "one", AskingManwon: 80000, Status: "active"}
	for _, name := range []string{"rent", "wrong_scope", "stale", "future", "duplicate", "missing_total", "partial", "null_items", "invalid_price"} {
		t.Run(name, func(t *testing.T) {
			snapshot := marketTestSnapshot("boundary", now, item)
			switch name {
			case "rent":
				snapshot.TradeType = "jeonse"
			case "wrong_scope":
				snapshot.Scope = "other"
			case "stale":
				snapshot.ObservedAt = now.Add(-16 * time.Minute)
			case "future":
				snapshot.ObservedAt = now.Add(2 * time.Minute)
			case "duplicate":
				snapshot.Items = append(snapshot.Items, item)
				n := 2
				snapshot.Total = &n
			case "missing_total":
				snapshot.Total = nil
			case "partial":
				no := false
				snapshot.Complete = &no
			case "null_items":
				snapshot.Items = nil
				n := 0
				snapshot.Total = &n
			case "invalid_price":
				snapshot.Items[0].AskingManwon = 0
			}
			if err := snapshot.Validate(source, now); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
}
