# POST-FABLE HANDOFF — 전체 프로젝트 인수인계 (2026-07-12 갱신판)

detection/HANDOFF.md(7/7자)의 전면 갱신. 후임 모델은 이 문서를 우선하고,
detection 세부는 구판을 보조 참조. 모든 수치는 실측·커밋 근거 있음.

## 1. 저장소·운영 상태

- repo: github.com/JO-HEEJIN/b2d_geo (private). push는 deploy key
  (~/.ssh/id_ed25519) — gh OAuth 토큰엔 workflow scope 없음
- 최종 커밋: 868c038 (Phase 2·3·4 코어 완결)
- **가동 중 프로세스**: bin/b2d-server (localhost:8080, .env의
  MOLIT_API_KEY/REB_API_KEY export 필요). 재기동 후 반드시
  `pgrep -fl bin/b2d-server`로 PID 확인 — 포트 유령 함정(§6-2)
- 테스트 API 키: `b2d-etl apikey create`로 신규 발급 (기존 harness 키는
  transcript에 평문 노출됨 — 운영 전 revoke 권장)
- DB: docker b2d_postgis, localhost:5433 (b2d/b2d_local_dev)
- RunPod: pod 전부 종료, 잔액 ~$10.8. 재개 절차는 detection/HANDOFF.md §9

## 2. b2d_geo 본선 상태 (Phase별)

| Phase | 상태 | 비고 |
|---|---|---|
| 0~1 | ✅ | 데이터 전량 + dataspec 6종 + 검증 5/5 (하남 포함) |
| 2 | ✅ | 3단 폴백 지오코더 — **정확도 800/800 = 100%** (200주소×4변형) |
| 3 | 90% | 초성 검색 완료 (choseong 272만 백필, 0.2s). 반경/카테고리 필터 잔여 |
| 4 | 90% | auth/ratelimit/meter/키CLI/openapi.yaml 완료·실검증. 잔여: geocode batch, places/category, /v1/usage 조회, 부하테스트(4-5) |
| 5 | 90% | 지적도 13.3M 적재. **8개 시도(41/43/44/46/47/48/51/52) 추가 조각 = 사용자 다운로드 대기** — rows=정확히 1,000,000이 잘림 신호 |
| 6 | ❌ | 보안감사 대상 알려진 이슈 4건: auth 캐시 30s revoke 지연 / 버킷 맵 무한 성장 / meter kill 시 flush 유실 / 신규 폴백 로직 자동테스트 부재(LOW-1) |
| 7 | 30% | 실거래·지가변동률 API 가동. 표준지공시지가·용도지역(UPIS)·토지특성 데이터 미확보 |

DB 규모: parcels 13.3M / buildings 12.8M (geom 있는 출입구 6.4M) /
places 2.7M(초성 전량) / admin_areas 18,716 / land_sales 43(강남 표본).

## 3. detection 트랙 상태

- 학습 완결: DEIMv2 1차 0.161 / 2차 0.151 (비단조 오버샘플링 발견:
  ×20 +64%, ×50 −51%) / YOLO11m 0.0768 (2.1× 격차)
- 논문 detection/paper/draft.md **v0.3** — 잔여 [PENDING] 5: 저자명(사용자),
  bootstrap CI×2(재추론 필요·GPU), 공개 결정(사용자). LaTeX 변환 미착수
- 자산: detection/runs/* 에 체크포인트·로그 전량 (md5 검증됨)

## 4. 남은 작업 우선순위 (권장 순)

1. 지적도 8시도 조각 수령 → `b2d-etl jijeok` 재실행 (멱등)
2. 4-1 잔여: /v1/geocode/batch, /v1/places/category, /v1/usage
3. Phase 6 감사: §2의 알려진 이슈 4건 + govulncheck(go 1.26.5 bump)
   + 부하테스트 p95 기록 (4-5)
4. Phase 7 데이터 확보 (V-World 용도지역 "용도지역지구도" 검색,
   공시지가 알리미) → ETL
5. 논문 LaTeX (ICML 템플릿, 표는 1:1 매핑되게 작성돼 있음)

## 5. 운영 규약 (요약)

- 자율 진행 위임 유효 (메모리 autonomous-execution-grant): 가역 작업은
  실행 후 보고, 잔액 초과 과금·비가역만 확인
- 생성/검증 분리: 검수자 사양 docs/ops/VERIFIER_SUBAGENT_SPEC.md
- learning_log 서사 기록 관례, dataspec 실측 우선, caveman 보고
- API 키(.env): JUSO_API_KEY(ADS 미승인 상태), MOLIT_API_KEY(UA 필수),
  REB_API_KEY(지가변동률 A_2024_00903) — 전부 실검증됨

## 6. 함정 목록 (구판 11건 + 신규 4건)

구판(detection/HANDOFF.md §7): macOS unzip 인코딩 / rsync 2.6.9 /
MooseFS chown / cgroup CPU 쿼터 / DEIM 0-기반 category_id / MPS AMP·LR /
pgrep 자기매치 / zsh word split / 병렬 tar 스트림 사망 / 파이프라인 exit /
모니터 이벤트 지연.

신규 (7/8~7/12):

12. **pg_trgm 임계**: 기본 0.3은 긴 한국 주소에서 `%` 후보 100만 행 →
    112초. set_limit(0.55)로 4초. 짧은 상호명("약국")은 임계로도 부족 —
    전역 similarity 정렬 자체를 버리고 동일→접두→포함 3단 UNION으로
    (10s→0.16s)
13. **set_limit은 커넥션(세션) 단위** — 풀에서 set과 query가 다른 커넥션을
    타면 무효. tx로 커넥션을 고정할 것 (트랜잭션 의미 아님)
14. **포트 유령**: 서버 재기동 시 "bind: address already in use"가
    백그라운드 로그에 묻히면 구버전이 계속 응답 — 지표가 코드 탓처럼 보임
    (하니스 9.5% 미스터리의 진범). 재기동 후 pgrep으로 신 PID 확인 필수
15. **측정 우선**: 지오코더 첫 하니스 14%가 읍·면 키 누락과 붙임 토큰
    결함을 캤다. 대표 케이스 수동 확인만으로 "완성" 선언 금지 —
    200×4 변형 하니스가 표준 (스크립트는 learning_log 2026-07-12 참조)
