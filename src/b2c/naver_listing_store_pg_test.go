package b2c

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNaverSnapshotAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("B2D_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("B2D_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := NewListingStore(pool)
	const complexNo = "999999901"
	const complexName = "b2c_test_naver_snapshot"
	defer pool.Exec(ctx, `DELETE FROM b2c_listings WHERE complex_name=$1`, complexName)
	defer pool.Exec(ctx, `DELETE FROM b2c_naver_sync WHERE complex_no=$1`, complexNo)
	defer pool.Exec(ctx, `DELETE FROM b2c_naver_changes WHERE complex_no=$1`, complexNo)
	if _, err := store.Register(ctx, Listing{ComplexName: complexName, AskingManwon: 90000}, "b2c_test_real_owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RegisterMany(ctx, []Listing{{ComplexName: complexName, AskingManwon: 1}}, SampleRegistrantPrefix+"b2c_test_hidden"); err != nil {
		t.Fatal(err)
	}
	one := Listing{ID: "1", ComplexName: complexName, TypeName: "127A", AskingManwon: 80000, Source: "naver"}
	two := Listing{ID: "2", ComplexName: complexName, AskingManwon: 70000, Source: "naver"}
	stats, err := store.SyncNaverSnapshot(ctx, complexNo, []Listing{one, two})
	if err != nil || stats.Added != 2 || stats.Gone != 0 {
		t.Fatalf("first snapshot %+v %v", stats, err)
	}
	one.AskingManwon = 80500
	three := Listing{ID: "3", ComplexName: complexName, AskingManwon: 85000, Source: "naver"}
	stats, err = store.SyncNaverSnapshot(ctx, complexNo, []Listing{one, three})
	if err != nil || stats.Added != 1 || stats.Gone != 1 || stats.PriceChanged != 1 {
		t.Fatalf("second snapshot %+v %v", stats, err)
	}
	page, err := store.Page(ctx, complexName, 50, 0)
	if err != nil || page.Total != 3 {
		t.Fatalf("public real listings %+v %v", page, err)
	}
	var externalID string
	for _, listing := range page.Listings {
		if listing.Source == "sample" {
			t.Fatal("sample data leaked into public listings")
		}
		if listing.Source == "naver" {
			externalID = listing.ID
		}
	}
	if externalID == "" {
		t.Fatal("Naver source not exposed")
	}
	if err := store.Close(ctx, externalID, "sold", "naver_sync"); !errors.Is(err, ErrListingNotFound) {
		t.Fatalf("external listing accepted direct close: %v", err)
	}
	direct, err := store.Fetch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, listing := range direct {
		if listing.Source == "naver" || listing.Source == "sample" {
			t.Fatalf("direct tracker mixed sources: %+v", listing)
		}
	}
	sold, err := store.Sold(ctx, complexName, time.Time{})
	if err != nil || len(sold) != 0 {
		t.Fatalf("portal disappearance was reported as a sale: %+v %v", sold, err)
	}
	statuses, err := store.NaverSyncStatuses(ctx, complexName)
	if err != nil || len(statuses) != 1 || statuses[0].ActiveCount != 2 || !statuses[0].Fresh {
		t.Fatalf("sync status %+v %v", statuses, err)
	}
	changes, err := store.NaverRecentChanges(ctx, complexName)
	if err != nil || len(changes) != 5 {
		t.Fatalf("change log %+v %v", changes, err)
	}
	invalid := one
	invalid.AskingManwon = 0
	if _, err := store.SyncNaverSnapshot(ctx, complexNo, []Listing{invalid}); err == nil {
		t.Fatal("invalid snapshot was accepted")
	}
	after, err := store.Page(ctx, complexName, 50, 0)
	if err != nil || after.Total != 3 {
		t.Fatalf("invalid snapshot changed live rows %+v %v", after, err)
	}
	var withdrawn int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM b2c_listings WHERE external_source='naver' AND external_key=$1 AND status='withdrawn'`, complexNo+":"+strconv.Itoa(2)).Scan(&withdrawn); err != nil || withdrawn != 1 {
		t.Fatalf("gone listing status %d %v", withdrawn, err)
	}
}
