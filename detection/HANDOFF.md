# HANDOFF — v2 Detection 인수인계 (Fable 5 → Opus, 2026-07-07)

이 문서만 읽으면 진행 중인 작업을 끊김 없이 이어받을 수 있도록 작성했다.
모든 수치·경로·규칙은 2026-07-05~07 세션에서 실제 확정된 사실이다.

## 1. 프로젝트 개요

- **v2 detection**: AI Hub 040 "교량 3D 외관점검 영상 데이터"로 교량 외관 손상
  9클래스 bbox 탐지 모델을 학습하는 트랙. b2d_geo 본선(주소/지오코딩 Go 프로젝트,
  Phase 0~7)과 **별개 병렬 트랙** — 본선 todo.md의 Phase 계획과 무관하게 진행.
- 최상위 지시서는 `b2d_geo_claude_code_prompt.md` (데이터 출처 화이트리스트,
  생성/검증 분리, git author 규칙 등 — 본 트랙에도 적용).
- 사용자 승인 사항: 상용 전제 (라이선스 게이트 적용), 아키텍처는 DEIMv2 주력.

## 2. 데이터 파이프라인 (전부 완료·검증됨)

| 스크립트 | 입력 → 출력 | 검증 상태 |
|---|---|---|
| `bridge_converter.py` | AI Hub 라벨 zip 20개(labelme 폴리곤, 스트리밍) → `labels/{train,val}/*.txt` (YOLO) + `classes.txt` | 3-렌즈 독립 QA 전수 통과 (재계산 최대편차 5.0e-7, train 336,068 / val 42,004, 빈 txt = 정상데이터 78,715/9,839) |
| `coco_converter.py` | YOLO txt → `coco/instances_{train,val}.json` (**category_id 0-기반** — DEIM remap off가 id를 라벨 인덱스로 직접 사용) | 왕복 스팟체크 1,000건 불일치 0. train 658,076 / val 96,916 박스 |
| `rdd_converter.py` | RDD2022 Japan/India/Czech zip(VOC xml) → `rdd/{images,labels}/train` | 12,291장 채택, AspaltCrack +19,643 / Pothole +5,627. 어휘 스캔치와 전수 일치, 수동 재계산 17장 일치 |
| `coco_merge_rdd.py` | AI Hub + RDD 병합 → `coco/instances_train_rdd.json` (file_name 경로 프리픽스: `images/train/...`, `rdd/images/train/...` — img_folder는 공통 루트) | 348,359장 / 683,346박스, 300박스 스팟체크 통과 |
| `coco_oversample.py` | 병합 json → `coco/instances_train_rdd_os.json` (희소 클래스 이미지 복제: SteelDefect ×50, PaintDamage ×40, Spalling ×20 — 0-기반 id {5,6,1}) | 373,681장 / 726,939박스 (SteelDefect 258→12,900, PaintDamage 399→15,960, Spalling 810→16,200), 경로 200개 실존 확인 |

데이터 위치:
- 로컬: `detection/images/{train,val}` (27GB), `detection/rdd/images/train` (1.1GB),
  `detection/coco/*.json`, `detection/labels/` — 전부 gitignore
- 2차 pod: `/workspace/{images,rdd,coco,deimv2,deimv2_configs}` + RAM 디스크
  `/dev/shm/{images,rdd/images}` (학습이 여기서 읽음; **pod 재시작 시 shm 재복사 필요**)
- 원본 zip: 리포 루트 `040.교량 3D 외관점검 영상 데이터/`, `data/rdd2022/`

라이선스 게이트 (확정):

| 데이터/코드 | 라이선스 | 판정 |
|---|---|---|
| AI Hub 040 | AI Hub 약관 | 코어 편입 |
| RDD2022 | **CC BY 4.0** (figshare API `api.figshare.com/v2/articles/21431547`로 확정) | 편입, 출처표기 필요 |
| dacl10k | **CC BY-NC 4.0** | **학습 배제** — 외부 벤치마크 전용 |
| DEIMv2 코드 | Apache-2.0 | 사용 |
| Ultralytics YOLO11 | AGPL-3.0 | **내부 대조군 전용, 배포 금지** |

## 3. 학습 상태 (2026-07-07 시점)

### 1차 DEIMv2-M — 완료
- **val mAP@[.50:.95] = 0.161** (10 epochs, 512px, batch 32, COCO ckpt warm-start)
- AP 궤적: 0.127 → 0.097(증강 딥) → 0.106 → 0.118 → 0.117 → 0.127 → 0.129 → 0.137 → 0.141 → **0.161** (막판 no-aug+matcher 전환 점프)
- per-class: SteelDefect .361 / AspaltCrack .227 / ConcreteCrack .216 / PaintDamage .195 / Leak .166 / Pothole .107 / Exposure .064 / Spalling .058 / **Efflorescene .054 (최하 — 박스 108k인데도. 경계 모호성이 지배 변수라는 핵심 발견)**
- 자산: `detection/runs/deimv2_m_bridge/{best_stg2.pth, log.txt, train_full.log, perclass.log}` (md5 검증 완료, 1차 pod은 종료됨)

### 2차 DEIMv2-M (RDD+오버샘플링, 1차 best에서 fine-tune) — **진행 중**
- pod: `sjhew5qx4wjs06` (SECURE 4090, $0.69/h), **ssh: `ssh -p 40033 root@213.192.2.125`** (네트워크 다소 불안정 — ConnectTimeout 20 권장, 세션 끊겨도 nohup 학습은 생존)
- config: `configs/bridge/deimv2_dinov3_m_bridge_v2.yml` (10 epochs, flat 7, no-aug 2, policy [1,7,8], LR 반감 0.00025, `-t ckpts/best_stg2.pth`)
- 진행: 0.572s/it, 11,678 steps/epoch ≈ 1.9h. AP: **0.144(ep0) → 0.116 → 0.112 → 0.107 → 0.100(ep4)** — 증강 구간 하락 국면. 시작점이 1차 최종보다 낮은 원인 = RDD 도로뷰 + 복제 오버샘플링의 train/val 분포 이동 (설계상 감수)
- **판정은 no-aug 구간(ep8-9)과 per-class에서** — 실험 목적이 전체 평균이 아니라 희소 4클래스 개선임. 전체 mAP가 1차보다 낮아도 희소 클래스가 오르면 성공
- 원격 `det_solver.py`에 per-class 출력 패치 적용됨 (PERCLASS 라인)

### YOLO11m 대조군 (로컬 M5 Max MPS) — 존폐 결정 대기
- 1차 run: MPS+AMP로 ep3-4 val 붕괴 → 폐기
- 2차 run(`runs/baseline_y11m_v2`, amp=False, SGD lr0=0.01): **동일 지점 재붕괴** (ep4 mAP 0.0003, val_cls 6.48). 재진단: warmup 종료 후 lr0=0.01 도달 시점과 붕괴 일치 = **MPS에서 LR 과열**. ep5에서 회복 조짐 (mAP 0.0016, val_cls 4.94)
- **결정 규칙: ep7 mAP < 0.03이면 MPS 대조군 포기** → 2차 완료 후 같은 pod(4090)에서 YOLO 4-5h 학습으로 대체 (~$4, 소액 충전 필요할 수 있음)

## 4. 예산

- 잔액 추이: 1차 총지출 ~$18 → 충전 → 2차 시작 $17.14 → ep4 시점 **$8.00 안팎**
- 실측 소진: **~$1.4/epoch** (1.9h × $0.69 + val). 셋업·전송 유휴는 일회성이므로 epoch당 비용에 섞지 말 것 (과대추정 실수 이미 한 번 함)
- 완주 전망: 잔여 epoch × $1.4 vs 잔액 — ep4 시점 기준 마진 ~$1로 완주 가능
- **조기 중단 규칙: 잔액이 (잔여 epoch × $1.4)를 못 덮으면 epoch 8 완료 직후 중단** (no-aug 1 epoch 확보 상태라 성과 대부분 보존)
- 모니터가 잔액 $2.5 미만 시 BALANCE_LOW 경보

## 5. 실행 중인 자동화 (세션 종료 시 소멸 — 새 세션이면 재장착 필요)

| 태스크 ID | 대상 | 주기 | 이벤트와 대응 |
|---|---|---|---|
| `bsxfrqdna` | 2차 학습 | 15분 | `V2_EPOCH_N: <AP> (bal $X)` → 궤적 기록. `V2_TRAINING_FINISHED` → §6 완주 시퀀스. `V2_CRASHED` → train.log 진단. `BALANCE_LOW` → 조기 중단 판단. `V2_UNREACHABLE_OR_DEAD` 2회 연속 → pod 상태 API 확인 |
| `b6b7fs0ai` | YOLO 대조군 | 30분 | `YOLO_EPOCH ep=N ...` → ep7 판정 규칙 적용. `VAL_LOSS_HIGH` (val_cls>4) 동반 시 붕괴 의심. `YOLO_ENDED` → 최종 mAP 확인 |

## 6. 대기 중 작업 큐 (순서대로)

1. **ep7 체크포인트 중간 회수 + per-class 중간 평가** (ep7 이벤트 수신 시):
   ```bash
   # 회수 (잔액 사고 대비 보험)
   scp -P 40033 root@213.192.2.125:/workspace/deimv2/outputs/deimv2_dinov3_m_bridge_v2/best_stg*.pth /Users/momo/dev/b2d_geo/detection/runs/deimv2_m_bridge_v2/
   # per-class 중간 평가 (패치 적용돼 있음, ~10분)
   ssh -p 40033 root@213.192.2.125 "cd /workspace/deimv2 && nohup torchrun --master_port=7778 --nproc_per_node=1 train.py -c configs/bridge/deimv2_dinov3_m_bridge_v2.yml --test-only -r outputs/deimv2_dinov3_m_bridge_v2/best_stg2.pth > /workspace/perclass_mid.log 2>&1 &"
   # 결과: grep PERCLASS /workspace/perclass_mid.log → 1차 표와 비교 보고 (특히 Pothole/SteelDefect/Spalling/PaintDamage)
   ```
   주의: 학습과 동시 실행 시 VRAM 경합 — 학습이 18.9GB 쓰므로 **test-only는 epoch 사이 val 직후 타이밍이거나, 안전하게는 완주 후만**. ep7 시점엔 회수만 하고 평가는 완주 후가 안전.
2. **완주 시퀀스** (V2_TRAINING_FINISHED):
   최종 AP/best_stat 확인 → per-class 평가 (위 명령) → `best_stg2.pth`+로그+perclass 로그 scp 회수 + md5 대조 → **pod 종료** (아래 §9) → 1차/2차 per-class 비교 분석 보고 → learning_log 기록 → 커밋·푸시
3. **YOLO ep7 판정** → 포기 시: 대조군을 pod에서 YOLO11m CUDA 학습으로 대체할지 사용자에게 비용($4 안팎)과 함께 제안
4. **논문 초안 수치 채우기** — `detection/paper/draft.md` (별도 병렬 작업으로 골격 작성 중). 2차 최종 수치·비교표 반영, ICML 수준 목표
5. 사용자 예고: 최종 보고 후 논문 초안 요청 잡혀 있음 (영어 본문 + 한국어 초록 방침)

## 7. 함정 목록 (전부 실제로 밟은 것)

- **macOS unzip(Info-ZIP 6.0)**: UTF-8 플래그 정상 엔트리도 깨진 파일명으로 추출 시도 → "disk full?" 오판 후 대화형 프롬프트에서 사망. 대용량 zip은 Python zipfile로
- **macOS rsync 2.6.9**: `--info=progress2` 미지원 → 즉시 usage 에러인데 백그라운드에선 완료로 오인. 전송 후 원격 파일 수 검증 필수
- **MooseFS(/workspace)**: root여도 chown 불가 → tar 추출에 `--no-same-owner`
- **RunPod 컨테이너**: `nproc`/`free`는 호스트 수치. 실제 한도는 cgroup (`/sys/fs/cgroup/cpu.max` → 13.6코어였음). GPU util 저하 진단은 로그의 `data:` 시간으로 IO/CPU 분리부터
- **DEIM remap_mscoco_category=False**: json `category_id`를 라벨 인덱스로 직접 사용 → **0-기반 필수** (1-기반이면 CUDA index assert + SIGABRT, 비동기라 수백 iter 뒤 터짐)
- **MPS 학습**: AMP가 val/EMA 오염 유발 + lr0=0.01도 과열. "train loss 정상 + val 지수 악화"면 AMP/LR 의심
- **ssh 원격 pgrep**: `ssh host "pgrep -f train.py ..."`는 커맨드라인 자기매치 오탐 → `pgrep -f '[t]rain.py'`
- **zsh**: `set -- $var` word split 안 됨 (`${=var}` 필요). GraphQL 쿼리 조립이 조용히 깨졌음
- **병렬 tar 스트림**: 6개 중 2개가 connection reset으로 조용히 죽은 사례 — `DONE_` 에코 카운트와 원격 파일 수로 이중 검증, 죽은 청크만 재전송 (tar 덮어쓰기가 절단 파일 복구)
- **셸 워처 판정**: `grep X | cut ... && echo OK`는 파이프라인 exit이 cut 기준 → 판정 grep은 단독 실행
- **모니터 이벤트 지연**: 폴링 주기 탓에 이벤트가 이미 확인한 정보의 후행일 수 있음 — 수신 시 로그 재확인

## 8. 운영 규칙

- **자율 진행 위임** (메모리 `autonomous-execution-grant` 참조): 가역 작업(학습 재시작, config, 커밋, 잔액 내 pod 조작)은 묻지 않고 실행 후 보고. **잔액 초과 과금과 비가역 작업만 사용자 확인**
- **pod 종료 전 체크포인트·로그 회수 필수** — pod 볼륨은 종료와 함께 소멸
- 커밋/푸시: origin = `git@github.com:JO-HEEJIN/b2d_geo.git` (private). **gh OAuth 토큰은 workflow scope 없음** → repo deploy key(`~/.ssh/id_ed25519`, write)로 SSH push. author `JO-HEEJIN <midmost44@gmail.com>`
- caveman 모드 full 활성 (사용자 보고 terse, learning_log·논문·커밋 메시지는 일반 문체)
- learning_log.md: 사건마다 서사형 기록 (원인→조치→교훈), 커밋 해시 참조 관례
- `.claude/settings.local.json`에 Bash/Edit/Write/WebFetch/WebSearch 광역 허용 — 승인 프롬프트 없음. rm -rf 차단 훅도 사용자 요청으로 제거됨 (전역)
- RUNPOD_API_KEY는 `~/.zshrc` (비대화 셸에선 `source ~/.zshrc` 후 사용)

## 9. 복붙 명령어

```bash
# pod 접속
ssh -o ConnectTimeout=20 -p 40033 root@213.192.2.125

# 학습 상태 (AP 전체 궤적 + best + 프로세스)
ssh -p 40033 root@213.192.2.125 "grep 'Average Precision  (AP) @\[ IoU=0.50:0.95 | area=   all' /workspace/train.log | awk '{print NR-1\": \"\$NF}'; grep -oE 'best_stat: .{0,60}' /workspace/train.log | tail -1; pgrep -f '[t]rain.py' | head -1"

# 잔액
source ~/.zshrc; curl -s -X POST https://api.runpod.io/graphql -H "Authorization: Bearer $RUNPOD_API_KEY" -H "Content-Type: application/json" -d '{"query":"query { myself { clientBalance } }"}'

# per-class 평가 (완주 후, ~10분)
ssh -p 40033 root@213.192.2.125 "cd /workspace/deimv2 && torchrun --master_port=7778 --nproc_per_node=1 train.py -c configs/bridge/deimv2_dinov3_m_bridge_v2.yml --test-only -r outputs/deimv2_dinov3_m_bridge_v2/best_stg2.pth 2>&1 | grep -E 'PERCLASS|IoU=0.50:0.95 \| area=   all'"

# 체크포인트 회수 + 검증
mkdir -p /Users/momo/dev/b2d_geo/detection/runs/deimv2_m_bridge_v2
scp -P 40033 root@213.192.2.125:/workspace/deimv2/outputs/deimv2_dinov3_m_bridge_v2/best_stg2.pth /Users/momo/dev/b2d_geo/detection/runs/deimv2_m_bridge_v2/
scp -P 40033 root@213.192.2.125:/workspace/train.log /Users/momo/dev/b2d_geo/detection/runs/deimv2_m_bridge_v2/
md5 -q .../best_stg2.pth  # 원격 md5sum과 대조

# pod 종료 (회수 확인 후에만!)
source ~/.zshrc; curl -s -X POST https://api.runpod.io/graphql -H "Authorization: Bearer $RUNPOD_API_KEY" -H "Content-Type: application/json" -d '{"query":"mutation { podTerminate(input: {podId: \"sjhew5qx4wjs06\"}) }"}'

# 푸시
git -C /Users/momo/dev/b2d_geo push origin main
```
