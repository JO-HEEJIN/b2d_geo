# RESTORE — v2 Detection 백업·복원 가이드

> 작성: 2026-08-06. **로컬 대용량 데이터(74GB)를 삭제하기 직전** 작성한 복원 문서.
> 삭제하는 것은 전부 **재생성/재다운 가능한 공개 데이터**이고, **학습한 모델 가중치는
> 전부 Hugging Face에 백업**되어 있다. 이 문서로 언제든 데이터를 되받고 학습을 이어간다.
> 함께 읽을 것: `README.md`(아키텍처 결정), `HANDOFF.md`(파이프라인·학습 상세).

## 0. 한 줄 요약

교량 외관 손상 **9클래스 bbox 탐지**(v2 detection). AI Hub 040 교량데이터(37.8만장)
+ RDD2022로 **두 모델을 실제 파인튜닝**했다. 모델은 HF에 안전, 데이터는 삭제(재다운 가능).

## 1. 학습한 모델 (전부 파인튜닝 — 추론 아님)

| 모델 | 역할 | 성적(val) | 학습 |
|---|---|---|---|
| **DEIMv2-M (DINOv3 백본)** | **주력** | **mAP@[.5:.95] = 0.161** (1차) | 클라우드 CUDA pod, COCO ckpt warm-start, 10 epochs, 512px. RDD+오버샘플링 2차 fine-tune 진행중이었음 |
| YOLO11m | 대조군(accuracy floor, 배포 금지·AGPL) | mAP50 0.15 / mAP50-95 0.077 | **로컬 맥 MPS ≈ 39시간**, yolo11m.pt서 전이학습, 10 epochs |

- DEIMv2 per-class(1차): SteelDefect .361 / AspaltCrack .227 / ConcreteCrack .216 /
  PaintDamage .195 / Leak .166 / Pothole .107 / Exposure .064 / Spalling .058 /
  **Efflorescene .054(최하 — 박스 10.8만인데도. "경계 모호성이 지배 변수"가 핵심 발견)**
- 초기 베이스라인 수준(9클래스 극불균형). 다음 스텝은 HANDOFF.md §3 참고.

## 2. 모델 백업 위치 (Hugging Face, 비공개)

**`https://huggingface.co/momoJo/b2d-bridge-detection`** (private, repo-type=model)

내려받기:
```bash
pip install -U "huggingface_hub[cli]"
hf auth login                       # 본인 토큰(Write 아니어도 Read면 됨)
hf download momoJo/b2d-bridge-detection --repo-type model --local-dir detection_models
```
백업된 가중치:
- `runs/deimv2_m_bridge/best_stg2.pth` (280M) — **DEIMv2 1차 주력(mAP 0.161)**
- `runs/deimv2_m_bridge_v2/{best_stg1,best_stg2,last}.pth` — DEIMv2 2차(RDD+OS fine-tune)
- `runs/baseline_y11m{,_v2}/weights/{best,last}.pt` — YOLO11m 베이스라인
- `deimv2_ckpts/{deimv2_dinov3_m_coco.pth, vittplus_distill.pt}` — 사전학습 ckpt

## 3. 삭제한 데이터 → 재다운 방법

전부 공개 데이터. DB(주소/지오코딩)는 별개로 VPS에 이미 있으니, 아래는 **ML 재훈련 시**만 필요.

### AI Hub 040 "교량 3D 외관점검 영상 데이터" (코어, ~27GB 원본)
- 출처: **AI Hub** (aihub.or.kr) — 로그인 + 데이터 신청(무료, 승인). "교량 3D 외관점검
  영상 데이터"(과제번호 040) 검색.
- 다운로드: AI Hub의 `aihubshell` CLI 또는 웹 다운로드. 라벨 zip 20개 + 이미지.
- 라이선스: AI Hub 이용약관(코어 편입 허용, 재배포 금지 — **HF·R2 등 공개 재호스팅 불가**,
  그래서 로컬 삭제 후 재다운 방식).

### RDD2022 (AspaltCrack/Pothole 보강, ~1.7GB)
- 출처: **figshare** `https://figshare.com/articles/dataset/RDD2022_-_The_multi-national_Road_Damage_Dataset/21431547`
  (API로 라이선스 확정: `api.figshare.com/v2/articles/21431547`)
- 라이선스: **CC BY 4.0** — 상업 OK, **출처표기 필요**. train split만 사용(test 라벨 미공개).
- 사용: Japan/India/Czech zip(VOC xml). 매핑 D00/D10/D20→AspaltCrack, D40→Pothole.

## 4. 파생 데이터 재생성 (원본 받은 뒤)

코드는 전부 git에 있음(`detection/*.py`). 원본만 받으면 라벨·COCO json 재생성:
```bash
python bridge_converter.py   # AI Hub 라벨 zip → labels/{train,val}/*.txt (YOLO) + classes.txt
python coco_converter.py     # YOLO txt → coco/instances_{train,val}.json (category_id 0-기반)
python rdd_converter.py      # RDD2022 zip → rdd/{images,labels}/train
python coco_merge_rdd.py     # AI Hub + RDD 병합 → coco/instances_train_rdd.json
python coco_oversample.py    # 희소클래스 오버샘플 → coco/instances_train_rdd_os.json
```
검증 수치(HANDOFF.md §2): train 373,681장 / 726,939박스(오버샘플 후). classes.txt 순서 고정.

## 5. 학습 이어가기

- **YOLO(로컬 MPS)**: `dataset.yaml` 준비 후 `yolo detect train model=yolo11m.pt data=dataset.yaml epochs=10 imgsz=512 device=mps` (args는 `runs/baseline_y11m_v2/args.yaml` 참고).
- **DEIMv2(클라우드 CUDA)**: `deimv2_configs/deimv2_dinov3_m_bridge_v2.yml` + COCO json 입력.
  HF의 `runs/deimv2_m_bridge/best_stg2.pth`에서 warm-start. pod는 CUDA 전제(MPS 불가).
- 상세 다음 스텝·미해결(Efflorescene 최하 등)은 **HANDOFF.md §3** 이어읽기.

## 6. 라이선스 게이트 (배포 시 필수)

| 자산 | 라이선스 | 판정 |
|---|---|---|
| AI Hub 040 | AI Hub 약관 | 코어 편입, **공개 재호스팅 금지** |
| RDD2022 | CC BY 4.0 | 편입, **출처표기 필요** |
| dacl10k | CC BY-NC 4.0 | **학습 배제**, 외부 벤치마크 전용 |
| DEIMv2 코드 | Apache-2.0 | 사용·배포 OK |
| Ultralytics YOLO11 | AGPL-3.0 | **내부 대조군 전용, 배포 금지** → 배포 모델은 DEIMv2 |
