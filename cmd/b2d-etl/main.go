// b2d-etl은 공공데이터 다운로드/적재/인덱싱을 수행하는 CLI다.
// 서브커맨드: migrate, download, load, index
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JO-HEEJIN/b2d_geo/internal/etl/entrance"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/jijeok"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/juso"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/landsales"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/sbiz"
	"github.com/JO-HEEJIN/b2d_geo/internal/store"
	"github.com/JO-HEEJIN/b2d_geo/migrations"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: b2d-etl <migrate|juso|sbiz|jijeok|download|load|index>")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "migrate":
		if err := runMigrate(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, "migrate failed:", err)
			os.Exit(1)
		}
		fmt.Println("migrate: ok")
	case "sbiz":
		if err := runSbiz(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "sbiz failed:", err)
			os.Exit(1)
		}
	case "entrance":
		if err := runEntrance(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "entrance failed:", err)
			os.Exit(1)
		}
	case "jijeok":
		if err := runJijeok(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "jijeok failed:", err)
			os.Exit(1)
		}
	case "land-sales":
		if err := runLandSales(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "land-sales failed:", err)
			os.Exit(1)
		}
	case "juso":
		if err := runJuso(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "juso failed:", err)
			os.Exit(1)
		}
	case "download", "load", "index":
		fmt.Fprintf(os.Stderr, "%s: not implemented (Phase 1)\n", os.Args[1])
		os.Exit(1)
	default:
		fmt.Fprintln(os.Stderr, "unknown subcommand:", os.Args[1])
		os.Exit(2)
	}
}

func dbDSN() string {
	if dsn := os.Getenv("B2D_DATABASE_URL"); dsn != "" {
		return dsn
	}
	return "postgres://b2d:b2d_local_dev@localhost:5433/b2d_geo"
}

func runMigrate(ctx context.Context) error {
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	return store.Migrate(ctx, pool, migrations.FS)
}

// sbizSourceVersion은 현재 적재분의 기준 분기다. 데이터셋 갱신 시 함께 갱신한다.
// 근거: docs/dataspec/sbiz.md (기준분기 20260331).
const sbizSourceVersion = "2026-03-31"

// jusoSourceVersion은 현재 적재분의 전체분 기준월이다. 데이터셋 갱신 시 함께 갱신한다.
// 근거: docs/dataspec/juso.md (202605 전체분).
const jusoSourceVersion = "2026-05-01"

func runJuso(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: b2d-etl juso <download [destDir] | load <dir-or-zip-or-txt>>")
	}
	switch args[0] {
	case "download":
		destDir := "."
		if len(args) >= 2 {
			destDir = args[1]
		}
		path, err := juso.Download(ctx, destDir)
		if err != nil {
			return err
		}
		fmt.Println("downloaded:", path)
		return nil
	case "load":
		if len(args) < 2 {
			return fmt.Errorf("usage: b2d-etl juso load <dir-or-zip-or-txt>")
		}
		pool, err := pgxpool.New(ctx, dbDSN())
		if err != nil {
			return err
		}
		defer pool.Close()
		stats, err := juso.Load(ctx, pool, args[1], jusoSourceVersion)
		if err != nil {
			return err
		}
		fmt.Printf("juso load: files=%d staged=%d skipped=%d inserted=%d jibun_rows=%d source_version=%s\n",
			stats.Files, stats.Staged, stats.Skipped, stats.Inserted, stats.JibunRows, jusoSourceVersion)
		return nil
	default:
		return fmt.Errorf("unknown juso action: %s", args[0])
	}
}

func runSbiz(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: b2d-etl sbiz <download [destDir] | load <dir-or-zip-or-csv>>")
	}
	switch args[0] {
	case "download":
		destDir := "."
		if len(args) >= 2 {
			destDir = args[1]
		}
		path, err := sbiz.Download(ctx, destDir)
		if err != nil {
			return err
		}
		fmt.Println("downloaded:", path)
		return nil
	case "load":
		if len(args) < 2 {
			return fmt.Errorf("usage: b2d-etl sbiz load <dir-or-zip-or-csv>")
		}
		pool, err := pgxpool.New(ctx, dbDSN())
		if err != nil {
			return err
		}
		defer pool.Close()
		stats, err := sbiz.Load(ctx, pool, args[1], sbizSourceVersion)
		if err != nil {
			return err
		}
		fmt.Printf("sbiz load: files=%d staged=%d skipped=%d inserted=%d source_version=%s\n",
			stats.Files, stats.Staged, stats.Skipped, stats.Inserted, sbizSourceVersion)
		return nil
	default:
		return fmt.Errorf("unknown sbiz action: %s", args[0])
	}
}

// runJijeok: b2d-etl jijeok <zip...> — 연속지적도 시도 zip 적재.
// source_version은 파일명 AL_D002_{시도}_{YYYYMMDD}.zip의 날짜에서 취한다.
func runJijeok(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: b2d-etl jijeok <zip...>")
	}
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	for _, zp := range args {
		base := zp[len(zp)-12 : len(zp)-4] // YYYYMMDD
		ver := base[:4] + "-" + base[4:6] + "-" + base[6:8]
		stats, err := jijeok.Load(ctx, pool, zp, ver)
		if err != nil {
			return fmt.Errorf("%s: %w", zp, err)
		}
		fmt.Printf("jijeok %s: rows=%d skipped=%d\n", zp, stats.Rows, stats.Skipped)
	}
	return nil
}

// runLandSales: b2d-etl land-sales <lawd5> <YYYYMM> — 토지 실거래 월 적재 (멱등).
func runLandSales(ctx context.Context, args []string) error {
	if len(args) != 2 || len(args[0]) != 5 || len(args[1]) != 6 {
		return fmt.Errorf("usage: b2d-etl land-sales <lawd_cd(5)> <YYYYMM>")
	}
	key := os.Getenv("MOLIT_API_KEY")
	if key == "" {
		return fmt.Errorf("MOLIT_API_KEY not set (.env 참조)")
	}
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	stats, err := landsales.Load(ctx, pool, key, "data/juso_entrance", args[0], args[1])
	if err != nil {
		return err
	}
	fmt.Printf("land-sales %s %s: fetched=%d inserted=%d no_emd=%d ambiguous=%d no_geom=%d\n",
		args[0], args[1], stats.Fetched, stats.Inserted, stats.NoEmd, stats.Ambiguous, stats.NoGeom)
	return nil
}

// runEntrance: b2d-etl entrance <zip...> — 출입구 전체분 적재 (buildings).
func runEntrance(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: b2d-etl entrance <zip...>")
	}
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	for _, zp := range args {
		stats, err := entrance.Load(ctx, pool, zp, "2026-06-08")
		if err != nil {
			return fmt.Errorf("%s: %w", zp, err)
		}
		fmt.Printf("entrance %s: rows=%d dup=%d skipped=%d\n", zp, stats.Rows, stats.Dup, stats.Skipped)
	}
	return nil
}
