# 스킬 전체 감사 (2026-07-12, Fable 5 세션 실사용 기준)

감사 기준: 2026-07-05~12 세션(교량 탐지 + b2d_geo Phase 1~4)에서의 실제
호출·효과. "실사용"은 대화 기록 기반 실측이며, 0회 = 이 세션 한정 (다른
맥락에서의 가치와 별개).

## 요약 (5줄)

1. 30여 개 노출 스킬 중 이 세션 실사용 **6개** — 나머지는 컨텍스트 비용만 지불.
2. **caveman은 순효과 의심**: 압축 이득은 실재하나 세션 후반 반복 이탈로 사용자 교정 4회 소모 — 규칙 유지비가 절감분을 갉음. 이탈 트리거(민감 주제 auto-clarity)의 경계 재정의 필요.
3. **gitnexus 6종 + CLAUDE.md "Always Do"는 사문화** — MCP 툴이 세션에 미노출이라 impact/detect_changes 강제 규칙을 한 번도 이행 불가, 매 커밋마다 수동 대체 설명 비용 발생. 규칙을 "가능 시"로 완화하거나 MCP 연결 복구 필요.
4. **cookbook 계열 6종은 이 프로젝트와 무관** (Anthropic Cookbook 저장소 전용) — 사용자 레벨 노출이면 프로젝트별 skillOverrides로 꺼서 리스팅 비용 절약 권장.
5. 고효율 스킬은 **dataviz(검증 스크립트)·artifact-design·update-config** — 1회 호출로 각각 명확한 산출. deep-research는 시간 제약과 충돌해 중도 폐기됨 (수동 검색이 4분 데드라인에 적합했음).

## 판정 통계

유지 9 · 수정 5 · 제거(또는 프로젝트에서 숨김) 16 · 해당없음(에이전트) 별도.

## 상세 테이블

| 스킬 | 출처 | 용도 | 세션 사용 | 판정 | 근거 / 수정 제안 |
|---|---|---|---|---|---|
| caveman | plugin | 응답 압축 (~75% 절감 주장) | 상시(훅) — 단 이탈 4회 교정받음 | **수정** | 민감·설명 주제에서 auto-clarity가 사실상 모드 해제로 작동. 제안: auto-clarity 발동 시에도 "핵심만 일반 문체, 나머지 압축" 하이브리드 규칙 명문화 + 이탈 시 훅이 리마인더 재주입 |
| caveman-commit | plugin | 압축 커밋 메시지 | 0회 (커밋은 일반 문체 규칙 적용) | 제거 | caveman 본체 규칙("커밋은 일반")과 충돌 — 존재 이유 상실 |
| caveman-compress | plugin | 메모리 파일 압축 | 0회 | 제거 | 메모리 파일이 이미 짧음. 압축 시 후임 모델 가독성 손실 리스크 |
| caveman-stats | plugin | 절감 통계 | 0회 | 제거 | 훅 주입 방식이라 호출 안 하면 무의미 |
| caveman-help / caveman-review | plugin | 도움말/압축 리뷰 | 0회 | 제거 | 미사용, 리뷰는 b2d-qa 에이전트가 대체 |
| cavecrew (+investigator/builder/reviewer) | plugin | 압축 서브에이전트 3종 | 0회 | 제거 | 이 프로젝트는 b2d-* 전용 에이전트 + fork가 대체. 이중 체계 유지비만 |
| gitnexus-exploring/-impact-analysis/-debugging/-refactoring/-guide/-cli | project | 코드 인텔리전스 6종 | 0회 (MCP 미노출) | **수정** | 스킬 자체보다 CLAUDE.md의 MUST 규칙이 문제 — 이행 불가 규칙은 매 커밋 예외 설명 비용 유발. "MCP 가용 시 MUST, 아니면 수동 diff 확인" 조건부로 완화 + MCP 연결 점검 |
| cookbook-audit / add-registry / link-review / model-check / notebook-review / review-issue / review-pr / review-pr-ci | user/번들 | Anthropic Cookbook 저장소 전용 | 0회 | 제거(숨김) | 이 프로젝트와 무관 — skillOverrides "off" 권장 (리스팅 토큰 절약) |
| pptx | user | PPT 처리 | 0회 | 유지 | 프로젝트 무관하나 범용 가치, 트리거 명확해 오발동 없음 |
| gws-gmail-send | user | 메일 발송 | 0회 | 유지 | 향후 리크루팅 메일 등 사용 가능성 |
| deep-research | 번들 | 다단계 리서치 하니스 | 1회 (중도 kill) | **수정** | 4분 데드라인과 충돌 — 소요시간 안내/타임박스 파라미터 없으면 급한 맥락에서 매번 폐기됨. 발동 전 예상 소요 고지 제안 |
| dataviz | 번들 | 차트 설계+팔레트 검증 | 1회 (아티팩트) | 유지 | 검증 스크립트가 실제 FAIL→수정 유도 — 실효 최상 |
| artifact-design | 번들 | 아티팩트 디자인 지침 | 1회 | 유지 | 산출물 품질에 직접 기여 |
| update-config | 번들 | settings.json 수정 | 1회 (권한 개방) | 유지 | 절차(read-merge-validate)가 사고 방지 |
| verify / run / code-review / simplify / review / security-review | 번들 | 검증·실행·리뷰 | 0회 (QA는 에이전트로 대체) | 유지 | 에이전트 없는 세션에서 유효한 기본기 |
| loop / schedule / claude-api / keybindings-help / fewer-permission-prompts / init | 번들 | 유틸 | 0회 | 유지 | 저비용 범용. fewer-permission-prompts는 이번에 수동으로 한 일(광역 허용)의 정식 경로였음 — 다음엔 이것 사용 |

## 에이전트 (스킬 아님, 참고 1줄)

b2d-data/-engine/-api/-qa 4종 중 이 세션 실사용 b2d-api 1회·b2d-qa 2회 — 생성/검증 분리 원칙의 실효 입증. fork 3회(핸드오프·논문·본 감사).
