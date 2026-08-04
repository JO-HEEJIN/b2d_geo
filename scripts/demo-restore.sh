#!/usr/bin/env bash
# demo-snapshot.sh 산출물을 빈 대상 DB에 복원한다.
#
# 사용:
#   B2D_TARGET_URL="postgres://user:pw@host:5432/b2d_geo" ./scripts/demo-restore.sh <OUTDIR>
#
# 대상 DB는 비어 있어야 하며 postgis 확장을 만들 수 있는 권한이 필요하다. schema.sql이
# CREATE EXTENSION postgis 포함 → 별도 준비 불필요. 인덱스가 있는 채로 적재하므로
# 소형 데모 데이터에 한해 사용한다(대용량은 인덱스 후생성이 빠름).
set -euo pipefail

DIR="${1:?사용: demo-restore.sh <OUTDIR>}"
URL="${B2D_TARGET_URL:?B2D_TARGET_URL(대상 DB 접속 문자열)가 필요합니다}"

echo "스키마 생성…"
psql "$URL" -v ON_ERROR_STOP=1 -f "$DIR/schema.sql"

echo "데이터 적재…"
# 외래키가 없으므로 순서 무관. 존재하는 csv.gz만 적재.
for f in "$DIR"/*.csv.gz; do
  [ -e "$f" ] || continue
  t="$(basename "$f" .csv.gz)"
  echo "  $t"
  gunzip -c "$f" | psql "$URL" -v ON_ERROR_STOP=1 -c "\copy $t FROM STDIN WITH (FORMAT csv, HEADER true)"
done

echo "완료. 대상 DB에 데모 서브셋 적재됨."
