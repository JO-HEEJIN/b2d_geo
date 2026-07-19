# b2d_geo 정식 문서 6종 생성 계획

## 계획
- [x] 1. SDP (Software Development Plan) 작성 — 범위, 일정(Phase 0~6), 팀 구성, 리스크, 도구
- [x] 2. UML 문서 작성 — SDP 기반. 컴포넌트/클래스/시퀀스/ERD/배포 다이어그램 (Mermaid)
- [x] 3. SRS 작성 — SDP+UML 기반. FR/NFR/데이터/인터페이스 요구사항, ID 체계 부여
- [x] 4. BRD 작성 — SDP+UML+SRS 기반. 사업 배경, BR ID, 수익 모델 3축, 법적 준수
- [x] 5. 테스트 케이스 작성 — FR/NFR/BR에 매핑되는 TC, 실제 한국 주소 기반 입력값
- [x] 6. 사용자 프로세스 플로우 작성 — 페르소나 3종, end-to-end 흐름, 운영 흐름
- [x] 7. Verification phase — 문서 간 교차 일관성 검증 (엔드포인트, ID 참조, 스택 표기)
- [x] 8. Review 섹션 추가

## 일관성 기준 (모든 문서 공통)
- 엔드포인트: /v1/geocode, /v1/geocode/batch, /v1/reverse, /v1/places/search,
  /v1/places/category, /v1/parcel, /v1/parcel/by-address, /v1/parcel/at, /v1/health, /v1/usage
- 스택: Go 1.23+, net/http, PostgreSQL 16 + PostGIS 3.4, pgx/v5, pg_trgm
- 데이터: juso.go.kr 주소/위치정보 DB, data.go.kr 상가(상권)정보(15083033), 국가공간정보포털 지적/경계
- ID 체계: BR-xx (BRD), FR-xx / NFR-xx (SRS), TC-xx (테스트), UC-xx (유스케이스)

## Review

### 생성 결과
문서 6종 + 본 todo.md. 총 요구사항 ID: FR 32, NFR 10, BR 8, TC 50.
추적 체인: BR(사업) → FR/NFR(시스템) → TC(검증) → Flow(사용 맥락) 완결.

### 주요 설계 결정
1. BRD 포지셔닝을 "저가 경쟁"에서 "카카오가 팔지 않는 것(벌크/온프레미스/PNU)"으로
   확정. SDP R-07과 연동.
2. 테스트 케이스의 좌표/필지 기대값은 의도적으로 "실측 확인 후 기입"으로 비워둠.
   문서 작성 시점에 임의 좌표를 넣으면 환각 값이 테스트 기준이 되는 사고가 생긴다.
   테스트 셋 구축 단계에서 juso.go.kr / 토지이음 화면과 대조하여 채울 것.
3. v1 웹 콘솔/결제 부재를 06 문서에 "정직한 한계"로 명시. 고객 3곳 초과가
   v2 콘솔 착수 트리거.

### 검증 결과
- 자동: ID 미정의 참조 0건, TC 범위 누락 0건, 엔드포인트 오기 0건,
  스택 버전 표기 불일치 0건, 이모지 0건.
- 수기: PNU 자릿수 구조, 자모 분해 기대값, 데이터 갱신 주기 3건 대조 일치.
- 미검증으로 남긴 것: juso DB 원본 좌표계(EPSG 추정치)는 실물 명세서 확인 전까지
  미확정 상태 유지 (SDP R-04, TC-I03이 이를 강제).

### 남은 이슈
- ~~FR-43(초성 검색)은 TC 미커버~~ → TC-E13 추가로 해소 (2026-07-18).
- BRD 요금 수치는 초안 공란. 파일럿 협상 전 Phase 6에서 확정.
