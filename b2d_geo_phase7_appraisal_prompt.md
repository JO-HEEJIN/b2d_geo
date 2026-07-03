# b2d_geo Phase 7 — 감정평가 모듈 실행 지시서 (Claude Code / Opus 4.8)

이 문서는 b2d_geo_claude_code_prompt.md(메인 지시서)의 증보다.
메인 지시서의 절대 규칙(1절), 워크플로우(2절), 스택(4절), 에이전트 체계(7절),
learning_log 규칙(9절), 보안 체크리스트(10절)를 전부 그대로 상속한다.
이 문서는 Phase 7의 범위, 스키마, API, ETL, 검증 기준만 추가로 정의한다.
Phase 0~6이 완료된 저장소 상태를 전제한다.

---

## 0. 모듈 정의와 경계

**목적**: 감정평가사의 공시지가기준법 실무에 필요한 위치 기반 사실 조회를 제공한다.
비교표준지 검색, 토지 거래사례 검색, 토지이용계획, 토지특성, 지가변동률, 공시지가.

**경계 (절대 규칙, 위반 금지)**:
- 이 모듈은 **사실 반환만** 한다. 다음을 절대 구현하지 않는다:
  격차율 산정, 그 밖의 요인 보정, 평가액 계산, 비교표준지 "추천/유사도 점수",
  사례 "적정성" 판단. 이는 감정평가법상 평가사의 판단 영역이며,
  Birth2Death 감정평가 PoC의 human gate 설계와 역할이 겹치면 안 된다.
- 정렬은 객관 지표(거리, 날짜, 금액)만 제공한다. 복합 점수 컬럼 금지.
- LLM 호출 코드 금지 (b2d_geo 전체 원칙 유지).
- V-World 실시간 API 연동 금지. 개발자 키 만료 정책과 운영키 심사제가
  온프레미스 무통신 요구(NFR-08)와 충돌한다. 벌크 파일 스냅샷만 사용한다.
- 실거래가 데이터는 국토교통부 실거래가 공개시스템의 공식 CSV/오픈API만 사용.
  중개 플랫폼(직방, 호갱노노 등) 스크레이핑 절대 금지 (BR-01 연장).
- 시점수정용 누적 지가변동률 계산은 허용한다. 이는 (1+r/100)의 단순 곱이며
  산술이지 판단이 아니다. 단, 응답에 산식과 원자료 기간을 항상 병기한다.

---

## 1. 데이터 소스 (요약)

상세 명세는 docs/07_Appraisal_Datasets_b2d_geo.md를 따른다. ETL 작성 전
각 소스의 실물 파일 샘플과 명세서를 확보하고, 컬럼 정의를 코드 상수로 문서화한 뒤
learning_log에 기록한다 (메인 지시서 3절과 동일 원칙).

| # | 데이터셋 | 제공처 | 갱신 | 적재 범위 |
|---|---|---|---|---|
| 1 | 표준지공시지가 (토지특성 컬럼 포함) | data.go.kr / 국토교통부 | 연 1회 (1/1 기준) | 전국 필수 |
| 2 | 개별공시지가 | 국가공간정보포털 / data.go.kr | 연 1회 | 시군구 선택 (--region) |
| 3 | 토지특성정보 | data.go.kr / 국토교통부 | 연 1회 | 시군구 선택 (--region) |
| 4 | 토지이용계획 (용도지역·지구 폴리곤) | 국가공간정보포털 (UPIS) | 수시/연 | 전국 필수 |
| 5 | 토지 매매 실거래가 | rt.molit.go.kr CSV / 오픈API | 월 배치 | 전국 필수 |
| 6 | 지가변동률 (시군구×용도지역×월) | 한국부동산원 R-ONE | 월 (익월 25일경) | 전국 필수 |

---

## 2. 스키마 (migrations/007_appraisal.sql)

```sql
-- 표준지 (연도별 이력 보존)
CREATE TABLE standard_lots (
  pnu            char(19) NOT NULL,
  base_year      smallint NOT NULL,
  price_per_sqm  bigint   NOT NULL,          -- 원/제곱미터
  land_category  text,                        -- 지목
  use_zone       text,                        -- 용도지역
  land_use       text,                        -- 이용상황
  road_side      text,                        -- 도로접면
  terrain_height text,                        -- 지형고저
  terrain_shape  text,                        -- 지형형상
  area_sqm       numeric,
  geom           geometry(Point, 4326) NOT NULL,
  source         text NOT NULL DEFAULT 'molit_std',
  source_version date NOT NULL,
  PRIMARY KEY (pnu, base_year)
);
-- 개별공시지가 (대용량: 연도 파티셔닝)
CREATE TABLE official_land_prices (
  pnu char(19) NOT NULL, base_year smallint NOT NULL,
  price_per_sqm bigint NOT NULL,
  source text NOT NULL DEFAULT 'molit_ind', source_version date NOT NULL,
  PRIMARY KEY (pnu, base_year)
) PARTITION BY LIST (base_year);
-- 토지특성 (연도별)
CREATE TABLE land_features (
  pnu char(19) NOT NULL, base_year smallint NOT NULL,
  land_use text, terrain_height text, terrain_shape text, road_side text,
  source text NOT NULL DEFAULT 'molit_feat', source_version date NOT NULL,
  PRIMARY KEY (pnu, base_year)
);
-- 용도지역 폴리곤
CREATE TABLE use_zones (
  id bigserial PRIMARY KEY,
  zone_code text NOT NULL, zone_name text NOT NULL,
  geom geometry(MultiPolygon, 4326) NOT NULL,
  source text NOT NULL DEFAULT 'nsdi_upis', source_version date NOT NULL
);
-- 토지 실거래 (위치 정밀도 등급 필수)
CREATE TABLE land_sales (
  id bigserial PRIMARY KEY,
  emd_code char(10) NOT NULL,                 -- 법정동코드
  jibun_partial text,                          -- 공개 범위 내 지번 (일부 비공개)
  contract_date date NOT NULL,
  area_sqm numeric NOT NULL,
  price_total bigint NOT NULL,
  price_per_sqm bigint GENERATED ALWAYS AS
    (CASE WHEN area_sqm > 0 THEN (price_total / area_sqm)::bigint END) STORED,
  use_zone text, land_category text,
  geom geometry(Point, 4326),                  -- 정밀 좌표 불가 시 법정동 중심점
  location_precision text NOT NULL
    CHECK (location_precision IN ('PARCEL','JIBUN_PARTIAL','EMD_CENTROID')),
  source text NOT NULL DEFAULT 'molit_rt', source_version date NOT NULL
);
-- 지가변동률
CREATE TABLE land_price_index (
  sigungu_code char(5) NOT NULL,
  use_zone_class text NOT NULL,               -- 용도지역 분류 (원자료 분류 그대로)
  month char(6) NOT NULL,                     -- YYYYMM
  rate_pct numeric NOT NULL,                  -- 전월 대비 변동률 (%)
  source text NOT NULL DEFAULT 'reb_rone', source_version date NOT NULL,
  PRIMARY KEY (sigungu_code, use_zone_class, month)
);
```

인덱스: standard_lots.geom / use_zones.geom / land_sales.geom GiST,
standard_lots(use_zone, land_category), land_sales(emd_code, contract_date),
land_price_index는 PK로 충분. 모든 테이블에 provenance(NFR-10) 규칙 적용.

---

## 3. API 사양 (/v1/appraisal 네임스페이스)

인증/에러 포맷/미터링은 기존 미들웨어 체인을 그대로 통과한다 (FR-60~66 상속).
RealEstate 티어 key만 이 네임스페이스에 접근 가능하도록 key에 scope 필드를 추가한다
(migrations 007에 api_keys.scopes text[] 추가, 기본값은 전 scope).

| 엔드포인트 | 파라미터 | 응답 핵심 필드 |
|---|---|---|
| GET /v1/appraisal/standard-lots | pnu(앵커, 필수) 또는 lat/lng, radius(기본 2000, 최대 10000), base_year, use_zone, land_category, land_use, limit(최대 50) | 표준지 배열: pnu, 거리(m), 공시지가, 지목, 용도지역, 이용상황, 도로접면, 지형, 좌표. 정렬: 거리 오름차순 고정 |
| GET /v1/appraisal/sales | pnu 또는 lat/lng 또는 emd_code, radius, from/to(YYYYMM), use_zone, land_category, min_area, max_area | 사례 배열: 계약일, 면적, 총액, 단가, jibun_partial, location_precision, 좌표(정밀도 등급 병기) |
| GET /v1/appraisal/land-use | pnu | 용도지역/지구/구역 배열: zone_name, 관계(포함/저촉), 저촉 면적비(%) |
| GET /v1/appraisal/land-features | pnu, base_year(기본: 최신) | 이용상황, 지형고저, 지형형상, 도로접면, 기준연도 |
| GET /v1/appraisal/price-index | sigungu(5자리), use_zone_class, from/to(YYYYMM), cumulative(bool) | 월별 변동률 배열. cumulative=true 시 누적률 + 산식 문자열 + 원자료 기간 병기 |
| GET /v1/appraisal/official-price | pnu, base_year(기본: 최신) | price_per_sqm, price_type(STANDARD 또는 INDIVIDUAL), 기준연도 |

구현 규칙:
- land-use의 포함/저촉 판정: ST_Intersects 후 ST_Area 비율. 판정 임계값을 코드에
  하드코딩하지 말고 면적비 원값을 반환한다 (판단은 사용자 몫).
- sales의 반경 검색은 location_precision이 EMD_CENTROID인 행이 섞인다는 한계를
  응답 메타(notice 필드)로 항상 고지한다.
- official-price: 표준지 우선 조회, 없으면 개별공시지가. 개별이 미적재 지역이면
  404가 아니라 "REGION_NOT_LOADED" 에러 코드로 구분한다 (선택 적재 정책 반영).

---

## 4. ETL 사양 (cmd/b2d-etl 서브커맨드 확장)

```
b2d-etl load --source=std_price    --year=2026                 # 표준지 (전국)
b2d-etl load --source=ind_price    --year=2026 --region=41450  # 개별 (시군구 선택)
b2d-etl load --source=land_feat    --year=2026 --region=41450
b2d-etl load --source=use_zone                                  # UPIS 폴리곤
b2d-etl load --source=land_sales   --month=2026-06              # 월 배치
b2d-etl load --source=price_index  --month=2026-06              # 월 배치
```

공통 규칙 (메인 지시서 상속 + 추가):
- 적재 전 스키마 검증(FR-05)과 한반도 bbox 검증 동일 적용.
- 좌표계는 원본 명세 확인 후 ST_Transform. 표준지 SHP의 좌표계를 실파일에서 확정.
- 실거래가: 지번 일부 비공개 행은 geom을 법정동 중심점으로 대체하고
  location_precision='EMD_CENTROID'로 기록한다. 정밀 좌표를 추정·보간하지 않는다.
- 지가변동률: R-ONE 원자료의 용도지역 분류 체계를 임의 재분류하지 않고 그대로 적재.
- 각 파이프라인 완료 시 learning_log에 실파일 행수, 거부 행수, 파싱 이슈를 기록.

---

## 5. 서브 Phase 계획 (총 4주)

| Phase | 내용 | 담당 | Exit Criteria (검증 앵커) |
|---|---|---|---|
| 7.0 (0.5주) | 007 마이그레이션, api_keys.scopes, gitnexus 재인덱싱 | b2d-data | 마이그레이션 왕복(up/down) 성공 |
| 7.1 (1주) | ETL: 표준지 + 지가변동률 + 실거래가 | b2d-data | 표준지 3필지를 부동산공시가격알리미(realtyprice.kr)와 지가·특성 대조 일치. 지가변동률 1개 시군구 3개월치를 R-ONE 화면과 대조. 실거래 5건을 rt.molit.go.kr과 대조 |
| 7.2 (1주) | ETL: 용도지역 폴리곤 + 토지특성 + 개별공시지가(선택 지역 1곳) | b2d-data | 실필지 3건의 용도지역을 토지이음(eum.go.kr)과 대조 일치 |
| 7.3 (1주) | API 6종 + scope 검증 + OpenAPI 증보 | b2d-engine(쿼리), b2d-api(핸들러) | 전 엔드포인트 실호출, TC-V 전건 통과 |
| 7.4 (0.5주) | 실무 시나리오 검증 + 보안 감사 | b2d-qa | 시나리오: 실제 대상 필지 1건 → 표준지 검색 → 사례 검색 → 지수 조회 전 구간을 정부 서비스와 교차 대조. 판단 로직 부재 감사(코드 grep: score, weight, adjust 류 금지 패턴) |

검증 원칙 재확인: row count는 검증이 아니다. 위 표의 정부 서비스 화면 대조만
exit criteria로 인정한다 (메인 지시서 1.3절).

---

## 6. 시작 절차

1. 메인 지시서와 이 문서, docs/07_Appraisal_Datasets_b2d_geo.md를 정독한다.
2. tasks/todo.md에 Phase 7.0~7.4 체크리스트를 추가한다.
3. 사용자 승인을 기다린다. 승인 전 코드 작성 금지.
