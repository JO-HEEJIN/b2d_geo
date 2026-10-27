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

- [x] 1-1. 완료: juso.go.kr 파일 명세서 실물 확보 -> 컬럼 정의를 코드 상수로 문서화,
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
- [x] 1-2. 완료: 도로명주소 한글 DB 다운로드/파싱 (MS949 -> UTF-8, golang.org/x/text,
      `|` 구분자) — juso 로더(internal/etl/juso, parse_test 포함) 커밋 38eae64. 위치정보
      (건물도형·출입구)는 entrance 로더로 5179→4326 적재, 지오코딩 실검증은 1-6에서 완료.
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
- [x] 2-4. 완료 (2026-08-01): reverse = 건물 KNN(`<->`, 도로명주소·거리·건물좌표) +
      **포함필지 ST_Contains**(법정동 bcode·명). admin_areas.geom 미적재라 39.7M 필지
      폴리곤 포함관계로 법정동 판정(GiST 5ms), 포함필지 없으면 건물 bcode 폴백. 검증:
      강남/수원(신규)/제주 정확, 종로구 경계 divergence 실증(건물≠필지 bcode).
- [x] 2-5. 정확도 측정 완료: 실주소 200건×4변형 = 800질의 **100.0%**
      (초기 14% → 읍면 RoadKey·붙임토큰 결함 수정 후). learning_log 기록
- [x] 2-6. 완료 확인 (2026-07-18): learning_log 379행~ Phase 2·3·4 기록 실존

## Phase 3 — Places / Keyword (b2d-engine)

- [x] 3-1. 완료 (2026-08-01): 초성 검색(choseong 272만 백필 + trgm, 'ㅇㄴㄹㅇㄱ'→온누리약국
      0.2s), 일반 3단 UNION(10s→0.16s), **반경 필터**(lon/lat/radius, ST_DWithin,
      radius≤0 비활성, places geom GiST) + **카테고리 필터**(4-1에서). E2E 검증: 강남역
      반경500m 약국 최대거리 381m, 미지정 시 전국 섞임.
- [x] 3-2. 완료 (2026-08-02): 실존 상호 20건 이름검색 재현율 = **19/20(95%)**. 전국·다카테고리
      표본. 유일 미스 "색연필"은 정확동명 **30개**라 위치맥락 없이 top10에서 밀린 것(검색결함
      아닌 모호성). 위치필터(춘천 점 반경1km) 추가 시 정확히 1건 회수 → **맥락 있으면 20/20**.
- [x] 3-3. 완료 확인 (2026-07-18): 위와 동일 섹션에 포함

## Phase 4 — API 서버 (b2d-api)

- [x] 4-1. 완료 (2026-08-01): 잔여 3종 마감 — **geocode batch**(POST /v1/geocode/batch,
      최대 50건, 항목별 결과·배치 지속) · **places category 필터**(category_code 정확일치) ·
      **/v1/usage**(자기 API키 엔드포인트별 집계, scope 무관 접근). openapi.yaml 증보(11 paths).
      E2E curl 검증: category 정확필터, batch 유효/무효 항목별, usage geo·realestate 둘 다
      200(scope="" 은 usage 전용, 나머지 격리 403 유지), 무인증 401.
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

- [x] 5-1. 연속지적도 shapefile **전량 적재 완료** (2026-08-01). PNU 매핑, EPSG:5186→4326.
      - 데이터: 17개 시도 12GB, data/연속지적도/ (20260608)
      - 명세: docs/dataspec/jijeok.md — EPSG:5186(.prj), DBF는 CP949, PNU=A1 19자리
      - **잠복 버그 픽스**: V-World가 대용량 시도를 100만 레코드 단위 shapefile 여러 벌로
        분할하는데, extract()가 전부 layer.*로 덮어써 **도당 1벌(1M)만 적재**하던 버그.
        모든 shapefile 처리하도록 수정 → **전국 39,728,087필지**(경기 5.2M·전남 5.9M·경북 5.7M
        등, 8개 대용량 도 6·6·6·5·4·4·3·3벌). skipped=0, GiST/btree 재생성, ST_Contains 검증.
- [x] 5-2. 완료 확인 (2026-07-18): 3종 모두 실데이터 반환 검증 (4-5에서
      by-point/by-jibun/{pnu} 삼각 일치, PNU 1114010300100310000).
      (8개 도 1M 잘림 → 5-1 전량적재로 해소, 2026-08-01. 전국 39.7M 필지 전량 조회 가능)
- [x] 5-3. 완료 (2026-07-18): TC-E13 신설 (05_TestCases + 추적 매트릭스),
      docs/todo.md 남은 이슈 해소. 실측 근거: ㅇㄴㄹㅇㄱ→온누리약국 200, p95 56ms
- [x] 5-4. 완료 (2026-08-02): 실필지 3건 토지이음 대조 — 지목 3/3 "대" 일치, 면적은
      도형면적 vs 공부상면적 차 ±0.3% 이내(서울 +0.23%·수원 -0.002%·제주 +0.27%) 전부 통과.
      특히 수원(신규 적재 경기, 21,677.0 vs 21,677.4㎡ 거의 완전일치) → 5-1 지적 정확성 확증.
- [x] 5-5. 완료 (2026-08-02): Phase 5 기록(5-1 지적 다중shp 버그·5-4 정부대조 learning_log
      반영) + gitnexus 재인덱싱(1,425 nodes/2,492 edges/85 flows).

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
- [x] 7-키. API 키 선확보 완료 (2026-07-07, .env 저장·커밋 금지):
      MOLIT_API_KEY (토지매매 실거래 RTMSDataSvcLandTrade — 검증 완료: 강남구
      202605 실데이터 수신. 주의: curl 기본 UA는 WAF 차단, Mozilla UA 필수),
      REB_API_KEY (R-ONE — 실호출 검증 완료, 월 지역별 지가변동률
      STATBL_ID=A_2024_00903, 202605 실데이터 확인). JUSO_API_KEY는 Phase 1 참조
- [x] 7-1. ETL 3종 전부 완료 (2026-07-27~28):
      · 실거래가 = land_sales 로더(land-sales 서브커맨드, EMD_CENTROID) ✓
      · 지가변동률 = priceindex 로더(price-index): R-ONE 용도지역별 월 JSON →
        land_price_index 18,651행. 검증: 과천 녹지 202604~06 = 0.363/0.554/0.731 R-ONE 일치 ✓.
      · 표준지공시지가 = standardlots 로더(standard-lots): NSDI AL_D152 시도별 shapefile
        (EPSG:5186→4326, 폴리곤→ST_PointOnSurface) → standard_lots 596,986행(17시도).
        검증: 세종 3611010100108350000 전필드 일치, geom 한국 경계 내 100% ✓.
- [x] 7-2. ETL 3종 완료 (2026-07-29~08-02): land-use 전국 + 개별공시지가·토지특성(서울) 적재.
      NSDI AL_D155 CSV(필지별 사전판정형, 폴리곤 아님) → Option A: land_use_plan
      (pnu×zone, geom 미저장, 포함/저촉/접함 verbatim). **전국 222,379,474행**(staged 262.3M →
      DISTINCT ON dedup: work_mem 384MB + PK 일시제거, 컨테이너 1.9GB 한도). zone_name 클린.
      **발견**: sido 12 = 전남광주통합특별시(광주29+전남46 통합, 2026-07 시행) 정상 적재 —
      단 parcels/admin_areas는 통합前이라 stale → 크로스정합 후속(메모리 gwangju-jeonnam-merger-2026).
      저촉 면적비 원자료 미제공(land-use-option-a-decision).
      **개별공시지가 AL_D151(서울 887,699행 → official-price INDIVIDUAL 활성) + 토지특성 AL_D195
      (서울 888,806행, 원값 라벨) 적재 완료** (2026-08-02, 지역선택 upsert, 미적재는 REGION_NOT_LOADED).
- [x] 7-3. API 6종 완료 (2026-07-28~08-02): sales·price-index(cumulative)·standard-lots
      (거리순)·official-price(개별 우선 INDIVIDUAL/표준지 STANDARD+REGION_NOT_LOADED)·land-use
      (PNU→용도지역/지구/구역+포함/저촉/접함)·**land-features(PNU→이용상황·지형·도로접면 원값)**.
      전부 DB-backed 사실반환 + RealEstate scope(geo키 403). **OpenAPI 증보 완료(15 paths,
      appraisal 6종)**. E2E: 서울 INDIVIDUAL 활성·land-features 정상·미적재 REGION_NOT_LOADED.
      **TC-V 실행 완료(2026-08-02)**: V01/V02/V06/V08/V09/V10 통과 + 실버그 수정(REGION_NOT_LOADED
      판정이 substr로 seq-scan 타임아웃 → PK범위 hasRegion으로 Index Only Scan). V03/V07 Option A 대체.
- [x] 7-4. 독립 b2d-qa 감사 완료·PASS (2026-07-28): 판단로직 부재·보안(SQLi/키유출/
      scope/에러누출/입력검증)·데이터무결성 3요소 CLEAN. 실이슈 수정: official-price
      표준지우선+REGION_NOT_LOADED(스펙 §4), bbox /80000, lat/lng 범위검증. 남음:
      실무 시나리오 전구간 정부서비스 교차대조 — **완료 (2026-08-02)**: 서울 실주소 2건
      (세종대로 시청·역삼동 838) 토지이음 확인서 대조. geocode→parcel→공시지가→용도지역 전구간,
      **개별공시지가 원단위 완전일치**(65,730,000·58,300,000), 용도지역 전항목 일치, 지목 대 일치.
- [x] 7-5. 완료 (2026-08-02): Phase 7 기록(land-use 전국·전남광주통합시·개별공시지가·토지특성·
      7-4 정부대조 learning_log 반영) + gitnexus 재인덱싱(1,477 nodes/2,613 edges/92 flows).

## Review

Phase 0~7 전체 완료 (2026-08-02). todo 50/50 [x]. 변경 요약과 남은 이슈.

### 변경 요약
- **P0 골격**: PostGIS 스키마 + 마이그레이션 러너(`b2d-etl migrate`), 프로젝트 스켈레톤.
- **P1 ETL**: 도로명주소 한글DB(juso)·출입구(entrance, 지오코딩 주력 EPSG:5179→4326)·상가정보
  (sbiz 272만행) 적재, 전 경로 ST_Transform 4326 통일, GiST/trgm/btree 인덱스.
- **P2 Geocoding/Reverse**: 주소 파서 상태기계 + 3단 폴백(exact/exact_partial/jibun/trigram,
  set_limit 0.55로 112s→4s), reverse(건물 KNN `<->` + 포함필지 ST_Contains 법정동). 800질의 100%.
- **P3 Places**: 초성 검색(choseong 272만 백필+trgm), 일반 3단 UNION(10s→0.16s), 반경/카테고리
  필터. 재현율 19/20(위치맥락 시 20/20 — 미스는 정확동명 30개 모호성).
- **P4 API**: 11 엔드포인트 + 인증(SHA-256 키·scope)·IP/키 rate limit·usage meter 체인.
  geocode batch·places category·/v1/usage 추가.
- **P5 부동산**: 연속지적도 전량 적재(다중 shapefile 로더 버그 픽스 **13.3M→39.7M필지**),
  parcel 3종(point/jibun/pnu), 토지이음 정부대조 3/3(지목·면적 도형↔공부 ±0.3%).
- **P6 상용화**: 카카오 vs b2d 정확도 100건 100% 매치, 독립 QA 보안감사 CLEAN, govulncheck 통합,
  배포 3형태 + 요금 3축.
- **P7 감정평가**: ETL 6종(실거래·지가변동률 18,651행·표준지 596,986행·용도지역 전국 222,379,474행·
  개별공시지가 서울 887,699행·토지특성 서울 888,806행), API 6종, openapi 15 paths.
  정부대조 **개별공시지가 원단위 완전일치**(65,730,000·58,300,000원/㎡).

### 주요 기술 결정
- **land-use Option A**: 필지별 사전판정형(포함/저촉/접함) 직접 적재, geom·저촉면적비 미저장 —
  SHP(254자 truncation·콤마패킹) 폐기, CSV 채택. "속성만 필요하면 CSV > SHP".
- **대용량 dedup**: 1억+ 행은 work_mem 상향(384MB, 컨테이너 1.9GB 한도; 2GB는 OOM) + PK 일시제거.
- **감정평가 지역선택 적재**: TRUNCATE 없이 upsert(다른 지역 보존), 미적재는 REGION_NOT_LOADED.
- **사실 반환 원칙**: 보정/평가/추천 금지, 원값 라벨 그대로(재분류 금지).

### 발견·사고
- **전남광주통합특별시(sido 12)**: 광주29+전남46 2026-07 통합. 참조 테이블에 없어 "가짜·제외" 오판
  직전 사용자 제동으로 데이터 손실 방지 — 원자료(법정동명)로 확인. 행정개편은 학습지식·기존 참조로
  단정 금지(메모리 gwangju-jeonnam-merger-2026, rigor-verify-before-concluding).
- **지적 다중 shapefile 잠복 버그**: V-World가 대용량 시도를 100만 레코드 단위 shapefile 여러 벌로
  분할하는데 extract()가 1벌만 처리 → 8개 도 영구 1M 상한. 전 shapefile 처리로 수정.

### 남은 이슈 (후속)
1. **감정평가 지역 확장**: 개별공시지가·토지특성 현재 **서울·경기(41)**(2026-08-02 경기 확장,
   각 ~4.9M행). 타 시군구는 실물 확보 시 동일 로더(`individual-price`/`land-features`)로 upsert 확장 —
   미적재는 REGION_NOT_LOADED. landfeature 로더는 중첩 zip(서울)·직접 CSV(경기) 양형태 지원.
2. **광주-전남 크로스정합**: land_use_plan은 통합後(sido 12), parcels/admin_areas는 통합前(29/46)라
   해당 지역 PNU 크로스조회 불일치. **통합後 연속지적도·법정동코드 발행 시 갱신 필요**.
3. **TC-V**: 완료(2026-08-02) — V01/V02/V06/V08/V09/V10 실행 통과, 실버그(REGION_NOT_LOADED
   판정 seq-scan 타임아웃) 수정. V04/V05는 rt.molit 브라우저 대상, V03/V07 Option A 대체.
4. **인프라**: 대용량 테이블(land_use_plan 222M·parcels 39.7M)이 개발 컨테이너 1.9GB에서 동작 —
   프로덕션은 메모리·work_mem 상향 권장. 대량 재적재는 인덱스 임시제거 패턴 필수.
5. **저촉 면적비(%)**: 원자료 미제공 — ST_Area 재판정은 "판단 개입"이라 원칙상 금지(사실 반환 경계).

## 유지보수 (Phase 7 이후)

- [x] 2026-08-08: 지도 데모 버그 3종(마커 404·places 400·sales 502) + X-Request-Id 도입,
      운영 MOLIT 키 투입. (커밋 d7df4cd)
- [x] 2026-08-08: Sentry 에러 추적 도입. 서드파티 의존성 추가 사유(규칙 1.6):
      github.com/getsentry/sentry-go — panic/5xx 수집·콜스택 추적을 위한 공식 SDK,
      사용자 명시 요청·승인. DSN 비면 완전 비활성이라 런타임 필수 의존 아님.

# Track A 상용화 (지시서: 2026-08-16 Track A 실행 지시서)

진행 방식: Phase 1 -> 사용자 리뷰 -> Phase 2 -> 사용자 리뷰 -> Phase 3.
사전 저장소 대조(2026-08-16): 지시서 1절과 코드 일치 확인(15 라우트, 인증/rate limit/meter,
REGION_NOT_LOADED, web 데모). 불일치 없음.

## Track A Phase 1 — A-1 아웃리치 자산 (문서 중심)

- [x] A1-1. 요금/제품 원페이지: web/public에 pricing.html 신설(기존 docs.html 스타일 재사용).
      index·docs 네비게이션에서 링크 연결(고아 페이지 금지). 배포 3형태(관리형 API 구독 /
      온프레미스 연 라이선스 / 벌크 배치 건별 견적) 명시. 온프레미스 문단에 "주소 데이터가
      고객 인프라 밖으로 나가지 않는다" 핵심 문장. 개별공시지가·토지특성 서울+경기 우선 적재
      명시("계약 시 해당 지역 우선 적재" 로드맵 프레이밍). 가격 숫자는 {PRICE_*} 자리표시자.
      내부 운영 정보(서버 스펙·인프라 구성·DB 용량) 노출 금지 — 데이터 규모 수치는 허용.
      -> 검증: 브라우저 실제 렌더링 기준 확인(파일 생성만으로 완료 선언 금지)
- [x] A1-2. 아웃리치 문안 3버전: outreach/ 신설, (a) 프롭테크/부동산 데이터,
      (b) 물류/커머스 대량 주소 정제, (c) 감정평가법인(온프레미스+데이터 체인).
      문체 규칙 준수: 인사 오프너 금지, 첫 문장 훅(주소량 x 카카오 해당 API 단가 구체 숫자),
      상투구/병렬 나열/em-dash/과장/"국내 유일" 금지, 짧은 문장. 검증 수치는 사실만
      (800질의 100%, 카카오 100건 일치, 정부 원단위 일치, 0.03~0.1s). 끝은 15분 라이브 데모
      제안 한 문장. 카카오 단가는 developers.kakao.com 쿼터 페이지에서 API명 특정해 확인 후 기재.
      네이버는 "기존 무료 API 신규 차단 및 전량 유료화"로만. 구글은 공식 요금 미확인 시 제외.
      파일 상단 DRAFT 표기 — 초안 커밋까지만, 발송용 확정은 사용자가 별도 수행.
      -> 검증: 문체 규칙 체크리스트로 3버전 전수 대조 + 단가 출처 URL 기록
- [x] A1-3. outreach/targets.md: 회사명/담당자/채널/훅 숫자/상태 컬럼 표 골격만.
      회사명 지어내기 금지(빈 표).
- [ ] A1-4. Phase 1 고수준 요약 보고 + learning_log 기록(일반 모드) -> 사용자 리뷰 대기

## Track A Phase 2 — A-2 셀프서브 최소 구성 (Phase 1 리뷰 후 착수)

- [x] A2-1. API 키 티어 필드(demo/standard) + monthly_cap 추가 — migrations/012,
      기존 발급 코드 확장(CreateAPIKey, CLI --tier/--monthly-cap). 기존 키는 cap 0으로 불변.
      캡·rate·가격 수치는 OPS.md 7-1절 자리표시자({CAP_STANDARD} 등) — 사용자 확정 대기.
- [x] A2-2. 월 하드캡을 auth 미들웨어에 연결(usage_log 이달 합계, meter 재사용).
      초과 시 429 monthly_cap_exceeded + pricing.html 업그레이드 안내. /v1/usage는 캡
      판정·집계 제외(잔여량 확인은 캡 도달 후에도 가능). 자동 과금·초과 종량 없음.
- [x] A2-3. 토스 정기결제 링크 수동 플로우 문서화 — deploy/OPS.md 7-1절
      (결제 확인 -> apikey create -> 전달 -> 미납 시 revoke).
- [x] A2-4. Verification phase 완료: 로컬 PostGIS(docker)로 cap=3 키 발급 -> 3콜 ->
      4번째 429 body 확인, 캡 도달 후 /v1/usage 200 + month_remaining=0, cap=0 키
      회귀 없음(6콜 통과). go build/vet/test 전체 통과.

- [ ] 사용자 작업: 토스 대시보드에서 9,900원(부가세 포함) 정기결제 링크 생성 →
      OPS.md 7-1절 {TOSS_PAYMENT_LINK} 교체 + pricing.html의 mailto CTA(TODO 주석)를
      결제 링크로 교체.
- [x] 사용자 작업 완료(2026-08-18): 공개 저장소 JO-HEEJIN/b2d-geo-mcp 생성 -> push 완료.

## Track A 마무리 (2026-08-16 최종 지시)

- [x] F-1. 자리표시자 확정: standard 9,900원/월(부가세 포함)·100,000건·5rps —
      pricing.html과 OPS.md 7-1 양쪽 반영, grep으로 일치 확인. pricing.html의
      나머지 자리표시자 전부 제거(온프레미스 3,000만/갱신 800만·Pro 100만은
      docs/08 기확정값, 종량 지오코딩은 "볼륨 견적" — 30/20/10원은 카카오 50원
      오전제 기반이라 공개 보류, docs/08 재산정 필요).
- [x] F-2. 셀프서브 CTA: mailto:midmost44@gmail.com (제목 프리필 "b2d_geo standard
      구독 신청") + TODO 주석. UTC 월 기준 집계 문구 명시.
- [x] F-3. 원격 MCP: HTTP 모드 인증을 호출자 Authorization/X-API-Key 헤더 패스스루로
      변경(공개 라우트가 서버 키를 쓰면 인증 우회 — env는 폴백). Dockerfile에
      b2d-mcp 빌드 추가, compose mcp 서비스, Caddy handle /mcp 라우트.
- [x] F-4. 부채 기록: MCP 리비전(2025-06-18) 코드 상수 주석+README 명시, OPS.md에
      SDK 전환 검토·월 카운터 캐시 2줄.
- [ ] F-5. 배포: push 완료(378ab22) → CI 이미지 → VPS pull+up+migrate(012) →
      라이브 검증(pricing 자리표시자 grep·llms.txt 200·캡 429 e2e·/mcp 3종 curl).
- [x] F-6. 공개 미러 push + v0.1.0 릴리스 완료(2026-08-18). 최초 push에서 태그
      이벤트가 워크플로 등록보다 먼저 처리돼 릴리스가 안 잡힘 -> 태그 재푸시로 트리거,
      goreleaser 성공(darwin/linux/windows x amd64/arm64 6종 + checksums). 검증:
      GOPROXY=direct go install .../b2d-geo-mcp@latest -> 설치 바이너리로 stdio MCP
      geocode 실호출 body 확인. LICENSE는 미포함(사용자 결정 대기, MIT 추천).

## Track A Phase 3 — MCP 서버 (Phase 2 리뷰 후 착수)

- [x] A3-1. cmd/b2d-mcp — 표준 라이브러리만으로 stdio MCP 서버(JSON-RPC 2.0 줄 단위).
      도구 7종, 설명 한국어+영어 병기, 인증 env B2D_API_KEY, 비즈니스 로직 없음
      (파라미터 검증 + 기존 API GET 전달). 에러는 사람 전달용 문장으로 변환:
      monthly_cap_exceeded("이번 달 호출 한도 도달... 업그레이드: pricing.html"),
      REGION_NOT_LOADED(서울·경기 우선 적재 안내), unauthorized/forbidden/rate_limited.
- [x] A3-2. streamable HTTP 모드: b2d-mcp -http :8330 -> POST /mcp (stateless,
      알림은 202). initialize·tools/call 실호출 확인.
- [x] A3-3. cmd/b2d-mcp/README.md 튜토리얼(설치 복붙 -> Claude Desktop/Cursor JSON
      복붙 -> 예시 질문 3개) + web/public/llms.txt(API 표면 + 에러 코드 + MCP 설치법,
      / 루트에서 자동 서빙 — 로컬 기동으로 200 확인).
- [x] A3-4. Verification phase 완료: MCP inspector가 npm 불능이라 스크립트 JSON-RPC
      클라이언트로 대체(프로토콜 레벨 동일). initialize/tools/list + 7개 도구 전부
      라이브 API(api.birth2death.com) 실호출 body 확인 — geocode exact match,
      공시지가 65,730,000원/㎡(정부 대조 기록값 일치), 실거래 5월 실데이터, 용도지역
      17건. search_places 반경 500m 0건은 직접 API 대조로 래퍼 아닌 데이터 사실 확인
      (반경 1000m 1건 정상). 월캡 문장은 로컬 스택(캡 소진 키)으로, REGION_NOT_LOADED
      문장은 부산 PNU로 확인. 검증용 임시 키(id=38)는 revoke 완료.

## 비목표 (지시서 7절)

결제 대시보드/관리자 UI/사용량 시각화, 새 공공데이터 적재(공시지가 지역 확장 포함),
정액 초과 종량 과금, 기존 엔드포인트 리팩토링/스키마 변경, 감평사 앱(트랙 B),
타깃 회사 리스트 임의 작성.

## Track A 후속 — MCP 온보딩 간소화 (2026-08-18 합의: ①생성기+딥링크, ②키 내장 URL, CLI 원라이너)

- [x] G-1. 키 내장 URL: b2d-mcp HTTP 모드에 POST /mcp/{key} 추가(우선순위 헤더 >
      경로 키 > env). Caddy /mcp 계열 로그 스킵(키가 접근 로그에 남지 않게).
- [x] G-2. docs.html 키 발급 직후 "연동" 블록 자동 생성: Claude Code CLI 원라이너,
      Cursor 원클릭 딥링크, 완성 JSON(Claude Desktop 등), 키 내장 URL — 전부 발급된
      키가 박힌 채 복사 버튼. 키 노출 주의 문구.
- [x] G-3. llms.txt와 README(본 저장소 + 공개 미러)에 CLI 원라이너·키 내장 URL 반영.
- [x] G-4. Verification 완료(2026-08-18): 로컬 3경로(경로 키 성공·헤더 우선·무효 키
      안내문) body 확인, 연동 블록은 실제 docs.js로 프리뷰 렌더 + Cursor 딥링크
      base64 디코드 검증. 라이브: 데모 키 발급 -> /mcp/{key} geocode 성공, 헤더
      모드 회귀 없음, Caddy 로그에 /mcp·키 문자열 0건(log_skip 동작, 일반 요청
      13건은 기록됨). 미러 v0.1.2 릴리스 성공.

## Phase 4 — 법적 문서·사업자정보 게재 (PG 심사 준비, 2026-08-26 지시)

배경: 사업자등록 완료. 포트원 경유 PG 심사 접수에 필요한 사이트 요건(약관·개인정보·
환불정책·하단 사업자정보)을 갖춘다. 토스 정기결제 링크 계획은 폐기됨.
통신판매업 신고번호 `2026-경기하남-1593`을 신고증 확인 후 반영했다.

2026-08-27 승인 반영 (사용자 + claude.ai 리뷰):
- 전화 070-8983-4287 확정 — {PHONE_070} 전부 실값 교체.
- SLA 수치 금지 — {SLA_PLACEHOLDER}는 정성 조항(연속 24시간 장애 시 기간 연장 또는
  일할 크레딧, 불가항력 제외) 확정 문안으로 대체.
- privacy: "서비스 이용 기록"(usage_log)과 "웹서버 접근 로그"(IP 포함) 구분 기재,
  접근 로그 보존 기간 = 실제 로테이션 설정값. 현재 로테이션 없음(무제한) 확인됨 →
  Caddy 시간 기반 로테이션(90일) 이번 Phase에서 설정.
- ADR 신설: docs/adr/0001-payment-provider-portone.md (토스 직접 계약 33만원 vs
  포트원 경유 0원, MoR 고정비 12% 기각, 노코드 정기결제 링크 부재).
- review 섹션 해석: close-out 시 작성 → durable 내용 승격(CONTEXT.md/ADR) 후
  todo에서 완료 태스크 클리어. Phase 4 종료 시 CONTEXT.md 첫 생성.

- [x] P4-1. 공통 푸터 사업자정보: 모든 공개 페이지(index·docs·pricing·map·batch)에
      상호 비투디 / 대표 조희진 / 사업자등록번호 211-25-67093 / 소재지 경기도 하남시
      미사강변남로 99, 1401호 / 공개 문의 이메일 info@birth2death.com / 전화
      070-8983-4287 / 통신판매업 신고번호 2026-경기하남-1593 표기(작은 글씨, | 구분자).
      terms·privacy·refund 링크 포함. map·batch는 푸터 자체가 없어 신설.
      docs.html 푸터는 JS 렌더(docs-foot)라 주입 방식 확인 후 결정.
- [x] P4-2. web/public/terms.html: 실제 서비스 기준 이용약관. 서비스 정의(공간정보
      데이터 API), API 키 관리 책임, 요금·결제(월 9,900원 부가세 포함·100,000건·선불),
      이용 제한(rate limit·캡 429), 데이터 출처(공공데이터 원자료, 보정·추정 없음),
      책임 한계(참고용, 법적 증명은 정부 발급 문서), 변경·중단, 준거법(대한민국).
      템플릿 복붙 금지.
- [x] P4-3. web/public/privacy.html: 실체 대조 기반 개인정보처리방침. 코드 확인 결과
      반영 — 이메일(문의·구독 신청 수신, 가입 플로우 자체는 이메일 미수집), API 키
      해시(api_keys.key_hash), 사용량 로그는 usage_log(키 식별자·엔드포인트·집계
      시각·건수)이며 IP는 usage_log가 아니라 웹서버(Caddy) 접근 로그에 기록됨을
      정확히 구분 기재. 카드정보 미저장(PG사 처리) 문장 필수. 위탁: AWS(Lightsail)·
      Sentry(에러 추적, User 필드 스크럽 확인됨). 보유 기간·파기·제3자 미제공·정보주체
      권리·보호책임자(조희진, midmost44@gmail.com).
- [x] P4-4. web/public/refund.html: 결제 후 7일 이내+미사용 전액 환불(전자상거래법
      청약철회), 사용 개시 후 익월 중단 방식(일할 환불 없음), 장애 크레딧 기준
      {SLA_PLACEHOLDER}, 요청 채널 이메일. 과도한 약속 금지.
- [x] P4-5. pricing.html: CTA 문구 "결제 시스템 준비 중, 이메일 신청 시 수동 발급"
      명확화. 부가세 포함·UTC 월 기준 문구 유무 확인 후 보강.
- [x] P4-6. Verification phase: 로컬 기동 + 헤드리스 스크린샷으로 4페이지(terms·
      privacy·refund·pricing) 렌더 확인, 전 공개 페이지 푸터 존재 확인, grep 검사
      (a) 타사명·템플릿 잔재 0건 (b) 이모지 0건 (c) 211-25-67093 정확 일치.
      privacy 수집 항목 vs 실코드(발급 플로우·usage_log 스키마) 대조 결과 보고.
- [x] P4-7. 사용자 리뷰 반영 후 로컬 커밋 및 push 완료. 법적 문서 4종과 공개 문의 이메일을
      최종 확인했다.

### Phase 4 Review (2026-08-27)

- 산출: terms/privacy/refund 3종 신설, 전 공개 페이지(index·docs·pricing·map·batch)
  사업자정보 푸터(공개 문의 이메일 info@birth2death.com, 전화 070-8983-4287 실값,
  통신판매업 신고번호 2026-경기하남-1593 반영),
  pricing CTA "결제 시스템 준비 중" 문구 + 토스 잔재 주석 제거,
  docs/adr/0001-payment-provider-portone.md.
- 접근 로그 보존 실체화: Caddy 접근 로그를 파일 출력(roll_keep_for 2160h = 90일)로
  전환 + compose caddy_logs 볼륨. privacy의 "90일 자동 파기" 문구와 설정이 쌍.
  caddy validate로 문법 검증 완료. 주의: 배포 시 VPS에 Caddyfile·compose scp 필요,
  전환 후 접근 로그는 `docker compose logs caddy`가 아니라 볼륨 파일에서 본다.
- 검증: 로컬 기동(:8899) 후 8페이지 전부 푸터 확인(/index.html은 Go FileServer
  표준 301 -> /), 헤드리스 Chrome 스크린샷 4종 렌더 정상, grep 3종 통과
  (타사명·템플릿 잔재 0, 이모지 0, 사업자번호 8파일 정확 일치, 070 실값 8파일).
  go build/vet/test 전체 통과(8 ok, FAIL 0).
- privacy 실체 대조: usage_log에 IP 없음(001_init.sql) -> "이용 기록"과 "접근
  로그(IP 포함)"를 분리 기재. 데모 키 발급은 개인정보 미수집(issueDemoKey 확인).
  키는 해시만 저장. Sentry는 SendDefaultPII=false + 스크러버(User 비움) 확인.
  쿠키 미사용, 테마는 localStorage(서버 미전송). TLS 문구는 "이용자와 서비스 간"
  으로 한정(내부 compose 네트워크는 평문이므로).
- 남은 사용자 작업: (1) 포트원 PG 심사 접수.
- close-out은 완료되었고, 신고증 확인 후 통신판매업 신고번호를 반영했다.
- 부수 발견: map.html 데모 패널에 기존 이모지 1건(placeholder) — 이번 범위 밖,
  제거 여부는 사용자 결정.
- 추가 검증(2026-08-27): docs 옛 요금표 잔재 grep 전수 — web/public 전체(html·js·
  llms.txt)에 건당 30/20/10원·월 30만 등 숫자 요금 0건. docs.js 요금 섹션은 숫자
  없이 pricing.html 링크 단일화 확인(요금 출처 1곳).
- 대표 메일 실왕복 검증(2026-08-27): midmost44 -> info@birth2death.com 테스트 발송,
  Cloudflare Email Routing 활동 로그에서 Forwarded 확인(사용자 스크린샷). MX/SPF는
  Cloudflare. 받은편지함 미표시는 포워딩 목적지=발신 계정일 때 Gmail이 동일
  Message-ID 회귀 카피를 중복 폐기하는 동작 — 셀프 왕복 테스트는 활동 로그로
  판정할 것. 채널은 실수신 중(외부 발신 18건/24h 전부 Forwarded).

## 컨설팅 반영 — 일반 사용자용 화면 개편 (2026-10-24 계획, 사용자 승인 후 진행 중)

목표: 월요일 다나클라우드 미팅에서 보여줄 수 있는 상태. 근거는 황대표님 컨설팅 녹취
(00:01~00:43 구간, 피드백은 주로 00:31~00:43: 실거래·지가변동률·사진·레이아웃·리포트·명칭).
원칙: 지시서 1.1(상용 지도 자산 금지)과 1.6(의존성 최소)을 그대로 지킨다. 프런트는
vanilla JS + 기존 Leaflet만, 신규 서드파티 0개.

피드백 -> 처리 결정
| 피드백 | 처리 |
|---|---|
| 실거래가 무슨 내용인지 모르겠다 ("도로 4m"를 도로 폭으로 오해) | 라벨 명확화: "지목 도로 · 면적 4㎡(1.2평)", 평당가 병기 |
| 실거래를 지도에서 위치로 보고, 클릭하면 상세가 떠야 한다 | MOLIT 토지 실거래는 지번이 마스킹되어 필지 단위 핀이 불가능. 법정동 중심점 마커(건수 배지) + 클릭 시 그 동의 거래 목록으로 구현. 한계는 화면에 명시 |
| 거래월을 직접 입력하는 방식은 어렵다 | 최근 3개월 자동 조회(서버 상한 6개월), 월 입력 제거 |
| 지가변동률 그래프가 지도처럼만 보인다 | 축(x=월, y=%)·0선·hover 툴팁이 있는 SVG 차트, 클릭 시 확대 |
| 거리뷰·항공뷰 사진 자동 표시 | 거리뷰는 지시서 1.1 위반(카카오/네이버 자산)이라 제외. 항공사진은 V-World(지시서 3.3 허용 출처) 영상 타일 토글로 구현 |
| 주소 입력 후 한 화면에 지도·사진·거래·변동률·요약·출력 버튼 | 결과 패널을 "리포트 뷰"로 재구성: 상단 요약 카드 -> 세부 카드 -> 출력 버튼 |
| 보고서 출력 버튼 | 조회 결과를 A4 레이아웃으로 인쇄(브라우저 PDF 저장). 사실 데이터만, "감정평가 아님" 고지. 서버·LLM 불필요 |
| API·MCP·요금 같은 개발자 요소를 빼고 프로페셔널하게 | 상단 nav는 일반 사용자 동선만. API 문서·배치·요금은 푸터 "개발자" 묶음으로 이동 (페이지 자체는 유지) |
| 첫 화면 캐치프레이즈, 온보딩 | 랜딩 hero 카피를 일반 사용자 문장으로 교체, 주소 검색 하나를 중심에 |
| 평 단위 (3.3㎡ = 1평) | 면적·단가 전부 ㎡와 평 병기 |

화이트보드 스케치 반영 (2026-10-24, 사용자 제공 사진 2장)
- 서비스명: "All Map : 올맵" 확정 -> 공개 페이지 표시명 교체. 도메인·API 식별자·MCP 이름(b2d-geo)은 유지.
- 결과 화면 4열: [사진(항공) + 지도 = 1/3 | 거래내역 | 지가변동률 | 판단 요약 + 하단 출력 박스].
- 거래내역 Detail(주변지역) -> 시간별: 같은 시군구 거래를 월별 묶음 + 최신순, 같은 동 강조·필터.
- 지가변동률 X/Y축 Detail -> 범위 명시: 축 제목(X: 월, Y: 변동률 %), "표시 범위 YYYY.MM ~ YYYY.MM
  · N개월" 라벨, 월별 값 표.
- 해당 건축물 사진(거리뷰·항공뷰): 항공사진만(V-World). 거리뷰는 지시서 1.1로 제외.
- 단위: 평방미터 = 헤베, 3.3㎡ = 1평 안내 문구. (입방미터 = 루베는 토지 화면에 해당 값이 없어 미표시)
- 온프레미스는 황대표님이 줄을 그어 제외 -> 클라우드 + 회원 ID/PW 또는 IP 화이트리스트 (후속).

이번 범위 밖 (사용자 결정 또는 후속)
- 요금 페이지의 온프레미스 문구 정리: 황대표님 스케치상 제외 대상이나 요금제 개편과 함께 처리.
- 법무 페이지(약관·개인정보·환불) 본문의 서비스명 "b2d_geo": 약관 문구 변경은 PG 심사와 연결되어 사용자 결정 후 처리.
- AI 서술 요약 유료 리포트(건당 700~3,000원), 회원제(ID/PW, IP 화이트리스트), 정액/정량
  요금, 네이버페이·카카오페이: 포트원 PG 심사 이후.
- 아파트 실거래(단지명·지번 공개) 정밀 핀, 전국 공시지가 확대.
- 회원제: 정액(기간별)·정량(이용별), 비회원은 정보이용료 결제(네이버페이·카카오페이).

### To-do
- [x] C-0. 사용자 확인 (2026-10-24): 월요일 대상은 개편본, 로컬 시연 가능하나 가능하면 배포,
      서비스명은 지금 변경, V-World 키는 보유하나 값은 나중에 확인
- [x] C-1. V-World 이용 조건 확인 시도: 공식 가이드 페이지는 robots 차단으로 원문 열람 실패.
      OSM 커뮤니티 스레드의 운영사 답변(2차 출처)상 "표출 용도는 출처 표기 필수 조건으로 가능".
      상업적 이용 범위는 미확인 -> 키는 env(B2D_VWORLD_KEY)로만 주입, 비어 있으면 토글 숨김,
      출처 표기 상시 노출. 유료 판매 전 V-World 약관 원문 재확인 필요 (사용자 작업)
- [x] C-14. 서비스명 변경: 올맵 확정. 8개 페이지 title·topbar·og alt, docs/pricing 설명, 요금 메일 제목,
      llms.txt 머리말 교체. 법무 페이지 본문은 보류(위 범위 밖 참고)
- 설계 변경 (rate limit 근거): demo 티어는 2 rps, burst 4라서 실거래 월별 호출을 브라우저에서
  여러 번 하면 429가 난다. 실거래는 서버 엔드포인트 1개(/v1/appraisal/sales/recent)에서 최근
  N개월을 모아 한 번에 응답하고, 법정동 중심점도 같은 응답에 넣는다. MOLIT 응답은 프로세스
  메모리에 6시간 캐시(일일 호출 한도 보호, 시연 속도 확보).
- [x] C-2. impact 분석: api.New(server.go) LOW, 직접 호출 1(cmd/b2d-server). map.js queryPoint LOW,
      호출자는 같은 파일(searchAddress, 딥링크)뿐. appraisalSales는 수정 없음(라우트 추가만)
- [x] C-3. 공통 유틸: ㎡ -> 평 변환, 평당가 포맷 (shared.js)
- [x] C-4. 거래내역 열: 최근 3개월 자동 조회, 라벨 명확화, 월별 묶음 시간순, 같은 동 강조·필터
- [x] C-5. 실거래 지도 마커: 법정동 대표점 + 건수 배지 + 클릭 시 목록. 대표점은 admin_areas.geom이
      운영에서 전부 NULL이라(QA HIGH-1) 표준지 좌표의 법정동별 평균(최신 해)으로 계산
- [x] C-6. 지가변동률 열: SVG 차트(축 제목, 0선, hover, 확대), 기간 선택, 범위 라벨, 월별 값 표
- [x] C-7. 항공사진: 사진 칸 미니 지도(필지 경계 표시) + 메인 지도 토글. 키 없으면 안내 문구
- [x] C-8. 결과 화면 4열 재구성 (화이트보드 배치)
- [x] C-9. 리포트 출력: print CSS + 출력 버튼 + "감정평가 아님" 고지
- [x] C-10. 랜딩 카피와 nav 정리 (개발자 요소 푸터로 이동)
- [x] C-11. 보안 점검: 입력 검증·파라미터 바인딩, innerHTML 출력 esc() 경유(payload 실행 0건), MOLIT 키 노출 0건,
      실존하지 않는 시군구는 상류 호출 전 404
- [x] C-12. Verification phase: b2d-qa 독립 검증 2회(1차 불합격 HIGH 2·MED 4, 2차 회귀 1건) 후 수정.
      build/vet/test/race/staticcheck, 실 PostGIS SQL, fixture 스텁 + Playwright(데스크톱·모바일·인쇄·키 없음),
      XSS payload, 이모지 0, 신규 의존성 0. 실주소 실데이터 확인은 샌드박스 네트워크 차단으로 미실시
- [x] C-13. learning_log 기록, CONTEXT.md 갱신, 기능 브랜치 커밋(author JO-HEEJIN 단독, AI 표기 없음).
      main 머지(=배포)는 사용자 리뷰 후

### Review

변경 요약
- 서버: `/v1/appraisal/sales/recent` 신설(최근 N개월 묶음, 6시간 캐시, failed/truncated 표시,
  미존재 시군구 404, 법정동 대표점). `/config.json`에 브라우저용 `vworldKey`(env가 비면 빈 값).
- 화면: 지도 페이지를 화이트보드 4열로 재구성, 평 병기, 지가변동률 축·범위·값 표, 판단 요약,
  A4 보고서 출력. 랜딩을 일반 사용자 문구로 교체하고 개발자 링크는 푸터로. 표시명 올맵.
- 문서: openapi.yaml, docs.js, llms.txt, deploy(.env.example, compose, OPS.md).

검증 결과 요약
- QA 1차 불합격 원인(대표점 NULL, 지가변동률 race, 상류 증폭, 표준지 라벨 오류)과 2차 회귀(XSS
  fallback)를 모두 고쳤고, 마지막 수정분은 생성 세션이 실 PostGIS와 payload 테스트로 확인했다.

남은 결정과 후속
- 항공사진(V-World 실시간 타일): 2026-10-24 사용자가 WMTS 영상 타일만 예외로 허용(지시서에 예외
  주석 추가). 개발키를 발급받았고 키 값은 저장소에 넣지 않는다(VPS .env의 B2D_VWORLD_KEY에만).
- [ ] 사용자 작업: VPS .env에 B2D_VWORLD_KEY 추가 후 app 재기동, 실제 화면에서 항공사진 확인
- [ ] 유료 판매 전: V-World 운영키 심사 신청·교체, 상업적 이용 조건 원문 확인
- 법무 페이지 본문 서비스명(b2d_geo), 요금 페이지의 온프레미스 문구는 보류.
- 상용화 백로그: 실거래 라우트 가중 rate limit 또는 일일 상류 예산.
- [ ] 개별공시지가·토지특성 전국 적재(현재 서울·경기만). 서버 디스크 확인 후 시도별 AL_D151·AL_D195 수동 다운로드 -> 기존 로더
- [ ] 광주·전남 코드 정리: parcels(29·46, 통합 전) vs land_use_plan(12) 불일치로 용도지역 422, 실거래 0건
- [ ] 지가변동률 적재 범위 확대: 운영 DB는 2025.09~2026.06(10개월)뿐. 과거 3년 이상 + 2026.07 이후 월 갱신
- [ ] Caddy 접근 로그 90일 파일 보관 적용(개인정보처리방침 문구와 일치시키기, Caddy 재시작 필요)
- [x] 2026-10-23 04:32 운영 500 묶음 원인: 접경지(위도 38.2) 클릭의 reverse가 반경 200m 안에 건물이 없어
      KNN이 전국 건물을 거리순으로 끝까지 훑음 -> 연결 풀 고갈 -> parcel·geocode까지 대기 후 일괄 취소.
      수정: KNN 두 곳(revgeo, places.Nearby)에 geom && 상자 조건 추가(합성 150만 건 기준 7.6초 -> 6ms),
      API 서버 풀에 statement_timeout 20초
- [x] govulncheck 실패 해소 (2026-10-24, 96920c4): Docker 빌드 Go 1.26.5 -> 1.26.10, x/text v0.39.0 -> v0.41.0
      (GO-2026-6629), CI setup-go check-latest. 로컬 govulncheck "No vulnerabilities found".
- 운영 DB 확인 필요: 화성 일반구·광주전남 통합 코드가 parcels/standard_lots에 어떤 코드로 있는지.

## 개별공시지가·토지특성 자동 적재 (브이월드 국가중점데이터 API) — 2026-10-28 계획, 사용자 승인

배경: 브이월드 파일 다운로드는 브라우저 로그인 세션으로만 내려와(서버 curl은 200·0바이트) 자동화할 수 없다.
로봇 로그인은 크롤러 금지 규칙과 충돌한다. 대신 국가중점데이터 API를 서버에서 확인했다(2026-10-28):
- `getIndvdLandPriceAttr`, `getLandCharacteristics` 모두 지금 키로 응답, 법정동 10자리(pnu=4145010600)로
  그 동 필지 전체를 페이지 단위 조회(totalCount 3122). 필드: pnu, stdrYear, pblntfPclnd(원/㎡), lastUpdtDt,
  ladUseSittnNm(토지이용상황), tpgrphHgCodeNm(지형높이), tpgrphFrmCodeNm(지형형상), roadSideCodeNm(도로접면).

설계 (앱 코드 무변경, 같은 테이블에 같은 컬럼으로 적재)
- ETL 서브커맨드 `ned-sync`: 법정동 목록(parcels의 실제 bcode)을 돌며 API를 1,000건씩 페이지 조회 ->
  official_land_prices / land_features upsert. source='vworld_ned', source_version=lastUpdtDt.
- 진행 기록 테이블(마이그레이션 013)로 동 단위 이어받기. 호출 한도 오류면 멈추고 다음 날 이어서.
- 서버 크론: 매일 새벽 한도 안에서 미완료 동을 채움. 전국이 끝나면 연 1회(6월, 5월 말 공시 후) 재수집.
- 키는 서버 env B2D_VWORLD_KEY에서만 읽고 로그에 남기지 않는다(MOLIT 키와 같은 redact).
- 요청 경로에서는 API를 부르지 않는다(사용자 응답은 항상 우리 DB 스냅샷).

To-do
- [x] N-0. 사용자 승인 (2026-10-28): 배치 ETL만 예외 허용(지시서에 문구 추가), 범위는 세종 시범 후 전국 + 연 1회 갱신
- [x] N-1. impact: b2d-etl main은 진입점(호출자 없음), 서브커맨드 추가만. 마이그레이션 013 ned_sync_progress
- [x] N-2. internal/etl/ned: API 클라이언트(페이지·재시도·키 redact) + 행 변환 + 단위 테스트(실제 응답 모양)
      + 실 PostGIS 통합 테스트(B2D_TEST_DATABASE_URL: 적재·이어받기·한도·대조 모드)
- [x] N-3. cmd/b2d-etl ned-sync (dataset, year, sido, max-calls, max-dongs, compare, pause)
- [x] N-4. Verification phase (2026-10-28, 운영 서버 --compare): 서울 3개 동 1,311필지, 경기 3개 동 9,696필지에서
      개별공시지가 원단위와 토지특성 라벨 4종이 8월 파일 적재분과 전부 일치(diff 0). 페이지당 1,000건 동작 확인
- [x] N-5. 세종(36) 시범 적재 (2026-10-28): 140개 동, 개별공시지가 185,564행·토지특성 186,719행, 호출 517회.
      운영에서 세종 주소 5곳 모두 INDIVIDUAL 2026 공시지가·토지특성 표시 확인
- [ ] 세종 지가변동률 0건: price-index에 시군구 36110 자료가 없음(시군구 없는 특별자치시라 R-ONE 크로스워크 누락 추정, 미확인)
- [ ] N-6. 자동 실행: 서버 crontab 대신 compose 서비스 ned-sync(데몬, 매일 03:17 KST)로 변경(서버 이전 시 재등록 불필요,
      사용자 결정 2026-10-28). 운영 Lightsail은 override에 서비스 추가 후 기동, 첫 실행 로그 확인
- [ ] N-7. learning_log, OPS.md(크론·재수집 절차), 지시서 예외 문구
