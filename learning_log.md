# learning_log

이 파일은 b2d_geo 구축 전 과정의 학습 기록이다. 각 Phase 완료 시 상세히 기록하며,
실패한 시도와 그 원인을 성공한 결과보다 우선해서 남긴다.

---

## [2026-07-03] Phase 0 — 프로젝트 골격 구축 (메인 오케스트레이터)

### 무엇을 했는가

개발 환경을 구성하고 프로젝트 골격을 세웠다. 구체적으로:

1. 개발 도구 설치: Go 1.26.4, colima 0.10.3(컨테이너 런타임), docker CLI 29.6.1,
   docker compose 5.3.0. 모두 Homebrew로 설치했다.
2. `b2d_geo`를 독립 git 저장소로 초기화했다. 이전에는 홈 디렉터리에 있는 다른
   저장소의 하위 디렉터리(untracked)였기 때문에, 프로젝트 자체의 커밋 이력을
   가질 수 없는 상태였다.
3. Go 모듈(`github.com/JO-HEEJIN/b2d_geo`)과 지시서 5절의 디렉터리 구조를 만들었다.
   각 internal 패키지에는 역할을 설명하는 doc.go만 두었다 (실제 코드는 담당 Phase에서).
4. docker-compose.yml로 PostGIS 16-3.4 컨테이너를 정의하고 기동했다.
5. UML 문서의 ERD를 그대로 옮긴 초기 스키마(migrations/001_init.sql)와,
   Go 표준 라이브러리 embed 기반의 자체 마이그레이션 러너를 작성해 실제로 적용했다.
6. 서브에이전트 4종(b2d-data, b2d-engine, b2d-api, b2d-qa) 정의,
   GitHub Actions CI(go vet, go test, staticcheck), Makefile, README를 작성했다.

### 왜 이렇게 설계했는가 (대안과 기각 사유)

**colima vs Docker Desktop.** macOS에서 docker 컨테이너를 돌리려면 리눅스 VM이
필요하다(컨테이너는 리눅스 커널 기능이기 때문). Docker Desktop은 GUI 앱이라
설치에 수동 개입이 필요하고 기업 환경에서는 유료 라이선스 문제도 있다. colima는
brew 명령만으로 설치와 기동이 끝나는 CLI 도구라서 1인 운영 원칙(BR-07)에 더 맞다.

**마이그레이션 러너를 직접 작성한 이유.** golang-migrate 같은 기성 도구가 있지만,
지시서의 의존성 최소화 원칙(직접 의존성 5개 이하)에 따라 stdlib의 `embed`로 SQL
파일을 바이너리에 내장하고 60줄짜리 러너를 직접 작성했다. 우리에게 필요한 기능은
"번호 순서로 적용, 적용 이력 기록, 트랜잭션 보장" 세 가지뿐이고, 이는 기성 도구의
수천 줄을 들여올 이유가 되지 못한다. 롤백(down migration) 기능은 의도적으로 뺐다.
운영 원칙이 "적용된 파일은 수정하지 않고 신규 번호로 추가"이므로 down이 필요 없다.

**스키마에서 buildings의 자연키를 확정하지 않은 이유.** juso.go.kr 파일 명세서를
아직 실물로 확인하지 않았다. 건물관리번호(25자리)가 유니크 키일 가능성이 높지만,
명세 확인 전에 코드에 박으면 R-04(좌표계 오판)와 같은 부류의 사고가 된다.
Phase 1에서 명세를 확보한 뒤 별도 마이그레이션(002_)으로 추가한다.

**parcels.geom을 Polygon이 아닌 MultiPolygon으로 한 이유.** ERD 초안에는 Polygon으로
되어 있었으나, 실제 지적 데이터에는 하나의 필지가 도로 등으로 분리된 다중 폴리곤인
경우가 존재한다. Polygon으로 잡으면 적재 단계에서 해당 행이 전부 실패한다.
MultiPolygon은 Polygon을 포함하는 상위 타입이라 안전한 쪽으로 선택했다.

### 막힌 지점과 해결 과정 (에러 메시지 원문 포함)

**1. 마이그레이션 첫 실행이 인증 오류로 실패했다.**

```
migrate failed: create schema_migrations: failed to connect to `user=b2d database=b2d_geo`:
	[::1]:5432 (localhost): tls error: server refused TLS connection
	127.0.0.1:5432 (localhost): server error: FATAL: role "b2d" does not exist (SQLSTATE 28000)
```

컨테이너는 healthy 상태였는데 "role b2d does not exist"라니 이상했다. `lsof`로
5432 포트를 확인하니 원인이 드러났다: 이 머신에는 Homebrew로 설치된
postgresql@17이 서비스로 돌면서 이미 127.0.0.1:5432를 점유하고 있었다.
컨테이너의 포트 포워딩(ssh, 와일드카드 `*:5432`)도 함께 listen 중이었지만,
운영체제는 더 구체적인 바인딩(127.0.0.1)을 우선하므로 localhost로 접속하면
컨테이너가 아니라 로컬 postgres 17에 연결됐던 것이다. 그 서버에는 당연히 b2d
롤이 없다. 해결: 기존 서비스를 중단시키는 대신(다른 프로젝트가 쓸 수 있다)
컨테이너를 호스트 포트 5433으로 옮겼다. 교훈: "connection refused"가 아니라
"role does not exist"처럼 서버가 응답은 하는데 내용이 이상한 오류는,
엉뚱한 서버에 연결됐을 가능성부터 의심해야 한다.

**2. 과거 Docker Desktop의 잔재 설정이 남아 있었다.**

`~/.docker/config.json`에 `"credsStore": "desktop"`이 남아 있었는데, 해당 자격증명
헬퍼 바이너리(docker-credential-desktop)는 Docker Desktop 제거와 함께 사라진
상태였다. 이 상태로는 이미지 pull 시 자격증명 조회 단계에서 실패할 수 있어
해당 항목을 제거하고, brew로 설치한 compose 플러그인 경로(cliPluginsExtraDirs)를
추가했다. `docker compose version`과 hello-world pull로 정상 동작을 확인했다.

**3. PostGIS 이미지가 arm64 미지원이라 에뮬레이션으로 돈다.**

```
postgis The requested image's platform (linux/amd64) does not match the detected
host platform (linux/arm64/v8) and no specific platform was requested
```

postgis/postgis:16-3.4 공식 이미지는 amd64만 제공한다. Apple Silicon에서는
에뮬레이션으로 동작하므로 기능상 문제는 없지만 성능이 떨어진다. Phase 4의
성능 측정(NFR-01: p95 < 50ms)은 이 환경에서 측정하면 불리하게 나올 수 있다는
점을 기록해 둔다. 지시서가 이미지 버전을 명시하고 있으므로 임의로 바꾸지 않고,
성능 측정 시점에 재논의한다.

**4. caveman 도구가 이 머신에 없다.**

지시서 1.4절의 caveman 모드(에이전트 통신 토큰 압축)는 별도 도구를 전제하는
것으로 보이나 `which caveman`, Claude 플러그인 목록 모두에서 찾지 못했다.
도구 없이도 서브에이전트 보고를 "짧은 문장, 결과 중심"으로 강제하는 방식으로
취지(토큰 절약)는 지키되, 도구 설치가 필요하면 사용자 확인이 필요하다.
(해결: 사용자가 알려준 https://github.com/juliusbrussee/caveman 로 설치 완료.
Claude Code 스킬 7종이 ~/.claude/skills에 전역 설치되었고, 세션에서 /caveman
또는 "caveman mode"로 활성화한다. 설치 스크립트는 실행 전에 내려받아 내용을
검토했다 — 원격 스크립트 파이프 실행을 피하는 습관.)

### 이 작업에서 배울 수 있는 핵심 개념

**PostGIS와 공간 데이터베이스.** PostgreSQL은 범용 관계형 DB이고, PostGIS는 여기에
"공간 타입"을 추가하는 확장이다. `geometry(Point, 4326)` 같은 컬럼 선언에서
Point는 도형의 종류(점), 4326은 좌표계 식별번호(SRID)다. EPSG:4326은 흔히 아는
WGS84 경위도 좌표계(GPS가 쓰는 그것)이고, 한국 공공데이터가 자주 쓰는 EPSG:5179는
미터 단위의 평면 좌표계다. 서로 다른 좌표계의 값을 섞으면 좌표가 수백 km씩
어긋나므로, 적재 시점에 `ST_Transform`으로 4326으로 통일하는 것이 이 프로젝트의
원칙이다.

**GiST 인덱스와 pg_trgm.** 일반 B-tree 인덱스는 "크다/작다"로 정렬 가능한 값에만
쓸 수 있다. 공간 데이터의 "이 점이 이 폴리곤 안에 있는가" 같은 질의는 GiST
(Generalized Search Tree)라는 별도 인덱스 구조가 필요하다. pg_trgm은 문자열을
3글자 단위 조각(trigram)으로 쪼개 유사도를 계산하는 확장으로, "미사강변데로"
같은 오타 입력에서도 "미사강변대로"를 찾게 해 준다. 둘 다 이번 스키마에
미리 만들어 두었다.

**Go의 embed.** 다음 코드가 마이그레이션 내장의 전부다.

```go
//go:embed *.sql
var FS embed.FS
```

첫 줄은 주석처럼 보이지만 컴파일러 지시자다. 빌드 시점에 같은 디렉터리의 *.sql
파일들을 바이너리 안에 복사해 넣고, FS 변수로 파일시스템처럼 읽을 수 있게 한다.
덕분에 배포 시 SQL 파일을 따로 나를 필요가 없다. 단일 바이너리 배포라는
프로젝트 목표(온프레미스 설치 단순화)와 직결되는 기능이다.

**마이그레이션 러너의 핵심 로직** (internal/store/migrate.go):

```go
tx, err := pool.Begin(ctx)                                  // 트랜잭션 시작
if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {   // SQL 파일 전체 실행
    tx.Rollback(ctx)                                        // 실패 시 전부 되돌림
    return fmt.Errorf("apply %s: %w", name, err)
}
if _, err := tx.Exec(ctx,
    `INSERT INTO schema_migrations (filename) VALUES ($1)`, name,
); err != nil {                                             // 적용 이력 기록
    tx.Rollback(ctx)
    return fmt.Errorf("record %s: %w", name, err)
}
if err := tx.Commit(ctx); err != nil { ... }                // 둘 다 성공해야 확정
```

스키마 변경과 이력 기록이 하나의 트랜잭션 안에 있다는 점이 중요하다. 중간에
실패하면 절반만 적용된 상태가 남지 않고, 재실행하면 처음부터 다시 시도한다.
`$1` 자리표시자는 pgx의 파라미터 바인딩으로, 문자열 조합 없이 값을 전달해
SQL 인젝션을 원천 차단한다 (보안 체크리스트 1번 항목의 실례).

### 검증 방법과 결과 (실제 수치)

- `go build ./...`, `go vet ./...`: 통과. 직접 의존성은 jackc/pgx/v5 하나
  (상한 5개 대비 1개 사용).
- `docker compose up -d --wait` 후 psql 직접 조회:
  `SELECT postgis_version()` → `3.4 USE_GEOS=1 USE_PROJ=1 USE_STATS=1`.
  pg_trgm 확장 활성 확인.
- 마이그레이션 적용 후 `\dt`: public 스키마에 buildings, places, parcels,
  admin_areas, api_keys, usage_log, schema_migrations 7개 테이블 확인.
- 멱등성: `b2d-etl migrate` 재실행 후에도 schema_migrations는 1행 유지
  (001_init.sql, 2026-07-03 04:47 UTC 적용). 중복 적용 없음.
- 커밋 author 확인: `git log --format='%an <%ae>'` → `JO-HEEJIN <midmost44@gmail.com>`.

### 관련 커밋 해시

- 0a883ec feat: bootstrap project skeleton with PostGIS schema and migration runner

---

## v2 Detection — 교량 외관 손상 탐지 파이프라인 (2026-07-05)

### 배경

geo 본선(Phase 1 ETL)과 별도로, AI Hub 040 "교량 3D 외관점검 영상 데이터"를
학습 데이터로 하는 손상 탐지(v2 detection) 트랙을 시작했다. 리포지토리에
이미 확보돼 있던 라벨링 zip 20개(Training 10 + Validation 10)가 출발점이다.

### 데이터 변환 3종과 검증 수치

**bridge_converter.py** — labelme형 폴리곤 JSON을 압축 해제 없이 zip 스트리밍
으로 읽어 YOLO bbox txt로 변환. train 336,068 / val 42,004 파일이 JSON 수와
정확히 일치했고, 정상데이터(빈 shapes)는 빈 txt로 유지해 negative sample
78,715 + 9,839장을 확보했다. 생성/검증 분리 원칙에 따라 변환기 코드를 참조하지
않는 독립 검증 에이전트 3개(수치 재계산 / 전수 무결성 / 엣지케이스 반증)를
병렬로 돌렸고 전부 통과했다. 전 코퍼스 box-for-box 재계산의 최대 편차는
5.0e-7(순수 6자리 반올림 오차)이었다. 로그의 zero_area_bbox 2건은 실물 확인
결과 3점이 모두 y=340인 수평선 퇴화 폴리곤으로, 스킵이 정당했다.

**coco_converter.py** — DEIMv2 입력용 COCO json. annotations 658,076 / 96,916
개가 QA 확정치와 일치, 왕복 스팟체크 1,000건 불일치 0.

**rdd_converter.py** — RDD2022 3개국(Japan/India/Czech) VOC xml에서 균열
계열(D00/D01/D10/D11/D20/D0w0)→AspaltCrack, D40→Pothole로 매핑해 12,291장
채택. 최약 클래스 Pothole이 629 → 6,256 박스로 약 10배가 됐다. 채택 이미지
안의 균열 변형(D01 등)을 매핑하지 않으면 false negative로 학습되므로 포함했고,
차선 흐림/맨홀(D43/D44/D50)은 손상이 아니므로 배경 처리했다. val은 AI Hub
순수 유지(평가 일관성).

### 아키텍처 결정과 라이선스 게이트

- 주력 DEIMv2-M(DINOv3 ViT-Tiny+ 백본): 코드 Apache-2.0, COCO 54.7 AP급
  실시간 SOTA. RGB 산업검사 full fine-tune에서 DINOv3 초기화가 ImageNet보다
  낫다는 직접 비교 논문(arXiv 2605.23472)이 결정 근거.
- YOLO11m은 Ultralytics AGPL-3.0이라 배포 금지, 내부 대조군 전용으로 강등.
- dacl10k는 CC BY-NC 4.0(비상업)이라 학습 편입을 취소하고 외부 벤치마크로만.
- RDD2022는 공식 repo에 라이선스 명시가 없었는데, figshare **API**
  (api.figshare.com/v2/articles/21431547)로 CC BY 4.0을 확정했다. 웹페이지가
  403이어도 API는 열려 있는 경우가 있다.

### 함정 기록

macOS unzip(Info-ZIP 6.0)이 zip 엔트리명 "no124IG1P01_001495 - 복사본.jpg"
(UTF-8 플래그 정상)를 깨진 바이트로 추출 시도하다 "disk full?"로 오판하고
대화형 프롬프트에서 죽었다(exit 50). 디스크는 2.8TB 여유였다. Python
zipfile로 교체하니 문제없이 끝났다. 대용량 zip 추출은 처음부터 Python으로.

### 현재 진행

- YOLO11m 대조군: M5 Max MPS, 512px, batch 32, 12 epochs, 실측 1.5s/it
  (10,503 step/epoch) → 약 55시간 예상. detection/runs/baseline_y11m.
- DEIMv2는 클라우드 CUDA 전제: 백본/COCO 체크포인트 다운로드, 30 epoch 축소
  recipe config, 실행 명령까지 준비 완료 (detection/README.md).

### 관련 커밋 해시

- f704b68 feat: add v2 detection pipeline for bridge damage

### 클라우드 학습 환경 구축 기록 (RunPod, 2026-07-05)

DEIMv2 학습용으로 RunPod RTX 4090 (SECURE, $0.69/h, 80GB 볼륨)을 API로
프로비저닝했다. community($0.34/h)는 4090 재고가 없어 secure로 폴백.
Azure는 로그인돼 있었지만 GPU 쿼터가 0이라 제외했다 (쿼터 신청은 티켓 대기).

데이터 27GB(이미지 378,072개) 업로드에서 배운 것:

1. **macOS 기본 rsync는 2.6.9**라 `--info=progress2`를 모른다. 옵션 에러로
   즉시 종료했는데 백그라운드라 완료로 오인하기 쉬웠다. 원격 검증(파일 수
   카운트)으로 잡았다. 전송 후 검증은 항상 필수.
2. RunPod /workspace는 MooseFS 네트워크 볼륨이라 root여도 chown이 막힌다.
   tar 추출 시 `--no-same-owner` 필요.
3. **SSH 단일 스트림이 병목**이었다: 단일 tar 파이프 73MB/min → 파일 목록을
   6분할해 tar 파이프 6개 병렬로 204MB/min (약 3배). 회선이 아니라 스트림당
   처리량 한계였다. 대량 소파일 전송은 처음부터 분할 병렬로.
4. zsh에서 `set -- $var`는 공백 분리가 안 된다 (word splitting 기본 꺼짐).
   GraphQL 쿼리 조립이 조용히 깨졌다. `${=var}` 또는 명시적 파라미터로.

비용 계획: 잔액 $22, 예상 학습 30h × $0.69 ≈ $21 + 전송 유휴 ~$1.5로 빠듯.
부족 시 $10 추가 충전 필요할 수 있음을 사용자에게 고지해 둔 상태.

### DEIMv2 클라우드 학습 점화 기록 (2026-07-05)

전송 완료 검증(train 336,068 / val 42,004, 라벨 카운트와 정확 일치) 후
warm-start(-t ckpts/deimv2_dinov3_m_coco.pth)로 1차 점화했다.

**1차 점화 = CUDA OOM 실패.** 원인: COCO recipe의 multi-scale 학습(480~800px
랜덤)에서 batch 32가 4090 24GB를 초과 — 640px인 iter 0에서 이미 17.6GB를
쓰고, 800px 배치를 만나자 23.5GB 한도를 넘었다. iter 0 로그가 정상이어도
multi-scale 최대 해상도까지는 OOM을 배제할 수 없다는 것.

수정 사항 (deimv2_configs/deimv2_dinov3_m_bridge.yml):
- total_batch_size 32 → 16
- LR 선형 반감: base 0.0005 → 0.00025, 백본(DINOv3) 2.5e-5 → 1.25e-5.
  주의: DEIM config의 optimizer.params는 리스트라 deep merge가 아니라 통째
  교체 — 부분 오버라이드 불가, 전체 param group을 다시 명시해야 했다.
- 실행에 PYTORCH_CUDA_ALLOC_CONF=expandable_segments:True 추가 (단편화 완화)

1차 점화에서 확인된 정상 신호: tuning 로드 시 score head 10개만 unmatched
(COCO 80클래스 → 9클래스 재초기화, 예상된 동작), denoising_class_embed 1개
missed, iter 0 종합 loss 48.4 (DEIM은 aux/denoising loss 합산이라 이 스케일이
정상). 파라미터 18.05M, fwd 51.9 GFLOPs.

부수 교훈: 원격 프로세스 확인 시 `ssh host "pgrep -f train.py ..."`는 커맨드
라인 문자열 자체에 패턴이 들어가 자기 자신에 매치될 수 있다(오탐 경험).
패턴을 `[t]rain.py`로 쓰거나 pgrep -x를 쓸 것.

**2차 점화 = CUDA device-side assert 실패 (SIGABRT).** IndexKernel
"index out of bounds" 대량 출력 후 abort. 원인: DEIM은
remap_mscoco_category=False일 때 COCO json의 category_id를 라벨 인덱스로
**그대로** 사용한다 (engine/data/dataset/coco_dataset.py:135). 우리 json은
COCO 관례대로 1~9 (1-기반)라 클래스 9가 [0..8] 밖 → gather에서 assert.
coco_converter/coco_merge_rdd를 0-기반으로 수정하고 json 3종 재생성·검증
(cids ⊆ {0..8} assert) 후 재업로드했다. 커스텀 데이터셋 + remap off 조합에선
category_id를 0-기반으로 만들어야 한다.

부수 교훈 2: 셸 워처에서 `grep X | cut ... && echo VERDICT`는 파이프라인
종료코드가 cut 기준이라 grep 미매치여도 VERDICT가 찍힌다 (오탐 경험).
판정용 grep은 파이프 없이 단독 실행할 것.

부수 교훈 3: iter 0 로그가 정상이어도 CUDA assert는 비동기라 수백 iter 뒤에
터질 수 있다. multi-scale OOM과 마찬가지로 "초반 정상 = 안전"이 아니다.

**3~6차 점화: 성능 진단 체인.** 0-기반 수정 후 3차는 정상 학습에 들어갔으나
640px/batch16 기준 epoch 3h → 30 epochs $63로 예산(잔액 $19) 초과. 이후 진단:
(a) 512px 축소 → epoch 2.5h, util 29%. (b) MooseFS가 병목이라 보고 27GB를
/dev/shm RAM 디스크로 복사 → **효과 없음** (util 18%, data 시간 0.04s/0.42s
— IO는 애초에 9%뿐). 진범은 컨테이너 CPU 쿼터였다: nproc은 64를 보여주지만
cgroup cpu.max는 13.6코어. DETR 계열 Hungarian 매칭(CPU)이 이 쿼터에 묶여
GPU가 굶는다. (c) 최종: batch 32(512px에서 VRAM 18.8GB 실측 안전) + 10 epochs
+ LR 원복 → 0.57s/it, epoch 1.7h, 총 ~17.5h ≈ $12로 확정. 교훈: RunPod에서
nproc/free는 호스트 수치라 믿으면 안 되고 cgroup(cpu.max, memory.max)을 봐야
하며, GPU util 저하의 원인은 data 시간 필드로 IO/CPU를 먼저 분리해야 한다.

### YOLO 대조군 발산 및 재시작 (2026-07-06)

로컬 MPS 대조군이 epoch 2(mAP50-95 0.049)를 정점으로 val만 붕괴했다
(ep3 0.006, ep4 0.000; val cls loss 2.70→4.17→6.06 지수 증가). train loss는
정상 유지 — 훈련 자체가 아니라 검증/EMA 가중치가 오염되는 패턴으로, MPS +
AMP(fp16) 수치 불안정으로 진단했다. amp=False + 명시적 SGD(lr0=0.01,
cos_lr)로 재시작 (baseline_y11m_v2, epochs 12→10으로 amp off 속도 저하 보상).
교훈: MPS 학습에서 "train loss 정상 + val 지수 악화"를 보면 AMP부터 의심.

### DEIMv2 1차 학습 완료 (2026-07-06)

최종 val mAP@[.50:.95] = 0.161 (AP 궤적: 0.127 warm-start → 증강 딥 0.097 →
회복 → 막판 no-aug/matcher 전환에서 0.141→0.161 점프). 총 비용 약 $18
(RunPod 4090 secure, 재시작 4회 + 전송 유휴 포함), 잔액 $4.20. 체크포인트
best_stg2.pth(280MB)·로그·per-class 결과를 md5 검증 후 로컬 회수하고 pod을
종료했다 (detection/runs/deimv2_m_bridge/).

per-class에서 배운 것: 성능은 박스 수와 비례하지 않았다. Efflorescene은
train 박스 108k로 3위 규모인데 AP 0.054 최하위 — 경계가 모호한 확산형
손상이라 bbox 회귀 자체가 어렵다. 반대로 SteelDefect는 258박스뿐인데 AP
0.361 최고 (시각 변별력 크고 박스가 큼; 단 val 38개라 분산 큼). 결론:
다음 사이클의 leverage는 (1) 오버샘플링+copy-paste (이번 run 미적용),
(2) RDD 병합 (Pothole 0.107 보강), (3) 스케줄 연장 (막판 기울기 +2.0/epoch
남은 채 종료). per-class 평가는 원격 det_solver.val()에 pycocotools
precision 텐서 슬라이스 8줄을 패치해 test-only로 뽑았다.

### YOLO 대조군 2차 붕괴 — LR 과열로 재진단 (2026-07-07)

amp=False 재시작본도 동일 지점(epoch 3-4)에서 val 붕괴 (mAP 0.060→0.0003,
val_cls 3.3→6.5). LR 로그 확인 결과 붕괴 시점이 warmup 종료 후 lr0=0.01 도달
시점과 정확히 일치 — 1차 진단(AMP)은 부분 원인이었고 근본은 MPS+batch32에서
lr0 0.01 과열. epoch 5에서 cosine 감쇠(0.0066)와 함께 회복 조짐(val_cls 4.9).
ep7 mAP<0.03이면 MPS 대조군 포기하고 DEIMv2 2차 완료 후 4090에서 YOLO를
단시간(~4-5h) 학습하는 것으로 대체 예정. 교훈: 같은 recipe라도 백엔드(MPS)가
바뀌면 안정 LR 범위가 다르다. 붕괴 시점과 스케줄 이벤트(warmup 종료, 증강
발동)를 겹쳐 보는 것이 진단 지름길.

### DEIMv2 2차 완주 — 오버샘플링의 비단조 효과 발견 (2026-07-07)

최종 mAP 0.151 (1차 0.161 대비 -1.0). 그러나 per-class가 이 실험의 진짜
수확이다: 중간 강도 개입은 성공(복제 ×20 박락 +64%, RDD 주입 포트홀 +28%),
극단 복제는 역효과(×50 강재부식 0.361→0.178, ×40 도장박리 0.195→0.127).
수백 개뿐인 원본을 50배 복제하면 검출기가 그 인스턴스들을 암기해 같은
클래스의 일반화가 무너진다 — CopyBlend가 복제본을 합성 맥락으로 재노출하며
증폭했을 것으로 본다. 무개입 중간 클래스(누수 +32%, 철근노출 +46%)도 올랐는데
이는 수렴점에서의 2차 annealing 효과일 수 있어 ablation 없이 개입 효과로
단정하지 않는다 (논문 §5.3에 동일하게 기술). 실무 결론: 복제 오버샘플링에는
사용 가능한 창(×20 부근)이 있고, 배포는 클래스별 모델 선택(강재부식·도장은
1차, 나머지는 2차 가중치)이 무비용 최적이다. 총 비용 ~$34/45 GPU-hours.
pod 종료, 체크포인트·로그 전량 로컬 회수(md5 검증) 완료.

### 야간 자율 세션 — 파서·검증 (2026-07-08 새벽)

Phase 2-1 한국 주소 파서를 설계문서(docs/design/address_parser.md)와 함께
구현했다. 상태기계 + 시도 축약 사전(2024 특별자치도 반영), '가' 계열
법정동("태평로1가")이 도로명 접미('로') 판정보다 우선하는 규칙, 행정동
표기("성수1동") 경고 처리. 설계 표 11케이스 + 엣지 4종 전부 통과. 야간
무감독 원칙으로 라이브 API에는 연결하지 않음 (연결은 후속 세션).

Phase 1-6 실주소 검증 (geocode 실호출, 좌표를 실위치와 대조):
서울시청(세종대로 110)·서울역 인근(한강대로 405)·국회(의사당대로 1)·
경복궁(사직로 161) 4건 전부 정확. 처음에 "여의대로 1"로 국회를 찾다
실패했는데 주소 자체가 틀린 것이었다(의사당대로 1) — 검증 케이스의
정답부터 검증하라는 교훈. 하남 케이스는 경기 출입구 적재 완료 후 재검.

### 출입구 전국 적재 완결 + Phase 1-6 완료 (2026-07-09 아침)

출입구 17개 시도 전국 적재 완료 — buildings(juso_entrance) 6,407,095행,
16/16 시도 실패 0. 하남시청(경기도 하남시 대청로 10) geocode가 실위치
좌표를 반환해 Phase 1-6 검증 5건이 전부 닫혔고, 하남 reverse 왕복(신장동)
까지 확인했다. 이로써 지오코딩 v0가 서울 한정에서 전국 커버로 확장됐다.
