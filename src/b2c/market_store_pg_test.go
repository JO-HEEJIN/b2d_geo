package b2c

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMarketSnapshotsAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("B2D_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated B2D_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := NewListingStore(pool)
	name := "b2c_test_market_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	defer func() {
		pool.Exec(ctx, `DELETE FROM b2c_market_changes WHERE complex_name=$1`, name)
		pool.Exec(ctx, `DELETE FROM b2c_market_snapshots WHERE source_id IN ('test_market_a','test_market_b')`)
		pool.Exec(ctx, `DELETE FROM b2c_market_sources WHERE source_id IN ('test_market_a','test_market_b')`)
		pool.Exec(ctx, `DELETE FROM b2c_listings WHERE complex_name=$1`, name)
	}()
	source := MarketSource{ID: "test_market_a", Name: "market supplier A", Scope: "scope_1", ComplexName: name}
	coverage, err := store.MarketCoverage(ctx, name)
	if err != nil || coverage.Status != "not_connected" || coverage.ListingCount != nil {
		t.Fatalf("unknown inventory shown as zero: %+v %v", coverage, err)
	}
	if _, err = store.Register(ctx, Listing{ComplexName: name, AskingManwon: 99000}, "b2c_test_market_owner"); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UTC().Add(-time.Second).Truncate(time.Microsecond)
	one := MarketListing{ExternalID: "one", TypeName: "84A", AskingManwon: 80000, Status: "active"}
	two := MarketListing{ExternalID: "two", AskingManwon: 70000, Status: "active"}
	first := marketTestSnapshot("first", stamp, one, two)
	stats, err := store.SyncMarketSnapshot(ctx, source, first)
	if err != nil || stats.Added != 2 || stats.Active != 2 {
		t.Fatalf("first snapshot %+v %v", stats, err)
	}
	one.AskingManwon = 80500
	three := MarketListing{ExternalID: "three", AskingManwon: 90000, Status: "active"}
	second := marketTestSnapshot("second", stamp.Add(time.Millisecond), one, three)
	stats, err = store.SyncMarketSnapshot(ctx, source, second)
	if err != nil || stats.PriceChanged != 1 || stats.Missing != 1 || stats.Added != 1 || stats.Sold != 0 {
		t.Fatalf("second snapshot %+v %v", stats, err)
	}
	sold, err := store.Sold(ctx, name, stamp)
	if err != nil || len(sold) != 0 {
		t.Fatalf("missing listing falsely sold %+v %v", sold, err)
	}
	stats, err = store.SyncMarketSnapshot(ctx, source, second)
	if err != nil || !stats.Replay {
		t.Fatalf("idempotent snapshot %+v %v", stats, err)
	}
	bad := second
	bad.Items = append([]MarketListing{}, second.Items...)
	bad.Items[0].AskingManwon = 1
	if _, err = store.SyncMarketSnapshot(ctx, source, bad); err == nil {
		t.Fatal("snapshot id reused with modified content")
	}
	bad = marketTestSnapshot("partial", stamp.Add(2*time.Millisecond), one)
	no := false
	bad.Complete = &no
	if _, err = store.SyncMarketSnapshot(ctx, source, bad); err == nil {
		t.Fatal("partial snapshot accepted")
	}
	bad = marketTestSnapshot("old", stamp.Add(-time.Second), one)
	if _, err = store.SyncMarketSnapshot(ctx, source, bad); err == nil {
		t.Fatal("out-of-order snapshot accepted")
	}
	page, err := store.Page(ctx, name, 50, 0)
	if err != nil || page.Total != 3 {
		t.Fatalf("invalid input changed inventory %+v %v", page, err)
	}
	one.Status = "sold"
	three.Status = "withdrawn"
	third := marketTestSnapshot("third", stamp.Add(3*time.Millisecond), one, three)
	stats, err = store.SyncMarketSnapshot(ctx, source, third)
	if err != nil || stats.Sold != 1 || stats.Withdrawn != 1 {
		t.Fatalf("explicit closing statuses %+v %v", stats, err)
	}
	sold, err = store.Sold(ctx, name, stamp)
	if err != nil || len(sold) != 1 || sold[0].AskingManwon != 80500 {
		t.Fatalf("confirmed sale missing %+v %v", sold, err)
	}
	three.Status = "active"
	stats, err = store.SyncMarketSnapshot(ctx, source, marketTestSnapshot("fourth", stamp.Add(4*time.Millisecond), one, three))
	if err != nil || stats.Relisted != 1 {
		t.Fatalf("relisting %+v %v", stats, err)
	}
	other := source
	other.ID = "test_market_b"
	other.Name = "market supplier B"
	if _, err = store.SyncMarketSnapshot(ctx, other, marketTestSnapshot("independent", stamp, three)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SyncMarketSnapshot(ctx, source, marketTestSnapshot("empty", stamp.Add(5*time.Millisecond))); err != nil {
		t.Fatal(err)
	}
	page, err = store.Page(ctx, name, 50, 0)
	if err != nil || page.Total != 2 {
		t.Fatalf("source isolation lost %+v %v", page, err)
	}
	coverage, err = store.MarketCoverage(ctx, name)
	if err != nil || coverage.Status != "ready" || coverage.ListingCount == nil || *coverage.ListingCount != 1 {
		t.Fatalf("coverage %+v %v", coverage, err)
	}
	if len(coverage.Changes) != 10 {
		t.Fatalf("unexpected change count %d: %+v", len(coverage.Changes), coverage.Changes)
	}
	if err = store.RecordMarketFailure(ctx, other, "fetch_failed", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err = store.MarketSourceReady(ctx, other); err == nil {
		t.Fatal("persisted Retry-After ignored")
	}
	coverage, err = store.MarketCoverage(ctx, name)
	if err != nil || coverage.Status != "stale" || coverage.ListingCount == nil || *coverage.ListingCount != 1 {
		t.Fatalf("failure cleared known inventory %+v %v", coverage, err)
	}
	unknown := other
	unknown.Scope = "not_connected"
	if err = store.RecordMarketFailure(ctx, unknown, "fetch_failed", 0); err != nil {
		t.Fatal(err)
	}
	coverage, err = store.MarketCoverage(ctx, name)
	if err != nil || coverage.Status != "partial" || coverage.ListingCount != nil {
		t.Fatalf("partial coverage shown as certain count %+v %v", coverage, err)
	}
}
