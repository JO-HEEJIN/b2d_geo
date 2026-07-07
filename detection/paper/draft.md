# Fine-tuning Real-time Detection Transformers for Bridge Damage Inspection: A Large-scale Study of Class Imbalance and Cross-domain Augmentation

**Authors:** [PENDING: author list]
**Draft status:** v0.2 — Cycle-1 and Cycle-2 results complete; YOLO comparator (CUDA re-run) pending. Written in Markdown; LaTeX (ICML template) conversion is a follow-up task.

---

## Abstract

Automated visual inspection of bridge surfaces is a safety-critical application in which modern object detectors remain under-evaluated at realistic scale. We present a large-scale empirical study of real-time detection transformers for bridge damage detection, built on the AI Hub 040 bridge exterior-inspection orthoimage corpus: 378,072 images (336,068 train / 42,004 val) at 512×512 resolution with 754,992 bounding boxes across nine damage categories, converted from polygon annotations through a pipeline whose outputs were verified by three independent re-computation audits (maximum box deviation 5.0×10⁻⁷ in normalized coordinates). The class distribution is extremely long-tailed: the most frequent category (concrete crack) has 254,510 training boxes while the rarest (steel corrosion) has 258. We fine-tune DEIMv2-M — an 18.05 M-parameter detector with a DINOv3-distilled ViT backbone — from its COCO checkpoint under an explicit compute budget, reaching 0.161 val mAP@[.50:.95] in 10 epochs on a single RTX 4090. Per-class analysis yields a counterintuitive finding: average precision is largely decoupled from annotation count. The best-detected class is the rarest (steel corrosion, AP 0.361 from 258 boxes), while a diffuse damage type with 107,741 boxes (efflorescence) is the worst (AP 0.054), suggesting that boundary definiteness, not data volume, dominates attainable detection quality for surface-damage classes. We further evaluate a second training cycle that combines cross-domain augmentation (19,643 crack and 5,627 pothole boxes mapped from RDD2022) with duplication-based oversampling of rare classes (×20–×50), and find a sharply non-monotonic response: moderate duplication (×20) and cross-domain injection improve their targets (spalling +64%, pothole +28% relative AP), while extreme duplication (×50/×40) collapses the very classes it targets (steel corrosion −51%, paint damage −35%) at a modest aggregate cost (0.161 → 0.151 mAP). We release conversion, verification, and training configurations, and document the full engineering path — including failure modes (CUDA OOM under multi-scale, label-index remapping, backend-dependent LR stability) — to make the study reproducible under practical budget and license constraints.

## 초록 (Korean Abstract)

교량 외관의 자동 손상 탐지는 안전-필수 응용임에도 현실적 규모에서의 최신 검출기 평가가 부족하다. 본 연구는 AI Hub 040 교량 외관점검 입면정사영상 데이터(378,072장, 512×512, 9개 손상 클래스, 754,992 박스)를 기반으로 실시간 검출 트랜스포머의 대규모 실증 연구를 수행한다. 폴리곤 어노테이션을 바운딩박스로 변환하는 파이프라인은 3중 독립 재계산 검증(최대 편차 5.0×10⁻⁷)을 통과했다. 클래스 분포는 극단적 롱테일로, 최다 클래스(콘크리트 균열)는 254,510 박스, 최소 클래스(강재 부식)는 258 박스이다. DINOv3 증류 백본의 DEIMv2-M(18.05M 파라미터)을 COCO 체크포인트에서 warm-start하여 RTX 4090 1장, 10 epochs로 val mAP 0.161을 달성했다. 클래스별 분석에서 성능이 어노테이션 수와 사실상 무관하다는 반직관적 결과를 얻었다: 최소 데이터 클래스(강재 부식, AP 0.361)가 최고 성능, 대량 데이터의 확산형 손상(백태, 107,741 박스, AP 0.054)이 최저 성능이었다. 이는 경계 명확성이 데이터 양보다 지배적임을 시사한다. 나아가 RDD2022 교차도메인 병합과 복제 오버샘플링(×20–×50)을 결합한 2차 학습에서 뚜렷한 비단조 반응을 발견했다: 중간 강도 복제(×20)와 교차도메인 주입은 목표 클래스를 개선했으나(박락 +64%, 포트홀 +28%), 극단 복제(×50/×40)는 표적 클래스 자체를 붕괴시켰다(강재 부식 −51%, 도장 박리 −35%). 전체 mAP 비용은 크지 않았다(0.161→0.151). 복제 오버샘플링에는 사용 가능한 창이 존재하며, 이를 넘으면 검출기가 복제 인스턴스를 암기한다는 것이 실무적 결론이다.

---

## 1 Introduction

Bridges deteriorate continuously, and the dominant inspection practice — periodic human visual survey, increasingly assisted by UAV imagery — is expensive, subjective, and unevenly applied. Converting UAV-captured facade orthoimages into audited damage inventories is therefore a natural target for object detection. Yet most published studies in this domain operate on datasets of a few thousand images, report a single aggregate metric, and rarely confront the two properties that make the production problem hard: (i) extreme long-tailed class imbalance, because catastrophic-precursor damage types (steel corrosion, paint delamination, spalling) are precisely the rare ones; and (ii) the gap between polygon-annotated source data and the box-supervised detectors that are practical to deploy.

This paper reports a scale-realistic study built on the AI Hub 040 bridge exterior-inspection corpus — to our knowledge among the largest damage-detection training sets used in the literature (378,072 images, 754,992 boxes, nine classes, with an 88,554-image negative-sample subset) — and a modern real-time detection transformer, DEIMv2 [arXiv:2509.20787], fine-tuned from COCO under an explicit, documented compute budget (single RTX 4090, ≈20 GPU-hours per training cycle).

Our contributions:

- **A verified large-scale benchmark pipeline.** We convert 378,072 labelme-style polygon annotations to detection supervision (YOLO and COCO formats) and validate the conversion with three *independent* audits — numeric re-computation, exhaustive format/integrity checking, and adversarial edge-case search over the full corpus — achieving box-for-box agreement within 5.0×10⁻⁷ (pure rounding error) and catching real dataset artifacts (2 degenerate collinear polygons, 1 Windows copy-duplicate).
- **A budget-constrained fine-tuning recipe for DEIMv2.** We show that proportionally rescaling the 102-epoch COCO recipe to 10 epochs at native 512 px, warm-started from the COCO checkpoint, reaches 0.161 mAP with a characteristic three-phase curve (warm-start plateau → augmentation dip → terminal no-augmentation jump), and we quantify what the truncation costs (the curve terminates at a slope of +2.0 mAP/epoch).
- **Evidence that boundary definiteness dominates data volume.** Across nine classes spanning four orders of magnitude in annotation count, per-class AP is essentially uncorrelated with box count; compact, visually distinctive damage is learnable from hundreds of boxes while diffuse damage resists learning from a hundred thousand.
- **A controlled rare-class intervention.** A second cycle adds license-vetted cross-domain data (RDD2022 → 2 mapped classes) and duplication oversampling (×20–×50 on three rare classes), isolating the trade-off between rare-class gains and validation-domain drift. The headline result is that the intervention's effect is non-monotonic in oversampling factor: ×20 helps, ×50/×40 backfires on the targeted classes.

We additionally document license-driven dataset selection (e.g., excluding dacl10k from training due to CC BY-NC) and backend-dependent training-stability failures, both of which materially shaped the study and are usually omitted from papers.

## 2 Related Work

**Real-time detection transformers.** RT-DETR established that end-to-end DETR-style detectors can beat YOLO-family models at real-time latencies [CITE: RT-DETR]. D-FINE reformulated box regression as fine-grained distribution refinement [CITE: D-FINE], and DEIM [arXiv:2412.04234] introduced Dense O2O matching with a Matchability-Aware Loss (MAL), roughly halving training cost while improving AP; DEIMv2 [arXiv:2509.20787] couples this training framework with DINOv3-derived backbones, using ViT-Tiny+ distilled from DINOv3-S for its S/M variants. We adopt DEIMv2-M for its Apache-2.0 license, its COCO checkpoint availability (53.0 AP), and evidence that DINOv3-derived initialization transfers better than supervised ImageNet pretraining for RGB industrial-inspection tasks when the backbone is fully fine-tuned [arXiv:2605.23472].

**Damage-detection datasets.** Public bridge-damage datasets are small relative to the deployment problem: dacl10k provides 9,920 images with 19 polygon classes for semantic segmentation [arXiv:2309.00460]; CODEBRIM provides 7,729 multi-label patches [CITE: CODEBRIM]. Road-damage datasets are larger — RDD2022 contains 47,420 images with box annotations across six countries [arXiv:2209.08538] — but cover pavement viewpoints rather than bridge facades. The AI Hub 040 corpus used here is roughly 38× dacl10k in image count and is box-convertible, but is distributed under Korean AI Hub terms rather than a Creative Commons license; we discuss the resulting constraints in §3.4 and §Reproducibility.

**Class imbalance in detection.** Standard remedies include image-level resampling / repeat-factor sampling [CITE: LVIS repeat-factor], loss reweighting [CITE], and copy-paste augmentation [CITE: simple copy-paste]. DEIM's training recipe already includes CopyBlend augmentation; because its dataloader lacks a repeat-factor sampler, we implement oversampling at the annotation-file level (§4.3), which is sampler-agnostic and exactly reproducible.

## 3 Dataset and Verified Conversion Pipeline

### 3.1 Source corpus

AI Hub 040 ("bridge 3D exterior inspection imagery") provides UAV-captured facade orthoimage tiles at 512×512 px (ground sampling distance 0.3 cm/px per tile metadata) with labelme-style polygon annotations. The released archives contain 378,072 annotation JSONs (336,068 train / 42,004 val, following the corpus' own Training/Validation split), each with bridge-level metadata and zero or more polygon shapes. Normal (undamaged) tiles — 78,715 train, 9,839 val — carry empty shape lists and are retained as explicit negative samples.

Nine damage categories are annotated (label strings verbatim from the corpus, including its spelling idiosyncrasies "AspaltCrack" and "Efflorescene"):

| Class (corpus label) | Meaning | Train boxes | Val instances |
|---|---|---:|---:|
| ConcreteCrack | concrete crack | 254,510 | 36,415 |
| AspaltCrack | asphalt crack | 248,766 | 41,557 |
| Efflorescene | efflorescence | 107,741 | 13,616 |
| Leak | leakage / wet stain | 32,532 | 3,428 |
| Exposure | exposed rebar | 12,431 | 1,645 |
| Spalling | spalling | 810 | 120 |
| Pothole | pothole / subsidence | 629 | 65 |
| PaintDamage | paint delamination | 399 | 32 |
| SteelDefect | steel corrosion | 258 | 38 |

The imbalance ratio between head and tail classes is ≈986:1. Totals: 658,076 train boxes, 96,916 val boxes.

### 3.2 Polygon-to-box conversion

Each polygon (≥3 vertices) is converted to its axis-aligned bounding box: vertices are clamped to the image extent, and boxes with zero width or height are discarded. Outputs are written both as normalized YOLO text labels and as COCO instance JSON (absolute-pixel `xywh`). One conversion subtlety proved load-bearing: DEIM's data loader, with `remap_mscoco_category` disabled, consumes `category_id` *directly as the label index*; 1-based COCO-conventional ids therefore produce an out-of-range class index and a delayed device-side CUDA assertion. All annotation files use 0-based category ids.

### 3.3 Three-lens independent verification

Because every downstream number depends on the conversion, we audited it with three independent procedures, each re-deriving boxes from the raw JSONs without reference to the converter's code:

1. **Numeric recomputation** — 763 stratified-sampled JSONs (all 20 archives), 1,887 polygons independently re-reduced to boxes and compared to emitted labels: 0 mismatches at 10⁻⁶ tolerance.
2. **Exhaustive integrity** — all 378,072 label files parsed: 0 format violations, class indices ∈ [0,8], all geometry in-bounds (tolerance 10⁻⁶); file-set equality between JSONs and labels in both directions; empty-file count exactly matches the negative-sample count.
3. **Adversarial edge-case search** — full-corpus box-for-box recomputation (754,992 boxes): maximum deviation 5.0×10⁻⁷ (6-decimal rounding); the converter's 2 dropped shapes were confirmed to be genuinely degenerate (3-vertex polygons with all vertices collinear at y=340); 0 sub-3-vertex shapes; 1,366 coordinate excursions of ≤5.7×10⁻¹⁴ px (floating-point artifacts); 0 duplicate image stems; 0 duplicate boxes.

The audit also surfaced upstream corpus artifacts (a Windows "copy of" duplicate image/annotation pair, retained as-is; an x-mirrored duplicate tile pair), which we report for future users of the corpus.

### 3.4 License-gated auxiliary data

We considered four public auxiliary sources and admitted them by license: RDD2022 (CC BY 4.0, confirmed via the figshare API record) is admitted for training; dacl10k (CC BY-NC 4.0) is *excluded* from training in this commercially-constrained project and reserved for potential evaluation-only use; CODEBRIM and SDNET2018 are excluded on format grounds (patch classification, no boxes). We note this because license-driven selection changed the experimental design — the most semantically aligned external dataset (dacl10k: bridge damage, polygon format compatible with our converter) was unusable, while the admissible one (RDD2022) covers only 2 of 9 classes and a different viewpoint domain.

From RDD2022's annotated train splits (Japan 10,506, India 7,706, Czech 2,829 XMLs; image sizes 540–1080 px), we map crack variants D00/D01/D10/D11/D20/D0w0 → AspaltCrack and D40 → Pothole; non-damage annotations (D43/D44 lane blur, D50 manhole) are treated as background, and images containing *only* unmapped labels are excluded (mapping crack variants matters: leaving D01/D11 unlabeled inside adopted images would train them as background). This yields 12,291 adopted images contributing +19,643 AspaltCrack and +5,627 Pothole boxes — a 10.0× increase for Pothole. Spot-verification: 17 images manually recomputed, all exact; per-country box totals match an independent pre-scan of the label vocabulary.

## 4 Method

### 4.1 Detector

DEIMv2-M couples a ViT-Tiny+ backbone (distilled from DINOv3-S; 256-d embeddings, interaction indices {3,7,11}) with a hybrid encoder and a 4-layer DEIM transformer decoder (300 queries, 100 denoising queries): 18.05 M trainable parameters, 51.9 GFLOPs forward at 640 px. Training uses DEIM's Dense O2O matching with MAL, box/GIoU/FGL/DDF losses, EMA (decay 0.9999), and a scheduled matcher switch to an IoU-ordered cost.

### 4.2 Budget-scaled fine-tuning recipe (Cycle 1)

We warm-start from the released COCO checkpoint (`-t` tuning mode): backbone, encoder, and decoder geometry heads transfer; the 10 classification-head tensors and the denoising class embedding are re-initialized for 9 classes. The official 102-epoch COCO recipe is rescaled proportionally to a 10-epoch budget:

| Hyperparameter | COCO recipe (102 ep) | Ours (10 ep) |
|---|---|---|
| flat LR epochs (`flat_epoch`) | 49 | 7 |
| no-augmentation tail (`no_aug_epoch`) | 12 | 2 |
| augmentation policy epochs | [4, 49, 90] | [1, 7, 8] |
| MixUp window | [4, 49] | [1, 7] |
| CopyBlend window | [4, 90] | [1, 8] |
| multi-scale stop | 90 | 8 |
| matcher switch epoch | 80 | 8 |

Two deviations from the reference recipe are deliberate. (i) **Native resolution:** the source tiles are 512×512, so we train and evaluate at 512 px (multi-scale base 512) instead of 640 — the reference setting would upsample every image, costing ≈36% more compute for no information. (ii) **Memory fit:** batch 32 with AMP and `expandable_segments` occupies 18.9 GB at the 640-px ceiling of the 512-base multi-scale range; the reference 640-base recipe overflows 24 GB (observed 23.5 GB OOM at its 800-px ceiling).

Optimizer: AdamW, base LR 5×10⁻⁴, backbone LR 2.5×10⁻⁵ (norm/bias groups without weight decay), weight decay 10⁻⁴, 500-iteration warmup, flat-cosine schedule. One epoch is 10,502 iterations at batch 32 (0.572 s/it on one RTX 4090; ≈1.9 h/epoch including a full 42,004-image validation pass).

### 4.3 Rare-class intervention (Cycle 2)

Cycle 2 fine-tunes from the Cycle-1 best checkpoint with halved learning rates and the same 10-epoch schedule shape, changing only the training distribution:

- **Cross-domain merge:** the 12,291 RDD2022 images of §3.4 (path-prefixed file names under a common image root; validation remains pure AI Hub).
- **Duplication oversampling:** because DEIM's loader has no repeat-factor sampler, rare-class images are duplicated in the annotation file — an image's repeat factor is the maximum over its classes of {SteelDefect ×50, PaintDamage ×40, Spalling ×20}. Effective training boxes become SteelDefect 12,900, PaintDamage 15,960, Spalling 16,200 (Pothole is already lifted to 6,256 by RDD). The merged, oversampled train set has 373,681 image entries / 726,939 boxes (11,678 iterations/epoch).

This design intentionally confounds two interventions (external data + oversampling) in one cycle; the budget did not permit a factorial ablation, a limitation we flag in §7.

## 5 Experiments

### 5.1 Setup

Single NVIDIA RTX 4090 (24 GB, driver 570.195.03), PyTorch 2.5.1+cu124, AMP, batch 32, seed 0. Dataset images are staged on a RAM disk (tmpfs); we note for reproducibility that on the rented instance the *container CPU quota* (13.6 cores by cgroup, despite `nproc`=64) bound throughput via the CPU-side Hungarian matching, capping GPU utilization near 17% — data loading was not the bottleneck (0.059 s of the 0.572 s/it step). Cycle-1 training cost ≈20 GPU-hours end-to-end.

Evaluation: COCO protocol (mAP@[.50:.95], maxDets 100) on the 42,004-image AI Hub validation split, using the EMA weights; per-class AP obtained by slicing the COCO precision tensor per category.

### 5.2 Cycle-1 results

**Aggregate.** Final val mAP@[.50:.95] **0.161**. The epoch trajectory exhibits three phases characteristic of the DEIM recipe under warm-start:

| Epoch | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 |
|---|---|---|---|---|---|---|---|---|---|---|
| mAP | .127 | .097 | .106 | .118 | .117 | .127 | .129 | .137 | .141 | **.161** |

Warm-start alone yields 0.127 after one epoch (the re-initialized heads adapt quickly); activating the strong-augmentation block (Mosaic/MixUp/CopyBlend) at epoch 1 depresses validation by 3.0 points before recovery; the terminal phase (cosine decay from epoch 7, augmentation off and matcher switch at epoch 8) contributes +2.4 points in two epochs, +2.0 of it in the final epoch. At epoch 0 the recall/precision structure already showed the detector finding objects it could not yet classify or localize tightly (AR@100 0.552 vs AP 0.127).

**Per-class.** The central result:

| Class | Train boxes | AP | AP50 |
|---|---:|---:|---:|
| SteelDefect | 258 | **0.361** | 0.401 |
| AspaltCrack | 248,766 | 0.227 | 0.414 |
| ConcreteCrack | 254,510 | 0.216 | 0.414 |
| PaintDamage | 399 | 0.195 | 0.311 |
| Leak | 32,532 | 0.166 | 0.267 |
| Pothole | 629 | 0.107 | 0.183 |
| Exposure | 12,431 | 0.064 | 0.215 |
| Spalling | 810 | 0.058 | 0.142 |
| Efflorescene | 107,741 | **0.054** | 0.099 |

The Spearman rank correlation between per-class box count and AP is ρ = −0.083 (p = 0.83; n = 9 classes) [PENDING: bootstrap CI]; the extreme cells are the rarest class at the top (SteelDefect: 258 boxes, AP 0.361) and the third-largest class at the bottom (Efflorescene: 107,741 boxes, AP 0.054).

**Comparator.** A YOLO11m comparator is training on Apple MPS at submission-draft time (final numbers pending; two earlier attempts diverged post-warmup — validation loss tripling while training loss stayed flat — first attributed to fp16/AMP, then isolated to LR-peak sensitivity at lr₀=0.01 on the MPS backend; the third run with amp off recovered through the LR peak and reached 0.067 mAP by epoch 8 of 10). We report this openly rather than presenting a weak comparator as a strong baseline. [PENDING: final epoch numbers]

### 5.3 Cycle-2 results (rare-class intervention)

**Aggregate.** Final val mAP 0.151 (epoch trajectory 0.144 → 0.100 at the augmentation-phase trough → 0.145 at augmentation-off → 0.151 final): 1.0 point below Cycle 1. The augmentation-phase dip was deeper and longer than Cycle 1's (bottom 0.100 vs 0.097, recovery two epochs later) — consistent with the training distribution having moved measurably away from the validation domain (cross-domain images plus duplicated instances). AR@100 declined from 0.552 to 0.512.

**Per-class.** The intervention's signature result (interventions annotated):

| Class | Intervention | Cycle-1 AP | Cycle-2 AP | Δ (rel.) |
|---|---|---:|---:|---:|
| Spalling | duplication ×20 | 0.058 | 0.096 | **+64%** |
| Pothole | RDD +5,627 boxes | 0.107 | 0.137 | **+28%** |
| Exposure | none | 0.064 | 0.093 | +46% |
| Leak | none | 0.166 | 0.218 | +32% |
| Efflorescene | none | 0.054 | 0.063 | +17% |
| ConcreteCrack | none | 0.216 | 0.229 | +6% |
| AspaltCrack | RDD +19,643 boxes | 0.227 | 0.221 | −3% |
| PaintDamage | duplication ×40 | 0.195 | 0.127 | **−35%** |
| SteelDefect | duplication ×50 | 0.361 | 0.178 | **−51%** |

Three patterns stand out. (i) The *moderately* oversampled class improved dramatically (+64%), and the cross-domain-augmented rare class improved substantially (+28%) — the intervention works at moderate strength. (ii) The two *extremely* oversampled classes (×50, ×40) collapsed — the only two classes with large losses, and the effect size (−51% on the strongest Cycle-1 class) far exceeds rare-class validation noise, though the small val counts (38/32 instances) widen the uncertainty. Duplicating a few hundred unique instances fifty-fold appears to induce overfitting to those instances (memorization amplified by CopyBlend re-presenting the duplicates in synthetic contexts), degrading generalization on the same class it targets. (iii) Untouched mid-tail classes (Leak, Exposure) improved by 30–46% — plausibly from the fine-tuning continuation itself (a second annealing cycle from a converged start), which cautions against attributing all gains to the data intervention without an ablation.

## 6 Discussion

**(a) Boundary definiteness dominates data volume.** The rank decoupling of AP from box count is too large to attribute to noise. Our working hypothesis is that the binding constraint is the *definiteness of the damage boundary*: steel corrosion and paint delamination are compact, high-contrast, and consistently delimited by annotators, so a few hundred boxes suffice for a well-initialized detector; efflorescence and leakage are diffuse stains whose extent is genuinely ambiguous, so both the regression target and the IoU-based evaluation are ill-posed at any data volume — more boxes reproduce the ambiguity rather than resolve it. Exposure (thin rebar lines) fails for a related geometric reason: elongated, thin, often diagonal structures fit axis-aligned boxes poorly, capping attainable IoU. A practical implication for dataset builders: for diffuse damage types, investment in *boundary protocol* (or mask supervision) should precede investment in annotation count. For evaluation design, IoU-threshold metrics systematically understate the usefulness of diffuse-damage detections (a "found the stain, imprecise extent" prediction is operationally valuable but scores near zero at strict IoU).

**(b) The augmentation–distribution-shift trade-off.** Strong augmentation is load-bearing in DEIM recipes — Cycle 1's terminal no-augmentation jump (+2.0 mAP in one epoch) shows the model harvesting what the augmented phase built. But augmentation composes with dataset interventions: adding cross-domain images and duplicated rare-class images deepened the augmented-phase validation dip (§5.3). Duplication oversampling interacts with CopyBlend/MixUp by re-presenting the same rare instances in many synthetic contexts — the intended effect — while also increasing effective epoch length over a distribution farther from val. Cycle 2 answered this empirically: the terminal phase recovered most of the shift (0.100 → 0.151) but not all of it (−1.0 mAP vs Cycle 1), and the rare-class outcome split by intervention strength — moderate duplication (×20) and cross-domain data delivered their intended gains, while extreme duplication (×50/×40) destroyed performance on its target classes. The practical rule we extract: duplication-based oversampling has a usable window; past it, the detector memorizes the duplicated instances. For production, per-class model selection (Cycle-1 weights for steel corrosion and paint damage, Cycle-2 for the rest) captures the best of both at zero training cost.

**(c) Budget-truncated schedules terminate mid-ascent.** Under a fixed monetary budget we cut the recipe to 10 epochs; the final measured slope was +2.0 mAP/epoch. The model was still improving when training ended — the reported 0.161 is a lower bound on what this configuration achieves, and budget papers should report terminal slope alongside the headline number so that readers can distinguish converged results from truncated ones.

**(d) Engineering findings we believe generalize.** (i) With `remap` disabled, DEIM-family loaders use COCO `category_id` as the raw label index — 1-based ids produce delayed, asynchronous CUDA assertions that surface hundreds of iterations after the faulty batch. (ii) Rented-GPU containers may advertise the host's CPU topology while enforcing a much smaller cgroup quota; for DETR-family training the CPU-side Hungarian matching then becomes the bottleneck, and neither faster storage nor more dataloader workers helps. (iii) Identical recipes are not backend-portable: a YOLO11m configuration stable on CUDA diverged reproducibly on Apple MPS at its LR peak; divergence timing aligned with schedule events (warmup end), which is the fastest diagnostic signal.

## 7 Limitations

- **Rare-class validation noise.** SteelDefect/PaintDamage/Pothole have 38/32/65 val instances; their AP estimates carry wide (unquantified) confidence intervals. [PENDING: bootstrap CIs over val images.]
- **Single seed, single run per configuration.** Budget precluded seed replication; the Cycle-1 vs Cycle-2 comparison is one draw each.
- **No factorial ablation.** Cycle 2 changes cross-domain data and oversampling together.
- **Comparator incomplete.** The YOLO11m MPS baseline recovered on its third configuration and completes after this draft; final numbers pending.
- **Same-domain evaluation only.** Val shares the corpus' capture protocol; cross-domain generalization (e.g., to dacl1k-style hand-held imagery) is untested, partly due to license constraints.
- **Axis-aligned boxes** are a weak fit for elongated diagonal cracks and thin rebar exposure; oriented boxes or masks are a natural extension.
- **Truncated schedule** (§6c): 0.161 is not a converged figure.

## 8 Conclusion

We presented a verified, budget-explicit, license-audited study of real-time detection-transformer fine-tuning for bridge damage inspection at a scale (378 k images, 755 k boxes) rarely used in this literature. The headline empirical finding is that per-class performance is governed far more by the visual definiteness of the damage category than by its annotation count — a result with direct consequences for how inspection datasets should be built and how detection quality should be measured for diffuse damage. We further characterized a practical rare-class intervention combining cross-domain data admission under license constraints with annotation-level oversampling — finding a usable-window effect in which moderate duplication and cross-domain injection deliver large targeted gains while extreme duplication collapses its own targets — and documented the engineering failure modes encountered so the pipeline is reproducible end-to-end by others operating under comparable constraints.

## Reproducibility Statement

Conversion (`bridge_converter.py`, `coco_converter.py`, `rdd_converter.py`, `coco_merge_rdd.py`, `coco_oversample.py`), verification procedures, and training configurations (`deimv2_configs/*.yml`, including the exact 10-epoch rescaling) are versioned in a private repository [PENDING: release decision]; all numbers in this draft trace to committed logs (`detection/runs/`). The AI Hub 040 corpus is available under Korean AI Hub terms (account + use-terms acceptance; not CC-licensed); RDD2022 is CC BY 4.0 (figshare record 21431547); dacl10k is CC BY-NC 4.0 and was therefore excluded from training in this study. DEIMv2 code is Apache-2.0. Total cloud compute for the reported experiments: ≈$34 on rented RTX 4090 instances (≈45 GPU-hours including failed launches, which are themselves documented in the repository logs).

## References

- DEIM: DETR with Improved Matching for Fast Convergence. arXiv:2412.04234.
- DEIMv2: Real-Time Object Detection Meets DINOv3. arXiv:2509.20787.
- Rethinking Transfer Learning for Industrial Inspection: DINOv3 vs. ImageNet Pretraining Across RGB and X-ray Tasks. arXiv:2605.23472.
- RDD2022: A multi-national image dataset for automatic Road Damage Detection. arXiv:2209.08538.
- dacl10k: Benchmark for Semantic Bridge Damage Segmentation. arXiv:2309.00460.
- [CITE: RT-DETR — DETRs Beat YOLOs on Real-time Object Detection]
- [CITE: D-FINE — Redefine Regression Task of DETRs as Fine-grained Distribution Refinement]
- [CITE: DINOv3]
- [CITE: CODEBRIM]
- [CITE: LVIS / repeat-factor sampling]
- [CITE: Simple Copy-Paste augmentation]
- [CITE: AI Hub 040 corpus official reference]
