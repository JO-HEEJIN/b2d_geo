package b2c

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestInfraMapAgainstPostgres는 마이그레이션된 PostGIS에서 네 원천을 함께 본다.
// B2D_TEST_DATABASE_URL이 없으면 건너뛴다(운영 DB에 쓰지 말 것: 테스트 행을 넣고 지운다).
func TestInfraMapAgainstPostgres(t *testing.T) {
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
	cleanup := func() {
		pool.Exec(ctx, `DELETE FROM transit_stops WHERE source = 'b2c_test'`)
		pool.Exec(ctx, `DELETE FROM places WHERE source = 'b2c_test'`)
	}
	cleanup()
	defer cleanup()
	for _, q := range []string{
		`INSERT INTO transit_stops (kind, stop_id, name, line, geom, source, source_version) VALUES
		   ('subway_station', 't-sub', '테스트역', '수인분당선', ST_SetSRID(ST_MakePoint(127.0010, 37.0000), 4326), 'b2c_test', '2026-01-01'),
		   ('bus_stop', 't-bus', '테스트정류장', NULL, ST_SetSRID(ST_MakePoint(127.0003, 37.0000), 4326), 'b2c_test', '2026-01-01')`,
		`INSERT INTO places (store_number, name, norm_name, category_code, geom, source, source_version) VALUES
		   ('b2c-t1', '테스트편의점', '테스트편의점', 'G20405', ST_SetSRID(ST_MakePoint(127.0005, 37.0000), 4326), 'b2c_test', '2026-01-01'),
		   ('b2c-t2', '테스트내과', '테스트내과', 'Q10201', ST_SetSRID(ST_MakePoint(127.0040, 37.0000), 4326), 'b2c_test', '2026-01-01')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	lines, err := NewInfraMapFromDB(pool).Summary(ctx, 127.0000, 37.0000)
	if err != nil {
		t.Fatal(err)
	}
	want := map[InfraKind]string{InfraSubway: "테스트역", InfraBusStop: "테스트정류장", InfraConvenience: "테스트편의점", InfraHospital: "테스트내과"}
	for _, l := range lines {
		if !l.Found || l.Facility.Name != want[l.Kind] {
			t.Errorf("%s = %+v", l.Kind, l)
		}
	}
	if lines[0].Facility.Address != "수인분당선" || lines[3].Facility.WalkMinutes != 6 {
		t.Errorf("subway=%+v hospital=%+v", lines[0].Facility, lines[3].Facility)
	}
}
