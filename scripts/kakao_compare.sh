#!/usr/bin/env bash
# Phase 6-2: 카카오 로컬(주소검색) vs b2d_geo geocode 정확도 비교 하니스.
#
# 실주소 N건을 buildings(공식 juso 좌표)에서 재현가능하게 샘플 → 두 시스템에
# 동일 도로명주소 문자열을 질의 → 매치율 + 좌표 거리(공식 대비, 상호간)를 측정.
# 공식 좌표(juso)를 ground truth로 본다.
#
# 필요: b2d_postgis 컨테이너 up, b2d-server(:8080) up,
#       B2D_API_KEY(geo scope 로컬 키 — `b2d-etl apikey create --name x --scopes geo`),
#       (카카오 비교 시) KAKAO_REST_KEY.
# KAKAO_REST_KEY 없으면 b2d 절반만 측정하고 카카오 열은 비워 파이프라인만 검증.
#
# 사용:
#   B2D_API_KEY=b2d_… KAKAO_REST_KEY=xxx ./scripts/kakao_compare.sh   # 전체 비교
#   B2D_API_KEY=b2d_… ./scripts/kakao_compare.sh                      # b2d 절반만
#   N=200 SEED=0.7 B2D_API_KEY=b2d_… ./scripts/kakao_compare.sh       # 표본수/시드 변경
set -euo pipefail

N="${N:-100}"
SEED="${SEED:-0.42}"                 # 재현용 고정 시드
B2D="${B2D:-http://localhost:8080}"
pg() { docker exec b2d_postgis psql -U b2d -d b2d_geo -tA -F $'\t' "$@"; }   # 탭 구분
OUT="$(cd "$(dirname "$0")" && pwd)/out"
mkdir -p "$OUT"
SAMPLE="$OUT/sample_${N}.tsv"
CSV="$OUT/kakao_compare.csv"

# 1) 표본 (없으면 생성 — 재현가능: setseed 후 random())
if [[ ! -s "$SAMPLE" ]]; then
  echo "[sample] buildings에서 ${N}건 추출 (seed=$SEED)..." >&2
  pg -c "SELECT setseed($SEED); SELECT road_address, ST_X(geom), ST_Y(geom)
          FROM buildings
          WHERE geom IS NOT NULL AND road_address IS NOT NULL AND length(road_address) >= 5
          ORDER BY random() LIMIT $N;" \
    | grep -v '^$' > "$SAMPLE"
fi
echo "[sample] $(wc -l < "$SAMPLE" | tr -d ' ')건 준비: $SAMPLE" >&2

[[ -z "${B2D_API_KEY:-}" ]] && echo "[warn] B2D_API_KEY 없음 → b2d 전건 unauthorized(매치 0%). geo scope 키 필요." >&2
have_key=0; [[ -n "${KAKAO_REST_KEY:-}" ]] && have_key=1
[[ $have_key -eq 0 ]] && echo "[warn] KAKAO_REST_KEY 없음 → b2d 절반만 측정" >&2

# curl 단일 시도(--retry 금지: 부분응답 이어붙임→멀티JSON→CSV 깨짐). 재시도는 셸 루프가.
CURL="curl -sS --max-time 10"
# fetch: 일시적 오류는 셸 재시도, definitive 응답(좌표 or 명시적 no-match)에서 종료.
# 단일 JSON 여부(jq -e '.') 검사로 멀티/깨진 응답은 재시도.
b2d_get() { local r; for _ in 1 2 3; do
    r=$($CURL -H "X-API-Key: ${B2D_API_KEY:-}" --get "$B2D/v1/geocode" --data-urlencode "q=$1" 2>/dev/null || echo '{}')
    echo "$r" | jq -e 'if type=="object" and ((.point!=null) or (.error.message=="no match")) then . else empty end' >/dev/null 2>&1 && { echo "$r"; return; }
  done; echo "$r"; }
kko_get() { local r; for _ in 1 2 3; do
    r=$($CURL -H "Authorization: KakaoAK $KAKAO_REST_KEY" --get "https://dapi.kakao.com/v2/local/search/address.json" --data-urlencode "query=$1" 2>/dev/null || echo '{}')
    echo "$r" | jq -e 'if type=="object" and .documents!=null then . else empty end' >/dev/null 2>&1 && { echo "$r"; return; }
  done; echo "$r"; }

# 미터 단위 haversine
hav() { awk -v a1="$1" -v o1="$2" -v a2="$3" -v o2="$4" 'BEGIN{
  if(a1==""||a2=="") {print ""; exit}
  r=6371000; d=atan2(0,-1)/180;
  dlat=(a2-a1)*d; dlon=(o2-o1)*d;
  x=sin(dlat/2)^2 + cos(a1*d)*cos(a2*d)*sin(dlon/2)^2;
  printf "%.1f", 2*r*atan2(sqrt(x),sqrt(1-x)); }'; }

echo "road_address,off_lon,off_lat,b2d_match,b2d_lon,b2d_lat,b2d_dist_m,kko_lon,kko_lat,kko_dist_m,b2d_kko_dist_m" > "$CSV"

while IFS=$'\t' read -r addr olon olat; do
  [[ -z "$addr" ]] && continue
  # b2d (인증: X-API-Key. geo scope 로컬 키를 B2D_API_KEY로 주입)
  bj=$(b2d_get "$addr")
  bmatch=$(echo "$bj" | jq -r '.match // "none"')
  blon=$(echo "$bj" | jq -r '.point[0] // empty'); blat=$(echo "$bj" | jq -r '.point[1] // empty')
  bdist=$(hav "$olat" "$olon" "$blat" "$blon")
  # kakao
  klon=""; klat=""
  if [[ $have_key -eq 1 ]]; then
    kj=$(kko_get "$addr")
    klon=$(echo "$kj" | jq -r '(.documents[0].road_address.x // .documents[0].x) // empty')
    klat=$(echo "$kj" | jq -r '(.documents[0].road_address.y // .documents[0].y) // empty')
  fi
  kdist=$(hav "$olat" "$olon" "$klat" "$klon")
  bkdist=$(hav "$blat" "$blon" "$klat" "$klon")
  echo "\"$addr\",$olon,$olat,$bmatch,$blon,$blat,$bdist,$klon,$klat,$kdist,$bkdist" >> "$CSV"
done < "$SAMPLE"

# 2) 요약
echo "[summary]" >&2
awk -F, 'NR>1{
  n++;
  if($5!="") bm++;                      # b2d 좌표 반환
  if($4=="exact"||$4=="exact_partial") bex++;
  if($8!="") km++;                      # kakao 좌표 반환
  if($7!=""){bs+=$7; bc++; ba[bc]=$7}
  if($10!=""){ks+=$10; kc++; ka[kc]=$10}
}
function med(arr,c,  i,t){ if(c==0)return ""; for(i=1;i<=c;i++)for(t=i+1;t<=c;t++)if(arr[t]<arr[i]){x=arr[i];arr[i]=arr[t];arr[t]=x} return arr[int((c+1)/2)] }
END{
  printf "  표본 N=%d\n", n;
  printf "  b2d 매치율     = %d/%d (%.1f%%),  exact=%d\n", bm,n,100*bm/n,bex;
  printf "  kakao 매치율   = %d/%d (%.1f%%)\n", km,n, (n?100*km/n:0);
  printf "  b2d↔공식 거리   중앙값=%sm 평균=%.1fm (n=%d)\n", med(ba,bc), (bc?bs/bc:0), bc;
  printf "  kakao↔공식 거리 중앙값=%sm 평균=%.1fm (n=%d)\n", med(ka,kc), (kc?ks/kc:0), kc;
}' "$CSV" >&2
echo "[out] $CSV" >&2
