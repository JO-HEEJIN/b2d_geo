# 10. 데이터 재취득 (로컬 `data/` 삭제 후 복원)

> 작성: 2026-08-06. M1 이전을 위해 로컬 `data/`(약 15GB)와 ETL 원본을 **삭제**하면서
> 작성한 재다운 안내. **삭제해도 안전한 이유**: 이 원본에서 만든 **DB는 이미 VPS에
> 적재·서비스 중**(api.birth2death.com)이다. `data/`는 **ETL을 다시 돌릴 때만** 필요.
> ML(detection) 데이터 복원은 `detection/RESTORE.md` 참고.

## 무엇을 지웠나 (`data/`, gitignore)

| 디렉토리/파일 | 내용 | 출처 |
|---|---|---|
| `연속지적도/` (6.1G) | 필지 폴리곤 shapefile | 국가공간정보포털(NSDI)/V-World |
| `nsdi/` (4.1G) | 용도지역·토지이용 등 공간레이어 | NSDI |
| `juso_shape/`, `juso_entrance/` (1.5G) | 도로명주소 shapefile·출입구 | business.juso.go.kr |
| `rdd2022/` (1.7G) | ML 보강 데이터셋(도로손상) | → `detection/RESTORE.md` |
| `AL_D151_*`, `AL_D195_*` (csv/zip) | 도로명주소 한글 DB(전체분/시도별) | business.juso.go.kr |
| `표준지공시지가.zip`, `토지특성정보.zip` | 감정평가 원본 | data.go.kr / NSDI |
| `molit/`, `reb/` | 실거래·지가변동률 API 산출물 | 국토교통부(data.go.kr), 한국부동산원 R-ONE |

## 재다운 (필요 시)

전부 **공개데이터**. ETL 코드(`internal/etl/*`)와 CLI(`b2d-etl`)는 git에 있음.

- **도로명주소**: `business.juso.go.kr` → 개발자센터/자료제공. 또는 `b2d-etl download`
  경로가 참조하는 `business.juso.go.kr/api/jst/download`, `.../selectAttrbDBDwldList`.
- **data.go.kr 파일데이터**: 표준지공시지가·토지특성·실거래 등. 예: 데이터셋 `15083033`
  (`data.go.kr/data/15083033/fileData.do`). 상세 데이터셋은 `docs/07_Appraisal_Datasets_b2d_geo.md`.
- **연속지적도/NSDI 공간레이어**: 국가공간정보포털(nsdi.go.kr) / V-World.
- **실거래·지가변동률 API**: 국토교통부(MOLIT) 실거래 API, 한국부동산원 R-ONE. 키는
  `.env`(`MOLIT_API_KEY`, `REB_API_KEY`).

## 재적재 (원본 받은 뒤)

```bash
make up && make migrate                 # 로컬 PostGIS + 스키마
b2d-etl <juso|jijeok|land-use|standard-lots|price-index|...>   # 서브커맨드별 적재
```
서브커맨드 목록: `b2d-etl` (인자 없이 실행). 상세는 `learning_log.md`(Phase별 적재 기록).

## 더 빠른 대안 — VPS DB 스냅샷

ETL을 다시 돌리는 대신, **이미 적재된 VPS DB를 그대로 받는 게 빠르다**:
```bash
# VPS에서 덤프(약 7.2GB compressed)
docker compose exec -T db pg_dump -U b2d -Fc b2d_geo > b2d_snapshot.dump
# 로컬/새 환경에 복원 (deploy/README.md §4 참고)
```
즉 **원본 재다운 없이 DB 자체를 복제**하면 전국 데이터를 그대로 얻는다. 이게 사실상
`data/` 재다운보다 실용적이다(원본 → ETL 재적재는 느림). 라이선스: 공공누리 출처표시 준수.
