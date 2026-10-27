package rone

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestRunAgainstPostgres는 실제 PostGIS(마이그레이션 적용된 빈 DB)에서 크로스워크·대조·적재·이어받기를 본다.
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
		`TRUNCATE admin_areas, land_price_index`,
		// 수원시는 구가 둘이라 "경기도 수원시"가 두 코드에 걸린다(크로스워크에서 빠져야 함). 세종은 시군구 단계가 없다.
		`INSERT INTO admin_areas (bcode, name) VALUES
		   ('2611010100','부산광역시 중구 영주동'),
		   ('4111112900','경기도 수원시 장안구 파장동'),
		   ('4111312800','경기도 수원시 권선구 세류동'),
		   ('3611010100','세종특별자치시 반곡동')`,
		// 포털 파일로 넣은 기존 값(202605): 부산 중구 주거 = API와 같음, 세종 주거 = 다름, 장안구 녹지 = API에 없음
		`INSERT INTO land_price_index (sigungu_code, use_zone_class, month, rate_pct, source, source_version) VALUES
		   ('26110','주거지역','202605',0.016,'reb_rone','2026-07-01'),
		   ('36110','주거지역','202605',0.5,'reb_rone','2026-07-01'),
		   ('41111','녹지지역','202605',0.1,'reb_rone','2026-07-01')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		m := r.URL.Query().Get("WRTTIME_IDTFR_ID")
		if m > "202606" {
			fmt.Fprint(w, noDataPage)
			return
		}
		rows := []string{
			fmt.Sprintf(rowTmpl, m, `"부산>중구"`, "주거지역", "0.016"),
			fmt.Sprintf(rowTmpl, m, `"세종"`, "주거지역", "0.123"),
			fmt.Sprintf(rowTmpl, m, `"경기>수원시"`, "주거지역", "5"),
			fmt.Sprintf(rowTmpl, m, `"경기>수원시>장안구"`, "주거지역", "-0.007"),
			fmt.Sprintf(rowTmpl, m, `"경기"`, "주거지역", "1"),
		}
		fmt.Fprintf(w, okPage, len(rows), strings.Join(rows, ","))
	}))
	defer srv.Close()
	newClient := func() *Client { return &Client{Key: "k", Base: srv.URL} }

	// 1) 대조 모드: 쓰지 않는다. 202605에서 API 레코드 3(부산·세종·장안구 주거), DB에도 있는 것 2, 같음 1, 다름 1, DB에만 1.
	st, err := Run(ctx, pool, newClient(), Options{From: "202605", To: "202605", Compare: true})
	cs := st.Cmp
	if err != nil || cs.APIRecords != 3 || cs.InDB != 2 || cs.Same != 1 || cs.Diff != 1 || cs.OnlyDB != 1 {
		t.Fatalf("compare: %+v err=%v", cs, err)
	}
	if st.Aggregate != 2 || st.Unmapped != 0 {
		t.Fatalf("compare stats: %+v", st)
	}
	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM land_price_index WHERE source=$1`, Source).Scan(&n)
	if n != 0 {
		t.Fatalf("compare mode wrote %d rows", n)
	}

	// 2) 적재: 202605~202607. 202607은 공표 전(빈 달).
	o := Options{From: "202605", To: "202607", Refetch: 1}
	calls = 0
	st, err = Run(ctx, pool, newClient(), o)
	if err != nil || st.Months != 2 || st.Records != 6 || calls != 3 || len(st.NoData) != 1 || st.NoData[0] != "202607" {
		t.Fatalf("load: %+v calls=%d err=%v", st, calls, err)
	}
	var rate, src string
	pool.QueryRow(ctx, `SELECT rate_pct::text, source FROM land_price_index WHERE sigungu_code='36110' AND use_zone_class='주거지역' AND month='202605'`).Scan(&rate, &src)
	if rate != "0.123" || src != Source {
		t.Fatalf("세종 202605 = %s (%s), want 0.123 from API", rate, src)
	}
	pool.QueryRow(ctx, `SELECT rate_pct::text FROM land_price_index WHERE sigungu_code='41111' AND use_zone_class='주거지역' AND month='202606'`).Scan(&rate)
	if rate != "-0.007" {
		t.Fatalf("장안구 = %s, want -0.007 (시 집계 5가 들어가면 안 됨)", rate)
	}
	pool.QueryRow(ctx, `SELECT count(*) FROM land_price_index WHERE sigungu_code='41113'`).Scan(&n)
	if n != 0 {
		t.Fatalf("권선구에 시 집계가 들어감: %d", n)
	}

	// 3) 이어받기: 받은 달(202605, 202606) 중 마지막 1개월 + 빈 달만 다시 받는다 = 2호출.
	calls = 0
	st, err = Run(ctx, pool, newClient(), o)
	if err != nil || calls != 2 || st.Months != 1 {
		t.Fatalf("resume: %+v calls=%d err=%v", st, calls, err)
	}
}
