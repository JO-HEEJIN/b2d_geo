# 09. 웹 프론트 + API 공개 배포 가이드

`b2d-server` 하나가 **API(`/v1/*`)와 웹 사이트(랜딩·지도 데모·배치·API 문서)를
같은 오리진에서** 서빙한다(`web/`를 `go:embed`로 내장). 따라서 배포는 **바이너리/
Docker 이미지 1개 + PostGIS DB 1개**로 끝난다. CORS·별도 프론트 호스팅 불필요.

```
[사용자 브라우저] ──HTTPS──> [b2d-server (Docker)] ──> [PostGIS]
                              ├ /            랜딩
                              ├ /map.html    지도 데모
                              ├ /batch.html  배치 지오코딩
                              ├ /docs.html   API 문서(try-it-out)
                              ├ /config.json 런타임 설정(데모 키 주입)
                              └ /v1/*        REST API (인증·rate limit·계측)
```

## 1. 이미지 빌드

이미 있는 `Dockerfile`이 `web/`까지 포함해 빌드한다(`COPY . .` + `go:embed`).

```bash
docker build -t b2d-geo:latest .
```

멀티스테이지(golang:1.26.5-alpine → distroless/static:nonroot), 정적 바이너리
`b2d-server`·`b2d-etl` 내장, migrations는 `go:embed`.

## 2. 환경변수

| 변수 | 필수 | 설명 |
|---|---|---|
| `B2D_DATABASE_URL` | ✅ | `postgres://user:pw@host:5432/b2d_geo` |
| `B2D_LISTEN` | | 리슨 주소(기본 `:8080`) |
| `MOLIT_API_KEY` | | 토지 실거래(`/v1/appraisal/sales`) 조회용(data.go.kr) |
| `B2D_DEMO_API_KEY` | | 데모 키. 설정 시 웹의 지도/문서 try-it-out이 이 키를 자동 사용. **저장소 아닌 env로만.** |
| `B2D_SELF_SERVE_DEMO` | | `true`면 `POST /issue-demo-key`(geo scope·낮은 rate) 노출. **데모/미리보기 환경에서만 켤 것**(공개 키 발급은 DB에 행을 쌓음). |

데모 키는 먼저 발급해서 env에 넣는다:

```bash
docker run --rm --entrypoint /b2d-etl -e B2D_DATABASE_URL=... b2d-geo:latest \
  apikey create --name demo-frontend --scopes geo,realestate --rate 5
# 출력된 PLAINTEXT KEY를 B2D_DEMO_API_KEY로 설정
```

## 3. DB 프로비저닝 — 핵심 결정

전체 DB는 **약 61GB**(land_use_plan 31G · parcels 21G · buildings 6G · places 1.7G …).
관리형 Postgres로 이 용량을 올리면 비용이 크다. 두 갈래:

### (a) 전체 배포 (프로덕션/온프레미스)
`scripts/snapshot.sh`(전체 pg_dump custom) → `scripts/restore.sh`. 대용량 스토리지가
있는 관리형 Postgres 또는 자체 서버. 요금·근거는 `08_Deployment_Pricing_b2d_geo.md`.

### (b) 데모 축소 배포 (저비용 미리보기) — 권장 시작점
`scripts/demo-snapshot.sh`가 **지정 시군구만** 담아 작은 덤프를 만든다(기본 하남·성남).
저렴한 관리형 Postgres(수 GB급)나 단일 VPS로 충분하다.

```bash
# 원본(로컬 컨테이너)에서 서브셋 추출 — 시군구는 5자리 코드, 공백 구분
B2D_DEMO_SIGUNGU="41450 41135" ./scripts/demo-snapshot.sh demo_snapshot

# 대상(배포) 빈 DB에 복원 — schema.sql이 postgis 확장 포함
B2D_TARGET_URL="postgres://user:pw@db-host:5432/b2d_geo" \
  ./scripts/demo-restore.sh demo_snapshot
```

데모는 담긴 시군구 안에서만 필지·감정평가가 뜨고, 그 밖 지역은 "미적재"로 응답한다
(API가 부재 지역을 안전 처리 — `hasRegion` PK-range 체크).

## 4. 실행

```bash
docker run -d --name b2d -p 8080:8080 \
  -e B2D_DATABASE_URL="postgres://user:pw@db-host:5432/b2d_geo" \
  -e MOLIT_API_KEY="..." \
  -e B2D_DEMO_API_KEY="b2d_..." \
  -e B2D_SELF_SERVE_DEMO="true" \
  b2d-geo:latest
```

`http://<host>:8080/` → 랜딩. `/v1/health`로 상태 확인. HTTPS는 앞단 리버스 프록시
(Caddy/nginx) 또는 PaaS 제공.

## 5. 호스팅 옵션(참고)

| 옵션 | 앱 | DB | 비고 |
|---|---|---|---|
| 단일 VPS | Docker | 같은 호스트 PostGIS | 가장 저렴. 데모 서브셋에 적합. Caddy로 TLS 자동. |
| PaaS(Fly/Render/Railway) | 컨테이너 배포 | 관리형 Postgres+PostGIS | 간편. 관리형 Postgres의 PostGIS 지원·용량 확인 필요. |
| 자체 서버(온프레미스) | Docker/바이너리 | 자체 PostGIS | 전체 데이터·데이터 residency. `08` 라이선스 참고. |

> **실제 프로비저닝(클라우드 계정·결제·도메인)은 운영자 몫이다.** 이 가이드는 그
> 준비물(이미지·env·스냅샷 스크립트)까지를 담는다.
