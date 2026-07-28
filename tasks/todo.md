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
      - 연계신청 **승인 완료 (2026-07-07 확인)**: 도로명주소 건물 도형 + 출입구
        정보 2건 (신청 2026-07-03, Birth2Death LLC). 다운로드는 juso.go.kr
        로그인 필요 — 사용자 브라우저 작업. 파일 확보 후 레이아웃/EPSG 실측
        확정 진행 (ITRF2000/GRS80까지만 기확인)
      - **건물 도형 17개 시도 확보 + 좌표계 실측 확정: EPSG:5179** (2026-07-07,
        docs/dataspec/juso_shape.md — bbox 실측 판별, .prj 미제공). 출입구
        레이어(TL_SPBD_ENTRC)가 zip에 포함돼 있음
      - **출입구 정보 17개 시도 확보** (docs/dataspec/juso_entrance.md):
        텍스트 DB, 19컬럼/CP949/EPSG:5179, 주소+좌표 결합 — 지오코딩 주력
        원천. 서울 524,522행 전수 검증. 컬럼 14/15/17 의미는 공식 레이아웃
        대조 후 확정 (ETL 착수 전 필수)
      - 주소정보 제공 API 승인키 확보 (.env JUSO_API_KEY, 커밋 금지).
        엔드포인트 update.juso.go.kr/updateInfo.do 실존 확인 (HTTP 200).
        다음: 마이페이지 승인내역에서 자료구분코드(cntc_cd) 확인 → 품목별
        자동 다운로드 (도형 계열은 date_gb=M). 구역의 도형은 별도 품목 —
        월별 목록 '데이터 없음'은 변동분 리스트라서였음, 전체분은 승인 제공
- [>] 1-2. (in progress, juso-spec) 도로명주소 한글 DB 다운로드/파싱 (MS949 -> UTF-8,
      golang.org/x/text 사용, `|` 구분자) -> 검증: 샘플 행 파싱 결과 육안 대조.
      위치정보 DB 부분은 승인 후 진행
- [x] 1-3. 상가(상권)정보 CSV(15083033) 적재 완료
      -> 명세: docs/dataspec/sbiz.md (UTF-8/쉼표/39컬럼, 좌표 4326 실측 확정).
      전국 2,725,319행 (17개 시도, skip 0, 소분류 247종 = 명세 일치), 멱등 확인
- [x] 1-4. 좌표계 통일 완료 — 전 적재 경로가 ST_Transform으로 4326 저장
      (jijeok 5186→4326, entrance 5179→4326, sbiz 4326 원생)
- [x] 1-5. 인덱스 완료 — GiST(parcels/buildings/places geom), trgm(norm_road/
      norm_jibun/norm_name), btree(norm_road, bcode+jibun) 실존 확인
- [x] 1-6. Verification 완료 (2026-07-09): 서울 4건(시청·서울역·국회·경복궁)
      + 하남시청(대청로 10) geocode 실호출 — 5건 전부 좌표 실위치 일치.
      reverse 왕복(하남 신장동)까지 확인
- [x] 1-7. 완료 확인 (2026-07-18): learning_log 372행 Phase 1-6 기록 실존,
      재인덱싱 최신

## Phase 2 — Geocoding / Reverse (b2d-engine 주도)

- [x] 2-1. 주소 파서 상태기계 구현 (internal/address, 설계: docs/design/
      address_parser.md) — 단위테스트 15종 통과. geocode 연결은 후속
- [x] 2-2. 자모/초성 유틸 완료. 정규화는 파서 canonSido+RoadKey로 흡수
- [x] 2-3. 3단 폴백 완료 (2026-07-12): exact/exact_partial(시도생략 후보검출)/
      jibun(admin_areas→parcels 경로)/trigram(set_limit 0.55 — 112s→4s 실측).
      score=similarity 원값 노출
- [ ] 2-4. reverse: 법정동 폴리곤 ST_Contains + 건물 KNN (`<->`)
- [x] 2-5. 정확도 측정 완료: 실주소 200건×4변형 = 800질의 **100.0%**
      (초기 14% → 읍면 RoadKey·붙임토큰 결함 수정 후). learning_log 기록
- [x] 2-6. 완료 확인 (2026-07-18): learning_log 379행~ Phase 2·3·4 기록 실존

## Phase 3 — Places / Keyword (b2d-engine)

- [>] 3-1. 초성 검색 완료 (choseong 컬럼 272만 백필 + trgm, 'ㅇㄴㄹㅇㄱ'→온누리약국
      0.2s). 일반 검색 3단 UNION으로 10s→0.16s. 반경/카테고리 필터 잔여
- [ ] 3-2. Verification phase: 실존 상호 20건 검색 재현율 확인
- [x] 3-3. 완료 확인 (2026-07-18): 위와 동일 섹션에 포함

## Phase 4 — API 서버 (b2d-api)

- [>] 4-1. 9종 가동 (geocode/reverse/places/parcel×3/appraisal×2/health).
      잔여: geocode batch, places/category, /v1/usage
- [x] 4-2. 미들웨어 체인 완료 (2026-07-12): auth(SHA-256+scope)→ratelimit(token
      bucket key+IP)→meter(채널+CopyFrom 배치). 실검증 401/403/429/계측 5종 통과.
      알려진 이슈(Phase 6 감사 대상): auth 캐시 30s revoke 지연, 무인증 요청
      IP-rate 미적용(DB 조회 1회씩), 버킷 맵 무한 성장, meter kill 시 flush 유실
- [x] 4-3. `b2d-etl apikey create` 완료 (평문 1회 출력, 해시 저장 검증)
- [x] 4-4. openapi.yaml 완료 (9 엔드포인트 + 인증 스킴, 실응답 구조 반영)
- [x] 4-5. Verification phase 완료 (2026-07-18): 9/9 엔드포인트 200 (시청
      필지 geocode↔reverse↔parcel 삼각 일치), 인증 음성 401×2. 부하:
      IP리미터(20rps 하드코딩)가 동시 20에서 정확 발동(429) — p95는
      한도 내 유속(14rps, n=150/엔드포인트)으로 측정: geocode-road 11ms /
      jibun 26 / reverse 67 (p99 399 이상치) / places 30 / 초성 56 /
      parcel-point 11 / pnu 7. 발견: ① 로그 redact 함수 문자 단위 ***
      도배 (Phase 6 감사 추가) ② appraisal 2종 .env 키 필수 (미설정 502)
- [x] 4-6. 완료 (2026-07-18, b2d-qa): SQL 바인딩 전수 OK($N, 조립 0건) /
      키 SHA-256 해시저장 OK / XFF 스푸핑 우회 불가(RemoteAddr만) / 에러응답
      제네릭 OK / .env·커밋이력 시크릿 0건. 결함 5건 → 6-1 수정 대상:
      ① redact 빈키 문자도배 server.go:277 (트리거 정정: .env 미소싱 기동,
      실파일은 키 존재) [MED] ② 버킷맵 무한성장, evict 없음 [MED]
      ③ auth 캐시 30s revoke 지연 (설계) [LOW-MED] ④ meter graceful
      shutdown 부재, kill 시 flush 유실 [LOW] ⑤ govulncheck GO-2026-5856
      crypto/tls — go 1.26.5 범프 필요 [MED]. MED-1(무인증 IP-rate)은
      해소 확인
- [x] 4-7. 완료 (2026-07-18): learning_log Phase 4 완결 기록, gitnexus
      재인덱싱 (1,263노드/2,086엣지/63플로우)

## Phase 5 — 부동산 특화 (b2d-data + b2d-engine)

- [>] 5-1. 연속지적도 shapefile 적재 (V-World; NSDI는 2024-01 서비스 종료), PNU 매핑
      - 데이터 확보 완료 (2026-07-07): 17개 시도 전량 12GB, data/연속지적도/
      - 명세 실측 완료: docs/dataspec/jijeok.md — EPSG:5186 확정(.prj),
        DBF는 CP949 (.cpg의 UTF-8 표기는 거짓), PNU=A1 19자리, 서울 898,719필지
- [x] 5-2. 완료 확인 (2026-07-18): 3종 모두 실데이터 반환 검증 (4-5에서
      by-point/by-jibun/{pnu} 삼각 일치, PNU 1114010300100310000). 단 8개 도는
      5-1 잘림(1M) 범위 내에서만 조회 가능 — 5-1 완료 시 자동 확장
- [x] 5-3. 완료 (2026-07-18): TC-E13 신설 (05_TestCases + 추적 매트릭스),
      docs/todo.md 남은 이슈 해소. 실측 근거: ㅇㄴㄹㅇㄱ→온누리약국 200, p95 56ms
- [ ] 5-4. Verification phase: 실제 필지 3건 PNU를 정부 서비스와 대조
- [ ] 5-5. learning_log.md Phase 5 기록, gitnexus 재인덱싱

## Phase 6 — 상용화 준비 (전 팀)

- [x] 6-1. 완료 (2026-07-19): 감사(qa-46) → 수정(fix-61) → 재검증(qa-46)
      → 커밋 f94513c. SQL 바인딩·key 해시·rate limit 우회·에러 누출 전수
      OK, redact 가드·버킷 reaper·go1.26.5·meter graceful shutdown 반영,
      govulncheck CI 통합 (No vulnerabilities). 잔존→해소(6-3, 2026-07-27):
      auth 캐시 revoke 지연 = B2D_AUTH_CACHE_TTL env로 설정화(기본 30s, 온프레미스 0s)
- [x] 6-2. 완료 (2026-07-27): 카카오 vs b2d 정확도 100건 — 양쪽 매치 100%,
      b2d 0m(공식좌표), 카카오 중앙값 6.2m(기준점 차이, 카카오 오류 아님).
      하니스 scripts/kakao_compare.sh, 증거 general_100.csv/newgu_40.csv.
      이상치 수동검증(필지+건물 2레이어) + 신설-구(화성 2026-02-01 개편) 서브검증.
- [x] 6-3. 완료 (2026-07-27): docs/08 배포 3형태 + 3축 요금 확정
      (Bulk 30/20/10원·월최소30만, On-Prem 연3천만+갱신800만, RE 30/100/협의)
      + BRD §5.1 반영 + auth revoke 결정(B2D_AUTH_CACHE_TTL).
- [x] 6-4. 완료 (2026-07-27): learning_log Phase 6-2 기록 + todo 체크 반영
      + gitnexus 재인덱싱 (1,296 nodes / 2,136 edges / 65 flows).

## Phase 7 — 감정평가 모듈 (지시서: b2d_geo_phase7_appraisal_prompt.md, Phase 0~6 완료 후 착수)

절대 경계: 사실 반환만. 격차율/보정/평가액/추천점수/적정성 판단 금지.
정렬은 객관 지표(거리/날짜/금액)만. V-World 실시간 API 금지 (벌크 스냅샷만).
실거래가는 rt.molit.go.kr 공식 CSV/오픈API만 (중개 플랫폼 스크레이핑 금지).

- [x] 7-0. 완료 (실측 2026-07-27): 007_appraisal.sql 6테이블 존재, official_land_prices
      LIST(base_year) 파티셔닝(+default), api_keys.scopes=text[], GiST 인덱스
      (standard_lots/use_zones/land_sales geom) 확인.
- [>] 7-키. API 키 선확보 완료 (2026-07-07, .env 저장·커밋 금지):
      MOLIT_API_KEY (토지매매 실거래 RTMSDataSvcLandTrade — 검증 완료: 강남구
      202605 실데이터 수신. 주의: curl 기본 UA는 WAF 차단, Mozilla UA 필수),
      REB_API_KEY (R-ONE — 실호출 검증 완료, 월 지역별 지가변동률
      STATBL_ID=A_2024_00903, 202605 실데이터 확인). JUSO_API_KEY는 Phase 1 참조
- [>] 7-1. ETL 3종 중 2종 완료 (2026-07-27~28):
      · 실거래가 = land_sales 로더(land-sales 서브커맨드, EMD_CENTROID) ✓
      · 지가변동률 = priceindex 로더(price-index 서브커맨드): R-ONE 용도지역별 월 JSON →
        land_price_index 18,651행(255시군구×10용도지역×10월 202509~202606). 검증:
        과천 녹지 202604/05/06 = 0.363/0.554/0.731 R-ONE 일치 ✓. admin_areas 크로스워크(자치구 포함).
      · 남음: 표준지공시지가(전국) — 실물 파일 대기.
- [ ] 7-2. ETL: 용도지역 폴리곤(UPIS, ST_MakeValid 건수 기록) + 토지특성 +
      개별공시지가(--region 선택 1곳) -> 검증: 실필지 3건 용도지역 eum.go.kr 대조
      (b2d-data, 1주)
- [>] 7-3. API 6종 중 2종 (2026-07-28): sales(라이브 프록시)·price-index(DB-backed:
      land_price_index, sigungu×use_zone×from/to, cumulative=(1+r/100)곱+산식+기간) +
      RealEstate scope 검증(geo키 403 확인). 남음: standard-lots/land-use/land-features/
      official-price(ETL 데이터 대기) + OpenAPI 증보 + TC-V.
- [ ] 7-4. 실무 시나리오 검증(대상 필지 1건 전 구간 정부 서비스 교차 대조) +
      판단 로직 부재 감사 (score/weight/adjust 금지 패턴 grep) + 보안 감사
      (b2d-qa, 0.5주)
- [ ] 7-5. learning_log.md Phase 7 기록, gitnexus 재인덱싱

## Review

(전체 완료 후 작성: 변경 요약 + 남은 이슈)
