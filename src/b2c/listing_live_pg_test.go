package b2c

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestListingLiveAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("B2D_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("B2D_TEST_DATABASE_URL not set; use an isolated database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	listener, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close(ctx)
	if _, err := listener.Exec(ctx, "LISTEN b2c_listings_changed"); err != nil {
		t.Fatal(err)
	}
	quiet := func() {
		t.Helper()
		wait, cancel := context.WithTimeout(ctx, 120*time.Millisecond)
		defer cancel()
		if _, err := listener.WaitForNotification(wait); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("unexpected notification or listener error: %v", err)
		}
	}
	notified := func() {
		t.Helper()
		wait, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		n, err := listener.WaitForNotification(wait)
		if err != nil || n.Channel != "b2c_listings_changed" || n.Payload != "" {
			t.Fatalf("missing real inventory notification: %+v %v", n, err)
		}
	}
	name := "b2c_test_live_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	defer pool.Exec(ctx, `DELETE FROM b2c_listings WHERE complex_name=$1`, name)
	store := NewListingStore(pool)
	if _, err := store.Register(ctx, Listing{ComplexName: name, AskingManwon: 1}, SampleRegistrantPrefix+"b2c_test_live"); err != nil {
		t.Fatal(err)
	}
	quiet()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO b2c_listings(complex_name,asking_manwon,registered_by) VALUES($1,80000,'b2c_test_live_owner') RETURNING id::text`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	quiet() // Uncommitted changes must not be exposed.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	notified()
	if err := store.UpdatePrice(ctx, id, 80500, "someone_else"); !errors.Is(err, ErrListingNotFound) {
		t.Fatalf("another registrant changed the price: %v", err)
	}
	if err := store.UpdatePrice(ctx, id, 0, "b2c_test_live_owner"); !errors.Is(err, ErrInvalidListingPrice) {
		t.Fatalf("accepted invalid price: %v", err)
	}
	quiet()
	if err := store.UpdatePrice(ctx, id, 80500, "b2c_test_live_owner"); err != nil {
		t.Fatal(err)
	}
	notified()
	page, err := store.Page(ctx, name, 50, 0)
	if err != nil || page.Total != 1 || page.Listings[0].AskingManwon != 80500 {
		t.Fatalf("committed price not visible: %+v %v", page, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE b2c_listings SET updated_at=now() WHERE id=$1::bigint`, id); err != nil {
		t.Fatal(err)
	}
	quiet()
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE b2c_listings SET asking_manwon=1 WHERE id=$1::bigint`, id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	quiet()
	if err := store.Close(ctx, id, "withdrawn", "b2c_test_live_owner"); err != nil {
		t.Fatal(err)
	}
	notified()
	sold, err := store.Sold(ctx, name, time.Now().Add(-time.Hour))
	if err != nil || len(sold) != 0 {
		t.Fatalf("withdrawn listing shown as sold: %+v %v", sold, err)
	}
	if err := store.UpdatePrice(ctx, id, 90000, "b2c_test_live_owner"); !errors.Is(err, ErrListingNotFound) {
		t.Fatalf("closed listing changed: %v", err)
	}

	handler := NewHandler(pool, "")
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &http.Client{Timeout: 4 * time.Second}
	response, err := client.Get(server.URL + "/v1/b2c/listings/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("stream did not flush headers")
	}
	scanner := bufio.NewScanner(response.Body)
	waitEvent := func(event string) {
		t.Helper()
		for scanner.Scan() {
			if scanner.Text() == "event: "+event {
				return
			}
		}
		t.Fatalf("stream missing %s event: %v", event, scanner.Err())
	}
	waitEvent("ready")
	id, err = store.Register(ctx, Listing{ComplexName: name, AskingManwon: 90000}, "b2c_test_live_owner")
	if err != nil {
		t.Fatal(err)
	}
	waitEvent("changed")
	if err := store.Close(ctx, id, "sold", "b2c_test_live_owner"); err != nil {
		t.Fatal(err)
	}
	waitEvent("changed")
	sold, err = store.Sold(ctx, name, time.Now().Add(-time.Hour))
	if err != nil || len(sold) != 1 || sold[0].ID != id {
		t.Fatalf("sold listing missing: %+v %v", sold, err)
	}
}
