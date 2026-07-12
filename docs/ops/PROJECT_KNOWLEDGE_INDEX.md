# 프로젝트 지식 자산 색인 (PROJECT_KNOWLEDGE_INDEX)

이 저장소에 축적된 문서형 지식 자산의 위치와 용도를 한눈에 보는 색인이다.
근거는 2026-07-12 repo 실스캔. 코드 파일 색인은 `MASTER_FILE_INDEX.md`,
"질문 → 어디를 보나"는 `RAG_KNOWLEDGE_MAP.md` 참조.

## 최상위 지시서 (세션 시작 시 정독)

| 경로 | 내용 | 언제 찾아볼지 |
|------|------|--------------|
| `b2d_geo_claude_code_prompt.md` | 구축 지시서 본문 — 절대 규칙(§1: 데이터 출처 화이트리스트, git author, caveman 운용 §1.4, 단순성 §1.6), 스택 결정표 | 규칙 위반 여부 판단, 새 세션 착수 시 |
| `b2d_geo_phase7_appraisal_prompt.md` | Phase 7 감정평가 모듈 실행 지시서 — 스키마 verbatim, scope 정의 | 감정평가(appraisal) 관련 작업 시 |
| `CLAUDE.md` | 프로젝트 지침 요약 + 작업 재개 방법 + 현재 상태 | 매 세션 (자동 로드) |
| `AGENTS.md` | 서브에이전트 4종(data/engine/api/qa) 운용 규약 | 서브에이전트 위임 전 |

## 설계 문서 7종 (`docs/0N_*.md`)

| 경로 | 내용 | 언제 찾아볼지 |
|------|------|--------------|
| `docs/01_SDP_b2d_geo.md` | Software Development Plan | 전체 계획/일정 확인 |
| `docs/02_UML_b2d_geo.md` | UML 설계 (ERD 포함 — 마이그레이션의 근거) | 테이블 구조/관계 확인 |
| `docs/03_SRS_b2d_geo.md` | 요구사항 명세 (기능/비기능) | "이 기능이 요구사항이었나" |
| `docs/04_BRD_b2d_geo.md` | 비즈니스 요구사항 | 왜 이 기능을 만드나 |
| `docs/05_TestCases_b2d_geo.md` | 테스트 케이스 명세 | 검증 시나리오 작성 |
| `docs/06_UserProcessFlow_b2d_geo.md` | 사용자 프로세스 흐름 | API 사용 흐름 확인 |
| `docs/07_Appraisal_Datasets_b2d_geo.md` | 감정평가 모듈 데이터셋 명세 | Phase 7 데이터 확인 |

## 데이터 명세 5종 (`docs/dataspec/*.md`) — 원본 파일 실측 기반

| 경로 | 내용 | 언제 찾아볼지 |
|------|------|--------------|
| `docs/dataspec/juso.md` | 도로명주소 한글 DB 컬럼/유일키 명세 | juso 파싱·건물 자연키 |
| `docs/dataspec/juso_entrance.md` | 출입구 전체분(txt, CP949, 19컬럼) 명세 | 출입구 적재 |
| `docs/dataspec/juso_shape.md` | 건물 도형 전체분 명세 | 건물 폴리곤 적재 |
| `docs/dataspec/jijeok.md` | 연속지적도(V-World) 실측 명세 | 지적/필지 적재, 좌표계 |
| `docs/dataspec/sbiz.md` | 소상공인 상가(상권)정보 CSV 명세 | places 적재 |

## 설계 상세 (`docs/design/`)

| 경로 | 내용 | 언제 찾아볼지 |
|------|------|--------------|
| `docs/design/address_parser.md` | 한국 주소 파서 설계 (Phase 2-1, 상태기계) | 파서 수정 전 |

## 운영·회고 문서 (`docs/ops/`) — 모델 무관 자산

| 경로 | 내용 | 언제 찾아볼지 |
|------|------|--------------|
| `docs/ops/POST_FABLE_HANDOFF.md` | 전체 프로젝트 인수인계 (2026-07-12 갱신) | 인수인계·현황 파악 |
| `docs/ops/MEMORY_RULES_AND_LESSONS.md` | 메모리 규칙 + 기술 함정 대장(B절) + 프로세스 교훈(C절) | "이 함정 본 적 있나" |
| `docs/ops/ONE_PAGE_AI_WORKFLOW_MANUAL.md` | 사용자용 1페이지 운영 매뉴얼 | 사용자에게 워크플로 설명 |
| `docs/ops/PROJECT_INSTRUCTIONS_V2.md` | 프로젝트 지침 V2 제안본 | 지침 개정 검토 |
| `docs/ops/SYSTEM_PROMPT_V2.md` | 전역 시스템 지침 V2 제안본 | 시스템 프롬프트 개정 |
| `docs/ops/VERIFIER_SUBAGENT_SPEC.md` | 검수자 서브에이전트 표준 사양 | QA/검증 에이전트 정의 |
| `docs/ops/SKILLS_AUDIT_TABLE.md` | 스킬 전체 감사표 (Fable 5 실사용 기준) | 어떤 스킬 쓸지 |
| `docs/ops/FABLE_ONLY_TASKS.md` | 최상위 모델 전용 작업 판별 기준 | 모델 티어 위임 판단 |

## 학습 로그 (`learning_log.md`) — Phase별 상세 서사

일반 문장으로 작성된 사용자 학습 자료. 왜 이렇게 결정했는지·무엇에 막혔는지의 원본. 주요 섹션:

- Phase 0 — 프로젝트 골격 구축 (2026-07-03): 설계 대안·기각 사유, 핵심 개념
- v2 Detection — 교량 외관 손상 탐지 파이프라인 (2026-07-05~): 데이터 변환 3종, 라이선스 게이트, 함정 기록
- 클라우드 학습 기록 (RunPod / DEIMv2 점화, 2026-07-05)
- YOLO 대조군 발산·재시작 (2026-07-06~07), DEIMv2 1·2차 완주
- 야간 자율 세션 — 파서·검증 (2026-07-08)
- 출입구 전국 적재 완결 + Phase 1-6 완료 (2026-07-09)
- 울트라 세션 — Phase 2·3·4 대량 완결 (2026-07-12)

## 연구/논문 (`detection/`)

| 경로 | 내용 | 언제 찾아볼지 |
|------|------|--------------|
| `detection/paper/draft.md` | 교량 손상 탐지 논문 초안 | 방법론·수치 인용 |
| `detection/HANDOFF.md` | v2 Detection 인수인계 | detection 파이프라인 재개 |
| `detection/README.md` | 아키텍처 결정 근거 | detection 설계 이유 |

## 작업 상태

| 경로 | 내용 | 언제 찾아볼지 |
|------|------|--------------|
| `tasks/todo.md` | Phase 0~7 체크리스트 (미완료 `[ ]`가 다음 할 일) | 작업 재개 시 첫 참조 |
| `docs/todo.md` | (별도 todo — tasks/todo.md와 구분) | 교차 확인 |
