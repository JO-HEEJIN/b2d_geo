# b2d_geo 프로젝트 지침

이 저장소의 최상위 지시서는 `b2d_geo_claude_code_prompt.md`다. **세션 시작 시
반드시 정독할 것.** 절대 규칙(데이터 출처 화이트리스트, git author, 생성/검증
분리, caveman 운용, 단순성 원칙)이 모두 거기에 있다.

## 작업 재개 방법

1. `tasks/todo.md`에서 미완료(`[ ]`) 첫 항목부터 이어서 진행한다.
   전체 계획은 이미 사용자 승인을 받았다 (Phase 단위 재승인 불필요,
   단 각 Phase 완료 시 고수준 요약 보고는 필수).
2. 직전까지의 맥락은 `learning_log.md`가 담고 있다 (Phase별 상세 기록).
3. 서브에이전트 통신/커밋 메시지는 caveman 모드 (`/caveman`), learning_log
   작성과 사용자 보고는 일반 모드.
4. 개발 DB: `make up` (PostGIS, 호스트 포트 5433 — 5432는 로컬 Homebrew
   postgres가 점유 중). 마이그레이션: `make migrate`.

## 현재 상태 요약 (수동 갱신)

- **Phase 0~7 전체 완료 (2026-08-02, todo 50/50).** 감정평가 API 6종 전부 DB-backed
  운영 중: land_use_plan 전국 2.22억행, land_features·개별공시지가(서울), 표준지 596k,
  지가변동률 18.6k. 상세는 tasks/todo.md Review 섹션.
- 운영 유지보수 (2026-08-08): 지도 데모 버그 3종 수정(마커 404·places 400·sales 502)
  + `/v1` 전체 X-Request-Id 에러 추적 도입 + 운영 `.env`에 MOLIT 키 투입(실거래 활성).
  MOIS 법정동코드 API 키 확보(.env `MOIS_API_KEY`, ETL 미착수).
- Sentry 에러 추적 도입 (2026-08-08): panic/5xx 수집, X-Request-Id 태그 연동,
  DSN 비면 자동 비활성. GHCR 로그인 복구로 배포는 push → CI → VPS pull 정상화.
- 컨설팅 반영 (2026-10-24, 운영 배포 완료 sha-9d70560, deploy/DEPLOYMENT_2026-10-24.md): 표시명 "올맵",
  지도 화면 4열 리포트 + A4 보고서 출력, `/v1/appraisal/sales/recent` 신설. 항공사진은
  `B2D_VWORLD_KEY`가 있을 때만 켜진다. Phase 7 "V-World 실시간 연동 금지"는 항공사진 WMTS 타일만
  예외로 사용자 허용(2026-10-24). 현재 개발키 — 유료 판매 전 운영키 교체.
- 자동 적재 (2026-10-28): ned-sync(개별공시지가·토지특성, V-World 국가중점 API) compose 데몬 운영 중(sha-d41aab8, 매일 03:17 KST).
  rone-sync(지가변동률, R-ONE API)는 c9c574a 빌드만, 운영 반영은 todo P-3/P-4. 운영 .env 원본은 evnx `b2d-geo/production`.
  10-24~10-28 전체 기록: docs/devnotes/2026-10-24_28_devnote.md, 남은 일: todo "열린 작업 정리 (2026-10-28 기준)".
- 다음 작업 후보 (승인된 계획 아님 — 착수 전 사용자와 합의):
  · 상용화 백로그: deploy/OPS.md 7절 (결제·rate limit 실운영값·Cloudflare).
  · 개별공시지가/토지특성 서울 외 지역 확대 (지역선택 적재 정책).

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **b2d_geo** (1485 symbols, 2634 relationships, 92 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> Index stale? Run `node .gitnexus/run.cjs analyze` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? `npx gitnexus analyze` (npm 11 crash → `npm i -g gitnexus`; #1939).

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows. For regression review, compare against the default branch: `detect_changes({scope: "compare", base_ref: "main"})`.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `query({search_query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `context({name: "symbolName"})`.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method without first running `impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit changes without running `detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/b2d_geo/context` | Codebase overview, check index freshness |
| `gitnexus://repo/b2d_geo/clusters` | All functional areas |
| `gitnexus://repo/b2d_geo/processes` | All execution flows |
| `gitnexus://repo/b2d_geo/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
