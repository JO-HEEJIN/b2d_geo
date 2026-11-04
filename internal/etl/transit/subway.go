package transit

// 지하철: 전국도시철도역사정보표준데이터 (data.go.kr 15013205, 국가철도공단). XLSX 한 시트.
// 2026-06-30판 실물 확인: 1,099행, 좌표 이상치 없음. 역번호는 운영기관끼리 겹치므로
// (예: "101") stop_id는 "역번호-노선번호"로 만든다. GTX-A·SRT 동탄역은 이 자료에 없다.

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SubwaySourceName은 transit_stops.source 값이다.
const SubwaySourceName = "kric_urban_rail_stations"

// subwayColumns는 2026-06-30판 헤더 중 쓰는 열이다. 헤더에 없으면 적재를 멈춘다.
var subwayColumns = []string{"역번호", "역사명", "노선번호", "노선명", "역위도", "역경도"}

// Station은 도시철도 역 1곳(노선별)이다.
type Station struct {
	ID   string // 역번호-노선번호
	Name string
	Line string
	Lon  float64
	Lat  float64
}

// ParseSubwayXLSX는 표준데이터 XLSX 첫 시트를 읽는다. 좌표가 한국 범위를 벗어나거나
// 필수 값이 빈 행은 건너뛰고 그 수를 돌려준다.
func ParseSubwayXLSX(r io.ReaderAt, size int64, fn func(Station) error) (skipped int, err error) {
	rows, err := readFirstSheet(r, size)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("빈 시트")
	}
	idx := map[string]int{}
	for i, h := range rows[0] {
		idx[strings.TrimSpace(h)] = i
	}
	for _, c := range subwayColumns {
		if _, ok := idx[c]; !ok {
			return 0, fmt.Errorf("헤더에 %q 없음", c)
		}
	}
	get := func(row []string, col string) string {
		if i := idx[col]; i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	for _, row := range rows[1:] {
		id, name, lineNo := get(row, "역번호"), get(row, "역사명"), get(row, "노선번호")
		lat, errLat := strconv.ParseFloat(get(row, "역위도"), 64)
		lon, errLon := strconv.ParseFloat(get(row, "역경도"), 64)
		if id == "" || name == "" || lineNo == "" || errLat != nil || errLon != nil ||
			lat < 33 || lat > 39.5 || lon < 124 || lon > 132 {
			skipped++
			continue
		}
		if err := fn(Station{ID: id + "-" + lineNo, Name: name, Line: get(row, "노선명"), Lon: lon, Lat: lat}); err != nil {
			return skipped, err
		}
	}
	return skipped, nil
}

// LoadSubway는 역 XLSX를 transit_stops에 적재한다. 한 트랜잭션에서 기존 지하철 행을
// 지우고 다시 넣는다. 같은 stop_id가 두 번 나오면 첫 행만 남긴다.
func LoadSubway(ctx context.Context, pool *pgxpool.Pool, r io.ReaderAt, size int64, sourceVersion string) (inserted int64, skipped int, err error) {
	var rows [][]any
	skipped, err = ParseSubwayXLSX(r, size, func(s Station) error {
		rows = append(rows, []any{s.ID, s.Name, s.Line, s.Lon, s.Lat})
		return nil
	})
	if err != nil {
		return 0, skipped, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, skipped, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM transit_stops WHERE source = $1`, SubwaySourceName); err != nil {
		return 0, skipped, err
	}
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE subway_staging (stop_id text, name text, line text, lon float8, lat float8) ON COMMIT DROP`); err != nil {
		return 0, skipped, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"subway_staging"}, []string{"stop_id", "name", "line", "lon", "lat"}, pgx.CopyFromRows(rows)); err != nil {
		return 0, skipped, fmt.Errorf("CopyFrom: %w", err)
	}
	ct, err := tx.Exec(ctx, `INSERT INTO transit_stops (kind, stop_id, name, line, geom, source, source_version)
		SELECT 'subway_station', stop_id, name, NULLIF(line, ''), ST_SetSRID(ST_MakePoint(lon, lat), 4326), $1, $2::date
		FROM subway_staging
		ON CONFLICT (kind, stop_id) DO NOTHING`, SubwaySourceName, sourceVersion)
	if err != nil {
		return 0, skipped, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, skipped, err
	}
	return ct.RowsAffected(), skipped, nil
}

// readFirstSheet는 XLSX(zip)의 첫 시트를 문자열 표로 읽는다. 공유 문자열·인라인 문자열·
// 숫자 셀을 지원하고, 빈 셀은 열 위치(A, B, ...)를 보고 자리를 채운다. 외부 라이브러리 없이
// 표준 라이브러리만 쓴다(지시서 1.6).
func readFirstSheet(r io.ReaderAt, size int64) ([][]string, error) {
	z, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("xlsx 열기: %w", err)
	}
	files := map[string]*zip.File{}
	for _, f := range z.File {
		files[f.Name] = f
	}
	var shared []string
	if f, ok := files["xl/sharedStrings.xml"]; ok {
		var sst struct {
			SI []struct {
				T string `xml:"t"`
				R []struct {
					T string `xml:"t"`
				} `xml:"r"`
			} `xml:"si"`
		}
		if err := decodeZipXML(f, &sst); err != nil {
			return nil, fmt.Errorf("sharedStrings: %w", err)
		}
		for _, si := range sst.SI {
			s := si.T
			for _, run := range si.R {
				s += run.T
			}
			shared = append(shared, s)
		}
	}
	sheet, ok := files["xl/worksheets/sheet1.xml"]
	if !ok {
		return nil, fmt.Errorf("xl/worksheets/sheet1.xml 없음")
	}
	var ws struct {
		Rows []struct {
			Cells []struct {
				Ref    string `xml:"r,attr"`
				Type   string `xml:"t,attr"`
				V      string `xml:"v"`
				Inline string `xml:"is>t"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := decodeZipXML(sheet, &ws); err != nil {
		return nil, fmt.Errorf("sheet1: %w", err)
	}
	out := make([][]string, 0, len(ws.Rows))
	for _, row := range ws.Rows {
		var cells []string
		for _, c := range row.Cells {
			col := columnIndex(c.Ref)
			for len(cells) < col {
				cells = append(cells, "")
			}
			v := c.V
			switch c.Type {
			case "s":
				i, err := strconv.Atoi(c.V)
				if err != nil || i < 0 || i >= len(shared) {
					return nil, fmt.Errorf("셀 %s: 공유 문자열 %q 범위 밖", c.Ref, c.V)
				}
				v = shared[i]
			case "inlineStr":
				v = c.Inline
			}
			cells = append(cells, v)
		}
		out = append(out, cells)
	}
	return out, nil
}

func decodeZipXML(f *zip.File, v any) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return xml.NewDecoder(io.LimitReader(rc, 256<<20)).Decode(v)
}

// columnIndex는 셀 참조("C12")의 열 번호(0부터)를 돌려준다.
func columnIndex(ref string) int {
	n := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		n = n*26 + int(r-'A'+1)
	}
	return n - 1
}
