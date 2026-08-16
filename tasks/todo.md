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

- [ ] A2-1. API 키에 티어 필드 추가(무료 데모 + 유료 정액 1티어, 하드캡). 캡 수치는
      {CAP_*} 자리표시자로 두고 사용자 확인. 새 서비스/테이블 남발 금지 — 기존 발급 코드 확장.
- [ ] A2-2. rate limit에 티어별 하드캡 연결(기존 usage meter 재사용). 초과 시 429 +
      업그레이드 안내 메시지. 자동 과금·초과 종량 청구 금지.
- [ ] A2-3. 토스 정기결제 링크 + 수동 확인 후 키 발급 플로우 문서화(코드 최소).
- [ ] A2-4. Verification phase: 티어 키 발급 -> 캡 도달 -> 429 body 확인. 요약 보고 -> 리뷰 대기

## Track A Phase 3 — MCP 서버 (Phase 2 리뷰 후 착수)

- [ ] A3-1. 기존 HTTP API를 감싸는 얇은 MCP 래퍼(stdio 우선), 비즈니스 로직 재구현 금지.
      도구 7종: geocode, reverse_geocode, search_places, get_parcel, get_land_value
      (REGION_NOT_LOADED를 에이전트 친화 메시지로), get_transactions, get_zoning.
      도구 설명 한국어+영어 병기. 인증 env B2D_API_KEY.
- [ ] A3-2. 원격 HTTP(streamable) 형태(로컬 stdio 완성 후).
- [ ] A3-3. README 튜토리얼(설치 복붙 -> 설정 JSON 복붙 -> 예시 질문 3개) + llms.txt를
      api.birth2death.com 루트에 추가(API 표면 + MCP 설치법 한 파일).
- [ ] A3-4. Verification phase: MCP inspector(또는 Claude Desktop)로 연결, 7개 도구 실호출
      1건씩 응답 body 확인. 요약 보고.

## 비목표 (지시서 7절)

결제 대시보드/관리자 UI/사용량 시각화, 새 공공데이터 적재(공시지가 지역 확장 포함),
정액 초과 종량 과금, 기존 엔드포인트 리팩토링/스키마 변경, 감평사 앱(트랙 B),
타깃 회사 리스트 임의 작성.
