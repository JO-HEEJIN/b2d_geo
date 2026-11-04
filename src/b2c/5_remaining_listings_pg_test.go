package b2c

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestListingStoreAgainstPostgres는 등록 -> 갱신 -> 팔림 -> 갱신 흐름을 실제 DB로 본다.
// B2D_TEST_DATABASE_URL이 없으면 건너뛴다(운영 DB에 쓰지 말 것: 테스트 행을 넣고 지운다).
func TestListingStoreAgainstPostgres(t *testing.T) {
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
	cleanup := func() { pool.Exec(ctx, `DELETE FROM b2c_listings WHERE registered_by LIKE 'b2c_test%'`) }
	cleanup()
	defer cleanup()

	store := NewListingStore(pool)
	id1, err := store.Register(ctx, Listing{ComplexName: "동탄역시범", TypeName: "84A", AskingManwon: 99000, Floor: 12}, "b2c_test_agent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Register(ctx, Listing{ComplexName: "동탄역시범", AskingManwon: 97000}, "b2c_test_owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Register(ctx, Listing{ComplexName: "", AskingManwon: 1}, "b2c_test_agent"); err == nil {
		t.Error("expected validation error")
	}

	state := filepath.Join(t.TempDir(), "listings.json")
	rem := NewRemainingListings(store)
	ch, err := rem.RefreshOnce(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Added) < 2 {
		t.Fatalf("added = %+v", ch.Added)
	}

	if err := store.Close(ctx, id1, "sold", "b2c_test_owner"); !errors.Is(err, ErrListingNotFound) {
		t.Errorf("other registrant closed listing: %v", err)
	}
	if err := store.Close(ctx, id1, "sold", "b2c_test_agent"); err != nil {
		t.Fatal(err)
	}
	ch, err = NewRemainingListings(store).RefreshOnce(ctx, state) // 새 프로세스처럼 상태 파일에서 이어받는다
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Added) != 0 || len(ch.Gone) != 1 || ch.Gone[0].ID != id1 {
		t.Fatalf("second refresh = %+v", ch)
	}
}
