#!/usr/bin/env python3
"""YOLO 라벨(detection/labels) -> COCO instances json 변환기 (DEIMv2 입력용).

bridge_converter.py 산출물(정규화 cx cy w h, 512x512 전수 확인)을 읽어
detection/coco/instances_{train,val}.json 을 생성한다.
- bbox: 절대픽셀 [x, y, w, h], category_id: 0-기반 (DEIM은 remap off 시
  category_id를 라벨 인덱스로 직접 사용 — 1-기반이면 CUDA index assert)
- 빈 txt(정상데이터)도 images[]에 포함 (negative sample)

사용:
    python3 detection/coco_converter.py --labels detection/labels --out detection/coco
"""

import argparse
import json
from pathlib import Path

IMG_SIZE = 512  # AI Hub 040 전수 512x512 (QA 검증 완료)


def convert_split(label_dir, img_size):
    """txt 디렉터리 -> (images, annotations) 리스트."""
    images, annotations = [], []
    ann_id = 1
    for img_id, txt in enumerate(sorted(label_dir.glob("*.txt")), start=1):
        images.append({
            "id": img_id,
            "file_name": txt.stem + ".jpg",
            "width": img_size,
            "height": img_size,
        })
        for line in txt.read_text().splitlines():
            cls, cx, cy, w, h = line.split()
            bw, bh = float(w) * img_size, float(h) * img_size
            x = float(cx) * img_size - bw / 2
            y = float(cy) * img_size - bh / 2
            annotations.append({
                "id": ann_id,
                "image_id": img_id,
                "category_id": int(cls),
                "bbox": [round(x, 2), round(y, 2), round(bw, 2), round(bh, 2)],
                "area": round(bw * bh, 2),
                "iscrowd": 0,
            })
            ann_id += 1
    return images, annotations


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--labels", type=Path, default=Path("detection/labels"))
    ap.add_argument("--out", type=Path, default=Path("detection/coco"))
    args = ap.parse_args()

    classes = (args.labels / "classes.txt").read_text().split()
    categories = [{"id": i, "name": n} for i, n in enumerate(classes)]
    args.out.mkdir(parents=True, exist_ok=True)

    for split in ("train", "val"):
        images, annotations = convert_split(args.labels / split, IMG_SIZE)
        out_file = args.out / f"instances_{split}.json"
        out_file.write_text(json.dumps({
            "images": images,
            "annotations": annotations,
            "categories": categories,
        }))
        empty = len(images) - len({a["image_id"] for a in annotations})
        print(f"[{split}] images={len(images)} annotations={len(annotations)} "
              f"empty(정상)={empty} -> {out_file}")


if __name__ == "__main__":
    main()
