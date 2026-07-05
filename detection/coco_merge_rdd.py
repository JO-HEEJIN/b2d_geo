#!/usr/bin/env python3
"""AI Hub COCO json에 RDD2022 보강분을 병합 -> instances_train_rdd.json.

RDD 이미지는 크기가 제각각이라 jpg에서 실측한다 (PIL).
병합 json의 file_name은 detection/ 루트 기준 상대경로
(images/train/... , rdd/images/train/...) — DEIM config에서
img_folder를 detection 루트로 지정해 두 디렉터리를 혼용한다.
val은 병합하지 않는다 (AI Hub 순수 유지).

사용:
    detection/.venv/bin/python detection/coco_merge_rdd.py
"""

import json
from pathlib import Path

from PIL import Image

DET = Path(__file__).parent


def main():
    coco = json.loads((DET / "coco" / "instances_train.json").read_text())
    for img in coco["images"]:
        img["file_name"] = "images/train/" + img["file_name"]

    img_id = max(i["id"] for i in coco["images"])
    ann_id = max(a["id"] for a in coco["annotations"])
    added_img = added_ann = 0

    for txt in sorted((DET / "rdd" / "labels" / "train").glob("*.txt")):
        jpg = DET / "rdd" / "images" / "train" / (txt.stem + ".jpg")
        w, h = Image.open(jpg).size
        img_id += 1
        coco["images"].append({
            "id": img_id,
            "file_name": "rdd/images/train/" + jpg.name,
            "width": w,
            "height": h,
        })
        added_img += 1
        for line in txt.read_text().splitlines():
            cls, cx, cy, bw, bh = line.split()
            pw, ph = float(bw) * w, float(bh) * h
            x = float(cx) * w - pw / 2
            y = float(cy) * h - ph / 2
            ann_id += 1
            coco["annotations"].append({
                "id": ann_id,
                "image_id": img_id,
                "category_id": int(cls) + 1,
                "bbox": [round(x, 2), round(y, 2), round(pw, 2), round(ph, 2)],
                "area": round(pw * ph, 2),
                "iscrowd": 0,
            })
            added_ann += 1

    out = DET / "coco" / "instances_train_rdd.json"
    out.write_text(json.dumps(coco))
    print(f"images={len(coco['images'])} (+{added_img}) "
          f"annotations={len(coco['annotations'])} (+{added_ann}) -> {out}")


if __name__ == "__main__":
    main()
