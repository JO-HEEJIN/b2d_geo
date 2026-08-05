# b2d_geo — 단일 VPS 배포 런북 (전국 전체 DB)

VPS 하나에 **PostGIS + 앱(API+웹) + Caddy(자동 HTTPS)**를 docker-compose로 올린다.
앱은 API(`/v1/*`)와 웹 사이트를 같은 오리진에서 서빙하므로 프론트 별도 호스팅이 없다.

```
DNS ──> [VPS] ── Caddy(443, 자동 TLS) ──> app:8080 ──> PostGIS(내부)
```

## 권장 프로바이더 — AWS Lightsail 서울 리전

토지 데이터는 한국 사용자를 향하므로 **서울 리전(ap-northeast-2)**이 지연 면에서
유리하다. Lightsail은 고정 요금 + 정적 IP·DNS·스냅샷·방화벽이 내장돼 단일 VPS
운영이 가장 단순하다.

- **인스턴스**: Ubuntu 22.04, **8GB RAM / 2 vCPU / 160GB SSD** (월 약 $44).
  160GB가 전체 DB 61GB + 인덱스 + WAL을 수용하고, 8GB RAM이 공간쿼리 성능을 낸다.
  (4GB/80GB 플랜은 디스크가 부족하다.)
- **네트워킹**: 정적 IP 연결(Attach static IP). 방화벽에서 **HTTP(80)·HTTPS(443)**
  인바운드 규칙 추가(기본 22만 열려 있음).
- **DNS**: Lightsail DNS zone(또는 Route 53)에서 `A` 레코드 → 정적 IP.
- **백업**: Lightsail 자동 스냅샷 켜기(디스크 전체 스냅샷).

접속 후 아래 1~9를 그대로 따르면 된다(어느 Ubuntu VPS에서도 동일하게 동작한다 —
Vultr/라이트로드/Naver Cloud 등도 서울 리전 + 위 사양이면 대체 가능).

## 0. 준비물 체크

- [ ] VPS: **디스크 100GB+**, **RAM 8GB**(최소 4GB), 2 vCPU+. Ubuntu 22.04/24.04 권장.
      (전체 DB 61GB + 인덱스 + WAL 여유. RAM은 공간쿼리 성능을 좌우.)
- [ ] 도메인 + DNS A레코드를 VPS 공인 IP로.
- [ ] 로컬(현재 개발기)에서 전체 스냅샷 파일 **`b2d_snapshot.dump`(약 7.2GB)**.
- [ ] 방화벽에서 80/443 인바운드 허용(TLS 발급·서비스).

## 1. 스냅샷 만들기 (로컬 개발기)

```bash
# 전체 DB를 custom-format으로 덤프(약 7.2GB). api_keys/usage_log는 스키마만.
./scripts/snapshot.sh b2d_snapshot.dump
```

> 스냅샷 배포 = 공식 공공데이터 재배포 → 공공누리 출처표시 등 약관 준수(BRD §6).

## 2. VPS 기본 세팅

```bash
# Docker + compose 플러그인 설치(공식 편의 스크립트)
curl -fsSL https://get.docker.com | sh

# 저장소 가져오기
git clone https://github.com/JO-HEEJIN/b2d_geo.git
cd b2d_geo/deploy

# 환경 설정
cp .env.example .env
vi .env   # DOMAIN, POSTGRES_PASSWORD(강한 값), MOLIT_API_KEY 등
```

## 3. 앱 이미지 준비 (둘 중 하나)

**A. GHCR에서 pull(권장)** — `main` 푸시 시 CI가 `ghcr.io/jo-heejin/b2d_geo:latest`를 빌드해 둔다.
패키지가 private면 VPS에서 한 번 로그인:
```bash
echo <GITHUB_PAT_read:packages> | docker login ghcr.io -u <github-user> --password-stdin
docker compose pull app
```
(패키지를 public으로 바꾸면 로그인 불필요.)

**B. VPS에서 직접 빌드** — GHCR 없이:
```bash
docker compose build app
```

## 4. DB 기동 + 전체 데이터 복원

```bash
# DB만 먼저 올려 헬시 대기
docker compose up -d db
docker compose exec db pg_isready -U b2d -d b2d_geo   # accepting connections 확인

# 스냅샷을 VPS로 전송(로컬에서 실행)
#   scp b2d_snapshot.dump user@<vps>:~/b2d_geo/deploy/

# 컨테이너로 밀어넣어 복원(--clean 없이 빈 DB에). 7.2GB → 수십 분 소요.
docker compose cp b2d_snapshot.dump db:/tmp/snap.dump
docker compose exec db pg_restore -U b2d -d b2d_geo --no-owner --jobs=4 /tmp/snap.dump
docker compose exec db rm /tmp/snap.dump

# 스키마만 있는 api_keys는 복원 후에도 비어 있음(정상 — 아래서 데모 키 발급).
```

> 참고: 지역만 축소해 저비용으로 시작하려면 전체 대신 `scripts/demo-snapshot.sh`
> 산출물을 쓴다(README 상위 `docs/09_Web_Deploy_b2d_geo.md`).

## 5. 데모 키 발급 → .env 반영

```bash
docker compose run --rm --entrypoint /b2d-etl app \
  apikey create --name demo-frontend --scopes geo,realestate --rate 5
# 출력된 PLAINTEXT KEY를 .env의 B2D_DEMO_API_KEY에 붙여넣기
vi .env
```

## 6. 전체 기동

```bash
docker compose up -d          # db, app, caddy 모두
docker compose ps             # 상태 확인
docker compose logs -f caddy  # TLS 발급 로그(도메인 DNS가 맞아야 성공)
```

DNS가 VPS를 가리키면 Caddy가 Let's Encrypt로 인증서를 자동 발급한다.

## 7. 확인

```bash
curl -s https://<DOMAIN>/v1/health          # {"status":"ok"}
```
브라우저로 `https://<DOMAIN>/` → 랜딩, `/map.html` 지도 데모, `/docs.html` API 문서.

## 8. 운영

| 작업 | 명령 |
|---|---|
| 로그 | `docker compose logs -f app` |
| 재시작 | `docker compose restart app` |
| 앱 업데이트 | `docker compose pull app && docker compose up -d app` (또는 `build`) |
| DB 백업 | `docker compose exec db pg_dump -U b2d -Fc b2d_geo > backup.dump` |
| 중지 | `docker compose down` (볼륨 유지) |

## 9. 보안 메모

- `POSTGRES_PASSWORD`는 강한 값. DB 포트는 호스트에 노출하지 않는다(compose 내부 네트워크).
- 앱은 distroless·nonroot로 실행. 응답에 기본 보안 헤더 부착(nosniff·SAMEORIGIN·Referrer-Policy).
- `B2D_SELF_SERVE_DEMO=true`는 공개 키 발급을 여니 필요할 때만.
- `.env`는 커밋 금지(.gitignore 처리됨).
- 데이터 재배포는 공공누리 약관 준수(출처표시). 네이버/카카오/구글 데이터 혼입 금지.
