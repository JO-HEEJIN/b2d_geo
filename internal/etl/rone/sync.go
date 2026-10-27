package rone

import (
	"context"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JO-HEEJIN/b2d_geo/internal/etl/priceindex"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Source는 이 적재기가 남기는 land_price_index.source 값이다(포털 파일 적재는 'reb_rone').
const Source = "reb_rone_api"

// Options는 한 번 실행의 범위다.
type Options struct {
	From, To string // YYYYMM, 양끝 포함
	Refetch  int    // 이미 받은 달 중 마지막 N개월은 다시 받는다(공표 후 수정 대비)
	Compare  bool   // 쓰지 않고 DB와 대조만(검증용). 범위 안의 모든 달을 받는다
	Log      io.Writer
}

// Stats는 실행 결과다.
type Stats struct {
	Months     int      // 받은 달
	NoData     []string // 아직 공표 전이라 비어 있던 달
	Rows       int      // API 행
	Records    int      // 적재(또는 대조)한 (시군구, 용도지역, 월)
	Aggregate  int      // 집계 행이라 건너뜀
	Unmapped   int      // 주소 DB에서 시군구를 못 찾아 건너뜀
	Superseded int      // 같은 시군구에 현재 이름 행이 있어 버린 "(구)" 행
	Dup        int      // 같은 시군구로 겹친 행(첫 행만 둠)
	Unknown    []string // 미매핑 지역 이름(중복 없이)
	Calls      int
	Cmp        CompareStats
}

// CompareStats는 --compare 결과다.
type CompareStats struct {
	APIRecords, InDB, Same, Diff, OnlyDB int
	Examples                             []string
}

type rec struct {
	code, zone, month string
	rate              float64
	legacy            bool
}

// Run은 범위 안의 달을 받아 적재(또는 대조)한다. 받은 달은 DB에 남으므로 중간에 멈춰도 다음 실행이 이어 받는다.
func Run(ctx context.Context, pool *pgxpool.Pool, c *Client, o Options) (*Stats, error) {
	st := &Stats{}
	months, err := monthRange(o.From, o.To)
	if err != nil {
		return st, err
	}
	logf := func(f string, a ...any) {
		if o.Log != nil {
			fmt.Fprintf(o.Log, f+"\n", a...)
		}
	}
	xw, err := priceindex.Crosswalk(ctx, pool)
	if err != nil {
		return st, fmt.Errorf("crosswalk: %w", err)
	}
	todo := months
	if !o.Compare {
		if todo, err = pending(ctx, pool, months, o.Refetch); err != nil {
			return st, err
		}
	}
	logf("rone-sync: %s..%s months=%d todo=%d", o.From, o.To, len(months), len(todo))
	unknown := map[string]bool{}
	defer func() {
		st.Calls = c.Calls
		for n := range unknown {
			st.Unknown = append(st.Unknown, n)
		}
		sort.Strings(st.Unknown)
	}()
	for _, m := range todo {
		if err := ctx.Err(); err != nil {
			return st, err
		}
		rows, ok, err := c.FetchMonth(ctx, m)
		if err != nil {
			return st, err
		}
		if !ok {
			st.NoData = append(st.NoData, m)
			continue
		}
		recs := build(rows, xw, st, unknown)
		st.Months++
		st.Rows += len(rows)
		st.Records += len(recs)
		if o.Compare {
			if err := compare(ctx, pool, m, recs, &st.Cmp); err != nil {
				return st, err
			}
		} else if err := upsert(ctx, pool, recs); err != nil {
			return st, fmt.Errorf("%s: %w", m, err)
		}
		logf("rone-sync: %s rows=%d records=%d", m, len(rows), len(recs))
	}
	return st, nil
}

// pending은 받을 달이다: 아직 이 적재기로 받지 않은 달 + 받은 달 중 마지막 refetch개월.
func pending(ctx context.Context, pool *pgxpool.Pool, months []string, refetch int) ([]string, error) {
	rows, err := pool.Query(ctx, `SELECT DISTINCT month FROM land_price_index WHERE source=$1 AND month BETWEEN $2 AND $3 ORDER BY month`,
		Source, months[0], months[len(months)-1])
	if err != nil {
		return nil, err
	}
	var loaded []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			rows.Close()
			return nil, err
		}
		loaded = append(loaded, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return pick(months, loaded, refetch), nil
}

// pick은 pending의 순수 계산 부분이다.
func pick(months, loaded []string, refetch int) []string {
	skip := map[string]bool{}
	for i, m := range loaded {
		if i < len(loaded)-refetch {
			skip[m] = true
		}
	}
	var out []string
	for _, m := range months {
		if !skip[m] {
			out = append(out, m)
		}
	}
	return out
}

// build는 API 행을 (시군구, 용도지역, 월) 레코드로 바꾼다. 같은 키가 겹치면 현재 이름 행이 "(구)" 행을 이기고,
// 그 밖의 겹침은 첫 행만 둔다(한 번의 INSERT ... ON CONFLICT에 같은 키가 두 번 들어가면 실패한다).
func build(rows []Row, xw map[string]string, st *Stats, unknown map[string]bool) []rec {
	// 하위 지역이 있는 경로("경기>수원시")는 시 전체 집계다. 주소 DB에서 못 찾아도 미매핑으로 세지 않는다.
	parents := map[string]bool{}
	for _, r := range rows {
		parts := strings.Split(r.Region, ">")
		for i := 1; i < len(parts); i++ {
			parents[strings.Join(parts[:i], ">")] = true
		}
	}
	var out []rec
	at := map[string]int{}
	for _, r := range rows {
		p := parseRegion(r.Region)
		if p.Aggregate || r.Zone == "" {
			st.Aggregate++
			continue
		}
		code := p.Code
		if code == "" {
			code = xw[p.Name]
		}
		if code == "" {
			if parents[r.Region] {
				st.Aggregate++
			} else {
				st.Unmapped++
				unknown[r.Region] = true
			}
			continue
		}
		x := rec{code: code, zone: r.Zone, month: r.Month, rate: r.Rate, legacy: p.Legacy}
		k := code + "|" + r.Zone + "|" + r.Month
		i, seen := at[k]
		switch {
		case !seen:
			at[k] = len(out)
			out = append(out, x)
		case out[i].legacy && !x.legacy:
			out[i] = x
			st.Superseded++
		case x.legacy && !out[i].legacy:
			st.Superseded++
		default:
			st.Dup++
		}
	}
	return out
}

func upsert(ctx context.Context, pool *pgxpool.Pool, recs []rec) error {
	if len(recs) == 0 {
		return nil
	}
	codes := make([]string, len(recs))
	zones := make([]string, len(recs))
	months := make([]string, len(recs))
	rates := make([]string, len(recs))
	for i, r := range recs {
		codes[i], zones[i], months[i] = r.code, r.zone, r.month
		rates[i] = strconv.FormatFloat(r.rate, 'f', -1, 64) // 원값 그대로(numeric으로 넣어 부동소수 꼬리를 만들지 않는다)
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO land_price_index (sigungu_code, use_zone_class, month, rate_pct, source, source_version)
		SELECT c, z, m, r::numeric, $5, now()::date
		FROM unnest($1::text[], $2::text[], $3::text[], $4::text[]) AS t(c, z, m, r)
		ON CONFLICT (sigungu_code, use_zone_class, month)
		DO UPDATE SET rate_pct=EXCLUDED.rate_pct, source=EXCLUDED.source, source_version=EXCLUDED.source_version`,
		codes, zones, months, rates, Source)
	return err
}

// compare는 한 달치를 DB와 대조만 한다. 값 차이·API에만 있음·DB에만 있음을 센다.
func compare(ctx context.Context, pool *pgxpool.Pool, month string, recs []rec, cs *CompareStats) error {
	rows, err := pool.Query(ctx, `SELECT sigungu_code, use_zone_class, rate_pct::float8 FROM land_price_index WHERE month=$1`, month)
	if err != nil {
		return err
	}
	db := map[string]float64{}
	for rows.Next() {
		var c, z string
		var v float64
		if err := rows.Scan(&c, &z, &v); err != nil {
			rows.Close()
			return err
		}
		db[c+"|"+z] = v
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	example := func(s string) {
		if len(cs.Examples) < 20 {
			cs.Examples = append(cs.Examples, s)
		}
	}
	onlyDBShown := 0
	seen := map[string]bool{}
	for _, r := range recs {
		cs.APIRecords++
		k := r.code + "|" + r.zone
		seen[k] = true
		v, ok := db[k]
		if !ok {
			continue
		}
		cs.InDB++
		if math.Abs(v-r.rate) < 1e-9 {
			cs.Same++
		} else {
			cs.Diff++
			example(fmt.Sprintf("diff %s %s %s db=%v api=%v", month, r.code, r.zone, v, r.rate))
		}
	}
	for k := range db {
		if !seen[k] {
			cs.OnlyDB++
			if onlyDBShown < 3 { // 값 차이 예시가 묻히지 않게 달마다 3개까지만
				onlyDBShown++
				example(fmt.Sprintf("only-db %s %s", month, strings.Replace(k, "|", " ", 1)))
			}
		}
	}
	return nil
}

// monthRange는 from..to(YYYYMM, 양끝 포함)의 달 목록이다.
func monthRange(from, to string) ([]string, error) {
	f, err1 := time.Parse("200601", from)
	t, err2 := time.Parse("200601", to)
	if err1 != nil || err2 != nil || t.Before(f) {
		return nil, fmt.Errorf("month range must be YYYYMM and from <= to: %q..%q", from, to)
	}
	var out []string
	for m := f; !m.After(t); m = m.AddDate(0, 1, 0) {
		out = append(out, m.Format("200601"))
	}
	return out, nil
}
