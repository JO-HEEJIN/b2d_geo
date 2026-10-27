package ned

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestRunAgainstPostgres는 실제 PostGIS(마이그레이션 적용된 빈 DB)에서 적재·이어받기·대조·호출 한도를 본다.
// B2D_TEST_DATABASE_URL이 없으면 건너뛴다(운영 DB에 절대 쓰지 말 것: 아래에서 테이블을 비운다).
func TestRunAgainstPostgres(t *testing.T) {
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
	for _, q := range []string{
		`TRUNCATE parcels, official_land_prices, land_features, ned_sync_progress`,
		// 세종(36) 두 동과 다른 시도(11) 한 동
		`INSERT INTO parcels (pnu, bcode, jibun, source, source_version) VALUES
		   ('3611010100100010000','3611010100','1','t','2026-01-01'),
		   ('3611010100100020000','3611010100','2','t','2026-01-01'),
		   ('3611010200100010000','3611010200','1','t','2026-01-01'),
		   ('1111010100100010000','1111010100','1','t','2026-01-01')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		b := r.URL.Query().Get("pnu")
		if strings.HasSuffix(r.URL.Path, "getLandCharacteristics") {
			fmt.Fprintf(w, `{"landCharacteristicss":{"field":[{"pnu":"%s100010000","stdrYear":"2026","lastUpdtDt":"2026-05-12","ladUseSittnNm":"주거용","tpgrphHgCodeNm":"평지","tpgrphFrmCodeNm":"정방형","roadSideCodeNm":"세로한면(가)"}],"totalCount":"1","resultCode":""}}`, b)
			return
		}
		fmt.Fprintf(w, `{"indvdLandPrices":{"field":[{"pnu":"%s100010000","stdrYear":"2026","lastUpdtDt":"2026-05-21","pblntfPclnd":"500000"},{"pnu":"%s100020000","stdrYear":"2026","lastUpdtDt":"2026-05-21","pblntfPclnd":"600000"}],"totalCount":"2","resultCode":""}}`, b, b)
	}))
	defer srv.Close()
	newClient := func() *Client { return &Client{Key: "k", Domain: "d", Base: srv.URL + "/"} }
	var log bytes.Buffer
	o := Options{Datasets: []Dataset{Price, Features}, Year: 2026, Sido: []string{"36"}, Log: &log}

	// 1) 호출 한도: 1회 호출 후 멈추고, 그 동만 기록된다.
	lim := o
	lim.MaxCalls = 1
	st, err := Run(ctx, pool, newClient(), lim)
	if err != nil || st.Stopped == "" || st.Dongs != 1 {
		t.Fatalf("max-calls run: %+v err=%v", st, err)
	}
	// 2) 이어받기: 남은 것만 받는다(가격 1동 + 특성 2동 = 3호출).
	calls = 0
	st, err = Run(ctx, pool, newClient(), o)
	if err != nil || st.Stopped != "" || calls != 3 {
		t.Fatalf("resume run: %+v calls=%d err=%v log=%s", st, calls, err, log.String())
	}
	var nPrice, nFeat, nProg int
	pool.QueryRow(ctx, `SELECT count(*) FROM official_land_prices WHERE base_year=2026 AND source='vworld_ned'`).Scan(&nPrice)
	pool.QueryRow(ctx, `SELECT count(*) FROM land_features WHERE base_year=2026 AND road_side='세로한면(가)'`).Scan(&nFeat)
	pool.QueryRow(ctx, `SELECT count(*) FROM ned_sync_progress`).Scan(&nProg)
	if nPrice != 4 || nFeat != 2 || nProg != 4 {
		t.Fatalf("rows price=%d feat=%d progress=%d, want 4/2/4", nPrice, nFeat, nProg)
	}
	var p int64
	pool.QueryRow(ctx, `SELECT price_per_sqm FROM official_land_prices WHERE pnu='3611010200100020000' AND base_year=2026`).Scan(&p)
	if p != 600000 {
		t.Fatalf("price %d", p)
	}
	// 3) 다시 돌리면 할 일이 없다.
	calls = 0
	if st, err = Run(ctx, pool, newClient(), o); err != nil || calls != 0 || st.Dongs != 0 {
		t.Fatalf("idempotent run: %+v calls=%d err=%v", st, calls, err)
	}
	// 4) 대조 모드: 쓰지 않고 같은 값이면 same으로 센다. 한 필지 값을 바꿔 diff 1을 만든다.
	pool.Exec(ctx, `UPDATE official_land_prices SET price_per_sqm=1 WHERE pnu='3611010100100010000' AND base_year=2026`)
	cmp := o
	cmp.Compare = true
	cmp.Datasets = []Dataset{Price}
	st, err = Run(ctx, pool, newClient(), cmp)
	if err != nil || st.Cmp.InDB != 4 || st.Cmp.Same != 3 || st.Cmp.Diff != 1 || len(st.Cmp.Examples) != 1 {
		t.Fatalf("compare: %+v err=%v", st.Cmp, err)
	}
	pool.QueryRow(ctx, `SELECT price_per_sqm FROM official_land_prices WHERE pnu='3611010100100010000' AND base_year=2026`).Scan(&p)
	if p != 1 {
		t.Fatalf("compare mode must not write, price now %d", p)
	}
}
