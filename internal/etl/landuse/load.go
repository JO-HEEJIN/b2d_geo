// Package landuse는 토지이용계획공간정보(NSDI AL_D154, 시도별 중첩 zip)를
// land_use_plan에 적재한다 (Phase 7-2).
//
// 명세(실측 2026-07-29): 번들 zip 안에 시도별 AL_D154_<시도>_<날짜>.zip, 각 nested가
// shapefile 세트(EPSG:5186, DBF CP949, 필지 폴리곤). 필드 고정: 0=A0(PNU 19),
// 7=A7(용도지역지구 코드들, 콤마구분), 8=A8(명칭들, 콤마구분), 10=A10(포함/저촉 관계,
// 콤마구분), 11=A11(기준일 YYYYMMDD). A7/A8/A10은 병렬 배열.
//
// 원칙: 사실 저장만. 원자료의 포함/저촉 사전판정을 그대로 적재(우리가 재판정 안 함).
// geom은 저장하지 않는다 — 엔드포인트는 PNU 조회만 필요. 콤마 정렬이 어긋난 행은 스킵.
package landuse

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jonas-p/go-shp"
	"golang.org/x/text/encoding/korean"
)

const upsertSQL = `
INSERT INTO land_use_plan (pnu, zone_code, zone_name, relation, source, source_version)
VALUES ($1,$2,$3,$4,'nsdi_luz',$5::date)
ON CONFLICT (pnu, zone_code) DO UPDATE SET
  zone_name=EXCLUDED.zone_name, relation=EXCLUDED.relation, source_version=EXCLUDED.source_version`

// Stats는 번들 전체 적재 결과다.
type Stats struct {
	Files    int
	Parcels  int // 처리한 필지
	Rows     int // 적재된 (필지×용도지역) 행
	Misalign int // A7/A8/A10 콤마 개수 불일치로 스킵한 필지
	BadMeta  int // PNU/기준일 결측으로 스킵한 필지
}

// Load는 토지이용계획 번들 zip을 열어 시도별 AL_D154 nested zip을 차례로 적재한다.
func Load(ctx context.Context, pool *pgxpool.Pool, bundlePath string) (*Stats, error) {
	zr, err := zip.OpenReader(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("open bundle: %w", err)
	}
	defer zr.Close()
	total := &Stats{}
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if !strings.HasPrefix(base, "AL_D154_") || !strings.HasSuffix(strings.ToLower(base), ".zip") {
			continue
		}
		st, err := loadNested(ctx, pool, f)
		if err != nil {
			return total, fmt.Errorf("%s: %w", base, err)
		}
		total.Files++
		total.Parcels += st.Parcels
		total.Rows += st.Rows
		total.Misalign += st.Misalign
		total.BadMeta += st.BadMeta
		fmt.Fprintf(os.Stderr, "  %s: parcels=%d rows=%d misalign=%d badmeta=%d\n",
			base, st.Parcels, st.Rows, st.Misalign, st.BadMeta)
	}
	if total.Files == 0 {
		return total, fmt.Errorf("번들에 AL_D154_*.zip 없음: %s", bundlePath)
	}
	return total, nil
}

func loadNested(ctx context.Context, pool *pgxpool.Pool, f *zip.File) (*Stats, error) {
	tmpZip, err := writeTemp(f)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpZip)
	dir, err := os.MkdirTemp("", "landuse-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	shpPath, err := extractShp(tmpZip, dir)
	if err != nil {
		return nil, err
	}
	r, err := shp.Open(shpPath)
	if err != nil {
		return nil, fmt.Errorf("open shp: %w", err)
	}
	defer r.Close()

	dec := korean.EUCKR.NewDecoder()
	stats := &Stats{}
	batch := &pgx.Batch{}
	flush := func() error {
		if batch.Len() == 0 {
			return nil
		}
		br := pool.SendBatch(ctx, batch)
		defer br.Close()
		for i := 0; i < batch.Len(); i++ {
			if _, err := br.Exec(); err != nil {
				return err
			}
		}
		batch = &pgx.Batch{}
		return nil
	}

	for r.Next() {
		n, _ := r.Shape() // geom은 버린다; 인덱스만 사용
		pnu := strings.TrimSpace(r.ReadAttribute(int(n), 0))
		srcVer := dateDashes(strings.TrimSpace(r.ReadAttribute(int(n), 11)))
		if len(pnu) != 19 || srcVer == "" {
			stats.BadMeta++
			continue
		}
		codes := strings.Split(dcz(dec, r.ReadAttribute(int(n), 7)), ",")
		names := strings.Split(dcz(dec, r.ReadAttribute(int(n), 8)), ",")
		rels := strings.Split(dcz(dec, r.ReadAttribute(int(n), 10)), ",")
		if len(codes) != len(names) || len(codes) != len(rels) {
			stats.Misalign++ // 명칭 내부 콤마 등으로 병렬 정렬 어긋남
			continue
		}
		stats.Parcels++
		for i := range codes {
			zc := strings.TrimSpace(codes[i])
			if zc == "" {
				continue
			}
			batch.Queue(upsertSQL, pnu, zc, strings.TrimSpace(names[i]), strings.TrimSpace(rels[i]), srcVer)
			stats.Rows++
			if batch.Len() >= 1000 {
				if err := flush(); err != nil {
					return stats, err
				}
			}
		}
	}
	if err := flush(); err != nil {
		return stats, err
	}
	return stats, nil
}

// writeTemp는 zip 안의 nested zip 엔트리를 임시 파일로 쓴다.
func writeTemp(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	tmp, err := os.CreateTemp("", "landuse-*.zip")
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	if _, err := io.Copy(tmp, rc); err != nil {
		return "", err
	}
	return tmp.Name(), nil
}

// extractShp는 nested zip에서 shp/dbf/shx를 풀고 .shp 경로를 돌려준다.
func extractShp(zipPath, dir string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	var shpPath string
	for _, f := range zr.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext != ".shp" && ext != ".dbf" && ext != ".shx" {
			continue
		}
		dst := filepath.Join(dir, "layer"+ext)
		if err := copyEntry(f, dst); err != nil {
			return "", err
		}
		if ext == ".shp" {
			shpPath = dst
		}
	}
	if shpPath == "" {
		return "", fmt.Errorf("no .shp in %s", zipPath)
	}
	return shpPath, nil
}

func copyEntry(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	w, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer w.Close()
	_, err = io.Copy(w, rc)
	return err
}

// dcz는 CP949 바이트를 UTF-8로 디코드·트림한다(실패 시 원문 트림).
func dcz(dec interface{ String(string) (string, error) }, raw string) string {
	v, err := dec.String(raw)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return strings.TrimSpace(v)
}

// dateDashes는 "20260707" → "2026-07-07"(8자리 숫자 아니면 "").
func dateDashes(s string) string {
	if len(s) != 8 {
		return ""
	}
	for i := 0; i < 8; i++ {
		if s[i] < '0' || s[i] > '9' {
			return ""
		}
	}
	return s[:4] + "-" + s[4:6] + "-" + s[6:8]
}
