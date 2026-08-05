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

## 7. "진짜 서비스" 백로그 (M1에서 이어서)

지금도 세 부류(일반인·개발자·감평사) 조회는 다 되지만, 상용화하려면:

1. **결제·과금** — 현재 데모키(무료)뿐. Lemon Squeezy 등 결제 연동 + 티어별 발급 자동화.
   (요금표는 `docs/08_Deployment_Pricing_b2d_geo.md`.)
2. **rate limit 실운영값** — 티어별 rps/월한도 조정(현재 데모는 낮게).
3. **MOLIT 키 투입** — 실거래 활성화하려면 `.env`의 `MOLIT_API_KEY` 채우고 `restart app`.
4. **모니터링/알림** — 업타임·에러·비용 알림(Lightsail 스냅샷 백업은 켜져 있음).
5. **Cloudflare 프록시 전환**(선택) — DDoS/캐시가 필요하면 origin 인증서 방식으로.

## 8. 보안 메모

- `.pem` 키·`.env`·`*_snapshot.dump`는 `.gitignore` 처리됨 — 커밋 금지.
- DB 포트는 호스트 비노출(compose 내부 네트워크만).
- 앱은 distroless·nonroot. 응답에 기본 보안 헤더(nosniff·SAMEORIGIN·Referrer-Policy).
- 데이터 재배포는 공공누리 약관(출처표시) 준수. 네이버/카카오/구글 데이터 혼입 금지.
