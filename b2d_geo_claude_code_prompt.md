# b2d_geo 구축 지시서 — Claude Code (Opus 4.8) 실행 프롬프트

이 문서 전체를 Claude Code 세션의 첫 메시지로 붙여넣는다.
저장소 루트에서 실행하며, 이 문서가 프로젝트의 최상위 지시서다.

---

## 0. 프로젝트 정의

**b2d_geo**: 카카오/네이버 API에 의존하지 않는 자체 위치 서비스 엔진.
공공데이터만으로 geocoding, reverse geocoding, keyword search, places 기능을 제공하고,
부동산 도메인 특화 기능(PNU, 지적, 필지 조회)을 차별점으로 갖는 상용 API 서버.

- 언어: Go 1.23+
- 저장소명: `b2d_geo`
- 최종 산출물: 단일 바이너리 API 서버 + ETL 파이프라인 + Docker 배포 구성

---

## 1. 절대 규칙 (위반 금지)

### 1.1 데이터 출처
- 네이버, 카카오, 구글 등 상용 서비스의 스크레이핑 데이터는 **어떤 형태로도 사용 금지**.
  크롤러 코드 작성도 금지. 이 프로젝트는 상업 판매용이므로 데이터베이스권 침해 리스크를
  원천 차단한다.
- 로드뷰/거리뷰 이미지 사용 금지 (카카오/네이버 자산).
- 허용 데이터 소스는 아래 3절에 명시된 공공데이터만.

### 1.2 Git 규칙
- 커밋 author: `JO-HEEJIN <midmost44@gmail.com>` 단독.
- "Generated with Claude Code", "Co-Authored-By: Claude" 등 AI 표기 절대 금지.
- 커밋 메시지는 Conventional Commits, 자연스러운 사람의 문장으로.
- 이모지 금지 (커밋, 코드 주석, 문서 전부).

### 1.3 생성/검증 분리
- 모든 작업은 생성(generation)과 검증(verification)을 별도 단계로 수행한다.
- 코드 생성 직후 "완료" 선언 금지. 반드시 `Verification phase:`를 선언하고
  실제 실행 결과(테스트 통과, 실 데이터 쿼리 결과)로 검증한다.
- HTTP 200, 파일 존재 여부 같은 프록시 신호로 "검증됨" 선언 금지.
- 데이터 다운로드 URL은 실제로 접속해 파일 헤더/샘플 행을 확인한 뒤에만 코드에 반영.

### 1.4 caveman 운용 규칙
- 에이전트 간 통신, 서브에이전트 결과 보고, 커밋 메시지: caveman 모드 사용 (토큰 절약).
- **learning_log.md 작성 시에는 caveman 해제.** 이 파일은 사용자의 학습 자료이므로
  완전한 문장, 배경 설명, 의사결정 이유를 생략 없이 기록한다.
- 사용자에게 직접 하는 phase 완료 보고도 일반 모드.

### 1.5 GitNexus 운용 규칙
- Phase 0에서 `npx gitnexus analyze` 실행으로 초기 인덱싱.
- 각 Phase 완료 시마다 `gitnexus analyze`로 재인덱싱하여 그래프를 HEAD와 동기화.
- 3개 파일 이상을 건드리는 리팩터링 전에는 반드시 gitnexus impact 도구로
  blast radius를 확인하고 결과를 learning_log.md에 기록.

### 1.6 단순성 원칙
- 모든 변경은 가능한 최소 범위로. 거대한 일괄 변경 금지.
- 외부 의존성 최소화: Go 표준 라이브러리 우선. 서드파티 패키지 추가 시
  todo.md에 사유를 한 줄 기록하고 승인 후 추가.
- npm 스타일의 의존성 폭발 금지. supply chain 공격 면적 최소화가 언어 선택 이유다.

---

## 2. 워크플로우

1. 이 지시서를 읽고 `tasks/todo.md`에 Phase별 체크리스트 형태의 계획을 작성한다.
2. **작업 시작 전 사용자에게 계획을 보여주고 승인을 받는다.**
3. 승인 후 Phase 순서대로 진행하며 완료 항목을 체크한다.
4. 각 단계마다 변경 사항의 고수준 요약을 사용자에게 보고한다.
5. 각 Phase 완료 시 learning_log.md에 상세 기록 (7절 형식 준수).
6. 전체 완료 후 todo.md에 Review 섹션 추가 (변경 요약 + 남은 이슈).

---

## 3. 데이터 소스 (허용 목록)

Claude Code는 각 소스의 실제 다운로드 방법을 웹에서 확인한 후 ETL을 작성한다.
URL, 파일 포맷, 인코딩은 변할 수 있으므로 코드 작성 전 실물 파일 샘플을 확인할 것.

### 3.1 주소/좌표 (geocoding, reverse geocoding의 원천)
- **주소기반산업지원서비스 (business.juso.go.kr)**
  - 도로명주소 한글 DB (건물정보): 도로명주소, 지번주소, 건물명, 법정동코드
  - 위치정보 DB (내비게이션용 DB / 위치정보요약 DB): 건물 중심점 좌표
  - 전체분 + 월 변동분 제공. 인코딩 주의 (EUC-KR 가능성 높음), 구분자 `|`.
  - 좌표계: GRS80 기반 EPSG:5179 (Korea 2000 / Unified CS)일 가능성이 높다.
    실제 파일 명세서를 다운로드해 확인 후 확정할 것.

### 3.2 상호/업종 (places, keyword search의 원천)
- **소상공인시장진흥공단 상가(상권)정보** (data.go.kr, 데이터셋 ID 15083033)
  - 전국 영업 중 상가업소: 상가업소번호, 상호명, 업종코드(대/중/소분류),
    지번주소, 도로명주소, 경도, 위도. CSV, UTF-8, 콤마 구분.
  - 분기별 갱신. WGS84 경위도 그대로 제공되므로 변환 불필요 (파일 명세로 재확인).

### 3.3 부동산 특화 (차별화 레이어)
- **국가공간정보포털 (nsdi.go.kr) / V-World**
  - 연속지적도 (필지 폴리곤, PNU 코드)
  - GIS건물통합정보 (건물 폴리곤, 용도, 층수)
- **행정표준코드관리시스템**: 법정동코드 전체 목록
- **법정동/행정동 경계**: reverse geocoding의 행정구역 판정용

### 3.4 보조 (선택)
- OpenStreetMap Korea 추출본 (ODbL 라이선스 준수 조건 확인 후): POI 보강용.
  ODbL의 share-alike 조항이 상용 API와 충돌할 수 있으므로 Phase 6 전에는 사용 보류.

---

## 4. 기술 스택

| 레이어 | 선택 | 사유 |
|---|---|---|
| 언어 | Go 1.23+ | 표준 라이브러리 중심, 공급망 공격 면적 최소, 단일 바이너리 배포 |
| HTTP | `net/http` (1.22+ 라우팅 패턴) | 프레임워크 불필요. stdlib로 충분 |
| DB | PostgreSQL 16 + PostGIS 3.4 | 공간 인덱스(GiST), 좌표 변환(ST_Transform), 성숙도 |
| DB 드라이버 | `jackc/pgx/v5` | 표준 database/sql보다 성능/기능. 유일하게 허용되는 필수 외부 패키지군 |
| 검색 | PostgreSQL `pg_trgm` + 자체 한글 정규화 | 별도 검색엔진(ES 등) 도입 금지. 단순성 우선 |
| 마이그레이션 | 순수 SQL 파일 + `embed` + 자체 러너 | golang-migrate 의존 대신 stdlib |
| 캐시 | 자체 in-memory LRU (`container/list` + map) | Redis는 Phase 6 이후 필요 시 |
| 설정 | 환경변수 + `os.Getenv` | viper 등 금지 |
| 테스트 | 표준 `testing` + 실 PostGIS 컨테이너 | testcontainers-go 허용 (테스트 전용 의존성) |
| 배포 | Docker multi-stage build, distroless | |
| 좌표 변환 | PostGIS `ST_Transform(geom, 4326)` | Go 레벨 proj 바인딩 금지. DB에 위임 |

의존성 상한: 프로덕션 바이너리의 직접 의존성은 5개 이하를 목표로 한다.

---

## 5. 저장소 구조

```
b2d_geo/
  cmd/
    b2d-server/main.go        # API 서버
    b2d-etl/main.go           # ETL CLI (download, load, index 서브커맨드)
  internal/
    address/                   # 한국 주소 파서/정규화 (핵심 자산)
      parser.go                # 도로명/지번 주소 파싱 상태기계
      normalize.go             # 자모 분해, 공백/약어 정규화
      jamo.go                  # 한글 자모 분해 유틸
    geocode/                   # 주소 -> 좌표
    revgeo/                    # 좌표 -> 주소/행정구역
    places/                    # 상호/업종/반경 검색
    parcel/                    # PNU/지적 조회 (부동산 특화)
    etl/
      juso/                    # 주소 DB 파서 (EUC-KR 디코딩 포함)
      sbiz/                    # 상가정보 CSV 파서
      nsdi/                    # 지적도/경계 shapefile 로더
    api/                       # 핸들러, 미들웨어, JSON 스키마
    auth/                      # API key 발급/검증
    meter/                     # 사용량 계측 (과금 기반)
    store/                     # pgx 쿼리 레이어
  migrations/                  # 001_init.sql, 002_...
  tasks/todo.md
  learning_log.md
  docker-compose.yml           # postgis + server
  Makefile
  README.md
```

---

## 6. API 사양 (v1)

모든 응답은 JSON. 에러 포맷 통일: `{"error": {"code": "...", "message": "..."}}`.
인증: `Authorization: Bearer <api_key>` 헤더. 키는 SHA-256 해시로 저장.

### 6.1 Geocoding
```
GET /v1/geocode?query=경기도 하남시 미사강변대로 100
-> { "matches": [ { "road_address", "jibun_address", "lat", "lng",
     "building_name", "pnu", "bcode", "score" } ] }
```
- 정확 일치 -> 정규화 일치 -> trigram 유사도 순의 3단 폴백.
- 배치 엔드포인트 필수: `POST /v1/geocode/batch` (최대 10,000건/요청).
  벌크 지오코딩이 핵심 판매 포인트다.

### 6.2 Reverse Geocoding
```
GET /v1/reverse?lat=37.56&lng=127.19
-> 최근접 건물 주소 + 소속 법정동/행정동 + PNU
```
- PostGIS: 법정동 폴리곤 ST_Contains + 건물 포인트 KNN (`<->` 연산자).

### 6.3 Places / Keyword
```
GET /v1/places/search?query=약국&lat=..&lng=..&radius=1000&category=..
GET /v1/places/category?code=..&bbox=..
```
- 상호명 검색: pg_trgm + 자모 분해 인덱스 (초성 검색 "ㅇㄱ" -> "약국"은 Phase 5).

### 6.4 부동산 특화 (차별화)
```
GET /v1/parcel?pnu=4145010100...      # 필지 정보 + 폴리곤(GeoJSON)
GET /v1/parcel/by-address?query=..    # 주소 -> PNU 매핑
GET /v1/parcel/at?lat=..&lng=..       # 좌표 -> 필지
```

### 6.5 운영
```
GET /v1/health
GET /v1/usage        # 해당 API key의 기간별 호출량 (과금 근거)
```
- 미들웨어: API key 검증 -> rate limit (token bucket, key별) -> 사용량 계측 -> 핸들러.
- 계측은 요청 경로 blocking 금지: 채널 + 배치 insert.

---

## 7. 에이전트 팀 구성

`.claude/agents/`에 아래 서브에이전트를 정의하고 팀 단위로 작업을 위임한다.
메인 스레드는 오케스트레이터 역할만 한다 (직접 대량 코드 작성 금지).

| 에이전트 | 담당 | 산출물 |
|---|---|---|
| `b2d-data` | ETL: 다운로드, 파싱(EUC-KR/shapefile), 적재, 인덱스 생성 | internal/etl, migrations |
| `b2d-engine` | 주소 파서, geocode/revgeo/places/parcel 코어 로직 | internal/address, geocode, revgeo, places, parcel |
| `b2d-api` | HTTP 서버, 인증, rate limit, 계측, OpenAPI 문서 | internal/api, auth, meter, cmd/b2d-server |
| `b2d-qa` | 테스트, 벤치마크, 보안 점검, learning_log 감수 | *_test.go, benchmarks/ |

운영 규칙:
- 각 팀 작업 시작 전 todo.md의 해당 항목을 in-progress로 표시.
- 팀 간 인터페이스(함수 시그니처, 테이블 스키마)는 메인 스레드가 먼저 확정 후 위임.
- b2d-qa는 다른 팀 코드를 생성한 세션과 별도로 검증한다 (생성/검증 분리 원칙의
  에이전트 레벨 적용).
- 서브에이전트 보고는 caveman 압축, learning_log 기재 시 일반 문장으로 복원.

---

## 8. Phase 계획

### Phase 0 — 골격 (0.5일)
- go mod init, 디렉터리 구조, docker-compose (postgis), Makefile
- migrations/001_init.sql: 스키마 초안
- `npx gitnexus analyze` 초기 인덱싱, caveman 활성화 확인
- GitHub Actions: go vet, go test, staticcheck
- Verification phase: `docker compose up` 후 psql로 PostGIS 확장 확인

### Phase 1 — ETL (b2d-data 주도)
- juso 주소 DB + 위치정보 DB 다운로드/파싱/적재
  - 파일 명세서를 먼저 확보해 컬럼 정의를 코드 상수로 문서화
  - EUC-KR -> UTF-8 변환 (golang.org/x/text 허용)
- 상가(상권)정보 CSV 적재
- 좌표계 통일: 원본 좌표계 확인 -> ST_Transform으로 4326 컬럼 생성
- GiST 인덱스, trgm 인덱스 생성
- Verification phase: 실제 알고 있는 주소 5건을 SQL로 직접 조회해
  좌표가 실제 위치와 일치하는지 확인 (하남시, 서울 주요 지점 등).
  카운트만 확인하고 넘어가는 것 금지.

### Phase 2 — Geocoding / Reverse (b2d-engine 주도)
- 한국 주소 파서: 시도/시군구/읍면동/도로명/건물번호/지번 상태기계
  - 케이스: 구주소, 신주소, 동 표기 변형(1동/일동), 산 지번, 괄호 병기
- 3단 폴백 매칭, score 산정
- reverse: KNN + 행정구역 판정
- Verification phase: 최소 200건 실주소 테스트 셋으로 정확도 측정,
  learning_log에 실패 케이스와 원인 기록

### Phase 3 — Places / Keyword (b2d-engine)
- 자모 분해 정규화, trgm 검색, 반경/카테고리 필터
- Verification phase: 실존 상호 20건 검색 재현율 확인

### Phase 4 — API 서버 (b2d-api)
- 6절 사양 전체 구현, API key 발급 CLI, rate limit, 계측
- OpenAPI 3.1 스펙 파일 생성
- Verification phase: curl로 전 엔드포인트 실호출 + 부하 테스트
  (`hey` 또는 자체 Go 벤치, p95 latency 기록)

### Phase 5 — 부동산 특화 (b2d-data + b2d-engine)
- 연속지적도 적재, PNU 엔드포인트, 초성 검색
- Verification phase: 실제 필지 3건 PNU를 정부 서비스와 대조

### Phase 6 — 상용화 준비 (전 팀)
- b2d-qa 보안 감사: 프론트/응답에 민감정보 노출 여부, SQL 파라미터 바인딩 전수 확인,
  API key 해시 저장 확인, rate limit 우회 가능성, 에러 메시지의 내부 정보 누출
- 카카오 로컬 API 대비 정확도 샘플 비교 (수동, 100건) -> 판매 자료의 근거 수치
- 배포 문서, 요금제 설계 초안(벌크/온프레미스/부동산 필드 3축)

---

## 9. learning_log.md 작성 규칙

사용자가 이 파일로 프로젝트 전체를 역으로 학습한다. 형식:

```
## [날짜] Phase N — 작업 제목 (담당 에이전트)

### 무엇을 했는가
### 왜 이렇게 설계했는가 (대안과 기각 사유 포함)
### 막힌 지점과 해결 과정 (에러 메시지 원문 포함)
### 이 작업에서 배울 수 있는 핵심 개념 (Go/PostGIS/GIS 개념을 처음 보는 사람 기준으로 설명)
### 검증 방법과 결과 (실제 수치)
### 관련 커밋 해시
```

- caveman 압축 금지. 완전한 문장.
- "잘 됐다" 같은 결론만 쓰지 말고 과정을 쓴다. 실패한 시도가 성공한 시도보다 중요하다.
- 코드 스니펫은 5~15줄 단위로 인용하고 줄 단위 설명을 붙인다.

---

## 10. 보안 체크리스트 (Phase 4, 6에서 b2d-qa가 전수 점검)

- [ ] 모든 SQL은 pgx 파라미터 바인딩. 문자열 조합 쿼리 0건
- [ ] API key 평문 저장 금지 (SHA-256 + prefix 노출 방식)
- [ ] 시크릿은 환경변수만. 저장소 내 .env 커밋 금지 (.gitignore 확인)
- [ ] 입력 검증: 좌표 범위(한반도 bbox), query 길이 상한, batch 건수 상한
- [ ] 에러 응답에 스택트레이스/쿼리문 노출 금지
- [ ] 의존성 감사: `govulncheck` CI 통합
- [ ] rate limit는 key별 + IP별 이중

---

## 시작 절차

1. 이 지시서를 정독한다.
2. tasks/todo.md에 Phase 0~6 체크리스트를 작성한다.
3. 사용자 승인을 기다린다. 승인 전 코드 작성 금지.
