# v2 Detection — 교량 외관 손상 탐지

결정일: 2026-07-05. 리서치 근거는 세션 로그, 검증 결과는 아래 참조.

## 아키텍처 결정

| 항목 | 결정 | 근거 |
|---|---|---|
| 주력 모델 | **DEIMv2-M (DINOv3 백본)** | Apache-2.0(상용 안전), 실시간 SOTA(COCO 54.7 AP@124FPS 급), DEIM 매칭이 얇고 긴 균열의 IoU 매칭 불리를 완화, 학습시간 ~50% 절감 |
| 프리트레이닝 | DINOv3 (DEIMv2 내장) | RGB 산업검사 full fine-tune에서 ImageNet 대비 우위 실증 (arXiv 2605.23472). ImageNet 대조군 1회 병행 |
| 대조군 | YOLO11m — **내부 비교 전용, 배포 금지** | Ultralytics AGPL-3.0. 로컬 MPS로 즉시 학습 가능해 accuracy floor 용도로만 사용 |
| 학습 하드웨어 | DEIMv2: 클라우드 CUDA GPU 1장 (A100/4090급). YOLO 대조군: 로컬 M5 Max(MPS) | DEIM 코드베이스 CUDA 전제 — MPS 포팅 비용 > GPU 대여 비용 |
| 어노테이션 | YOLO txt(생성 완료) → COCO json 변환 추가 | DEIMv2 입력이 COCO 형식 |

## 데이터 구성 (라이선스 게이트)

| 데이터셋 | 라이선스 | 판정 |
|---|---|---|
| AI Hub 040 교량 외관점검 (코어) | AI Hub 이용약관 | **편입 완료** — 378,072장, 9클래스, 3-렌즈 전수 검증 통과 |
| RDD2022 (AspaltCrack/Pothole 보강) | **CC BY 4.0** (figshare API 확인, 2026-07-05: api.figshare.com/v2/articles/21431547) | **편입 가능** — 상업 OK, 출처표기 필요. train split만 사용 (test는 어노테이션 미공개). 매핑: D00/D10/D20→AspaltCrack, D40→Pothole |
| dacl10k | CC BY-NC 4.0 | **학습 편입 배제** (상용 불가). 외부 일반화 평가 벤치마크로만 사용. labelme 폴리곤 — bridge_converter.py 재사용 가능 |
| CODEBRIM / SDNET2018 / Crack500 | — | 미편입 (patch 분류/세그 형식, detection 직접 활용 낮음) |

## 클래스 (classes.txt 순서 고정)

ConcreteCrack, Spalling, Efflorescene, Leak, Exposure, SteelDefect,
PaintDamage, AspaltCrack, Pothole — 라벨 문자열은 데이터셋 원문 오타 포함 그대로.

## 클래스 불균형 대책

train 박스: ConcreteCrack 254,510 / AspaltCrack 248,766 / Efflorescene 107,741 /
Leak 32,532 / Exposure 12,431 vs **Spalling 810 / Pothole 629 / PaintDamage 399 / SteelDefect 258**.

- 희소 4클래스 이미지 오버샘플링(×50~100) + copy-paste 증강
- 평가는 per-class AP 필수 (mAP 단독 보고 금지 — 다수 클래스에 매몰됨)
- 미달 시 후순위: 희소 클래스 전용 2-stage 검출 검토

## 파일

- `bridge_converter.py` — AI Hub 040 라벨 zip(labelme 폴리곤) → YOLO bbox txt. 스트리밍, 압축해제 불필요
- `labels/{train,val}/*.txt`, `labels/classes.txt` — 변환 산출물 (git 미추적)
- `images/{train,val}/` — 원천 zip 추출 이미지 (git 미추적, 이미지↔라벨 stem 전수 일치 확인)
- `coco_converter.py` — YOLO txt → COCO instances json (DEIMv2 입력, category_id 1-기반)
- `coco/instances_{train,val}.json` — COCO 어노테이션 (git 미추적)
- `rdd_converter.py` — RDD2022(Japan/India/Czech) VOC xml → YOLO. 균열 계열
  D00/D01/D10/D11/D20/D0w0→AspaltCrack, D40→Pothole. D43/D44/D50(차선/맨홀)은
  배경 스킵. 매핑 박스 있는 이미지만 채택
- `rdd/{images,labels}/train/` — RDD 보강분 12,291장: AspaltCrack +19,643,
  **Pothole +5,627** (기존 629의 ~10배). val 없음 — 평가는 AI Hub val 고정

## DEIMv2 학습 준비 (완료 — CUDA 박스에서 바로 실행 가능)

- `deimv2/` — Intellindust-AI-Lab/DEIMv2 클론 (Apache-2.0, git 미추적)
- `deimv2/ckpts/vittplus_distill.pt` — ViT-Tiny+ 백본 (DINOv3-S distill, 37MB)
- `deimv2/ckpts/deimv2_dinov3_m_coco.pth` — COCO 사전학습 체크포인트 (warm-start용, 70MB)
- `deimv2_configs/deimv2_dinov3_m_bridge.yml` — 9클래스 커스텀 config
  (30 epoch 비례 축소 recipe, 상대경로 = 로컬/클라우드 동일 레이아웃 전제)

클라우드 실행 (deimv2/ 루트에서, requirements.txt 설치 후):

```bash
cp -r ../deimv2_configs configs/bridge
CUDA_VISIBLE_DEVICES=0 torchrun --master_port=7777 --nproc_per_node=1 train.py \
  -c configs/bridge/deimv2_dinov3_m_bridge.yml --use-amp --seed=0 \
  -t ckpts/deimv2_dinov3_m_coco.pth
```

필요 데이터 동기화: `detection/{images,coco}` (27GB). RDD 보강분은 대조군 비교 후
COCO json 재생성으로 병합 예정.

## 1차 결과 (2026-07-06)

DEIMv2-M 10 epochs, RunPod 4090: **val mAP@[.50:.95] = 0.161**.
체크포인트/로그: `runs/deimv2_m_bridge/` (best_stg2.pth, train_full.log,
perclass.log). per-class: SteelDefect .361 / AspaltCrack .227 / ConcreteCrack
.216 / PaintDamage .195 / Leak .166 / Pothole .107 / Exposure .064 / Spalling
.058 / Efflorescene .054. 오버샘플링 미적용 순정 baseline — 상세는
learning_log 참조.

## 2차 결과 (2026-07-07)

RDD 병합 + 오버샘플링(×20/×40/×50), 1차 best에서 10 epochs fine-tune:
**val mAP 0.151** (runs/deimv2_m_bridge_v2/). per-class 요지 — 중간 개입 성공
(Spalling +64%, Pothole +28%), 극단 복제 역효과 (SteelDefect −51%,
PaintDamage −35%), 상세는 paper/draft.md §5.3. 배포 권고: 클래스별 모델 선택.

## 다음 작업

1. YOLO11m 대조군 완주 (내일) → 논문 comparator 수치 채우기
2. 오버샘플링 ablation (×20 단독 등) — 후속 실험 후보
3. 논문 LaTeX 변환 (ICML 템플릿) — Opus
