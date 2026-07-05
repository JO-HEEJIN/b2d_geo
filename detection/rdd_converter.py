#!/usr/bin/env python3
"""RDD2022 (Japan/India/Czech) VOC xml -> YOLO 변환 + 이미지 추출.

data/rdd2022/RDD2022_{국가}.zip 의 train 분할에서, 9클래스 매핑되는 박스가
1개 이상인 이미지만 채택해 detection/rdd/{images,labels}/train 에 쓴다.
val 은 만들지 않는다 (평가는 AI Hub val 고정, RDD 는 학습 보강 전용).

매핑: 균열 계열(D00/D01/D10/D11/D20/D0w0) -> AspaltCrack(7), D40 -> Pothole(8).
D43/D44/D50(차선 흐림/맨홀)은 손상이 아니므로 배경 처리(스킵).

사용:
    python3 detection/rdd_converter.py --src data/rdd2022 --out detection/rdd
"""

import argparse
import collections
import os
import xml.etree.ElementTree as ET
import zipfile
from pathlib import Path

COUNTRIES = ("Japan", "India", "Czech")
CLASS_MAP = {
    "D00": 7, "D01": 7, "D10": 7, "D11": 7, "D20": 7, "D0w0": 7,  # AspaltCrack
    "D40": 8,                                                      # Pothole
}


def xml_to_lines(root, stats):
    """VOC annotation -> YOLO 라인 리스트 (매핑 없는 라벨은 stats 집계 후 스킵)."""
    w = float(root.find("size").findtext("width"))
    h = float(root.find("size").findtext("height"))
    lines = []
    for obj in root.findall("object"):
        name = obj.findtext("name")
        cls = CLASS_MAP.get(name)
        if cls is None:
            stats["skip_label:" + name] += 1
            continue
        bb = obj.find("bndbox")
        x0 = min(max(float(bb.findtext("xmin")), 0), w)
        x1 = min(max(float(bb.findtext("xmax")), 0), w)
        y0 = min(max(float(bb.findtext("ymin")), 0), h)
        y1 = min(max(float(bb.findtext("ymax")), 0), h)
        if x1 - x0 <= 0 or y1 - y0 <= 0:
            stats["zero_area_bbox"] += 1
            continue
        lines.append(f"{cls} {(x0 + x1) / 2 / w:.6f} {(y0 + y1) / 2 / h:.6f} "
                     f"{(x1 - x0) / w:.6f} {(y1 - y0) / h:.6f}")
        stats["box_cls" + str(cls)] += 1
    return lines


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--src", type=Path, default=Path("data/rdd2022"))
    ap.add_argument("--out", type=Path, default=Path("detection/rdd"))
    args = ap.parse_args()

    img_dir = args.out / "images" / "train"
    lab_dir = args.out / "labels" / "train"
    img_dir.mkdir(parents=True, exist_ok=True)
    lab_dir.mkdir(parents=True, exist_ok=True)

    for country in COUNTRIES:
        stats = collections.Counter()
        with zipfile.ZipFile(args.src / f"RDD2022_{country}.zip") as zf:
            for name in zf.namelist():
                if "/train/annotations/" not in name or not name.endswith(".xml"):
                    continue
                stats["xml"] += 1
                root = ET.fromstring(zf.read(name))
                lines = xml_to_lines(root, stats)
                if not lines:
                    stats["img_rejected"] += 1
                    continue
                stem = Path(name).stem
                img_entry = f"{country}/train/images/{stem}.jpg"
                with zf.open(img_entry) as fsrc, \
                        open(img_dir / f"{stem}.jpg", "wb") as fdst:
                    fdst.write(fsrc.read())
                (lab_dir / f"{stem}.txt").write_text("\n".join(lines) + "\n")
                stats["img_adopted"] += 1
        print(f"[{country}] " + " ".join(f"{k}={v}" for k, v in sorted(stats.items())))

    n_img = len(os.listdir(img_dir))
    n_lab = len(os.listdir(lab_dir))
    print(f"\nTOTAL images={n_img} labels={n_lab} (일치: {n_img == n_lab})")


if __name__ == "__main__":
    main()
