#!/usr/bin/env bash
# 스냅샷 복원. 대상: PostGIS 확장이 있는 빈 b2d_geo DB (postgis/postgis 이미지는 기본 제공).
# 덤프에 CREATE EXTENSION IF NOT EXISTS가 포함되어 확장·테이블·데이터가 순서대로 복원된다.
#
# 사용: ./scripts/restore.sh <snapshot.dump>
# 컨테이너명 재정의: B2D_PG_CONTAINER=<name> ./scripts/restore.sh <파일>
set -euo pipefail
SNAP="${1:?usage: restore.sh <snapshot.dump>}"
C="${B2D_PG_CONTAINER:-b2d_postgis}"
docker exec -i "$C" pg_restore -U b2d -d b2d_geo --no-owner --no-privileges < "$SNAP"
echo "restored from $SNAP → $C:b2d_geo"
