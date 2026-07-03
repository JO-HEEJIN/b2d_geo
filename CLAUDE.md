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

- Phase 0 (골격) 완료: 스키마, 마이그레이션 러너, compose, CI, 에이전트 정의.
- 다음 작업: Phase 1 ETL — juso.go.kr 파일 명세서 실물 확보와 원본 좌표계
  확정이 첫 항목. 전체분 다운로드에 juso.go.kr 신청이 필요하면 사용자에게
  브라우저 작업을 요청할 것.

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **b2d_geo** (199 symbols, 189 relationships, 0 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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