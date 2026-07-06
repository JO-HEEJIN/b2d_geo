#!/usr/bin/env python3
"""희소 클래스 오버샘플링: 병합 COCO json에서 희소 클래스 포함 이미지를 복제.

DEIM 데이터로더에는 repeat-factor sampler가 없어 json 수준에서 이미지 엔트리를
복제한다 (file_name 동일, image_id/ann_id 신규). 이미지에 여러 희소 클래스가
있으면 최대 배율 하나만 적용.

배율 근거 (병합 후 train 박스): SteelDefect 258 / PaintDamage 399 / Spalling 810
— 각각 x50/x40/x20으로 유효 박스를 1만 안팎까지 끌어올림. Pothole은 RDD 병합으로
6,256이라 제외.

사용:
    detection/.venv/bin/python detection/coco_oversample.py
"""

import json
from pathlib import Path

REPEAT = {5: 50, 6: 40, 1: 20}  # SteelDefect, PaintDamage, Spalling (0-기반 id)

DET = Path(__file__).parent


def main():
    src = DET / "coco" / "instances_train_rdd.json"
    coco = json.loads(src.read_text())

    anns_by_img = {}
    for a in coco["annotations"]:
        anns_by_img.setdefault(a["image_id"], []).append(a)

    img_id = max(i["id"] for i in coco["images"])
    ann_id = max(a["id"] for a in coco["annotations"])
    added_img = added_box = 0

    for img in list(coco["images"]):
        anns = anns_by_img.get(img["id"], [])
        factor = max((REPEAT.get(a["category_id"], 1) for a in anns), default=1)
        for _ in range(factor - 1):
            img_id += 1
            coco["images"].append({**img, "id": img_id})
            added_img += 1
            for a in anns:
                ann_id += 1
                coco["annotations"].append({**a, "id": ann_id, "image_id": img_id})
                added_box += 1

    out = DET / "coco" / "instances_train_rdd_os.json"
    out.write_text(json.dumps(coco))

    from collections import Counter
    cnt = Counter(a["category_id"] for a in coco["annotations"])
    names = {c["id"]: c["name"] for c in coco["categories"]}
    print(f"images={len(coco['images'])} (+{added_img}) annotations={len(coco['annotations'])} (+{added_box})")
    for cid in sorted(cnt):
        print(f"  {names[cid]}: {cnt[cid]}")


if __name__ == "__main__":
    main()
