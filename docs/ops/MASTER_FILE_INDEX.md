# 핵심 파일 색인 (MASTER_FILE_INDEX)

코드·데이터·설정 실체 파일의 위치와 역할. 근거는 2026-07-12 repo 실스캔
(각 패키지 헤더 주석에서 한 줄 추출). 문서형 지식은 `PROJECT_KNOWLEDGE_INDEX.md`.

## 실행 진입점 (`cmd/`)

| 경로 | 역할 |
|------|------|
| `cmd/b2d-etl/main.go` | 공공데이터 다운로드/적재/인덱싱 CLI. 서브커맨드(juso/sbiz/… , apikey create) |
| `cmd/b2d-server/main.go` | 위치 서비스 API 서버 (부동산 특화 최소 구성) |

## 내부 패키지 (`internal/`) — doc.go / 헤더 주석 발췌

| 패키지 | 역할 |
|--------|------|
| `internal/address` | 한국 주소 파서·정규화 (핵심 자산). `parser.go` 상태기계, `jamo.go` 자모/초성, `dict.go` 사전 |
| `internal/geocode` | 주소 → 좌표 변환 (3단 폴백 매칭) |
| `internal/revgeo` | 좌표 → 주소/행정구역 변환 |
| `internal/places` | 상호/업종/반경 검색 |
| `internal/parcel` | PNU/지적/필지 조회 (부동산 특화) |
| `internal/api` | HTTP 핸들러·미들웨어·JSON 스키마 (`server.go`) |
| `internal/auth` | API key 발급/검증 (`apikey.go`), rate limit (`ratelimit.go`) |
| `internal/meter` | 사용량 계측 (과금 근거) |
| `internal/store` | pgx 기반 DB 접근 레이어 + 마이그레이션 러너 (`migrate.go`) |
| `internal/etl/juso` | 도로명주소 한글 DB 파서 (EUC-KR/MS949 디코딩) |
| `internal/etl/entrance` | 출입구 전체분 (txt, CP949, 19컬럼) 로더 |
| `internal/etl/jijeok` | V-World 연속지적도(시도별 shapefile zip) → parcels 로더 |
| `internal/etl/sbiz` | 소상공인 상가(상권)정보 CSV 파서 (`choseong.go` 초성) |
| `internal/etl/nsdi` | 지적도/행정경계 shapefile 로더 |
| `internal/etl/landsales` | 국토부 토지매매 실거래(RTMSDataSvcLandTrade) 로더 |

## 마이그레이션 (`migrations/`) — 번호 비연속(003–006, 009 파일 없음)

| 파일 | 내용 |
|------|------|
| `001_init.sql` | 스키마 초안 (02_UML ERD 기반, 6테이블 + GiST/trgm 인덱스) |
| `002_buildings_juso_key.sql` | buildings 자연키 확정 (근거: dataspec/juso.md §2.1) |
| `007_appraisal.sql` | 감정평가 사실조회 스키마 (Phase 7-0) + scopes |
| `008_buildings_norm_idx.sql` | geocode v0 정확 매칭용 인덱스 |
| `010_places_choseong.sql` | 초성 검색 컬럼 + trigram 인덱스 (Phase 3) |
| `migrations.go` | SQL 파일을 embed로 내장하는 러너 |

## API 계약

| 경로 | 내용 |
|------|------|
| `openapi.yaml` | 9 엔드포인트: health / geocode / reverse / places/search / parcel(by-point·by-jibun·{pnu}) / appraisal(sales·price-index). scope: geo, realestate |

## 주요 데이터 디렉터리 (du -sh, 원본 — git 미추적)

| 경로 | 크기 | 내용 (대응 명세) |
|------|------|------------------|
| `data/` (합계) | 9.3G | 공공데이터 원본 루트 |
| `data/연속지적도` | 6.1G | V-World 연속지적도 shapefile (→ dataspec/jijeok.md) |
| `data/juso_shape` | 1.4G | 도로명주소 건물 도형 (→ dataspec/juso_shape.md) |
| `data/rdd2022` | 1.7G | 도로 손상 탐지(detection) 학습 원본 |
| `data/juso_entrance` | 155M | 도로명주소 출입구 (→ dataspec/juso_entrance.md) |
| `data/ADSClient_DLL.zip` | 3.5M | juso.go.kr 연계 다운로드 클라이언트 (DLL zip) |
| `detection/` | 32G | 교량/도로 손상 탐지 파이프라인 (coco/rdd/runs/deimv2/labels/images/paper) |
| 루트 `202605_도로명주소 한글_전체분.zip` | 88M | 도로명주소 한글 전체분 원본 zip |
| 루트 `appraisal.zip` | 34K | 감정평가 모듈 자료 |

## 환경 변수 (`.env`) — 키 이름만 (값 비공개)

| 키 | 용도(이름 기준) |
|------|------|
| `B2D_JUSO_API_KEY` | juso.go.kr 연계 API 키 |
| `MOLIT_API_KEY` | 국토부(MOLIT) API 키 |
| `MOLIT_LANDTRADE_ENDPOINT` | 국토부 토지매매 실거래 엔드포인트 |
| `REB_API_KEY` | 한국부동산원(REB) API 키 |

## 빌드·운영 (`Makefile`)

`make up`(PostGIS, 호스트 포트 5433) / `make migrate` / `make build`(bin/b2d-server, bin/b2d-etl) / `make test` / `make psql`.
