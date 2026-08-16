# b2d_geo 운영·인수인계 (라이브 인프라 + 다른 맥에서 이어가기)

> 이 저장소는 **private**이라 아래 인프라 식별자(IP·도메인)를 담는다.
> **SSH 개인키(.pem)·DB 비밀번호·API 키는 절대 커밋하지 않는다**(git 밖에서 관리).

`deploy/README.md`는 "처음 프로비저닝" 런북이고, 이 문서는 **이미 떠 있는 서비스의
운영과 다른 컴퓨터에서 작업을 이어가는 법**이다.

## 1. 라이브 인프라 (현재 상태)

| 항목 | 값 |
|---|---|
| 클라우드 | AWS Lightsail, **서울 리전 (ap-northeast-2a)** |
| 인스턴스 | 이름 `b2d-geo`, Ubuntu 22.04, **8GB RAM / 2 vCPU / 160GB SSD** |
| 정적 IP | **3.35.88.130** (StaticIp-1) |
| 방화벽 | 22(SSH) · 80(HTTP) · 443(HTTPS) 인바운드 열림 |
| 도메인 | **api.birth2death.com** |
| DNS | Cloudflare(birth2death.com) → `A` 레코드 `api` → 3.35.88.130, **DNS only(회색 구름)** |
| TLS | Caddy가 Let's Encrypt로 자동 발급/갱신 |
| 앱 배치 | VPS `~/b2d_geo/deploy`에서 `docker compose`(db + app + caddy) |
| DB | **전국 61GB PostGIS, VPS의 `pgdata` 볼륨에 상주**(로컬 아님) |
| 이미지 | `ghcr.io/jo-heejin/b2d_geo:latest` (main 푸시 시 CI가 빌드) |
| 자동완성 | `~/finish-watcher.sh`(복원 완료 시 키 발급+기동), 로그 `~/finish.log` |

**Cloudflare 회색 구름 유지**: 주황 구름(프록시)으로 바꾸면 Caddy의 Let's Encrypt
발급이 막힌다. 프록시가 필요하면 Caddy를 DNS-01 챌린지로 바꾸거나 origin 인증서를 써야 한다.

## 2. SSH 접속

```bash
# Lightsail 기본 키를 안전한 곳에 두고(권장 ~/.ssh/, chmod 400), git 밖에 보관
ssh -i ~/.ssh/LightsailDefaultKey-ap-northeast-2.pem ubuntu@3.35.88.130
```
키는 Lightsail 콘솔 → Account → SSH keys(또는 인스턴스 Connect 탭) → Download default key.
**저장소에 두지 말 것**(`.gitignore`에 `*.pem` 등록됨).

## 3. 다른 맥(M1 등)에서 작업 이어가기

DB가 VPS에 있으므로 **로컬 DB가 필요 없다.** 새 맥엔 다음만:

```bash
# 1) 저장소
git clone git@github.com:JO-HEEJIN/b2d_geo.git
cd b2d_geo
git config user.name "JO-HEEJIN" && git config user.email "midmost44@gmail.com"

# 2) (선택) 로컬 빌드·테스트용 Go — brew install go
#    코드만 고칠 거면 CI가 이미지를 빌드하므로 없어도 됨.

# 3) SSH 키를 이 맥으로 안전 복사(USB/암호화 전송) → ~/.ssh/, chmod 400
```

### 배포 흐름 (코드 → 라이브)
```
코드 수정 → git push (main) → CI(release.yml)가 이미지 빌드 → GHCR
          → VPS에서:  cd ~/b2d_geo/deploy && docker compose pull app && docker compose up -d app
```
- 마이그레이션이 생겼으면(스키마 변경): `docker compose run --rm --entrypoint /b2d-etl app migrate`
- 웹(프론트) 변경도 같은 이미지에 embed되므로 위 pull+up이면 반영.

> GHCR 이미지가 private면 VPS에서 1회: `echo <PAT read:packages> | docker login ghcr.io -u <user> --password-stdin`. 또는 VPS에서 `docker compose build app`(로컬 빌드)로 대체.

## 4. VPS 환경변수 (`~/b2d_geo/deploy/.env`)

| 변수 | 용도 |
|---|---|
| `DOMAIN` | api.birth2death.com |
| `POSTGRES_USER/PASSWORD/DB` | DB 접속(비번은 이 파일에만, git 밖) |
| `MOLIT_API_KEY` | 실거래(/v1/appraisal/sales)용 data.go.kr 키. **비면 실거래만 비활성** |
| `B2D_DEMO_API_KEY` | 웹 try-it-out 데모 키 |
| `B2D_SELF_SERVE_DEMO` | `true`면 개발자 셀프 키발급 노출 |
| `SENTRY_DSN` | Sentry 에러 추적 DSN. **비면 Sentry 비활성(기존 로그만)** |
| `SENTRY_ENVIRONMENT` | Sentry 환경 라벨 (기본 production) |

### Sentry 설정·확인 (에러/panic 추적)

1. sentry.io에서 프로젝트 생성(Platform: **Go**) → DSN 복사.
2. `.env`에 `SENTRY_DSN=<dsn>` 추가 → `docker compose up -d app`(재생성; `restart`는
   .env 변경을 안 읽음).
3. 확인: `docker compose logs app | grep "sentry enabled"`. 즉시 이벤트로 확인하려면
   `.env`에 `B2D_SENTRY_BOOT_PING=true`를 잠깐 넣고 재생성 — Issues에 "b2d-server
   boot ping"이 떠야 한다(확인 후 제거).
4. 수집 범위: 핸들러 panic(스택 포함, 500 응답으로 복구)과 5xx 응답. 이벤트 태그
   `request_id`가 응답 헤더 `X-Request-Id`·서버 로그(`req <id>:`)와 대조된다.
   헤더·쿼리스트링·바디·IP는 전송하지 않고, MOLIT 키는 스크러버가 치환한다.

## 5. 운영 명령 (VPS `~/b2d_geo/deploy`)

| 작업 | 명령 |
|---|---|
| 상태 | `docker compose ps` |
| 앱 로그 | `docker compose logs -f app` |
| Caddy(TLS) 로그 | `docker compose logs -f caddy` |
| 앱 재시작 | `docker compose restart app` |
| 앱 업데이트 | `docker compose pull app && docker compose up -d app` |
| 헬스 | `curl -s https://api.birth2death.com/v1/health` |
| 키 발급 | `docker compose run --rm --entrypoint /b2d-etl app apikey create --name <n> --scopes geo,realestate --rate <rps>` |
| DB 백업 | `docker compose exec -T db pg_dump -U b2d -Fc b2d_geo > backup_$(date +%F).dump` |
| DB 크기 | `docker compose exec -T db psql -U b2d -d b2d_geo -tAc "SELECT pg_size_pretty(pg_database_size('b2d_geo'));"` |

## 6. 자가 완성 워처 (초기 데이터 적재용)

`~/finish-watcher.sh`는 최초 복원(pg_restore) 완료를 기다렸다가 자동으로 데모키 발급
→ `docker compose up -d` → 공개 헬스 확인까지 한다. 진행/결과는 `~/finish.log`.
- 완료 시 로그에 `공개 완료 🎉`.
- `parcels < 3970만`이면(OOM 등 불완전) 앱을 올리지 않고 `중단` 기록 → 수동 재적재 필요
  (`--jobs=1`로 재시작 권장: 2 vCPU에선 과병렬이 I/O 스로틀을 유발).

## 6-1. geocode 인덱스 = HASH (btree 아님) — 알아둘 것

`buildings.norm_road`의 exact-match 인덱스는 **btree가 아니라 hash**다:
`idx_buildings_norm_road_hash` (`USING hash (norm_road)`).

- **왜**: 최초 복원 때 원본 스키마의 btree `idx_buildings_norm_road`가 1,283만 텍스트
  "sorting live tuples" 단계에서 **2시간+ 진행 0**(2 vCPU + Lightsail I/O 스로틀에서 대용량
  텍스트 정렬이 사실상 안 끝남). geocode 쿼리(`geocode.go` 87행 `WHERE norm_road = $1`)는
  **등호만** 쓰므로 **정렬이 없는 hash 인덱스로 충분** → **54초**에 빌드 완료.
- **효과**: geocode 20.7s → **0.03~0.1s** (전국 검증).
- **주의**: 스냅샷을 새 환경에 복원하면 원본 스키마의 **btree 정의가 딸려온다.** 큰 인스턴스가
  아니면 그 btree 빌드가 또 멈출 수 있으니, 필요 시 btree 대신 위 hash로 대체한다:
  ```sql
  SET maintenance_work_mem='2GB';
  CREATE INDEX idx_buildings_norm_road_hash ON buildings USING hash (norm_road);
  ```
  (LIKE/유사도 경로는 기존 GIN trgm 인덱스 `idx_buildings_norm_road_trgm`가 이미 처리.)

## 7. "진짜 서비스" 백로그 (M1에서 이어서)

지금도 세 부류(일반인·개발자·감평사) 조회는 다 되지만, 상용화하려면:

1. **결제·과금** — 최소 구성 도입(2026-08-16, Track A Phase 2): 티어 2종(demo/standard)
   + 월 하드캡 + 토스 정기결제 링크 수동 플로우(7-1절). 풀 빌링 자동화는 비목표.
2. **rate limit 실운영값** — 티어 파라미터는 키 발급 시 지정(7-1절 자리표시자 확정 필요).
3. **MOLIT 키 투입** — 완료(2026-08-08). `.env`에 키 투입 후 `docker compose up -d app`으로
   재생성해야 env가 반영된다 (`restart`는 .env 변경을 다시 읽지 않음). 키가 비면 앱이
   상류 호출 없이 503 `SALES_DISABLED`를 반환한다.
4. **모니터링/알림** — 업타임·에러·비용 알림(Lightsail 스냅샷 백업은 켜져 있음).
5. **Cloudflare 프록시 전환**(선택) — DDoS/캐시가 필요하면 origin 인증서 방식으로.

## 7-1. 셀프서브 유료 티어 운영 플로우 (수동, 2026-08-16 도입)

원칙: 자동 과금 없음. 정액 + 월 하드캡. 초과분 종량 청구를 만들지 않는다
(이 세그먼트의 구매 이유가 요금 예측 가능성이기 때문).

### 티어 구조

| 티어 | 대상 | rate | 월 캡 | 가격 |
|---|---|---|---|---|
| demo | 무료 소량(기존 정책) | 2 rps | 없음 | 무료 |
| standard | 유료 정액 1티어 | {RATE_STANDARD} rps | {CAP_STANDARD} 호출/월 | {PRICE_SELF_SERVE}/월 |

자리표시자 3개({RATE_STANDARD}, {CAP_STANDARD}, {PRICE_SELF_SERVE})는 사용자가 확정한다.
pricing.html의 가격 자리표시자와 일치시킬 것.

### 플로우

1. 고객이 토스 정기결제 링크({TOSS_PAYMENT_LINK})로 결제한다.
2. 결제 확인(토스 알림 또는 수동 대조) 후 VPS에서 키 발급:
   ```bash
   docker compose run --rm --entrypoint /b2d-etl app apikey create \
     --name "<고객명>" --scopes geo,realestate \
     --rate {RATE_STANDARD} --tier standard --monthly-cap {CAP_STANDARD}
   ```
3. 평문 키를 고객에게 1회 전달(DB에는 해시만 저장, 재표시 불가).
4. 해지·미납 시 revoke: `UPDATE api_keys SET revoked_at = now() WHERE id = <id>;`
   (B2D_AUTH_CACHE_TTL 기본 30초 내 반영.)

### 동작 방식

- 월 캡은 usage_log 이달(UTC 기준) 합계로 판정. 캡 도달 시 429 +
  `monthly_cap_exceeded` 코드와 업그레이드 안내(pricing.html 링크)를 반환하고,
  다음 달에 자동으로 풀린다. meter가 1초 주기 비동기 flush라 캡 판정에 그 정도
  오차가 있다(하드캡 허용 오차).
- `/v1/usage`가 tier, monthly_cap, month_used, month_remaining을 반환하므로
  고객이 잔여량을 스스로 확인할 수 있다. 이 메타 엔드포인트는 캡 도달 후에도
  호출 가능하고, 캡 소모로 집계되지도 않는다(잔여량 폴링이 쿼터를 깎지 않게).
- 기존 키는 마이그레이션 시 tier='demo', monthly_cap=0(캡 없음)으로 남아 동작 불변.
  운영 키의 티어 라벨을 바꾸려면: `UPDATE api_keys SET tier='standard', monthly_cap=<n> WHERE id=<id>;`

## 8. 보안 메모

- `.pem` 키·`.env`·`*_snapshot.dump`는 `.gitignore` 처리됨 — 커밋 금지.
- DB 포트는 호스트 비노출(compose 내부 네트워크만).
- 앱은 distroless·nonroot. 응답에 기본 보안 헤더(nosniff·SAMEORIGIN·Referrer-Policy).
- 데이터 재배포는 공공누리 약관(출처표시) 준수. 네이버/카카오/구글 데이터 혼입 금지.
