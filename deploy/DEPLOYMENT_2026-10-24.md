# 2026-10-24 앱 배포 기록 (올맵 리포트 화면)

상태: **운영 앱 교체 완료** (2026-10-24 저녁 KST). 9월 13일과 같은 방식으로 앱 컨테이너만 교체했다.

## 적용 범위

- 지도 화면 4열 리포트(사진+지도 | 거래내역 | 지가변동률 | 판단 요약+A4 보고서), 표시명 올맵.
- `/v1/appraisal/sales/recent` 신설, V-World 항공사진(env `B2D_VWORLD_KEY`, 개발키).
- 대표 용도지역 선택 수정(운영 실데이터에서 발견).
- 빈 반경 KNN 전수 탐색 수정(2026-10-23 500 묶음 원인)과 API 서버 DB statement_timeout 20초.
- Go 1.26.10, golang.org/x/text v0.41.0 (govulncheck 실패 해소).
- DB 마이그레이션·Caddy·MCP·DB 컨테이너 변경 없음.

## 이미지

| 항목 | 값 |
|---|---|
| 배포 소스 커밋 | `9d70560` |
| 배포 이미지 | `ghcr.io/jo-heejin/b2d_geo:sha-9d70560` (digest `sha256:2e2468fbbf9c3278b664be903d47242efd67933f520c7b505db7e450593fd1c6`) |
| 이전 실행 이미지 | `ghcr.io/jo-heejin/b2d_geo@sha256:e690b1def06d0fdd7387942316b4cfa6627c6e69f3feaac475726536477bad4d` (2026-09-13) |

## 운영 설정

- 운영 체크아웃은 `git pull`로 f21518e까지 받았으나, base `docker-compose.yml`과 `Caddyfile`은 운영 중이던
  내용(`*.vps-20261024` 백업과 동일)으로 되돌려 두었다. 따라서 git 상태에 두 파일이 수정으로 보인다.
- 앱 버전과 `B2D_VWORLD_KEY` 전달은 `docker-compose.override.yml`에서만 바꾼다.
- 되돌리기용 override: `docker-compose.rollback-20261024.yml` (9월 13일 digest).

```sh
# 적용
docker compose -f docker-compose.yml -f docker-compose.override.yml up -d --no-deps --pull never app
# 되돌리기
cp -p docker-compose.rollback-20261024.yml docker-compose.override.yml
docker compose -f docker-compose.yml -f docker-compose.override.yml up -d --no-deps --pull never app
```

## 운영 검증 (배포 후, 외부에서 실요청)

| 점검 | 결과 |
|---|---|
| `/v1/health` | 200 |
| `/config.json` | `vworldKey` 포함 |
| 장애 좌표 reverse (38.22, 127.0) 등 빈 지역 3곳 | 404 "no building within 200m", 약 0.6초(이전 1분 이상 후 500) |
| 바다·접경지 클릭(브라우저) | 약 2초 안에 "필지를 찾지 못했습니다" 표시 |
| 실주소 3곳(하남·서울 중구·화성 만세구) | 4열 렌더, 용도지역 정확(일반상업·일반상업·자연녹지), 거래 80/14/1,630건, 동 위치 17/8/106곳, 콘솔 오류 0, API 실패 0 |
| V-World 타일 요청 | 올바른 WMTS URL로 요청됨. 실제 사진 표시는 검증 환경에서 V-World 접속이 막혀 브라우저 확인 필요 |

소량의 정상 요청 검증이며 부하 테스트는 하지 않았다.
