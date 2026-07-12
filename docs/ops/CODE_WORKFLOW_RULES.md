# 코드 작업 표준 (CODE WORKFLOW RULES — 모델 무관)

목적: 코드를 만지는 매 작업의 최소 절차. 2026-07-05~12 세션에서 실측으로
검증된 게이트만 남겼다. 최상위 지시서의 단순성·수술적 변경 원칙을 코드
작업에 구체화한 것으로, 프로젝트 CLAUDE.md와 함께 읽는다.

## 1. 관례 조사 먼저 (착수 전)

- 새 코드를 쓰기 전에 **기존 관례를 grep으로 조사**하고 맞춘다. 관례 충돌은
  신규 코드가 양보한다 (SYSTEM_PROMPT_V2 §4).
- 확인 대상: 정규화 키(어절 단일 공백 결합, buildings.norm_road 기준),
  에러 응답 형식, SRID 변환 경로, 바인딩 방식($n / CopyFrom).
- 유사 로직이 이미 있으면 재사용한다. "새로 짜는 게 빠르다"는 대개 착각 —
  기존 관례와 어긋난 두 번째 구현이 하류를 오염시킨다.

## 2. build + vet + test 게이트 (반영 전 필수)

- 커밋·완료 선언 전 세 개를 순서대로 통과: `go build ./...` →
  `go vet ./...` → `go test ./...`. 하나라도 실패하면 완료가 아니다.
- 컴파일만 되고 테스트가 없으면 "완성" 금지 (TEST_AND_VERIFICATION_STANDARD).
- 의존성·버전 변경 시 `go mod tidy`와 `govulncheck`도 게이트에 포함.

## 3. 대용량 쿼리는 EXPLAIN 실측 (반영 전)

- 신규/수정 쿼리는 **실제 규모 테이블(parcels 13.3M, buildings 12.8M,
  places 2.7M) 기준 EXPLAIN ANALYZE**로 실측한 뒤 반영한다. 소규모
  샘플에서 빠른 것은 규모에서 안 선다.
- pg_trgm은 기본 임계 0.3 금지 — set_limit 상향 + **tx로 커넥션 고정**
  (set_limit은 커넥션 단위, 풀에서 다른 커넥션 타면 무효). 실례: 긴 주소
  trigram 폴백 112초 → set_limit(0.55) 4초. 전역 similarity 정렬은
  3단 UNION+LIMIT로 대체 (10초 → 0.16초).
- 성능 주장은 EXPLAIN/프로파일 전후 수치 병기로만 한다.

## 4. 커밋 메시지에 검증 수치

- 커밋 본문에 **무엇을 어떻게 검증했는지 수치**를 남긴다 (예: "지오코더
  하니스 800/800 = 100%", "EXPLAIN 112s→4s"). git author는
  JO-HEEJIN <midmost44@gmail.com> 고정.
- 코드와 문서가 모순되면 문서를 즉시 고친다. 알려진 이슈는 발견 즉시
  tasks/todo.md 해당 Phase 감사 항목에 근거와 함께 기록한다.

## 5. 파일 소유권 분할 (병렬 작업 시)

- 여러 에이전트/작업이 동시에 돌 때, **파일 단위로 소유권을 미리 분할**해
  같은 파일에 동시 접촉하지 않게 한다. 검수자에게는 메인 동시 작업 파일
  접촉 금지를 명시한다 (VERIFIER_SUBAGENT_SPEC §2).
- 분할 경계는 프로젝트 에이전트 산출 디렉터리를 기준으로: internal/etl+
  migrations(data), internal/address·geocode 등(engine), internal/api·
  auth·meter(api). 경계가 겹치는 변경은 순차화한다.
