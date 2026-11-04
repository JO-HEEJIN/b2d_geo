// b2d-etl은 공공데이터 다운로드/적재/인덱싱을 수행하는 CLI다.
// 서브커맨드: migrate, download, load, index
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JO-HEEJIN/b2d_geo/internal/auth"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/entrance"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/jijeok"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/juso"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/landfeature"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/landsales"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/landuse"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/ned"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/officialprice"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/priceindex"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/rone"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/sbiz"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/standardlots"
	"github.com/JO-HEEJIN/b2d_geo/internal/etl/transit"
	"github.com/JO-HEEJIN/b2d_geo/internal/store"
	"github.com/JO-HEEJIN/b2d_geo/migrations"
	"github.com/JO-HEEJIN/b2d_geo/src/b2c"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: b2d-etl <migrate|juso|sbiz|jijeok|land-sales|price-index|standard-lots|land-use|ned-sync|rone-sync|transit-bus|transit-subway|b2c-floor-plans|b2c-listings|apikey|download|load|index>")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "apikey":
		if err := runApikey(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "apikey failed:", err)
			os.Exit(1)
		}
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
	case "places-choseong":
		pool, err := pgxpool.New(context.Background(), dbDSN())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		n, err := sbiz.BackfillChoseong(context.Background(), pool)
		pool.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "places-choseong failed:", err)
			os.Exit(1)
		}
		fmt.Printf("places-choseong: %d rows filled\n", n)
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
	case "price-index":
		if err := runPriceIndex(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "price-index failed:", err)
			os.Exit(1)
		}
	case "standard-lots":
		if err := runStandardLots(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "standard-lots failed:", err)
			os.Exit(1)
		}
	case "land-use":
		if err := runLandUse(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "land-use failed:", err)
			os.Exit(1)
		}
	case "individual-price":
		if err := runIndividualPrice(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "individual-price failed:", err)
			os.Exit(1)
		}
	case "land-features":
		if err := runLandFeatures(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "land-features failed:", err)
			os.Exit(1)
		}
	case "ned-sync":
		if err := runNedSync(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "ned-sync failed:", err)
			os.Exit(1)
		}
	case "rone-sync":
		if err := runRoneSync(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "rone-sync failed:", err)
			os.Exit(1)
		}
	case "transit-bus":
		if err := runTransitBus(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "transit-bus failed:", err)
			os.Exit(1)
		}
	case "transit-subway":
		if err := runTransitSubway(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "transit-subway failed:", err)
			os.Exit(1)
		}
	case "b2c-floor-plans":
		if err := runB2CFloorPlans(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "b2c-floor-plans failed:", err)
			os.Exit(1)
		}
	case "b2c-listings":
		if err := runB2CListings(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "b2c-listings failed:", err)
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

// runTransitBus는 국토교통부 전국 버스정류장 위치정보 CSV를 transit_stops에 적재한다.
func runTransitBus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("transit-bus", flag.ContinueOnError)
	version := fs.String("version", "", "자료 기준일 YYYY-MM-DD (파일명의 날짜, 예: 2026-10-20)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *version == "" {
		return fmt.Errorf("usage: b2d-etl transit-bus --version YYYY-MM-DD <csv>")
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer f.Close()
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	inserted, skipped, err := transit.LoadBus(ctx, pool, f, *version)
	if err != nil {
		return err
	}
	fmt.Printf("transit-bus load: inserted=%d skipped=%d source_version=%s\n", inserted, skipped, *version)
	return nil
}

// runTransitSubway는 전국도시철도역사정보표준데이터 XLSX를 transit_stops에 적재한다.
func runTransitSubway(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("transit-subway", flag.ContinueOnError)
	version := fs.String("version", "", "자료 기준일 YYYY-MM-DD (파일명의 날짜, 예: 2026-06-30)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *version == "" {
		return fmt.Errorf("usage: b2d-etl transit-subway --version YYYY-MM-DD <xlsx>")
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	inserted, skipped, err := transit.LoadSubway(ctx, pool, f, st.Size(), *version)
	if err != nil {
		return err
	}
	fmt.Printf("transit-subway load: inserted=%d skipped=%d source_version=%s\n", inserted, skipped, *version)
	return nil
}

// runB2CFloorPlans는 건축물대장 전유부로 단지(필지)의 평면 타입 후보를 만들어 저장한다.
// 키는 env MOLIT_API_KEY(같은 data.go.kr 계정 키)를 쓴다. 이미 있는 타입은 덮어쓰지 않는다.
func runB2CFloorPlans(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("b2c-floor-plans", flag.ContinueOnError)
	sigungu := fs.String("sigungu", "", "시군구코드 5자리 (예: 41597)")
	bjdong := fs.String("bjdong", "", "법정동코드 뒤 5자리 (예: 11500)")
	bun := fs.String("bun", "", "번 4자리 (예: 0964)")
	ji := fs.String("ji", "0000", "지 4자리")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(*sigungu) != 5 || len(*bjdong) != 5 || len(*bun) != 4 || len(*ji) != 4 {
		return fmt.Errorf("usage: b2d-etl b2c-floor-plans --sigungu 41597 --bjdong 11500 --bun 0964 [--ji 0000]")
	}
	src := &b2c.BuildingRegisterSource{Key: os.Getenv("MOLIT_API_KEY")}
	units, err := src.Units(ctx, *sigungu, *bjdong, *bun, *ji)
	if err != nil {
		return err
	}
	id := b2c.ParcelComplexID(*sigungu, *bjdong, *bun, *ji)
	candidates := b2c.NewFloorPlanCatalog().CandidatesFromUnits(id, units)
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	inserted, err := b2c.NewFloorPlanStore(pool).SaveCandidates(ctx, candidates)
	if err != nil {
		return err
	}
	name := ""
	if len(units) > 0 {
		name = units[0].BuildingName
	}
	fmt.Printf("b2c-floor-plans: complex_id=%s name=%q units=%d candidates=%d inserted=%d\n", id, name, len(units), len(candidates), inserted)
	for _, c := range candidates {
		fmt.Printf("  %s  %d세대\n", c.TypeName, len(c.Units))
	}
	return nil
}

// b2cListingsExecStart는 서버 systemd 서비스가 부르는 명령이다. compose의 b2c-listings
// 서비스(profile oneshot)를 한 번 돌리고 끝난다.
const b2cListingsExecStart = "/usr/bin/docker compose --project-directory /home/ubuntu/b2d_geo/deploy run --rm --no-deps b2c-listings"

// runB2CListings는 B2C 남은 매물을 한 번 갱신한다(systemd 타이머가 매분 부른다).
// --print-units면 갱신 대신 서버에 둘 systemd 서비스·타이머 파일 내용을 출력한다.
func runB2CListings(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("b2c-listings", flag.ContinueOnError)
	state := fs.String("state", "/state/listings.json", "실행 사이 상태를 잇는 JSON 파일")
	printUnits := fs.Bool("print-units", false, "systemd 서비스·타이머 파일 내용만 출력")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *printUnits {
		units := b2c.NewRemainingListings(nil).SystemdUnits(b2cListingsExecStart)
		for _, name := range []string{"b2c-listings.service", "b2c-listings.timer"} {
			fmt.Printf("# /etc/systemd/system/%s\n%s\n", name, units[name])
		}
		return nil
	}
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	change, err := b2c.NewRemainingListings(b2c.NewListingStore(pool)).RefreshOnce(ctx, *state)
	if err != nil {
		return err
	}
	fmt.Printf("b2c-listings: added=%d gone=%d\n", len(change.Added), len(change.Gone))
	return nil
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

// runPriceIndex: b2d-etl price-index <json파일> — 용도지역별 지가변동률 적재 (멱등).
func runPriceIndex(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: b2d-etl price-index <용도지역별 지가변동률.json>")
	}
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	stats, err := priceindex.Load(ctx, pool, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("price-index: rows=%d records=%d skipped=%d empty=%d\n",
		stats.DataRows, stats.Records, stats.Skipped, stats.EmptyCells)
	return nil
}

// runStandardLots: b2d-etl standard-lots <표준지공시지가 번들.zip> — 표준지공시지가 적재 (멱등).
func runStandardLots(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: b2d-etl standard-lots <표준지공시지가.zip>")
	}
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	stats, err := standardlots.Load(ctx, pool, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("standard-lots: files=%d rows=%d skipped=%d\n", stats.Files, stats.Rows, stats.Skipped)
	return nil
}

// runLandUse: b2d-etl land-use <토지이용계획공간정보 번들.zip> — 토지이용계획 적재 (멱등).
func runLandUse(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: b2d-etl land-use <토지이용계획공간정보.zip>")
	}
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	stats, err := landuse.Load(ctx, pool, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("land-use: files=%d rows=%d final=%d skipped=%d\n",
		stats.Files, stats.Rows, stats.Final, stats.Skipped)
	return nil
}

// runIndividualPrice: b2d-etl individual-price <AL_D151 개별공시지가.zip> — 개별공시지가 적재(멱등 upsert).
func runIndividualPrice(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: b2d-etl individual-price <AL_D151_개별공시지가.zip>")
	}
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	stats, err := officialprice.Load(ctx, pool, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("individual-price: rows=%d final=%d skipped=%d\n", stats.Rows, stats.Final, stats.Skipped)
	return nil
}

// runLandFeatures: b2d-etl land-features <토지특성정보.zip> — 토지특성 최신 날짜분 적재(멱등 upsert).
func runLandFeatures(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: b2d-etl land-features <토지특성정보.zip>")
	}
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	stats, err := landfeature.Load(ctx, pool, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("land-features: rows=%d final=%d skipped=%d\n", stats.Rows, stats.Final, stats.Skipped)
	return nil
}

// runNedSync: b2d-etl ned-sync [--dataset price,features] [--year 2026] [--sido 26,27] [--max-calls N]
// [--max-dongs N] [--compare] [--daemon --at 03:17 [--run-now]] — 브이월드 국가중점데이터 API로
// 개별공시지가·토지특성을 법정동 단위로 받아 적재한다(이어받기). --compare는 쓰지 않고 이미 있는 DB 값과
// 대조만 한다(검증용). --daemon은 매일 --at(KST)에 한 번씩 돌며 기다린다. 일정이 서버 crontab이 아니라
// compose 서비스로 코드와 함께 움직이게 하려는 것이다(서버 이전 시 다시 등록할 필요 없음).
// 키는 env B2D_VWORLD_KEY, 등록 서비스 URL은 env B2D_VWORLD_DOMAIN(기본 https://api.birth2death.com).
func runNedSync(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ned-sync", flag.ContinueOnError)
	datasets := fs.String("dataset", "price,features", "price,features 중 쉼표로")
	year := fs.Int("year", 0, "기준연도. 0이면 실행할 때마다 6월부터 올해, 그 전에는 작년(개별공시지가는 5월 말 공시)")
	sido := fs.String("sido", "", "시도 2자리 코드, 쉼표로. 비면 전국")
	maxCalls := fs.Int("max-calls", 0, "한 번 실행에서 이만큼 호출하면 멈춤(0=제한 없음)")
	maxDongs := fs.Int("max-dongs", 0, "법정동 수 제한(0=제한 없음)")
	pause := fs.Duration("pause", 200*time.Millisecond, "호출 사이 간격")
	cmp := fs.Bool("compare", false, "적재하지 않고 DB와 대조만")
	daemon := fs.Bool("daemon", false, "매일 --at(KST)에 실행하며 계속 대기")
	at := fs.String("at", "03:17", "--daemon 실행 시각(KST, HH:MM)")
	runNow := fs.Bool("run-now", false, "--daemon 시작 직후 한 번 먼저 실행")
	if err := fs.Parse(args); err != nil {
		return err
	}
	key := os.Getenv("B2D_VWORLD_KEY")
	if key == "" {
		return fmt.Errorf("B2D_VWORLD_KEY is empty")
	}
	domain := os.Getenv("B2D_VWORLD_DOMAIN")
	if domain == "" {
		domain = "https://api.birth2death.com"
	}
	o := ned.Options{MaxCalls: *maxCalls, MaxDongs: *maxDongs, Compare: *cmp, Log: os.Stdout}
	for _, d := range strings.Split(*datasets, ",") {
		if d = strings.TrimSpace(d); d != "" {
			o.Datasets = append(o.Datasets, ned.Dataset(d))
		}
	}
	for _, sd := range strings.Split(*sido, ",") {
		if sd = strings.TrimSpace(sd); sd != "" {
			if len(sd) != 2 {
				return fmt.Errorf("sido must be 2 digits: %q", sd)
			}
			o.Sido = append(o.Sido, sd)
		}
	}
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()

	// 한 번 실행. 연도는 실행 시점마다 다시 정하므로 데몬이 6월을 넘기면 자동으로 새 연도를 받는다.
	once := func(ctx context.Context) error {
		o := o
		o.Year = *year
		if o.Year == 0 {
			o.Year = ned.AutoYear(time.Now())
		}
		c := &ned.Client{Key: key, Domain: domain, Pause: *pause}
		st, err := ned.Run(ctx, pool, c, o)
		fmt.Printf("ned-sync: year=%d dongs=%d rows=%d calls=%d", o.Year, st.Dongs, st.Rows, st.Calls)
		if st.Stopped != "" {
			fmt.Printf(" stopped=%q", st.Stopped)
		}
		fmt.Println()
		if o.Compare {
			cs := st.Cmp
			fmt.Printf("compare: api_rows=%d in_db=%d same=%d diff=%d\n", cs.APIRows, cs.InDB, cs.Same, cs.Diff)
			for _, e := range cs.Examples {
				fmt.Println("  ", e)
			}
		}
		return err
	}
	if !*daemon {
		return once(ctx)
	}
	return daily(ctx, "ned-sync", *at, *runNow, once)
}

// daily는 매일 at(KST, HH:MM)에 once를 한 번씩 돌리며 기다린다(compose 데몬용). 실패해도 죽지 않고 다음 날
// 다시 시도한다. 컨테이너 중지(SIGTERM) 시 진행 중인 단위만 버리고 끝낸다. 끝낸 단위는 DB에 남아 다음에 이어진다.
func daily(ctx context.Context, name, at string, runNow bool, once func(context.Context) error) error {
	hh, mm, err := ned.ParseClock(at)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()
	if runNow {
		if err := once(ctx); err != nil && ctx.Err() == nil {
			fmt.Printf("%s: run failed: %v\n", name, err)
		}
	}
	for {
		next := ned.NextRun(time.Now(), hh, mm)
		fmt.Printf("%s: next run %s\n", name, next.Format("2006-01-02 15:04 MST"))
		t := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			t.Stop()
			fmt.Printf("%s: stopped\n", name)
			return nil
		case <-t.C:
		}
		if err := once(ctx); err != nil && ctx.Err() == nil {
			fmt.Printf("%s: run failed: %v\n", name, err)
		}
	}
}

// runRoneSync: b2d-etl rone-sync [--from 200501] [--to YYYYMM] [--refetch 3] [--compare]
// [--daemon --at 03:17 [--run-now]] — 한국부동산원 R-ONE Open API에서 (월) 용도지역별 지가변동률을 받아
// land_price_index에 적재한다. 처음엔 --from부터 백필하고, 이후엔 아직 없는 달과 마지막 --refetch개월만 받는다.
// --compare는 쓰지 않고 범위 안의 모든 달을 DB와 대조만 한다(검증용). 키는 env RONE_API_KEY.
func runRoneSync(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("rone-sync", flag.ContinueOnError)
	from := fs.String("from", "200501", "시작 월 YYYYMM")
	to := fs.String("to", "", "끝 월 YYYYMM. 비면 실행 시점의 이번 달(KST)")
	refetch := fs.Int("refetch", 3, "이미 받은 달 중 마지막 N개월은 다시 받음")
	pause := fs.Duration("pause", 300*time.Millisecond, "호출 사이 간격")
	cmp := fs.Bool("compare", false, "적재하지 않고 DB와 대조만")
	daemon := fs.Bool("daemon", false, "매일 --at(KST)에 실행하며 계속 대기")
	at := fs.String("at", "03:17", "--daemon 실행 시각(KST, HH:MM)")
	runNow := fs.Bool("run-now", false, "--daemon 시작 직후 한 번 먼저 실행")
	if err := fs.Parse(args); err != nil {
		return err
	}
	key := os.Getenv("RONE_API_KEY")
	if key == "" {
		return fmt.Errorf("RONE_API_KEY is empty")
	}
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()

	once := func(ctx context.Context) error {
		o := rone.Options{From: *from, To: *to, Refetch: *refetch, Compare: *cmp, Log: os.Stdout}
		if o.To == "" {
			o.To = time.Now().In(ned.KST).Format("200601") // 데몬은 실행할 때마다 새 달을 범위에 넣는다
		}
		c := &rone.Client{Key: key, Pause: *pause}
		st, err := rone.Run(ctx, pool, c, o)
		fmt.Printf("rone-sync: months=%d rows=%d records=%d aggregate=%d unmapped=%d superseded=%d dup=%d calls=%d no_data=%v\n",
			st.Months, st.Rows, st.Records, st.Aggregate, st.Unmapped, st.Superseded, st.Dup, st.Calls, st.NoData)
		if len(st.Unknown) > 0 {
			fmt.Printf("rone-sync: unmapped regions: %s\n", strings.Join(st.Unknown, ", "))
		}
		if o.Compare {
			cs := st.Cmp
			fmt.Printf("compare: api_records=%d in_db=%d same=%d diff=%d only_db=%d\n", cs.APIRecords, cs.InDB, cs.Same, cs.Diff, cs.OnlyDB)
			for _, e := range cs.Examples {
				fmt.Println("  ", e)
			}
		}
		return err
	}
	if !*daemon {
		return once(ctx)
	}
	return daily(ctx, "rone-sync", *at, *runNow, once)
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

// runApikey: b2d-etl apikey create --name <이름> [--scopes geo,realestate] [--rate <rps>]
//
//	[--tier demo|standard] [--monthly-cap <n>]
//
// 평문 키를 1회만 출력하고 DB에는 SHA-256 해시만 저장한다.
func runApikey(ctx context.Context, args []string) error {
	if len(args) < 1 || args[0] != "create" {
		return fmt.Errorf("usage: b2d-etl apikey create --name <name> [--scopes geo,realestate] [--rate <rps>] [--tier demo|standard] [--monthly-cap <n>]")
	}
	fs := flag.NewFlagSet("apikey create", flag.ContinueOnError)
	name := fs.String("name", "", "owner name (required)")
	scopesCSV := fs.String("scopes", "geo", "comma-separated scopes: geo,realestate,b2c_write")
	rate := fs.Int("rate", 10, "rate limit requests per second (>0)")
	tier := fs.String("tier", "demo", "pricing tier: demo|standard")
	monthlyCap := fs.Int64("monthly-cap", 0, "monthly call hard cap (0 = no cap)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name required")
	}
	if *rate <= 0 {
		return fmt.Errorf("--rate must be positive")
	}
	scopes, err := auth.ParseScopes(*scopesCSV)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, dbDSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	id, plaintext, prefix, err := auth.CreateAPIKey(ctx, pool, *name, scopes, *rate, *tier, *monthlyCap)
	if err != nil {
		return err
	}
	fmt.Printf("api key created: id=%d prefix=%s scopes=%s rate_limit_rps=%d tier=%s monthly_cap=%d\n",
		id, prefix, strings.Join(scopes, ","), *rate, *tier, *monthlyCap)
	fmt.Println("PLAINTEXT KEY (shown once, store securely):")
	fmt.Println("  " + plaintext)
	return nil
}
