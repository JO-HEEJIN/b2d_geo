#!/usr/bin/env bash
# On-prem 데이터 스냅샷: b2d_geo 전체를 custom-format으로 덤프한다.
# api_keys/usage_log는 스키마만 남기고 데이터 제외(배포별 상태 — 대상 배포가 자체 키 발급).
#
# 사용: ./scripts/snapshot.sh [출력파일]        # 기본 b2d_snapshot.dump
# 복원: ./scripts/restore.sh <파일>
# 컨테이너명 재정의: B2D_PG_CONTAINER=<name> ./scripts/snapshot.sh
#
# 라이선스: 스냅샷 배포 = 공식 공공데이터 재배포 → 공공누리 출처표시 등 각 데이터셋
# 약관 준수 필수 (BRD §6, 04_BRD_b2d_geo.md). 네이버/카카오/구글 데이터 혼입 금지.
set -euo pipefail
OUT="${1:-b2d_snapshot.dump}"
C="${B2D_PG_CONTAINER:-b2d_postgis}"
docker exec "$C" pg_dump -U b2d -d b2d_geo \
  --format=custom --compress=9 \
  --exclude-table-data=api_keys \
  --exclude-table-data=usage_log \
  > "$OUT"
echo "snapshot → $OUT ($(du -h "$OUT" | cut -f1))"
