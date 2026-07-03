# Software Development Plan (SDP)
# b2d_geo — 공공데이터 기반 위치 서비스 엔진

- 문서 버전: 1.0
- 작성일: 2026-07-03
- 작성자: Heejin Jo (Birth2Death LLC)
- 상태: Draft (착수 승인 대기)

---

## 1. 프로젝트 개요

### 1.1 목적
카카오/네이버 지도 API에 의존하지 않는 자체 위치 서비스 엔진을 구축한다.
정부 공공데이터만을 원천으로 geocoding, reverse geocoding, keyword search, places
기능을 제공하며, 부동산 도메인 특화 기능(PNU, 지적, 필지 조회)을 차별점으로 갖는
상용 API 서버를 개발한다.

### 1.2 배경
- 국내 위치 API 시장은 카카오/네이버가 과점하고 있으나, 원천 데이터의 상당 부분은
  정부가 공공데이터로 무상 개방 중이다 (주소기반산업지원서비스, 상가(상권)정보,
  국가공간정보포털).
- 기존 상용 API의 공백 지대: 일일 호출 제한 없는 벌크 지오코딩, 외부 API 호출이
  불가능한 기관을 위한 온프레미스 배포, PNU/지적 등 부동산 특화 필드.
- 개발자의 기존 자산과 정합: 온프레미스 PHI 보호 아키텍처 경험, 감정평가 AI PoC,
  토지용역 워크플로우 자동화. b2d_geo는 이들의 공통 인프라 레이어가 된다.

### 1.3 범위 (Scope)

포함:
- ETL 파이프라인 (공공데이터 다운로드/파싱/적재/인덱싱)
- 한국 주소 파서/정규화 엔진
- Geocoding / Reverse geocoding / Places / Keyword search / Parcel(PNU) API
- API key 인증, rate limiting, 사용량 계측 (과금 근거)
- Docker 기반 배포 구성 (SaaS + 온프레미스 공용)

제외 (v1 범위 외):
- 지도 타일 렌더링, 길찾기(routing), 실시간 교통
- 웹 콘솔 UI (API key 발급은 CLI로 대체)
- 결제 게이트웨이 연동 (계측 데이터만 제공, 청구는 수동)
- 로드뷰류 이미지 서비스
- CV 기반 건물 특성 태깅 (Phase 7 이후 별도 계획)

### 1.4 산출물 (Deliverables)
| ID | 산출물 | Phase |
|---|---|---|
| D-01 | b2d_geo Git 저장소 (Go 소스 전체) | 0~6 |
| D-02 | ETL CLI (b2d-etl) | 1 |
| D-03 | API 서버 바이너리 (b2d-server) | 4 |
| D-04 | migrations/ SQL 스키마 | 0~5 |
| D-05 | OpenAPI 3.1 스펙 | 4 |
| D-06 | docker-compose 배포 구성 (온프레미스 패키지 겸용) | 0, 6 |
| D-07 | learning_log.md (전 과정 학습 기록) | 상시 |
| D-08 | 정확도 벤치마크 리포트 (카카오 대비 샘플 100건) | 6 |
| D-09 | 본 문서 포함 정식 문서 6종 (SDP/UML/SRS/BRD/TC/Flow) | 착수 전 |

---

## 2. 개발 방법론 및 프로세스

### 2.1 방법론
- Phase 기반 반복 개발. 각 Phase는 생성(generation)과 검증(verification)을
  분리된 단계로 수행한다. 생성 직후 완료 선언 금지, 검증은 실 데이터 조회와
  테스트 실행 결과로만 인정한다.
- AI 페어 개발: Claude Code (Opus 4.8)를 실행 엔진으로 사용하되, 아키텍처 결정과
  계획 승인은 개발자(Heejin)가 수행한다. 상세 규칙은 별도 실행 지시서
  (b2d_geo_claude_code_prompt.md)를 따른다.

### 2.2 개발 도구 체계
| 도구 | 용도 |
|---|---|
| Claude Code (Opus 4.8) | 구현 실행 엔진, 서브에이전트 오케스트레이션 |
| caveman | 에이전트 출력 토큰 압축. learning_log.md 작성 시에는 해제 |
| GitNexus | 코드베이스 지식 그래프. Phase 완료마다 재인덱싱, 리팩터링 전 blast radius 확인 |
| GitHub Actions | CI: go vet, go test, staticcheck, govulncheck |

### 2.3 팀 구성
1인 창업자 + AI 에이전트 팀 구조. 에이전트는 Claude Code 서브에이전트로 정의한다.

| 역할 | 담당 | 책임 |
|---|---|---|
| Product Owner / Architect | Heejin Jo | 요구사항 확정, 계획 승인, 법적 판단, 릴리스 결정 |
| b2d-data (에이전트) | ETL | 데이터 다운로드/파싱/적재, 스키마, 인덱스 |
| b2d-engine (에이전트) | 코어 엔진 | 주소 파서, geocode/revgeo/places/parcel 로직 |
| b2d-api (에이전트) | API 계층 | HTTP 서버, 인증, rate limit, 계측, OpenAPI |
| b2d-qa (에이전트) | 품질/보안 | 테스트, 벤치마크, 보안 감사. 생성 에이전트와 분리 운영 |

### 2.4 형상 관리
- Git 단일 저장소 (b2d_geo), main 브랜치 보호, Phase별 feature 브랜치.
- 커밋 author: JO-HEEJIN <midmost44@gmail.com> 단독. AI 표기 금지.
- Conventional Commits. 마이그레이션 파일은 번호 순차, 수정 대신 신규 추가.

---

## 3. 일정 계획

전제: 파트타임 투입 (주 20시간 내외), Claude Code 병행 작업.

| Phase | 내용 | 기간 | 완료 기준 (Exit Criteria) |
|---|---|---|---|
| 0 | 골격: 저장소, docker-compose(PostGIS), CI, GitNexus 인덱싱 | 0.5주 | docker compose up 후 PostGIS 확장 활성 확인 |
| 1 | ETL: juso 주소/위치정보 DB + 상가(상권)정보 적재 | 1.5주 | 실주소 5건 SQL 직접 조회로 좌표 실측 일치 확인 |
| 2 | Geocoding / Reverse geocoding 엔진 | 2주 | 실주소 200건 테스트 셋 정확도 측정 및 기록 |
| 3 | Places / Keyword search | 1주 | 실존 상호 20건 검색 재현율 확인 |
| 4 | API 서버 + 인증/계측 | 1.5주 | 전 엔드포인트 실호출 + 부하 테스트 p95 기록 |
| 5 | 부동산 특화 (지적도, PNU) | 1.5주 | 실필지 3건 PNU 정부 서비스 대조 |
| 6 | 상용화 준비: 보안 감사, 벤치마크, 배포 문서 | 1주 | 보안 체크리스트 전항목 통과, 벤치마크 리포트 |

총 예상: 9주 (버퍼 미포함). Phase 1~2가 임계 경로다. juso DB의 파일 명세 확인과
주소 파서 엣지 케이스 처리에서 지연이 발생하면 전체가 밀린다.

---

## 4. 리스크 관리

| ID | 리스크 | 확률 | 영향 | 대응 |
|---|---|---|---|---|
| R-01 | 상용 서비스 데이터 혼입으로 인한 법적 분쟁 (데이터베이스권, 부정경쟁방지법) | 낮음(통제 가능) | 치명 | 허용 데이터 소스 화이트리스트 강제. 크롤러 코드 작성 자체를 금지. 전 데이터의 출처를 provenance 테이블에 기록 |
| R-02 | 공공데이터 포맷/URL 변경으로 ETL 파손 | 중 | 중 | ETL에 스키마 검증 단계 내장, 명세 불일치 시 적재 중단. 월간 갱신 자동화는 알림 우선 |
| R-03 | 주소 파서 정확도 미달 (구주소/변형 표기) | 높음 | 높음 | Phase 2에서 200건 테스트 셋으로 조기 측정, 실패 케이스 유형화 후 반복 개선. 목표 미달 시 출시 지연 결정 |
| R-04 | 좌표계 오판 (EPSG:5179 vs 5174 등) | 중 | 높음 | 파일 명세서 확보 전 코드 확정 금지. 실측 지점 대조를 Phase 1 exit criteria로 강제 |
| R-05 | 1인 개발 리소스 분산 (Birth2Death, 구직 등 병행) | 높음 | 중 | Phase 단위 완결 구조 유지. 어느 시점에 중단해도 직전 Phase까지는 동작하는 상태 보장 |
| R-06 | OSM(ODbL) 데이터 혼입 시 share-alike 의무 발생 | 낮음 | 높음 | v1에서 OSM 사용 자체를 보류 (실행 지시서에 명시) |
| R-07 | 가격 경쟁력 가정 붕괴 (카카오 무료 쿼터 내 수요) | 중 | 높음 | 저가 경쟁이 아닌 벌크/온프레미스/부동산 필드 3축 차별화. BRD 참조 |

---

## 5. 품질 보증

- 정적 분석: go vet, staticcheck, govulncheck을 CI 필수 게이트로.
- 테스트: 단위(주소 파서 중심) + 통합(실 PostGIS 컨테이너) + E2E(API 실호출).
  상세는 05_TestCases 문서.
- 검증 원칙: HTTP 200, row count 등 프록시 신호로 검증 선언 금지.
  실측 좌표 대조, 정부 서비스 대조 등 1차 소스 비교만 인정.
- 보안: 10항목 체크리스트 (실행 지시서 10절) Phase 4, 6에서 b2d-qa 전수 점검.

## 6. 참조 문서
- b2d_geo_claude_code_prompt.md (실행 지시서, 데이터 소스 상세 포함)
- 02_UML_b2d_geo.md / 03_SRS_b2d_geo.md / 04_BRD_b2d_geo.md
- 05_TestCases_b2d_geo.md / 06_UserProcessFlow_b2d_geo.md
