# RAG 지식 맵 (RAG_KNOWLEDGE_MAP)

"이 질문이 생기면 어디를 먼저 여나"를 매핑한다. 근거는 2026-07-12 repo 실스캔.
자산 목록은 `PROJECT_KNOWLEDGE_INDEX.md`, 파일 색인은 `MASTER_FILE_INDEX.md`.

## 질문 유형 → 참조처

| # | 질문 유형 (예) | 어디를 보나 |
|---|----------------|-------------|
| 1 | "이 원본 좌표계가 뭐지 / 어떤 EPSG인가" | `docs/dataspec/{jijeok,juso_shape,juso_entrance}.md` (원본 실측 명세) |
| 2 | "이 컬럼/유일키가 뭐였지" | `docs/dataspec/juso.md` §2.1 (건물 자연키), 각 dataspec |
| 3 | "이 함정 전에 본 적 있나 / 왜 깨졌지" | `docs/ops/MEMORY_RULES_AND_LESSONS.md` B절 기술 함정 대장 |
| 4 | "API 스펙이 뭐지 / 요청·응답 형태" | `openapi.yaml` (9 엔드포인트, scope) |
| 5 | "다음 할 일이 뭐지 / 어디까지 했나" | `tasks/todo.md` 첫 `[ ]` 미완료 항목 |
| 6 | "왜 이렇게 결정했지 / 대안은 뭐였나" | `learning_log.md` (Phase별 "왜 이렇게 설계했는가" 섹션) |
| 7 | "이 데이터 써도 되나 / 출처 규칙" | `b2d_geo_claude_code_prompt.md` §1.1 데이터 출처 화이트리스트 |
| 8 | "테이블 구조/관계가 어떻게 되나" | `docs/02_UML_b2d_geo.md` ERD → `migrations/001,002` |
| 9 | "이게 요구사항이었나 / 왜 만드나" | `docs/03_SRS_b2d_geo.md`(기능), `docs/04_BRD_b2d_geo.md`(비즈니스) |
| 10 | "주소 파서가 어떻게 동작하나" | `docs/design/address_parser.md` → `internal/address/parser.go` |
| 11 | "geocode/reverse/places 로직 어디" | `internal/{geocode,revgeo,places,parcel}`, `MASTER_FILE_INDEX.md` |
| 12 | "이 ETL(적재) 코드 어디 있나" | `internal/etl/{juso,entrance,jijeok,sbiz,nsdi,landsales}` |
| 13 | "인증/rate limit/과금은 어디" | `internal/auth/{apikey,ratelimit}.go`, `internal/meter` |
| 14 | "감정평가 모듈 스펙/스키마" | `b2d_geo_phase7_appraisal_prompt.md`, `docs/07_*`, `migrations/007` |
| 15 | "detection(손상 탐지) 현황/수치" | `detection/HANDOFF.md`, `detection/README.md`, `detection/paper/draft.md` |
| 16 | "이 규칙(caveman/author/단순성)이 뭐였지" | `b2d_geo_claude_code_prompt.md` §1.4·§1.6, `CLAUDE.md` |
| 17 | "어떤 서브에이전트에 위임하나" | `AGENTS.md`, `docs/ops/VERIFIER_SUBAGENT_SPEC.md` |
| 18 | "어떤 스킬을 쓰나 / 이 스킬 효용" | `docs/ops/SKILLS_AUDIT_TABLE.md` |
| 19 | "테스트 시나리오/검증 케이스" | `docs/05_TestCases_b2d_geo.md`, 각 패키지 `*_test.go` |
| 20 | "프로젝트 전체를 인수인계 받는다면" | `docs/ops/POST_FABLE_HANDOFF.md` |
| 21 | "API 키 발급은 어떻게 하나" | `cmd/b2d-etl` apikey create → `internal/auth/apikey.go` |
| 22 | "DB 어떻게 띄우나 / 포트" | `README.md`, `Makefile` (`make up`, 호스트 포트 5433) |
| 23 | "이 환경변수/시크릿 이름이 뭐지" | `MASTER_FILE_INDEX.md` `.env` 표 (값은 비공개) |
| 24 | "원본 데이터가 어디 얼마나 있나" | `MASTER_FILE_INDEX.md` 데이터 디렉터리 표 (du -sh) |

## 우선순위 규칙 (충돌 시)

1. 절대 규칙·데이터 출처: `b2d_geo_claude_code_prompt.md`가 최우선.
2. "왜/어떻게 막혔나": `learning_log.md`(서사) + `MEMORY_RULES_AND_LESSONS.md`(함정 대장).
3. "무엇을/어디까지": `tasks/todo.md`.
4. 코드 실체와 문서가 다르면 코드가 진실 — 문서는 갱신 대상.
