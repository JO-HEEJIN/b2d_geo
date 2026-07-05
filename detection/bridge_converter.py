#!/usr/bin/env python3
"""AI Hub 040 교량 외관점검 라벨(폴리곤) -> YOLO 바운딩박스 변환기.

라벨링데이터 zip(TL_*/VL_*) 안의 labelme형 JSON을 압축 해제 없이 스트리밍으로
읽어, 이미지 1장당 YOLO txt 1개(`cls cx cy w h`, 0~1 정규화)를 생성한다.
정상데이터(shapes 없음)는 빈 txt(negative sample). 같은 이미지가 여러 zip에
등장하면 라인을 병합한다.

사용:
    python3 detection/bridge_converter.py "<dataset root>/3.개방데이터/1.데이터" \
        --out detection/labels
"""

import argparse
import collections
import json
import sys
import zipfile
from pathlib import Path

# 고정 클래스맵 (라벨 문자열은 데이터셋 원문 그대로 — Aspalt/Efflorescene 오타 포함)
CLASSES = [
    "ConcreteCrack",   # 콘크리트 균열
    "Spalling",        # 박락
    "Efflorescene",    # 백태
    "Leak",            # 누수
    "Exposure",        # 철근 노출
    "SteelDefect",     # 강재 부식
    "PaintDamage",     # 도장 박리
    "AspaltCrack",     # 아스팔트 균열
    "Pothole",         # 함몰
]
CLASS_ID = {name: i for i, name in enumerate(CLASSES)}

SPLIT_DIRS = {"train": "Training", "val": "Validation"}


def polygon_to_yolo_line(shape, img_w, img_h, stats):
    """폴리곤 1개 -> YOLO 라인. 무효 도형은 None (stats에 사유 집계)."""
    label = shape.get("label")
    cls = CLASS_ID.get(label)
    if cls is None:
        stats["unknown_label:" + str(label)] += 1
        return None
    points = shape.get("points") or []
    if len(points) < 3:
        stats["degenerate_polygon"] += 1
        return None
    xs = [min(max(x, 0.0), img_w) for x, y in points]
    ys = [min(max(y, 0.0), img_h) for x, y in points]
    x0, x1 = min(xs), max(xs)
    y0, y1 = min(ys), max(ys)
    bw, bh = x1 - x0, y1 - y0
    if bw <= 0 or bh <= 0:
        stats["zero_area_bbox"] += 1
        return None
    cx, cy = (x0 + x1) / 2 / img_w, (y0 + y1) / 2 / img_h
    return f"{cls} {cx:.6f} {cy:.6f} {bw / img_w:.6f} {bh / img_h:.6f}"


def convert_zip(zip_path, results, stats):
    """zip 1개의 JSON 전부를 results(stem -> 라인 리스트)에 병합."""
    with zipfile.ZipFile(zip_path) as zf:
        for name in zf.namelist():
            if not name.endswith(".json"):
                continue
            data = json.loads(zf.read(name))
            img_w, img_h = data["imageWidth"], data["imageHeight"]
            stem = Path(data.get("imagePath") or name).stem
            if stem in results:
                stats["merged_duplicate_stem"] += 1
            lines = results.setdefault(stem, [])
            for shape in data.get("shapes") or []:
                line = polygon_to_yolo_line(shape, img_w, img_h, stats)
                if line is not None and line not in lines:
                    lines.append(line)
            stats["json_files"] += 1


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("dataset_root", type=Path,
                    help="Training/Validation을 포함하는 1.데이터 디렉터리")
    ap.add_argument("--out", type=Path, default=Path("detection/labels"),
                    help="출력 루트 (기본 detection/labels)")
    args = ap.parse_args()

    for split, src_dir in SPLIT_DIRS.items():
        label_dir = args.dataset_root / src_dir / "02.라벨링데이터"
        zips = sorted(label_dir.glob("*.zip"))
        if not zips:
            sys.exit(f"라벨 zip 없음: {label_dir}")
        out_dir = args.out / split
        out_dir.mkdir(parents=True, exist_ok=True)

        results = {}
        stats = collections.Counter()
        for zp in zips:
            convert_zip(zp, results, stats)
            print(f"[{split}] {zp.name}: 누적 json={stats['json_files']}", flush=True)

        class_count = collections.Counter()
        for stem, lines in results.items():
            (out_dir / f"{stem}.txt").write_text(
                "\n".join(lines) + ("\n" if lines else ""))
            for ln in lines:
                class_count[CLASSES[int(ln.split()[0])]] += 1

        empty = sum(1 for lines in results.values() if not lines)
        print(f"\n[{split}] 요약: json={stats['json_files']} txt={len(results)} "
              f"empty(정상)={empty} merged={stats['merged_duplicate_stem']}")
        for k in sorted(stats):
            if k.startswith(("unknown_label", "degenerate", "zero_area")):
                print(f"[{split}] 경고 {k}: {stats[k]}")
        for name in CLASSES:
            print(f"[{split}] box {name}: {class_count[name]}")

    (args.out / "classes.txt").write_text("\n".join(CLASSES) + "\n")
    print(f"\nclasses.txt 작성: {args.out / 'classes.txt'}")


if __name__ == "__main__":
    main()
