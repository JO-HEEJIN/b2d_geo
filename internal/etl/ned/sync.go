package ned

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Options는 한 번 실행의 범위다.
type Options struct {
	Datasets []Dataset
	Year     int
	Sido     []string  // 시도 2자리 코드. 비면 전국
	MaxCalls int       // 이만큼 호출하면 멈춘다(일일 한도 보호). 0이면 제한 없음
	Compare  bool      // true면 적재하지 않고 DB와 대조만 한다(검증용, 진행 기록도 안 남김)
	MaxDongs int       // 0이면 제한 없음. 검증·시범용
	Log      io.Writer // 진행 로그
}

// Stats는 실행 결과 요약이다.
type Stats struct {
	Dongs, Rows, Calls int
	Stopped            string // 멈춘 이유(한도·API 오류). 끝까지 갔으면 빈 문자열
	Cmp                CompareStats
}

// CompareStats는 Compare 모드에서 API 값과 DB 값을 필지별로 맞춰 본 결과다.
type CompareStats struct {
	APIRows, InDB, Same, Diff int
	Examples                  []string
}

// Run은 법정동 목록을 돌며 받고 적재한다. 끝낸 동은 진행 기록에 남겨 다음 실행에서 건너뛴다.
func Run(ctx context.Context, pool *pgxpool.Pool, c *Client, o Options) (Stats, error) {
	var st Stats
	for _, d := range o.Datasets {
		dongs, err := pendingDongs(ctx, pool, d, o)
		if err != nil {
			return st, err
		}
		fmt.Fprintf(o.Log, "ned-sync %s %d: %d dongs to do\n", d, o.Year, len(dongs))
		for i, bcode := range dongs {
			if o.MaxCalls > 0 && c.Calls >= o.MaxCalls {
				st.Stopped = fmt.Sprintf("max-calls %d reached", o.MaxCalls)
				st.Calls = c.Calls
				return st, nil
			}
			rows, total, err := c.FetchDong(ctx, d, bcode, o.Year)
			if err != nil {
				var ae *APIError
				if errors.As(err, &ae) {
					st.Stopped = ae.Error() // 키·권한·한도 문제는 다음 날 이어서
					st.Calls = c.Calls
					return st, nil
				}
				return st, fmt.Errorf("%s %s: %w", d, bcode, err)
			}
			if o.Compare {
				if err := compare(ctx, pool, d, rows, &st.Cmp); err != nil {
					return st, err
				}
			} else {
				if err := upsert(ctx, pool, d, rows); err != nil {
					return st, fmt.Errorf("%s %s upsert: %w", d, bcode, err)
				}
				if _, err := pool.Exec(ctx, `
					INSERT INTO ned_sync_progress (dataset, base_year, bcode, rows) VALUES ($1,$2,$3,$4)
					ON CONFLICT (dataset, base_year, bcode) DO UPDATE SET rows = EXCLUDED.rows, synced_at = now()`,
					string(d), o.Year, bcode, len(rows)); err != nil {
					return st, err
				}
			}
			st.Dongs++
			st.Rows += len(rows)
			if (i+1)%100 == 0 || i == len(dongs)-1 {
				fmt.Fprintf(o.Log, "  %s %d/%d dongs, rows %d, calls %d (last %s total %d)\n",
					d, i+1, len(dongs), st.Rows, c.Calls, bcode, total)
			}
		}
	}
	st.Calls = c.Calls
	return st, nil
}

// pendingDongs는 필지가 실제로 있는 법정동 중 이번 연도에 아직 안 끝낸 동을 돌려준다.
// parcels(bcode, jibun) 인덱스의 범위 조건으로 시도별로 뽑는다.
func pendingDongs(ctx context.Context, pool *pgxpool.Pool, d Dataset, o Options) ([]string, error) {
	sidos := o.Sido
	if len(sidos) == 0 {
		sidos = []string{""}
	}
	var out []string
	for _, sd := range sidos {
		lo := (sd + "0000000000")[:10]
		hi := (sd + "9999999999")[:10]
		q := `SELECT DISTINCT p.bcode FROM parcels p
		      WHERE p.bcode >= $1 AND p.bcode <= $2`
		args := []any{lo, hi}
		if !o.Compare {
			q += ` AND NOT EXISTS (SELECT 1 FROM ned_sync_progress s
			        WHERE s.dataset = $3 AND s.base_year = $4 AND s.bcode = p.bcode)`
			args = append(args, string(d), o.Year)
		}
		q += ` ORDER BY 1`
		rows, err := pool.Query(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var b string
			if err := rows.Scan(&b); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, b)
			if o.MaxDongs > 0 && len(out) >= o.MaxDongs {
				break
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if o.MaxDongs > 0 && len(out) >= o.MaxDongs {
			break
		}
	}
	return out, nil
}

// upsert는 한 동의 행을 한 문장으로 넣는다. 같은 (pnu, 연도)가 두 번 오면 뒤의 것을 쓴다.
func upsert(ctx context.Context, pool *pgxpool.Pool, d Dataset, rows []Row) error {
	rows = dedupe(rows)
	if len(rows) == 0 {
		return nil
	}
	pnus := make([]string, len(rows))
	years := make([]int16, len(rows))
	vers := make([]time.Time, len(rows))
	for i, r := range rows {
		pnus[i], years[i], vers[i] = r.PNU, int16(r.BaseYear), r.SourceVersion
	}
	switch d {
	case Price:
		prices := make([]int64, len(rows))
		for i, r := range rows {
			prices[i] = r.PricePerSqm
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO official_land_prices (pnu, base_year, price_per_sqm, source, source_version)
			SELECT u.pnu, u.y, u.p, 'vworld_ned', u.v
			  FROM unnest($1::char(19)[], $2::smallint[], $3::bigint[], $4::date[]) AS u(pnu, y, p, v)
			ON CONFLICT (pnu, base_year) DO UPDATE
			   SET price_per_sqm = EXCLUDED.price_per_sqm, source = EXCLUDED.source,
			       source_version = EXCLUDED.source_version`,
			pnus, years, prices, vers)
		return err
	case Features:
		lu := make([]string, len(rows))
		th := make([]string, len(rows))
		ts := make([]string, len(rows))
		rs := make([]string, len(rows))
		for i, r := range rows {
			lu[i], th[i], ts[i], rs[i] = r.LandUse, r.TerrainHeight, r.TerrainShape, r.RoadSide
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO land_features (pnu, base_year, land_use, terrain_height, terrain_shape, road_side, source, source_version)
			SELECT u.pnu, u.y, u.lu, u.th, u.ts, u.rs, 'vworld_ned', u.v
			  FROM unnest($1::char(19)[], $2::smallint[], $3::text[], $4::text[], $5::text[], $6::text[], $7::date[])
			       AS u(pnu, y, lu, th, ts, rs, v)
			ON CONFLICT (pnu, base_year) DO UPDATE
			   SET land_use = EXCLUDED.land_use, terrain_height = EXCLUDED.terrain_height,
			       terrain_shape = EXCLUDED.terrain_shape, road_side = EXCLUDED.road_side,
			       source = EXCLUDED.source, source_version = EXCLUDED.source_version`,
			pnus, years, lu, th, ts, rs, vers)
		return err
	}
	return fmt.Errorf("unknown dataset %q", d)
}

func dedupe(rows []Row) []Row {
	idx := make(map[string]int, len(rows))
	out := rows[:0:0]
	for _, r := range rows {
		k := r.PNU + fmt.Sprint(r.BaseYear)
		if i, ok := idx[k]; ok {
			out[i] = r
			continue
		}
		idx[k] = len(out)
		out = append(out, r)
	}
	return out
}

// compare는 API 행과 DB에 이미 있는 행(파일 적재분)을 필지별로 대조한다. 쓰지 않는다.
func compare(ctx context.Context, pool *pgxpool.Pool, d Dataset, rows []Row, cs *CompareStats) error {
	rows = dedupe(rows)
	cs.APIRows += len(rows)
	for _, r := range rows {
		var same bool
		var dbv string
		var err error
		switch d {
		case Price:
			var v int64
			err = pool.QueryRow(ctx, `SELECT price_per_sqm FROM official_land_prices WHERE pnu=$1 AND base_year=$2`,
				r.PNU, r.BaseYear).Scan(&v)
			same, dbv = v == r.PricePerSqm, fmt.Sprint(v)
		case Features:
			var lu, th, ts, rs *string
			err = pool.QueryRow(ctx, `SELECT land_use, terrain_height, terrain_shape, road_side FROM land_features WHERE pnu=$1 AND base_year=$2`,
				r.PNU, r.BaseYear).Scan(&lu, &th, &ts, &rs)
			s := func(p *string) string {
				if p == nil {
					return ""
				}
				return *p
			}
			same = s(lu) == r.LandUse && s(th) == r.TerrainHeight && s(ts) == r.TerrainShape && s(rs) == r.RoadSide
			dbv = fmt.Sprintf("%s|%s|%s|%s", s(lu), s(th), s(ts), s(rs))
		}
		if err != nil {
			continue // DB에 없는 필지(파일엔 없고 API엔 있는 경우)는 InDB에 세지 않는다
		}
		cs.InDB++
		if same {
			cs.Same++
			continue
		}
		cs.Diff++
		if len(cs.Examples) < 8 {
			api := fmt.Sprint(r.PricePerSqm)
			if d == Features {
				api = fmt.Sprintf("%s|%s|%s|%s", r.LandUse, r.TerrainHeight, r.TerrainShape, r.RoadSide)
			}
			cs.Examples = append(cs.Examples, fmt.Sprintf("%s %s %d: db=%s api=%s", d, r.PNU, r.BaseYear, dbv, api))
		}
	}
	return nil
}
