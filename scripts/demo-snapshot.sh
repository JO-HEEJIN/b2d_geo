#!/usr/bin/env bash
# 데모용 축소 스냅샷. 지정한 시군구(기본: 하남 41450, 성남 분당 41135)만 담아
# 저렴한 관리형 Postgres/VPS에 올릴 수 있는 작은 덤프를 만든다.
#   - pnu 보유 테이블: 시군구(pnu 앞 5자리)로 필터
#   - land_sales: 읍면동코드(emd_code) 앞 5자리=시군구로 필터
#   - places: 선택 필지의 외곽(bbox)으로 필터(지역 컬럼이 없어 공간 필터)
#   - 참조/소형 테이블: 통째
#   - api_keys / usage_log: 데이터 제외(대상 배포가 자체 발급)
#
# 산출: <OUTDIR>/schema.sql + 테이블별 <table>.csv.gz
# 복원: ./scripts/demo-restore.sh <OUTDIR>  (빈 postgis DB에)
#
# 사용:
#   ./scripts/demo-snapshot.sh [OUTDIR]
#   B2D_DEMO_SIGUNGU="41450 41135 41131" ./scripts/demo-snapshot.sh
#
# 라이선스: 공식 공공데이터 재배포 → 공공누리 출처표시 등 각 데이터셋 약관 준수
# (BRD §6). 네이버/카카오/구글 데이터 혼입 금지.
set -euo pipefail

C="${B2D_PG_CONTAINER:-b2d_postgis}"
OUTDIR="${1:-demo_snapshot}"
SIGUNGU="${B2D_DEMO_SIGUNGU:-41450 41135}"

# 공백 구분 코드 → SQL IN 리스트 ('41450','41135')
IN=""
for s in $SIGUNGU; do IN="${IN}'${s}',"; done
IN="(${IN%,})"

mkdir -p "$OUTDIR"
echo "시군구=$SIGUNGU → $OUTDIR"

psql_copy() { # $1: 파일명(확장자 제외)  $2: COPY 대상 SELECT 또는 테이블
  docker exec "$C" psql -U b2d -d b2d_geo -v ON_ERROR_STOP=1 -c "\copy ($2) TO STDOUT WITH (FORMAT csv, HEADER true)" \
    | gzip > "$OUTDIR/$1.csv.gz"
  echo "  $1 → $(du -h "$OUTDIR/$1.csv.gz" | cut -f1)"
}

echo "스키마 덤프…"
docker exec "$C" pg_dump -U b2d -d b2d_geo --schema-only > "$OUTDIR/schema.sql"

echo "pnu 필터 테이블…"
for t in parcels land_use_plan buildings standard_lots land_features \
         official_land_prices_default official_land_prices; do
  psql_copy "$t" "SELECT * FROM $t WHERE left(pnu,5) IN $IN"
done

echo "실거래(emd_code 앞5자리=시군구 필터)…"
psql_copy land_sales "SELECT * FROM land_sales WHERE left(emd_code,5) IN $IN"

echo "places(선택 필지 외곽 bbox 필터)…"
psql_copy places "SELECT p.* FROM places p WHERE ST_Intersects(p.geom, (
  SELECT ST_SetSRID(ST_Extent(geom)::geometry, 4326) FROM parcels WHERE left(pnu,5) IN $IN))"

echo "참조/소형 테이블(통째)…"
for t in admin_areas land_price_index use_zones; do
  psql_copy "$t" "SELECT * FROM $t"
done

echo "완료: $OUTDIR ($(du -sh "$OUTDIR" | cut -f1))"
echo "복원: ./scripts/demo-restore.sh $OUTDIR"
