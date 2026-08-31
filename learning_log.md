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

### 울트라 세션 — Phase 2·3·4 대량 완결 (2026-07-12)

**3단 폴백 지오코더**: 파서 키 정확 매칭 → 지번 경로(admin_areas→parcels
대표점 — 지번 지오코딩이 이 경로로 성립) → trigram. 정확도 하니스(실주소
200건 × 원문/시도생략/축약/번호붙임 4변형 = 800질의)가 첫 실행에서 14%를
찍으며 중대 결함 둘을 캤다: ① 군 지역 도로명주소의 읍·면이 파서에서 지번
경로로 새서 RoadKey 탈락 ("완도군 군외면 회룡길 64") ② "시덕로249"류 붙임
토큰 미분리. 수정 후 **800/800 = 100.0%** (exact 600 / partial 199 /
trigram 1). 측정이 없었으면 서울 대표주소만 잘 되는 지오코더를 완성이라
불렀을 것.

**성능 함정 둘 다 pg_trgm 임계였다**: 기본 0.3에서 긴 주소 trigram 폴백이
후보 100만 행 → 112초. set_limit(0.55)로 4초. places "약국" 검색은 전역
similarity 정렬이 병목(10초) — 동일/접두/포함 3단 UNION + LIMIT 묶기로
0.16초. 교훈: trgm은 임계·정렬 설계 없이는 규모에서 안 선다.

**Phase 3 초성**: choseong 컬럼 272만 전량 백필, "ㅇㄴㄹㅇㄱ"→온누리약국
0.2초. **Phase 4**(b2d-api 에이전트): auth(SHA-256, scope)/ratelimit(token
bucket key+IP)/meter(논블로킹 CopyFrom)/키 CLI/openapi.yaml — 401·403·429·
계측 실검증 5종 통과. 미해결 4건은 todo 4-2에 Phase 6 감사 항목으로 명기.

**포트 유령**: 하니스 9.5% 미스터리의 진범은 코드가 아니라 "bind: address
already in use" — 구버전 서버가 계속 응답 중이었다. 재기동 검증은 반드시
pgrep으로 PID까지 확인할 것.

## paper_4 트랙 (2026-07-12 ~ 07-16): car-wash 재현 + activation oracle

상세 전과정은 `/Users/momo/interview_mate/car_wash/paper_4/STATE_stage1.md`
(게이트별 기록, 커밋 ed2f1f7~). 여기엔 교훈만.

**STAGE 1 (완료)**: Qwen3-8B에서 answer-before-reasoning 재현 확정 —
walk 오답 커밋 85~100%/조건, thinking 모드로도 안 고쳐짐 (~5%p 구제).
210 롤아웃, LLM-judge 채점, 수동 게이트 20/20.

- **채점기 과적합 루프의 정석 코스를 밟았다**: 정규식 스코어러를 자체
  32케이스에 맞춰 수정(6종) → 신규 4렌즈 홀드아웃에서 62.5% 불일치 →
  사전 임계(>15%) 규칙대로 기각 → LLM-judge 전환 → judge는 미튜닝
  합성 64케이스에서 96.9%로 게이트 통과 후 채택. 교훈: 홀드아웃과
  go/no-go 임계를 **수정 전에** 못박아야 미련 없이 버린다.
- **절단 아티팩트가 결론을 뒤집을 뻔했다**: thinking-on의 "완화 효과"는
  max_new_tokens=1024가 think 중간을 자른 산물. 4096 재생성 후 소멸.
  null 라벨 32건 전수 원문 확인이 잡았다 — 집계만 봤으면 "thinking이
  현상을 고친다"고 논문에 쓸 뻔.
- **하네스 bg 태스크 ~10분 kill**: 이 세션 환경에서 장수 백그라운드
  태스크가 순수 sleep 루프여도 죽는다. GPU 장기작업은 nohup+disown 분리
  + ScheduleWakeup 순찰이 정답. resume(완료 키 스킵) 내장 필수.

**STAGE 2 (진행)**: 공개 AO(Karvonen) oracle LoRA를 로컬 MPS로 —
bitsandbytes 없이 bf16, Colab 불필요.

- **양성통제 먼저, 위치 곡선은 그 다음**: 커밋 텍스트가 보이는 위치(P5)
  조차 못 읽으면 커밋 전 신호는 전부 노이즈. 실제로 1차 P2 50% "신호"는
  P5 3/16과 함께 무너졌다.
- **oracle은 질문 문구에 극도로 민감**: "What answer is this model going
  to give?" 2/16 → "Is the model going to say walk or drive?" 11/16.
  개방형은 LatentQA 분포 밖, 폐쇄형(선택지 명시)이 판독됨.
- **중립 통제 판정 (B)**: oracle 고유 walk율 17% (기본값 drive 83%) —
  walk 편향 우려 정반대. walk 판독만 정보적이란 비대칭 하에서 커밋 전
  walk 판독 15/22=68% vs 기준선 17%. drive 최종답 롤아웃조차 커밋 전엔
  walk-lean 5/6 — STAGE 1 행동 서사("기본 walk, thinking이 늦게 전복")의
  내부 대응물. 교훈: 편향 통제는 결과를 죽이기도 살리기도 한다 — 안
  돌렸으면 drive군 walk 판독을 편향으로 오독하고 버렸을 것.
- STAGE 2 프로빙 종료 (사전 스코프 선). 커밋 ed2f1f7(STAGE 1),
  d5b3e49(STAGE 2), 브랜치 docs/car-wash-repro.

**발행 전 감사 (07-16)**: 사용자의 "이 세팅 어디서 왔지?" 싸한 느낌 →
3렌즈 워크플로 감사 → 18건 확정 (블로커 2: 풀링 Fisher pseudoreplication,
§2.3 수치 오류; regex가 "driveway" 매치하는 오염도 발견). 교훈 셋:
① 층화/집계 통계는 클러스터 구조부터 — 110프로브가 22롤아웃이면 n은 22다.
② 어휘 매칭 regex는 경계 엄밀히 — \w* 하나가 통제 변수를 오염시킨다.
③ "근거 없는 선택값" 전수 훑기는 발행 전 필수 — 세팅이 옳아도 근거가
안 써 있으면 리뷰어에겐 임의값. 다행: 프로브 verdict 0/132 변화,
drive군 결론은 클러스터 보정 후에도 생존 (6/6, p=.002).

### Phase 4 완결 — 검증·감사 (2026-07-18)

4-5 실검증: 9/9 엔드포인트 200, 시청 필지 geocode↔reverse↔parcel 삼각
일치. p95(실사용 유속 14rps): geocode 11/26ms, reverse 67, places 30,
초성 56, parcel 7~11. 동시 20에선 IP리미터(20rps)가 정확히 429 —
부하테스트가 리미터 검증이 된 사례.

4-6 감사(b2d-qa): SQL 바인딩·키 해시·XFF 우회·에러 노출·시크릿 이력
전부 OK. 결함 5건은 6-1로: redact 빈키 문자도배(ReplaceAll("", …)의
Go 시맨틱 — 빈 패턴은 모든 문자 사이 삽입), 버킷맵 무한성장, auth
30s revoke 지연, meter graceful shutdown 부재, GO-2026-5856(go 1.26.5).

교훈 둘. ① 부하테스트에서 429 폭주는 실패가 아니라 방어 작동의 증거 —
단 백엔드 p95는 한도 아래 유속에서 따로 재야 한다. ② redact 도배의
표면 증상("문자마다 ***")에서 ReplaceAll 빈 패턴을 즉시 의심할 것 —
Go의 빈 문자열 매칭 시맨틱은 함정이다. 트리거는 코드가 아니라 기동
절차(.env 미소싱)였다 — 기동 스크립트에 source 명시로 이중 방어.

**paper_4 v2 재배치 (07-18)**: 제출 직후 pre-CoT probing 클러스터 5편
발견 (supervised probe로 >0.9 AUC + causal steering — 우리보다 강한
방법으로 "답의 사전 인코딩" 확립). 논문이 산 이유 = 애초에 우선권
주장을 안 하고 preliminary + 행동 기둥으로 스코핑해둔 것. 재배치 축:
"supervised probe는 되고(그들) 무훈련 oracle은 대체로 안 되는데(Karvonen)
질문만 맞으면 예비 신호가 난다(우리)" 삼각형. 교훈: **인용 검증(가진 것
확인)과 prior-art sweep(놓친 것 탐색)은 다른 게이트다** — 후자 없이
제출했다 뚫렸다. 영구 체크리스트에 추가.

### Phase 6-1 보안 수정 배치 (2026-07-19)

생성(fix-61)/검증(qa-46) 분리로 4건: ① redact 빈키 가드 (ReplaceAll
빈 패턴 도배 차단) ② rate limit 버킷 reaper (30분 TTL, 10분 주기,
시간주입식이라 테스트 결정적) ③ go 1.26.5 toolchain + govulncheck CI
스텝 (GO-2026-5856 해소, No vulnerabilities) ④ meter graceful shutdown
(done chan 드레인; 순서가 핵심 — srv.Shutdown으로 핸들러 먼저 멈추고
채널을 닫아야 send-on-closed 패닉이 없다. 통합검증: 5이벤트 Close 후
usage_log SUM=5, 유실 0, Close 14ms<1s로 드레인 경로임을 증명).
잔존: auth 캐시 30s revoke 지연 (설계 항목, 의도적 미착수).

### Phase 6-2 카카오 정확도 비교 + 만세구 오진단 (2026-07-27)

6-3 배포·요금 문서(`docs/08`)의 판매 근거를 세우려 6-2에 착수. 하니스
`scripts/kakao_compare.sh`: buildings에서 재현샘플(seed 0.42)을 뽑아 b2d
`/v1/geocode`와 카카오 로컬(주소검색)에 동일 문자열을 질의하고, 공식 juso
좌표를 기준으로 거리를 측정한다. 디버깅 둘 — ① psql `-F<TAB>` 문자열 변수의
탭이 word-split로 먹혀 `-F`가 뒤의 `-c`를 구분자로 삼음(함수로 교체) ② curl
`--retry`가 일시 오류 시 부분응답+재시도응답을 이어붙여 멀티 JSON → CSV 행
분리(셸 재시도로 대체). 표면 증상은 "b2d 6% 실패"였지만 실제는 하니스
flakiness였다 — 자기 데이터를 놓친 게 아니었다.

정본 수치(100건): 양쪽 매치 100/100, b2d↔공식 0m(정의상 juso 좌표 반환),
카카오↔공식 중앙값 6.2m·평균 10.6m·max 111m·≤10m 79%. 해석은 정직하게 —
"카카오 오류"가 아니라 기준점 차이(카카오는 자체 지오코딩점). 이상치 수동검증은
위성 대신 b2d의 두 독립 레이어(지적 필지 pnu + 전국 건물)로 대조했다: b2d는 늘
타겟 건물 위, 카카오는 12~60m 이탈, 그중 4건은 이웃 건물에 더 가까워 오도,
최악은 지하주소.

**핵심 교훈 — 만세구 오진단.** 이상치 중 "화성시 만세구"를 보고 "화성시엔 구가
없다"는 2026-01 학습지식으로 예제/픽스처 데이터라 단정했다. 105,684건을 오염으로
규정하고 메모리·하니스 코드(제외 라인)까지 고쳤다. 실제로는 2026-02-01 신설된
화성특례시 일반구(만세·효행·병점·동탄, bcode 41591/93/95/97)였고, 두 공식 juso
다운로드에 모두 있었으며 bcode도 유효했다 — 반증이 손에 있었는데 선입견에 맞춰
읽었다. 컨텍스트에 오늘 날짜가 있고 웹 도구도 이미 썼는데 "내 지식에 없음"을
"존재하지 않음"으로 직행시켜 검증 반사가 안 걸렸다. 사용자가 정정. 롤백(코드·메모리)
후 feedback 메모리 둘로 고정: 최신성 민감 사실(행정·요금·시사)은 웹검증이 기본,
낯선 고유명사를 "가짜"라 부르기 전 공식출처·식별자로 확인, 검증 전 행동 금지.
특히 공식 정부 데이터를 "가짜"라 부르는 건 그 자체로 검증을 의무화하는 강한 주장이었다.

역설적 마무리: 이후 신설-구 표본 검증에서 확인용 `awk '{print $3}'`가 세 번 연속
"전부 한 구"라고 거짓 보고했으나 `grep` 리터럴로 보니 데이터는 4구 균형이었다 —
이번엔 데이터가 아니라 검증도구가 틀렸다. 카카오의 신설-구 처리는 40/40 매치 +
새 구명 정확 에코(중앙값 7.2m)로, 카카오도 개편을 완전 반영했다. 신선도는 이 건의
차별점이 아니다 — 정직하게, b2d 우위는 공식좌표 정합과 0원이다. 증거:
`scripts/out/general_100.csv`(정본 100건), `newgu_40.csv`(신설-구 40건).

### Phase 7-1 지가변동률 ETL 슬라이스 (2026-07-28)

Phase 6 종료 후 Phase 7 상태 점검: 스키마(7-0)·API키(7-키) 완료, 실거래 로더 존재,
엔드포인트 2/6(라이브 프록시). 대부분 실물 데이터 대기라, 유일하게 막히지 않은
지가변동률을 자율 슬라이스로 잡음. **소스 불일치 발견**: 기존 엔드포인트가 쓰던
R-ONE A_2024_00903은 행정구역 "총괄" 변동률이라 스키마(시군구×용도지역)와 안 맞음.
인접 STATBL 탐침·카탈로그 API 모두 용도지역별을 못 찾음 → 사용자가 R-ONE 포털에서
"(월) 용도지역별 지가변동률" JSON을 받아옴.

포털 export 구조: `{sheet:{"1":{data:{...}}}}`, 각 행 col0=id/col1=시도/col2=시/
col3=자치구(또는 시와 동일)/col4=용도지역/col5~14=10개월 변동률. 월 라벨은 데이터에
섞인 헤더 행(col0="No")에 있음. 시군구 5자리 코드가 원자료에 없어 admin_areas(법정동)로
이름→코드 크로스워크 구성(자치구는 "시도 시 자치구" 3토큰 키).

**버그 — map 순서 의존 간헐 실패.** 헤더 행이 3개(월/변동률/% 행 모두 col0="No")인데
로더가 첫 "No" 행에서 break → map 무순회라 변동률·% 행을 먼저 만나면 월 매핑이 비어
"헤더 없음" 에러. 같은 명령 6회 중 2회 실패로 **재현→진단(다중 헤더행)→수정(월이
실제로 파싱된 헤더만 채택)→재검증(6/6 성공)**. Go map range 무순서를 "찾으면 break"
루프에서 잊으면 나오는 전형적 함정.

결과: land_price_index 18,651행, 과천 녹지 3개월이 R-ONE 값과 정확히 일치. 원칙 준수 —
용도지역 분류 원자료 그대로, 집계/(구)레거시/미매핑 행 스킵, 계산·판단 없음.
소스 파일은 gitignore된 `data/reb/`에 보관(공공데이터 비추적 규칙).

슬라이스 완결(API): `/v1/appraisal/price-index`를 틀린 라이브 프록시(A_2024_00903 총괄)에서
land_price_index **DB-backed**로 교체. sigungu×용도지역×from/to 월별 배열 + cumulative
((1+r/100)의 곱, 산식·기간 병기 — 지시서 허용 계산, 평가·판단 없음). 검증: 과천 녹지
202604~06 누적 1.657%(=0.363/0.554/0.731 곱), geo 스코프 키는 403. 교체로 고아가 된
RebKey/REB_API_KEY 배선을 struct/New/main에서 제거(내 변경이 만든 orphan만 정리).
이제 ETL→DB→API 수직 슬라이스가 하나 완결됨.

### Phase 7-1 표준지공시지가 ETL (2026-07-28)

사용자가 NSDI 표준지공시지가 번들(169MB, 시도별 AL_D152 nested zip)을 받아옴. go-shp
인스펙터로 실물 필드 확인(29필드 A0~A28): A0=PNU/A7=기준연도/A9=㎡당공시지가/A13=지목/
A16=용도지역/A18=이용상황/A20=형상/A22=도로접면/A14=면적/A23=기준일. 지세(고저)는 원자료에
없어 NULL. 표준지는 **폴리곤**(점 아님)이라 geom(Point)은 ST_PointOnSurface로 대표점 산출,
EPSG:5186→4326 변환(jijeok go-shp·polygonWKT 패턴 재사용).

**버그: DBF 폭 초과 마스킹.** 충북 로드 중 area_sqm에 `"******************"`(DBF 숫자필드
폭 초과 표기) → numeric 캐스트 실패로 중단(9시도 후). 유효 숫자 아니면 NULL 처리(numOrNil)로
수정 후 완주. price/base_year는 이미 파싱 실패 시 skip이라 무사.

결과: standard_lots **596,986행 17시도**, skipped 0, geom 한국 경계 내 100%. 검증:
세종 표준지 전필드가 shapefile 원값과 일치. 번들은 gitignore된 data/에 보관.
**7-1 ETL 3종(실거래·지가변동률·표준지) 전부 완료.**

이어서 비교표준지 엔드포인트(`/v1/appraisal/standard-lots`, 7-3): 앵커(pnu는 parcels에서
ST_PointOnSurface, 또는 lat/lng) 반경 내 표준지를 거리 오름차순으로. 정렬·필터는 객관
지표(거리)만 — 유사도/추천 점수 없음(절대경계). 성능: `geom && ST_Expand(anchor, radius/88000)`
GiST bbox prefilter + geography ST_DWithin 정밀 → 596k행에서 7ms. 검증: 세종 표준지 앵커 시
자기 0m + 인근 148/156m, use_zone 필터, 서울 좌표 앵커 96.6m, geo 키 403. 7-3 = 3/6.

official-price 엔드포인트(`/v1/appraisal/official-price`): PNU의 공시지가를 개별공시지가
(official_land_prices, INDIVIDUAL) 우선, 없으면 표준지(standard_lots, STANDARD)로 폴백.
base_year 미지정 시 최신. 개별은 아직 빈 테이블이라 현재는 표준지 PNU만 STANDARD로 응답,
7-2에서 개별 적재되면 INDIVIDUAL이 자동 우선. 검증: 세종 표준지→STANDARD 878600/2026,
base_year=2025→404, 없는 PNU→404, geo 403. 7-3 = 4/6.

### Phase 7-4 독립 감사 (2026-07-28)

생성/검증 분리 원칙대로 Phase 7 감정평가 코드를 **독립 b2d-qa 에이전트**가 적대적 감사.
결과 PASS: ① 판단로직 부재 — 사실만, cumulative는 정확히 ∏(1+r/100)-1 재현(자체 recompute
일치), standard-lots는 거리순뿐 유사도/추천 없음 ② 보안 — SQL 전부 바운드, 키 이중 redact,
scope traversal까지 우회 불가, 에러 누출 없음 ③ 무결성 — geom 596,986건 100% 한국내, 환각좌표
없음. 실이슈 4건 중 3건 수정:
- **MED(진짜 스펙위반)**: official-price를 개별 우선으로 짰는데 지시서 §4는 "표준지 우선,
  없으면 개별, 미적재 지역은 REGION_NOT_LOADED". 개별이 시군구 선택 적재라 전국 필수인
  표준지가 신뢰 소스 — 스펙이 옳았다. 표준지 우선 + 422 REGION_NOT_LOADED로 수정.
- LOW: standard-lots bbox prefilter 상수 /88000이 북위 고위도서 E-W 경도폭 과소추정
  (경험상 0건 누락이나 잠재 false-neg) → /80000. lat/lng 앵커 한국 범위 검증 추가.
- 미수정(스코프 밖): writeErr 문자열 envelope vs 미들웨어 객체 envelope 불일치 — Phase 7
  이전 기존 코드라 전 엔드포인트 계약 변경 회피.

교훈: 생성자(나)가 개별 우선을 "상식적"이라 넣었지만 이 시스템의 데이터 정책(개별 선택적재)
때문에 스펙의 표준지 우선이 정답. 독립 감사가 생성자의 그럴듯한 오판을 잡았다 — 생성/검증
분리의 가치. 남은 실무 시나리오 전구간 정부서비스 대조는 나머지 엔드포인트+브라우저 필요.

### Phase 7-2 land-use ETL + 설계 결정 (2026-07-29)

사용자가 토지이용계획공간정보(NSDI AL_D154, 9.6GB, 시도별 nested SHP) 받아옴. 실물 확인 결과
스펙 가정(use_zones=용도지역 폴리곤)과 달리 **필지별(PNU) 사전판정형**: 각 필지에 걸린
용도지역/지구/구역이 콤마구분 코드(A7)·명칭(A8)·관계(A10=포함/저촉/접함)로 이미 판정돼 있음.

**설계 결정 — Option A(필지별 직접 적재) 채택.** 저촉 면적비(%) 없는 trade-off:
1. 대부분 필지는 포함(면적비 100% 자명), 면적비는 소수 경계필지에만 의미.
2. A도 "저촉" 사실은 정부 공식 판정 그대로 전달 — 평가사가 경계에 걸침을 앎.
3. 잃는 정확한 %는 평가사가 지도/토지이음으로 확인(판단은 평가사 몫).
4. 역설적으로 B(폴리곤+ST_Area)의 %는 우리 임계값 판정이라 덜 권위적 — A가 판단금지에 부합.
5. 하이브리드 가능: 나중에 용도지역 폴리곤 추가하면 % 보강. 막다른 길 아님.
→ 사실반환·판단금지 절대경계엔 정부 공식 판정을 그대로 쓰는 A가 더 맞다. (메모리:
land-use-option-a-decision)

**구현:** 새 마이그레이션 011 land_use_plan(pnu, zone_code, zone_name, relation) — **geom 미저장**
(엔드포인트는 PNU 조회만 → 9.6GB 폴리곤 버려 가볍게). 로더 internal/etl/landuse:
A7/A8/A10 콤마분해해 필지×용도지역 행으로 upsert. relation은 verbatim(포함/저촉/접함 — 접함이
있어 하드코딩 안 한 게 다행). 콤마 정렬 어긋난 필지(명칭 내부 콤마)는 스킵.

**세종 검증:** 206,995 필지 → 1,010,982 행, misalign 948(0.46%), badmeta 0. 샘플 PNU 4개 zone이
shapefile 원값과 일치. relation 분포 포함 82.5만/저촉 14.5만/접함 3.2만.

**SHP 삽질 전말 (SHP → CSV 피봇, 2026-07-29).** 위 SHP(AL_D154) 접근은 결국 CSV로 교체됐다.
전 과정과 에러를 기록한다:

1. **세종만으론 괜찮아 보였다** (misalign 0.46%). 전국 적재 시작하자 두 버그 폭발:
   - **0x00 인코딩**(경북): dcz가 CP949 디코드 실패 시 raw 바이트를 그대로 넣어 Postgres UTF8
     거부(SQLSTATE 22021, `0xc0 0xd3`). → `ToValidUTF8`로 무효 바이트 제거.
   - **콤마 대량 어긋남**(서울 misalign 813,830 vs 필지 84,676 = 90% 스킵!): A8(명칭)에 내부
     콤마(`지구단위계획구역(상도지구, 문의:...)`, `토지거래계약...(대상자: 외국인 등, 용도:
     단독, 다가구, ...)`) + **254자 truncation**(`토지거`에서 잘림). A8 split 불가.
2. **A7/A10만 신뢰** 판단(코드·관계는 콤마 없어 항상 개수 일치). zone_name은 A8에서 못 뽑으니
   **코드→명칭 맵**으로 우회. 시도한 것들: (a) 정렬 레코드에서 first-seen 맵 → `ZQ0001→다세`
   조각 오염. (b) A8 254 가득 참 레코드 제외(truncation 스킵) → `ZQ0001→아파` 여전. (c) 다수결
   argmax → `ZQ0001→다가`(ZQ가 항상 A7 마지막이라 정상 명칭이 아예 클린 레코드에 없음). (d)
   cleanName 필터(2자·숫자접미 제거) → `ZQ0001→토지거래`(4자 조각이 필터 통과). **A8 fragment는
   정상 명칭과 구분 불가 — 근본 해결 불가.** + 속도 위해 batch→CopyFrom+staging까지 갔음.
3. **CSV가 정답이었다.** 사용자가 토지이용계획**정보**(AL_D155) CSV를 받아옴: **한 행 = (필지 ×
   용도지역) 하나**, 따옴표로 콤마 처리 + 254 truncation 없음 → 명칭 온전. 컬럼 0=PNU, 8=관계,
   9=코드, 10=명칭, 12=기준일. 로더를 CSV 직접 파싱으로 전면 교체(CP949 transform reader +
   encoding/csv, 최신 날짜만, UNLOGGED staging에 CopyFrom → DISTINCT ON dedup). 세종 검증:
   1,215,098행, **ZQ0001=중점경관관리구역**(클린!), 오염 9/205코드.

**핵심 교훈 — 내 미스.** Option A(geom 미저장)로 갈 거면 **CSV가 애초에 정답**이었다. 필지별
사전판정형이라 geom이 불필요했는데, 나는 "SHP만, CSV 불필요"라고 잘못 안내했다(SHP의 .dbf에
속성이 있으니 된다고 봤으나 254 truncation·콤마-패킹으로 깨짐). 9.6GB SHP는 폴리곤 다 버려서
과했고, truncation 삽질(4~5회 재작성)을 자초했다. **속성만 필요하면(no geometry) CSV > SHP.**
데이터 형태를 실물로 확인한 뒤에도(필지별 사전판정 = geom 불요), 다운로드 포맷 재추천을 안 한 게
실수. 이후엔 "이 산출물에 geometry가 실제로 필요한가?"를 포맷 추천 전에 먼저 물을 것.

**전국 적재 성능 문제 (2026-07-29).** CSV 로더로 전국 적재 시 규모가 컸다: 17시도 → UNLOGGED
staging(lu_stg) **262,280,096 행**(전국 필지×용도지역). 문제는 마지막 단계
`INSERT ... SELECT DISTINCT ON (pnu,zone_code) ... FROM lu_stg ORDER BY pnu,zone_code`가
기본 work_mem(4MB)로 262M 행을 디스크 외부정렬 → **3시간 42분째 안 끝남**(land_use_plan 0 유지).
staging은 이미 끝났으므로 프로세스를 kill(INSERT 롤백, lu_stg 보존)하고 dedup만 고성능으로 재실행:
`SET work_mem='1GB'; SET maintenance_work_mem='2GB'; ALTER TABLE ... DROP CONSTRAINT pkey;`
→ DISTINCT ON INSERT(정렬 청크 커져 merge 패스 급감) → `ADD PRIMARY KEY`(빈→벌크 인덱스 빌드,
행별 유지 회피) → `DROP lu_stg`. 교훈: **1억+ 행 dedup은 work_mem 상향 + PK 일시제거(벌크 인덱스)가
필수.** 향후 대용량 로더는 처음부터 이 패턴으로 짤 것(또는 COPY 전 Go에서 dedup 불가 시 staging+
고성능 dedup을 로더에 내장).

**dedup 메모리 — OOM·좀비 실경과 (2026-07-29).** 위 "work_mem 1GB/2GB"는 실패했다. 컨테이너
메모리 한도가 ≈1.9GB인데 `work_mem='2GB'`로 dedup 정렬 → **Postgres OOM 크래시**. UNLOGGED
lu_stg(262M staged)는 크래시 복구 시 truncate로 날아갔다(standard_lots는 무사). 재적재 후
`work_mem='384MB'` + `maintenance_work_mem='384MB'`로 낮춰(컨테이너 한도 내) 안전화 →
성공. 또 하나: 로더 클라이언트를 `pkill`해도 **DB 백엔드는 INSERT를 계속 실행**(락 보유, 4h30m
잔존) → `pg_terminate_backend`로 서버측 종료 필요했다. 교훈: **work_mem은 컨테이너 물리한도
아래로** 잡을 것(2GB>1.9GB=OOM). 클라이언트 kill ≠ 서버 쿼리 중단.

**전국 적재 완료 (2026-07-30).** 최종 `land_use_plan` = **222,379,474행**(staged 262,280,096 →
DISTINCT ON dedup, 384MB work_mem 외부정렬 ~1시간+, PK 일시제거 후 벌크 재생성, lu_stg drop).
skipped=0. zone_name 클린(과밀억제권역·농업진흥구역·보전산지 등). 16개 현행 시도.

**"누락 시도"의 함정 = 광주·전남 행정통합 (2026-07-30 검증).** 검증 중 시도코드 분포가
11,12,26,27,28,30,31,36,41,43,44,47,48,50,51,52 — **광주(29)·전남(46) 없고 비표준 "12" 존재**.
`admin_areas`·`parcels` 두 참조테이블에 12가 없어 "고아/가짜"로 판단하고 **제외를 권고했다 —
오판.** 사용자가 "믿어도 되냐, 다시 확인해봐"로 제동. 소스 CSV `법정동명` 컬럼 확인 →
**"전남광주통합특별시 목포시…"**: 12 = **광주광역시+전라남도 통합 신설 특별시**(2026-07-08 기준,
시군구 27=광주5구+전남22시군, 데이터에 목포시·동구 공존). 즉 광주/전남은 누락이 아니라 12로
통합됐고, 로더의 "전역 최신 날짜" 로직은 오히려 통합시를 정확히 잡았다. **제외했으면 현재
광주+전남 데이터를 통째로 삭제할 뻔.** 교훈([[verify-recency-sensitive-facts]]): 행정개편은
학습지식·기존 참조테이블로 "가짜" 단정 금지, **원자료(법정동명)로 확인**. 참조테이블 자체가
개편 전이라 stale일 수 있다. 후속: `parcels`/`admin_areas`를 통합후로 갱신하거나 29/46→12
매핑 필요(안 하면 광주/전남 지역 API가 land_use(12) ↔ parcels(29/46) 크로스 불일치).
메모리: [[gwangju-jeonnam-merger-2026]].

**연속지적도 다중 shapefile 잠복 버그 (2026-08-01, Phase 5-1).** parcels가 8개 대용량 도에서
정확히 100만 행씩 = 도당 정확히 1,000,000은 자연수치일 수 없음 → 조사. 처음엔 "1M 상한"이
로더 코드나 DB에 있는 줄 알았으나 로더엔 없음. **실물 확인**: 시도 zip을 열어보니 대용량 도는
V-World가 **100만 레코드 단위로 shapefile을 여러 벌**로 쪼갬(경기·전남·경북 6벌, 경남 5, 충남·
전북 4, 충북·강원 3). `.shx` 크기로 벌당 정확히 1M 확인. 근본 원인: `extract()`가 zip 안 모든
shp/dbf/shx를 **같은 `layer.*`로 덮어써** 마지막 1벌(=1M)만 go-shp가 읽음. 재run으로도 안 고쳐질
잠복 버그였다. **수정**: extract()가 원래 basename 보존해 전부 풀고 모든 .shp 반환 →
`loadShapefile` 헬퍼로 각 shapefile upsert. 재적재: **13.3M → 39,728,087필지**(전국, skipped=0).
대량 upsert 전 보조 인덱스(geom GiST·bcode_jibun) 임시제거 → 로드 → 재생성으로 가속(pnu pkey는
ON CONFLICT용 유지). 교훈: **"정확히 라운드넘버(1M)" = 인위적 절단 신호**, 원자료 구조를 실물로
열어볼 것. 압축 안에 동일 확장자 다수 = 덮어쓰기 버그 의심.

**reverse 법정동 폴리곤 ST_Contains (2026-08-01, Phase 2-4).** 원래 revgeo는 최근접 건물의
bcode로 법정동을 대체했다(admin_areas.geom이 전부 NULL — 구역 폴리곤 미적재). 2-4의 "법정동
폴리곤 ST_Contains"가 미구현이었음. 5-1로 parcels가 전량(39.7M, geom+GiST) 확보되자 별도 구역
폴리곤 없이 **필지 포함관계**로 법정동을 정확히 얻을 수 있게 됨(dissolve 불필요). revgeo.ByPoint를
쿼리 2개로: ① 건물 KNN(도로명주소·거리) ② 포함필지 ST_Contains(법정동 bcode·명), 포함필지
없으면 건물 bcode 폴백. LATERAL 한 쿼리로 합치면 후보 건물마다 ST_Contains가 반복돼 느려서 분리.
검증: GiST 5ms, 다지역 정확, 종로구 경계에서 건물 bcode≠포함필지 bcode divergence 실증(점의
법정동은 그 땅=필지 기준이 맞음).

**API 잔여 3종 (2026-08-01, Phase 4-1).** ① geocode batch: POST /v1/geocode/batch(최대 50),
단건 geocode.Search를 항목별로 돌리고 한 항목 실패는 그 항목 error에만 담아 배치는 200 유지.
② places category: Search에 category 파라미터 추가, 3단 UNION 각 branch·초성경로에
`AND ($n='' OR category_code=$n)` — 빈값이면 무필터. ③ /v1/usage: usage_log를 `auth.APIKeyID(ctx)`로
호출자 키에만 스코핑 집계. **설계 판단**: usage는 메타(자기 사용량)라 geo/realestate 어느 스코프든
접근돼야 함 → `requiredScope("/v1/usage")=""` + `hasScope(_,"")=true`(scope 불요=인증만). scope=""는
usage 경로에만 매핑되므로 다른 엔드포인트 격리(realestate→geo=403) 유지 — E2E로 확인. openapi.yaml
11 paths로 증보. auth는 보안 미들웨어라 기존 테스트 회귀 통과 확인 후 변경.

**places 반경 필터 (2026-08-01, Phase 3-1 마감).** placesSearch에 lon/lat/radius 추가. SQL에서
`AND ($6 <= 0 OR ST_DWithin(geom::geography, ST_MakePoint($4,$5)::geography, $6))` — radius≤0이면
필터 무시(Go 분기 불필요, 한 쿼리로 활성/비활성). 핸들러는 lon·lat 둘 다 파싱되면 radius를 기본
1km·1..20km로 클램프, 아니면 0. 반경조건을 3단 UNION 각 branch·초성경로에 넣음. **성능**: geography
캐스트가 geom GiST를 직접 못 써도, 키워드 필터(exact/prefix/ILIKE + tier별 LIMIT)가 후보를 이미
좁혀 반경은 그 소수에만 계산 → 저비용. E2E: 강남역 반경500m 약국 반환점 최대거리 381m(≤500 확인),
미지정 시 전국. 교훈: 필터 on/off는 `$param<=0 OR <cond>` 한 쿼리 관용구로 분기 회피.

**places 재현율 검증 (2026-08-02, Phase 3-2).** 실존 상호 20건(전국·다카테고리 랜덤)을 이름으로
검색해 name+road_address 정확일치로 회수 판정 → **19/20(95%)**. 유일 미스 "색연필"을 그냥
"실패"로 넘기지 않고 규명: 정확동명이 **30개**라 exact-tier가 limit 10에 걸려 특정(춘천) 건이 밀린
것 — 검색 결함이 아니라 모호성. 위치필터(그 점 반경1km)로 재검색하니 정확히 1건 회수 → **위치맥락
있으면 20/20**(3-1 반경필터의 실효성 입증). 교훈: 재현율 미스는 숫자로 끝내지 말고 근본(모호성 vs
정규화 vs 인덱스)까지 파고들 것([[rigor-verify-before-concluding]]).

**필지 정부대조 (2026-08-02, Phase 5-4).** 실필지 3건(서울·수원·제주 시청 지번) 토지이음 대조:
지목 3/3 "대" 일치, 면적은 우리 도형면적(연속지적도 폴리곤 ST_Area) vs 정부 공부상면적이 ±0.3%
이내(서울 12,738.9 vs 12,709.4 = +0.23%, 수원 21,677.0 vs 21,677.4 = -0.002%, 제주 15,468.4 vs
15,426.1 = +0.27%). 도형↔공부 미세차는 정상(측량 반올림). 수원(신규 적재 경기)이 거의 완전일치라
5-1 다중 shapefile 픽스로 채운 필지의 지오메트리 정확성까지 확인됨. PNU는 토지이음에 잘 안 뜨지만
법정동코드+본번+부번 조립이라 지목·지번·면적 일치로 필지 동일 확정.

**개별공시지가·토지특성 ETL + land-features 엔드포인트 (2026-08-02, Phase 7-2/7-3 완성).**
사용자가 서울(sido 11) 실물 확보: 개별공시지가 AL_D151(단일 CSV), 토지특성 AL_D195(중첩 zip 3날짜).
land-use 교훈대로 **코드 짜기 전 CSV 헤더 실측** → 컬럼 확정. 매핑 판단: land_features.land_use ←
**토지이용상황(col17, "상업기타"·"연립")** 이지 용도지역이 아님(용도지역/지구는 이미 land_use_plan).
doc07이 토지특성 4속성을 "토지이용상황·지형고저·지형형상·도로접면"으로 명시 → 4필드 1:1. **원값 한글
라벨 그대로 적재**(재분류 금지). 로더 2종(officialprice·landfeature): UNLOGGED staging + CopyFrom(CP949)
+ INSERT ON CONFLICT(pnu,base_year) DO UPDATE — **지역선택 적재라 TRUNCATE 금지**(다른 지역 보존).
파티션 테이블(official_land_prices)에 ON CONFLICT 정상 작동(base_year가 파티션키+PK). 적재: 개별공시지가
887,699행·토지특성 888,806행(서울, skipped 극소). land-features 엔드포인트는 appraisalLandUse 패턴
(PNU 검증 → 조회 → 미적재 시 substr(pnu,1,5) EXISTS로 REGION_NOT_LOADED 422). E2E: 서울시청 필지
개별공시지가 65,730,000원/㎡ INDIVIDUAL 활성·토지특성 상업기타/평지/세로장방/광대소각·수원(미적재)
REGION_NOT_LOADED. openapi 15 paths(appraisal 6종 문서화). 7-2/7-3 완성. 잔여: 다른 시군구 확장·7-4 실무대조.

**실무 시나리오 전구간 정부대조 (2026-08-02, Phase 7-4 완료).** 서울 실주소 2건(세종대로 시청·
역삼동 838)을 전구간(geocode→parcel→official-price→land-use→land-features)으로 돌리고 사용자가
토지이음 토지이용계획 확인서 PDF를 받아 대조. **개별공시지가 2건 다 원단위까지 완전일치**
(65,730,000·58,300,000원/㎡, 2026) — 개별공시지가 로더·official-price INDIVIDUAL 활성 완전 검증.
용도지역도 전항목 일치(일반상업지역·과밀억제권역·문화유산·지구단위계획·광로 등 중첩 지정 모두;
우리 데이터는 '접함' 관계까지 보유). 지목 대·면적 도형 vs 공부 ±0.3%. 토지특성(이용상황/지형/도로접면)은
토지이용계획 확인서엔 없어(부동산종합증명서 대상) 정부 교차대조 미포함이나 로더 적재값은 검증됨.
Phase 7(감정평가 모듈) 완료 — 서울 파일럿 스코프, 다른 시군구는 실물 확보 시 동일 로더로 확장.

**감정평가 경기 확장 + 로더 형태 유연화 (2026-08-02).** 경기(41) 개별공시지가(AL_D151_41
4,937,955행) + 토지특성(AL_D195_41 4,949,721행) 적재, 서울 upsert 보존(경기만 추가). **발견**:
토지특성 zip 형태가 다운로드마다 다름 — 서울은 중첩 zip(토지특성정보.zip→AL_D195_11_*.zip→CSV),
경기는 **직접 CSV**(AL_D195_41.zip→CSV). landfeature 로더를 둘 다 처리하게 유연화: dateOf가 .csv도
인식(filepath.Ext), 확장자로 loadNested(중첩)/loadCSVEntry(직접) 분기, 파싱 루프는 loadCSVReader로
공유. (개별공시지가는 서울·경기 둘 다 단일-CSV zip이라 무변경.) 성능: dedup DISTINCT ON이 기본
work_mem(4MB)라 5M행 외부정렬 ~10분/파일 — 대량 시도 다수 확장 시 work_mem 상향 최적화 여지.
검증: 수원시청 REGION_NOT_LOADED→INDIVIDUAL 5,799,000원/㎡ + land-features, 서울 값 불변. 교훈:
같은 데이터셋도 다운로드 경로/시점 따라 압축 구조가 달라질 수 있다 — 로더는 형태에 유연하게.

**TC-V 실행 + REGION_NOT_LOADED seq-scan 버그 (2026-08-02, Phase 7-3 TC-V).** 감정평가 6종
테스트케이스(설계 시점 작성분)를 실호출로 실행: V02 부산(미적재) official-price 조회가 **HTTP 000
(타임아웃)**. 진단: 미적재 판정 `EXISTS(... WHERE substr(pnu,1,5)=$1)`가 substr 함수라 pnu PK
인덱스를 못 쓰고 **seq scan** — 서울만(888K)일 땐 빨랐는데 서울+경기(5.8M) 누적 + 부재 지역(부산)이라
전체 스캔(EXPLAIN: Rows Removed 5,825,654) → 10s 초과 → net/http가 연결 끊어 000. 수정: `pnu BETWEEN
prefix+'0'×14 AND prefix+'9'×14`(PK 범위)로 교체 → **Index Only Scan 0.05s**. official-price·land-use
(222M!)·land-features 3핸들러 공통 `hasRegion` 헬퍼로. 교훈: **함수를 씌운 컬럼 조건(substr/lower/cast)은
인덱스를 못 탄다** — 프리픽스 매칭은 PK 범위로. 테이블이 커질수록(지역 확장) 표면화되는 잠복 성능버그.
TC-V 나머지: V08 cumulative 누적 0.5147% 수기검산 일치, V09 코드에 금지 판단로직 0건, V10 scope 403,
V06 거리순+점수필드 부재 — 전부 통과. **TC-V가 실버그를 잡음(설계TC 실행의 가치).** V03/V07은 Option A
대체(use_zones 미사용, 저촉 면적비 미제공), V04/V05는 rt.molit 브라우저 대상.

---

## 2026-08-06 — 웹 프론트·전국 배포·M1 이전 준비 (Opus 4.8 세션)

### 웹 프론트 + 공개 배포 (b2d_geo API 실서비스화)
- **프론트 3종 + 배포**: `b2d-server`가 API(`/v1`)와 정적 사이트를 같은 오리진에서 서빙(embed).
  랜딩(`/`)·지도데모(`/map.html`, Leaflet+필지폴리곤·공시지가·용도지역·토지특성·실거래·표준지)·
  API문서(`/docs.html`, try-it-out+셀프키발급)·배치(`/batch.html`). PR #1~#6 머지, main green.
- **VPS 배포**: AWS Lightsail 서울(3.35.88.130, 8GB/2vCPU/160GB) + docker-compose(db+app+caddy)
  + Cloudflare DNS(api.birth2death.com, 회색구름) + Caddy 자동 TLS. **전국 61GB 라이브.**
  운영/인수인계 문서 = `deploy/OPS.md`, `deploy/README.md`.
- **잡은 실버그**: (1) `.dockerignore` allowlist에 `web` 누락 → embed용 web/이 빌드컨텍스트서
  빠져 이미지 빌드 실패(호스트 go build로는 안 잡힘). (2) x/text GO-2026-5970 취약점 → v0.39.0.
- **복원 교훈**: pg_restore를 `docker compose exec -T`(attached)로 돌리면 SSH 끊길 때 딸려 죽어
  좀비 워커 잔존 → **detached(`exec -d`)로 돌려야 세션 무관하게 완주.** 2vCPU에 `--jobs=4`는
  I/O·IPC 과경합 → 마지막 text btree(idx_buildings_norm_road)가 병렬로 수시간 헛돎 → **단일스레드
  재빌드가 정답.** 전국 각지(부산·광주·제주 등) 실호출로 데이터 완전성 검증.

### M1 이전 + 대용량 백업 (77GB → clone 4.5MB)
- **핵심**: 저장소는 4.5MB(코드), 나머지 77GB는 전부 gitignore된 재생성 가능 데이터.
  M1(20GB)엔 clone만 하면 되지만, 사용자는 키 등 편의로 디렉토리 통째 이동 선택.
- **ML 모델(진짜 소중, 재생성 불가)**: DEIMv2-M 주력(best_stg2.pth 280M, val mAP 0.161) + v2 +
  YOLO11m 베이스라인(로컬 MPS ~39h) → **HF 비공개 백업 `momoJo/b2d-bridge-detection`.** 전부 검증.
- **74GB(AI Hub 040·변환이미지·ETL data)**: 전부 공개 재다운 가능 → **삭제 결정**(R2 업로드 대신).
  재다운·재훈련 문서 = `detection/RESTORE.md`(ML), `docs/10_Data_Reacquisition_b2d_geo.md`(ETL).
- 교훈: 백업 대상 선별 = "재생성 불가한 것만"(모델 1.5GB). 재생성 가능 공개데이터(74GB)는
  출처만 문서화하고 삭제. DB는 이미 VPS에 있어 ETL 원본 불필요(스냅샷 복제가 재적재보다 빠름).

## 2026-08-16 — Track A 상용화 Phase 1: 아웃리치 자산 (Fable 5 세션)

### 산출물
- **요금/제품 원페이지** `web/public/pricing.html`: 배포 3형태(관리형 API 구독 /
  온프레미스 연 라이선스 / 벌크 배치) + 적재 데이터 표 + 검증 수치 + 문의 CTA.
  가격은 전부 `{PRICE_*}` 자리표시자(사용자가 확정). 온프레미스 핵심 문장
  "주소 데이터가 고객 인프라 밖으로 나가지 않습니다"를 카드 첫 문장으로 배치.
  개별공시지가·토지특성의 서울·경기 우선 적재와 REGION_NOT_LOADED를 숨기지 않고
  "계약 시 해당 지역 우선 적재"로 로드맵 프레이밍. 내부 인프라 정보(서버 스펙·DB 용량)는
  공격 표면이므로 배제하고 데이터 규모 수치만 노출.
- **네비게이션 연결**: index·docs 상단바에 "요금" 추가, index 푸터의 기존
  `/docs.html#pricing` 링크를 `/pricing.html`로 교체, docs 하단 요금 섹션에 페이지 링크 추가.
  아웃리치를 받은 사람이 메인에서 요금 페이지를 못 찾으면 문안의 신뢰가 끊긴다는 이유.
- **아웃리치 문안 3버전** `outreach/{proptech,logistics,appraisal}.md`: 전부 DRAFT 표기,
  발송용 확정은 사용자가 별도 수행. 타깃 리스트 골격 `outreach/targets.md`(회사명 기입 금지).

### 핵심 발견 — 카카오 단가 검증에서 지시서 전제가 일부 틀렸다
- 지시서의 "정상가 50원/한시 할인 10원(2026-12-31 종료)"을 developers.kakao.com 쿼터
  페이지에서 검증한 결과, 그 단가는 **카카오톡 공유(메시지) API**의 것이고 지도/로컬 API가
  아니었다. 실제 지도 계열 단가: **주소로 좌표 변환 0.5원/건, 좌표로 주소 변환 0.5원/건,
  키워드로 장소 검색 2원/건, 지도 SDK 로딩 0.1원/건**. 무료 쿼터는 월 300만 건이되
  카카오맵은 개발자 계정의 첫 활성화 앱에만 제공.
- 따라서 "할인 종료 임박" 훅은 지오코딩 문안에 쓰면 팩트체크 한 번에 무너진다. 문안은
  검증된 숫자로만 재구성: 물류 버전 훅은 "월 3,000만 건 기준 월 1,350만 원(무료 300만 건
  공제, 0.5원/건)", 프롭테크 버전은 비용이 아니라 깊이(0.5원짜리 좌표 변환의 가격표에
  없는 필지·공시지가 체인), 감정평가 버전은 조회 이력 주권(온프레미스)으로 훅을 잡았다.
- 교훈: 아웃리치 단가는 반드시 발송 직전 공식 페이지에서 API명 특정으로 재확인.
  각 문안 상단에 단가 출처와 확인 일자를 명기해 두었다.

### 검증 방식
- pricing.html은 파일 생성이 아니라 **헤드리스 Chrome 렌더링 스크린샷**으로 검증.
  1차 렌더에서 좁은 카드 안 price-table의 한글이 글자 단위로 세로 낙하하는 문제 발견 →
  포함 내역을 플랜 셀 하위 muted 줄로 이동 + 페이지 한정 `word-break: keep-all`로 해결.
  index·docs 상단바의 "요금" 링크도 렌더링으로 확인.
- 문체 규칙은 grep으로 전수 점검: em-dash·인사 오프너·"국내 유일"·이모지 전부 부재 확인.
- GitNexus detect_changes는 실행 불가(로컬에 .gitnexus 러너 없음, npm 깨짐). 이번 변경은
  HTML/JS 프론트와 md 문서만이라 Go 심볼 영향 없음.

## 2026-08-16 — Track A Phase 2: 셀프서브 최소 과금 구조 (Fable 5 세션)

### 무엇을 만들었나
- **스키마**: `migrations/012_api_key_tiers.sql` — api_keys에 `tier`(demo|standard)와
  `monthly_cap`(0=캡 없음) 두 컬럼. 기존 키는 기본값(demo, 0)으로 남아 동작이 변하지 않는다.
- **월 하드캡**: auth 미들웨어가 캡 있는 키에 한해 usage_log의 이달(UTC) 합계를 조회해
  캡 도달 시 429 `monthly_cap_exceeded`와 pricing.html 업그레이드 안내를 반환한다.
  새 카운터 테이블을 만들지 않고 기존 meter의 usage_log를 그대로 재사용했다
  ((api_key_id, bucket) 인덱스가 이미 있어 한 달 범위 SUM으로 충분).
- **발급 경로 확장**: `auth.CreateAPIKey`에 tier/monthlyCap 파라미터 추가, CLI
  `apikey create --tier --monthly-cap`, 셀프 데모키 발급은 (demo, 0) 고정.
  티어는 화이트리스트 검증(없는 티어명은 발급 단계에서 거부).
- **가시성**: /v1/usage 응답에 tier, monthly_cap, month_used, (캡 있으면)
  month_remaining 추가. openapi.yaml 갱신.
- **운영 문서**: deploy/OPS.md 7-1절 — 토스 정기결제 링크 수동 플로우(결제 확인 후
  CLI 발급, 미납 시 revoke). 가격·rate·캡은 {PLACEHOLDER}로 두고 사용자 확정 대기.

### 검증에서 잡은 실버그 — 캡이 잔여량 확인까지 잠갔다
로컬 PostGIS(docker compose)에서 cap=3 키로 e2e를 돌리자, 4번째 호출의 429는 정상인데
**/v1/usage(잔여량 확인)까지 429가 났다.** 셀프서브 구매 이유가 요금 예측 가능성인데
캡에 도달한 고객이 자기 잔여량과 리셋 시점을 볼 수 없으면 429 안내가 무의미해진다.
수정: /v1/usage는 캡 판정에서 제외하고, 캡 소모 집계에서도 제외(잔여량 폴링이 쿼터를
깎으면 그것대로 왜곡이므로 양쪽을 일치시켰다). 재검증으로 캡 도달 후 usage 200 +
month_remaining=0, 데이터 콜 429, cap=0 키 무영향(회귀 6콜)까지 확인.

### 판단 기록
- 요청마다 SUM 쿼리 vs 캐시: 단순성 원칙에 따라 캡 있는 키만 요청당 인덱스 SUM 1회.
  셀프서브 rate 수준에서 부담이 없고, 캐시를 두면 TTL만큼 초과 허용이 생긴다.
  meter의 1초 비동기 flush가 만드는 오차만 하드캡 허용 오차로 문서화했다.
- 429 코드 분리: rate_limited(초 단위, Retry-After 있음)와 monthly_cap_exceeded
  (월 단위, Retry-After 없음)를 코드로 구분해 클라이언트가 다르게 처리할 수 있게 했다.
- 이 맥의 docker 데몬이 꺼져 있었을 뿐 Docker Desktop은 살아 있었다(메모리의
  "docker 없음"은 CLI/데몬 상태 혼동). `open -a Docker` 20초면 로컬 e2e가 가능하다.

## 2026-08-16 — Track A Phase 3: MCP 서버 (Fable 5 세션)

### 무엇을 만들었나
- `cmd/b2d-mcp` — b2d_geo HTTP API를 MCP 도구 7종으로 노출하는 얇은 래퍼.
  파라미터 검증과 GET 전달이 전부이고 판정은 전부 서버가 한다(재구현 금지 원칙).
  stdio(줄 단위 JSON-RPC 2.0)와 streamable HTTP(-http, POST /mcp, stateless) 두 전송.
- 의존성 결정: MCP SDK를 쓰지 않고 표준 라이브러리로 구현했다. 서버 subset이
  initialize/tools/list/tools/call/ping 4개뿐이라 프로토콜 손구현이 SDK 의존(공급망
  면적)보다 싸다고 판단(규칙 1.6). 프로토콜 버전은 클라이언트 요청이 지원 목록에
  있으면 에코, 아니면 최신 제안.
- 에러 = 영업 채널: MCP 도구 에러는 에이전트가 읽고 사용자에게 말해주는 구조라,
  기계용 코드 대신 그대로 전달 가능한 문장을 내려보낸다. monthly_cap_exceeded는
  한도 도달 + 리셋 시점 + pricing.html 업그레이드 안내, REGION_NOT_LOADED는
  서울·경기 우선 적재 + 나머지 도구는 전국 동작 안내(한국어+영어 병기).
- 온보딩 자산: cmd/b2d-mcp/README.md(복붙 튜토리얼: 설치 -> 설정 JSON -> 예시 질문
  3개, 매뉴얼 안 읽는다는 전제), web/public/llms.txt(에이전트 크롤링용 한 파일:
  전체 API 표면 + 에러 코드 + MCP 설치법; embed로 루트 /llms.txt 자동 서빙).

### 검증 (inspector 대체 경위 포함)
- MCP inspector는 npm 불능(이 맥 npm 깨짐)이라 스크립트 JSON-RPC 클라이언트로
  대체 — inspector가 하는 일이 정확히 이 프로토콜 왕복이라 검증력은 동일하다.
- 라이브 API 대상 7개 도구 전부 실호출 body 확인: geocode(세종대로 110 exact),
  reverse, search_places, get_parcel(pnu 반환), get_land_value(개별공시지가
  65,730,000원/㎡ — Phase 7 정부 대조 기록값과 일치), get_transactions(2026-05
  실데이터), get_zoning(17건). 검증 키는 VPS에서 임시 발급 후 revoke.
- search_places가 시청 반경 500m에서 0건: 래퍼 버그 의심 -> 동일 파라미터로 직접
  API 호출도 0건 -> 상가DB상 사실(최근접 스타벅스가 종로 ~950m). 래퍼 충실성 확인.
  교훈: 빈 응답은 버그 가정 말고 원 API와 파라미터 동일 대조가 먼저다.
- 월캡 문장은 라이브에 캡 코드가 아직 미배포라 로컬 스택(도커 PostGIS + 캡 소진
  키)으로 확인. streamable HTTP는 curl로 initialize/tools/call/알림(202) 확인.

### 남은 것 (배포·유통)
- go install 배포는 저장소가 private라 현재 외부인 불가 — 공개 미러, 바이너리
  릴리스, 저장소 공개 중 결정 필요(사용자 판단).
- VPS 배포 시퀀스: push -> CI -> pull + migrate(012). llms.txt·pricing.html도
  같은 이미지에 embed되므로 함께 반영된다.

## 2026-08-16 — Track A 마무리: 확정 단가 반영·원격 MCP·라이브 배포 (Fable 5 세션)

### 결정과 근거
- **자리표시자 전부 제거**: standard 9,900원/월(부가세 포함)·100,000건·5rps 확정 반영.
  온프레미스(연 3,000만/갱신 800만)와 Pro(월 100만)는 docs/08의 2026-07-27 기확정값.
  단, 종량 지오코딩 30/20/10원은 공개하지 않고 "볼륨 견적"으로 돌렸다 — 그 사다리는
  "카카오 50원/건" 오전제 위에서 설계됐고, 실제 카카오 지오코딩이 0.5원/건으로
  판명된 지금 공개하면 즉시 반증당한다. docs/08 §2-1·§2-4 재산정이 필요하다(후속).
- **원격 MCP의 인증 설계**: HTTP 모드가 서버 env 키로 상류를 호출하면 공개 /mcp가
  인증 우회 통로가 된다. 호출자의 Authorization/X-API-Key 헤더를 상류 API에 그대로
  전달하는 패스스루로 바꾸고, env 키는 사설 배치용 폴백으로 강등했다.
- **배포 구성**: 같은 이미지에 /b2d-mcp를 추가하고 compose에 mcp 서비스(entrypoint
  교체), Caddy는 handle /mcp -> mcp:8330, 나머지는 기본 handle. VPS의 deploy/
  구성 파일은 git pull이 안 되므로(HTTPS 인증 없음) scp로 전송하는 게 정규 경로다.

### 검증 (라이브, 실응답 body)
- pricing.html: 자리표시자 정규식 매치 0건, 9,900원·부가세 포함·UTC 월 기준·데이터
  residency 문장 확인. llms.txt 200.
- 캡 e2e: cap=3 키(id=40)로 3콜 200 -> 4번째 429 monthly_cap_exceeded(업그레이드
  안내 포함) -> /v1/usage 200 + month_remaining 0 -> revoke. 마이그레이션 012는
  기본값 컬럼 추가라 무중단 적용(migrate: ok).
- 원격 /mcp: initialize(2025-06-18 에코)·tools/list 7종·tools/call geocode exact
  match를 공개 엔드포인트에서 curl로 확인. 헤더 패스스루는 로컬에서 env 키 없이
  호출자 Bearer만으로 성공하는 것으로 별도 확인.
- 독립 감사: 에러 경로(무인증 tools/call 안내 문장, 알림 202, malformed JSON,
  GET /mcp 405)와 사이트 정합은 별도 에이전트 감사로 교차 확인.

### 운영 함정 기록
- zsh에서 `SSH="ssh -i ..."; $SSH host cmd` 패턴은 word-splitting이 없어 통째로
  명령 경로가 된다 — 원격 반복 호출은 함수(vps() { ssh ... "$@"; })로 감쌀 것.
- 공개 미러(go install용)는 로컬에 GitHub API 토큰이 없어 저장소 생성만 수동이다.
  ~/dev/b2d-geo-mcp에 모듈·README·goreleaser·Actions·v0.1.0 태그까지 준비 완료,
  빈 저장소 생성 즉시 push 가능. 모듈 루트 설치라 바이너리명은 b2d-geo-mcp.

## 2026-08-31 — Phase 4 사용자 리뷰 반영 및 close-out

### 변경

- 공개 페이지 8개(index, docs, pricing, map, batch, terms, privacy, refund)의 푸터를
  최종 정리했다.
- 사용자에게 보이는 문의 이메일은 `info@birth2death.com`으로 통일했다.
- 전화번호는 `070-8983-4287`, 통신판매업 신고번호는 정부24 신고증에서 확인한
  `2026-경기하남-1593`으로 확정했다.
- 개인정보처리방침 내부의 보호책임자 개인 이메일 `midmost44@gmail.com`은 유지했다.

### 검증

- 공개 HTML 전수 검색으로 푸터의 사업자번호·전화번호·공개 문의 이메일·신고번호를 확인했다.
- `신고 진행 중` 및 푸터의 잘못된 `info@` 누락 상태가 남아 있지 않음을 확인했다.
- `git diff --check` 통과.

## 2026-08-18 — MCP 온보딩 간소화: 복붙 사고 없애기 (Fable 5 세션)

### 문제 인식 (사용자 지적)
JSON 여섯 줄 복붙은 바이브코더 퍼널의 실제 사고 지점이다. 채워야 할 칸이 하나라도
있으면 거기서 이탈이 난다. 해법은 "설명을 더 잘 쓰기"가 아니라 "채울 칸을 0개로".

### 구현
- **키 내장 URL**: POST /mcp/{key} (Go 1.22 PathValue). 우선순위 헤더 > 경로 키 >
  env 폴백. 전제 조건으로 Caddy에서 MCP 트래픽 접근 로그를 log_skip — 키가 URL에
  실리는 방식은 로그 마스킹 없이는 열면 안 된다. log_skip은 2.8+ 문법이라 배포 전
  VPS Caddy 버전(2.11.4) 확인 후 적용(구버전이면 기동 실패로 장애가 났을 것).
- **발급 즉시 완성본**: docs.html에서 데모 키가 발급되는 순간 renderConnect()가
  키가 이미 박힌 4형태를 만든다 — Claude Code 터미널 원라이너(claude mcp add),
  Cursor 원클릭 딥링크(cursor://anysphere.cursor-deeplink/mcp/install, config는
  base64 JSON), Claude Desktop용 설정 JSON, URL만 받는 도구용 키 내장 URL.
  전부 복사 버튼, 키 노출 주의 문구 동반. CLI 사용자에겐 JSON이 아니라 명령
  한 줄이 정답이라는 것이 이번 범위 합의의 핵심.
- 미러 저장소 동기화(v0.1.2 릴리스, MIT는 v0.1.1부터), llms.txt·README 양쪽 갱신.

### 검증
- 로컬: /mcp/{key} 3경로(경로 키 성공, 나쁜 경로 키+좋은 헤더 = 헤더 승, 무효 키
  = 안내 문장) body 확인. 연동 블록은 실제 docs.js를 프리뷰 하니스로 로드해 헤드리스
  렌더 확인, Cursor 딥링크는 DOM에서 추출해 base64 디코드로 config 구조 검증
  (href의 &는 HTML 엔티티로 이스케이프돼 있어 unescape 후 파싱해야 한다).
- 라이브: 실제 퍼널 그대로 — 데모 키 발급 -> /mcp/{키} geocode exact match ->
  헤더 모드 회귀 없음 -> Caddy 로그 5분 창에서 일반 요청 13건 기록되는 동안
  /mcp 경로·키 문자열 0건(log_skip 실증).
