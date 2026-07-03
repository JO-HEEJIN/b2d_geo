# b2d_geo 구축 계획 (Phase 0~7)

지시서: b2d_geo_claude_code_prompt.md + b2d_geo_phase7_appraisal_prompt.md (증보).
설계 문서: docs/ 7종 (SDP, UML, SRS, BRD, TestCases, UserProcessFlow, Appraisal_Datasets — 전부 v1.1 증보 반영).
각 Phase는 생성 단계와 Verification phase를 분리하여 수행한다.

## 선행 조건 (Phase 0 착수 전 해소 필요)

- [x] P-1. Git 저장소 분리: `git init -b main`, author `JO-HEEJIN <midmost44@gmail.com>`
      로컬 설정 완료 -> 검증: rev-parse가 b2d_geo 루트 반환
- [x] P-2. Go 설치 완료 (go1.26.4 darwin/arm64) -> 검증: `go version`
- [x] P-3. colima + docker CLI 설치 및 기동 완료 -> 검증: `docker compose version` 5.3.0.
      참고: 호스트 5432는 Homebrew postgresql@17이 점유 중이라 컨테이너는 5433 사용

## Phase 0 — 골격 (0.5일)

- [x] 0-1. go mod init (github.com/JO-HEEJIN/b2d_geo), 디렉터리 구조 생성
      -> 검증: `go build ./...`, `go vet ./...` 통과. 직접 의존성 pgx/v5 1개
- [x] 0-2. docker-compose.yml (postgis/postgis:16-3.4, 호스트 포트 5433), Makefile
      -> 검증: psql `SELECT postgis_version()` = 3.4
- [x] 0-3. migrations/001_init.sql (ERD 6테이블 + GiST/trgm 인덱스) + embed 러너
      -> 검증: `\dt` 테이블 확인, 재실행 멱등성 확인 (schema_migrations 1행 유지)
- [x] 0-4. `.claude/agents/` b2d-data, b2d-engine, b2d-api, b2d-qa 정의
- [x] 0-5. `npx gitnexus analyze` 초기 인덱싱, caveman 확인
- [x] 0-6. GitHub Actions: go vet, go test, staticcheck
- [x] 0-7. .gitignore (.env, 데이터 파일, bin) -> 검증: 시크릿 커밋 없음
- [x] 0-8. learning_log.md Phase 0 기록

## Phase 1 — ETL (b2d-data 주도)

- [>] 1-1. (거의 완료) juso.go.kr 파일 명세서 실물 확보 -> 컬럼 정의를 코드 상수로 문서화,
      원본 좌표계(EPSG) 확정 (docs 미검증 항목 SDP R-04, TC-I03 해소)
      - 공개분(한글DB/건물DB/주소DB) 레이아웃 verbatim 확보: docs/dataspec/juso.md
      - 위치정보요약DB/내비게이션용DB 레이아웃+EPSG 확정은 연계신청 승인 대기
        (2026-07-03 사용자 신청 진행, 승인기관 행안부). ITRF2000/GRS80까지만 확인
- [>] 1-2. (in progress, juso-spec) 도로명주소 한글 DB 다운로드/파싱 (MS949 -> UTF-8,
      golang.org/x/text 사용, `|` 구분자) -> 검증: 샘플 행 파싱 결과 육안 대조.
      위치정보 DB 부분은 승인 후 진행
- [x] 1-3. 상가(상권)정보 CSV(15083033) 적재 완료
      -> 명세: docs/dataspec/sbiz.md (UTF-8/쉼표/39컬럼, 좌표 4326 실측 확정).
      전국 2,725,319행 (17개 시도, skip 0, 소분류 247종 = 명세 일치), 멱등 확인
- [ ] 1-4. 좌표계 통일: ST_Transform으로 4326 컬럼 생성
- [ ] 1-5. GiST 인덱스, pg_trgm 인덱스 생성
- [ ] 1-6. Verification phase: 실제 알고 있는 주소 5건(하남시, 서울 주요 지점)을
      SQL 직접 조회, 좌표가 실위치와 일치하는지 확인. 카운트만 확인 금지
- [ ] 1-7. learning_log.md Phase 1 기록, gitnexus 재인덱싱

## Phase 2 — Geocoding / Reverse (b2d-engine 주도)

- [ ] 2-1. 한국 주소 파서 상태기계: 시도/시군구/읍면동/도로명/건물번호/지번
      (구주소, 신주소, 1동/일동 변형, 산 지번, 괄호 병기) -> 검증: 케이스별 단위 테스트
- [ ] 2-2. 자모 분해/정규화 유틸 (internal/address/jamo.go, normalize.go)
- [ ] 2-3. 3단 폴백 매칭(정확 -> 정규화 -> trigram) + score 산정
- [ ] 2-4. reverse: 법정동 폴리곤 ST_Contains + 건물 KNN (`<->`)
- [ ] 2-5. Verification phase: 실주소 200건 이상 테스트 셋으로 정확도 측정,
      실패 케이스와 원인을 learning_log에 기록 (docs TestCases 기대값 실측 기입)
- [ ] 2-6. learning_log.md Phase 2 기록, gitnexus 재인덱싱

## Phase 3 — Places / Keyword (b2d-engine)

- [ ] 3-1. 자모 분해 정규화 기반 trgm 검색, 반경/카테고리 필터
- [ ] 3-2. Verification phase: 실존 상호 20건 검색 재현율 확인
- [ ] 3-3. learning_log.md Phase 3 기록, gitnexus 재인덱싱

## Phase 4 — API 서버 (b2d-api)

- [ ] 4-1. 6절 사양 전체: /v1/geocode(+batch 10,000건), /v1/reverse,
      /v1/places/search, /v1/places/category, /v1/parcel 3종, /v1/health, /v1/usage
- [ ] 4-2. 미들웨어 체인: API key 검증(SHA-256 저장) -> rate limit(token bucket,
      key별+IP별) -> 사용량 계측(채널 + 배치 insert, 논블로킹)
- [ ] 4-3. API key 발급 CLI
- [ ] 4-4. OpenAPI 3.1 스펙 파일
- [ ] 4-5. Verification phase: curl 전 엔드포인트 실호출 + 부하 테스트, p95 latency 기록
- [ ] 4-6. 보안 체크리스트(10절) 1차 점검 (b2d-qa)
- [ ] 4-7. learning_log.md Phase 4 기록, gitnexus 재인덱싱

## Phase 5 — 부동산 특화 (b2d-data + b2d-engine)

- [ ] 5-1. 연속지적도 shapefile 적재 (nsdi/V-World), PNU 매핑
- [ ] 5-2. /v1/parcel 3종 엔드포인트 데이터 연결
- [ ] 5-3. 초성 검색 ("ㅇㄱ" -> "약국") — FR-43 TC 추가 (docs 남은 이슈 해소)
- [ ] 5-4. Verification phase: 실제 필지 3건 PNU를 정부 서비스와 대조
- [ ] 5-5. learning_log.md Phase 5 기록, gitnexus 재인덱싱

## Phase 6 — 상용화 준비 (전 팀)

- [ ] 6-1. b2d-qa 보안 감사: SQL 바인딩 전수, key 해시, rate limit 우회,
      에러 메시지 내부정보 누출, govulncheck CI 통합
- [ ] 6-2. 카카오 로컬 API 대비 정확도 샘플 비교 (수동 100건) -> 판매 근거 수치
- [ ] 6-3. 배포 문서, 요금제 설계 초안 (벌크/온프레미스/부동산 필드 3축,
      docs BRD 공란 확정)
- [ ] 6-4. learning_log.md Phase 6 기록, gitnexus 재인덱싱

## Phase 7 — 감정평가 모듈 (지시서: b2d_geo_phase7_appraisal_prompt.md, Phase 0~6 완료 후 착수)

절대 경계: 사실 반환만. 격차율/보정/평가액/추천점수/적정성 판단 금지.
정렬은 객관 지표(거리/날짜/금액)만. V-World 실시간 API 금지 (벌크 스냅샷만).
실거래가는 rt.molit.go.kr 공식 CSV/오픈API만 (중개 플랫폼 스크레이핑 금지).

- [ ] 7-0. migrations/007_appraisal.sql (6테이블: standard_lots, official_land_prices
      연도 파티셔닝, land_features, use_zones, land_sales, land_price_index)
      + api_keys.scopes text[] + GiST 인덱스 -> 검증: 마이그레이션 왕복 성공,
      gitnexus 재인덱싱 (b2d-data, 0.5주)
- [ ] 7-1. ETL: 표준지공시지가(전국) + 지가변동률(R-ONE, 월) + 실거래가(월 배치,
      직전 3개월 upsert, 좌표 불가 행은 EMD_CENTROID 등급) -> 검증: 표준지 3필지를
      realtyprice.kr과 대조, 지가변동률 1개 시군구 3개월치 R-ONE 대조, 실거래 5건
      rt.molit.go.kr 대조 (b2d-data, 1주). 착수 전 실물 명세 확보 원칙 동일 적용
- [ ] 7-2. ETL: 용도지역 폴리곤(UPIS, ST_MakeValid 건수 기록) + 토지특성 +
      개별공시지가(--region 선택 1곳) -> 검증: 실필지 3건 용도지역 eum.go.kr 대조
      (b2d-data, 1주)
- [ ] 7-3. API 6종 (/v1/appraisal/{standard-lots,sales,land-use,land-features,
      price-index,official-price}) + RealEstate scope 검증 + OpenAPI 증보
      -> 검증: 전 엔드포인트 실호출, TC-V 전건 통과 (b2d-engine 쿼리 + b2d-api 핸들러, 1주)
- [ ] 7-4. 실무 시나리오 검증(대상 필지 1건 전 구간 정부 서비스 교차 대조) +
      판단 로직 부재 감사 (score/weight/adjust 금지 패턴 grep) + 보안 감사
      (b2d-qa, 0.5주)
- [ ] 7-5. learning_log.md Phase 7 기록, gitnexus 재인덱싱

## Review

(전체 완료 후 작성: 변경 요약 + 남은 이슈)
