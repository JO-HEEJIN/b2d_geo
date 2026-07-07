// b2d-server는 위치 서비스 API 서버다. 현재는 부동산 특화 최소 구성
// (/v1/health, /v1/parcel 3종, /v1/appraisal 2종). 인증/rate limit/계측과
// 나머지 엔드포인트는 Phase 4에서 추가한다.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JO-HEEJIN/b2d_geo/internal/api"
)

func main() {
	dsn := os.Getenv("B2D_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://b2d:b2d_local_dev@localhost:5433/b2d_geo"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db connect failed:", err)
		os.Exit(1)
	}
	defer pool.Close()

	addr := os.Getenv("B2D_LISTEN")
	if addr == "" {
		addr = ":8080"
	}
	h := api.New(pool, os.Getenv("MOLIT_API_KEY"), os.Getenv("REB_API_KEY"))
	fmt.Println("b2d-server listening on", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
