# 2026-09-13 앱 배포·공개 문서 게시 기록

상태: **운영 앱 배포 및 공개 문서 게시 완료**. 배포 시각 2026-09-13 03:40 KST, 후속 확인 03:42 KST.

이 기록은 [배포 전 운영 안내](RUNTIME_CACHE_RATE_LIMIT.md)와 [상세 설계](../docs/design/etl-temporal-architecture.md)의 배포 전 상태표를 보완한다. 상세 설계의 ETL·과거 시점 조회·Cloudflare 구성을 구현했다는 뜻이 아니다.

## 적용 범위

- 사용자 제공 PNG를 변경 없이 로고·파비콘·Apple touch icon·공유 이미지로 연결. 8개 HTML 페이지에 적용했다.
- 프록시 뒤 IP 제한을 위한 명시적 신뢰 CIDR 처리를 배포했다. 현재 앱 방향 Caddy 주소만 `172.18.0.4/32`로 지정했다.
- 기존 키별/IP별 제한과 기본 30초 인증 결과 캐시는 유지했다. Go API 응답 캐시는 추가하지 않았다.
- 운영 앱 컨테이너만 교체했다. DB 마이그레이션·ETL·DB 데이터 변경·Caddy/MCP 재시작은 하지 않았다. 정상 API 점검에 따른 기존 사용량 계측은 발생할 수 있다.
- Cloudflare 계정·Cache Rules·Rate Limiting Rules·DNS·방화벽은 변경하지 않았다. 배포 전 DNS 조회는 원본 직접 연결이었고 후속 API 응답에도 Cloudflare 캐시 상태 헤더는 없었다.

## 소스·이미지 식별

| 항목 | 값 |
|---|---|
| 로고·프록시·상세 설계 커밋 | `0f5edd2e19e279e08efca9c4975d1b58a9686374` |
| 최종 배포 소스 커밋 | `28f3d0db5ab573e1bc4053d2aea0b0f4abe46c1b` |
| 배포 이미지 | `ghcr.io/jo-heejin/b2d_geo@sha256:e690b1def06d0fdd7387942316b4cfa6627c6e69f3feaac475726536477bad4d` |
| 이전 실행 이미지 ID | `sha256:927b2bea5d76bd24c84bf0d153a60e06a3216c221e7477c40db0c6084391cd8f` |
| 보존한 이전 이미지 태그 | `b2d-geo-app:rollback-20260913-28f3d0d` |
| 브랜드 자산 SHA-256 | `c1abd2177e3819c5d37fb68c7865f852ce1cc739dac6eebd8b55752527485545` |

첫 CI는 테스트의 동일한 함수 호출을 `||`로 이어 쓴 표현식 때문에 staticcheck SA4000에서 실패했다. 최종 커밋은 해당 테스트만 순차 검증 형태로 고쳤으며, 서로 다른 IP를 명시해 같은 키의 제한이 유지되는지도 검사한다. 제품 실행 코드에는 추가 변경이 없다.

- [최종 CI 성공](https://github.com/JO-HEEJIN/b2d_geo/actions/runs/34711676810): vet, test, govulncheck, staticcheck.
- [최종 이미지 빌드 성공](https://github.com/JO-HEEJIN/b2d_geo/actions/runs/34711676816).
- 로컬 전체 Go 테스트·vet·관련 API/auth/web race 검사·staticcheck 통과.
- GitNexus는 제품 프록시 변경의 공통 인증 경로 8개를 HIGH로 분류했다. 테스트 표현 수정은 테스트 함수 범위의 LOW였다. 정적 분석을 운영 무장애 보증으로 해석하지 않는다.

## 기존 운영 설정 보존

운영 체크아웃에는 기존 Caddyfile·docker-compose.yml 수정이 있어 git pull/reset으로 교체하지 않았다. 배포 전 base compose의 앱 환경값과 기존 컨테이너의 환경값을 메모리에서 비교했으며 차이가 없었다. 비밀값은 출력하지 않았다.

운영 deploy 디렉터리에 다음 파일을 새로 만들었다.

- `docker-compose.override.yml`: 최종 이미지 digest, 명시한 Caddy CIDR, 인증 캐시 TTL만 덮어쓴다.
- `docker-compose.rollback-20260913.yml`: 보존한 이전 앱 이미지 태그를 가리킨다.

운영의 base compose와 Caddyfile은 배포 전후 체크섬이 같았다. DB·Caddy·MCP 컨테이너 ID도 같았다. 앱 새 컨테이너는 `05b5daa16e20`, 후속 점검 시 재시작 횟수는 0이었다.

운영 체크아웃의 Git HEAD는 실행 중 앱 소스 버전을 뜻하지 않는다. 실제 앱 버전은 위 digest 및 이미지 revision label로 확인한다. 이후 배포 작업에서 override를 누락하면 다른 이미지로 되돌아갈 수 있다.

명시적 적용 명령은 운영 deploy 디렉터리에서 다음과 같다.

```sh
docker compose -f docker-compose.yml -f docker-compose.override.yml up -d --no-deps --pull never app
```

`-f docker-compose.yml`만 사용하는 기존 자동화는 override를 자동 포함하지 않으므로 검토해야 한다. 전체 스택을 재생성하거나 Caddy 주소가 바뀌면 실제 프록시 IP를 재확인한 뒤 신뢰 설정을 갱신한다. 신뢰 주소를 넓은 사설망 전체로 확대하지 않는다.

## 롤백

롤백 이미지와 파일은 서버에 보존했다. 실제 롤백은 필요하지 않아 실행하지 않았다. 배포 스크립트에는 이미지·상태·브랜드 검증 실패 시 이전 앱으로 복구하는 경로를 준비했다.

필요 시 운영 deploy 디렉터리에서 현재 override를 별도 이름으로 백업한 뒤, 승인된 롤백 파일을 활성 override로 복사하고 앱만 다시 생성한다.

```sh
cp -p docker-compose.override.yml docker-compose.pre-rollback-20260913.yml
cp -p docker-compose.rollback-20260913.yml docker-compose.override.yml
docker compose -f docker-compose.yml -f docker-compose.override.yml up -d --no-deps --pull never app
```

위 백업 이름이 이미 있으면 덮어쓰지 말고 새 이름을 사용한다. 롤백 후에도 health·인증 API·MCP를 다시 확인한다. 이미지 태그 보존은 DB 백업의 대체물이 아니다.

## 운영 검증 결과

| 점검 | 결과 |
|---|---|
| `/`, map/docs/pricing/batch/privacy/terms/refund HTML | 8개 모두 200, 동일 브랜드 이미지·파비콘·버전 CSS 참조 |
| 브랜드 PNG | 200, image/png, 사용자 원본과 SHA-256 일치 |
| 실제 브라우저 홈 화면 | 새 로고 로딩, 34×34px, 파비콘 경로 확인 |
| `/v1/health` | 200, X-Request-Id 존재 |
| 인증 없는 지오코딩 | 401, X-Request-Id 존재 |
| 기존 공개 데모 키로 지오코딩 | 200, 주소·좌표 결과 구조 확인; 키는 출력·저장하지 않음 |
| MCP tools/list | 200, 도구 7개 |
| 원본 변경 보존 | base compose/Caddyfile 체크섬 및 DB·Caddy·MCP ID 불변 |

이 검증은 소량의 정상 요청이다. 운영 부하 테스트, 외부 IP 여러 개를 이용한 429 격리 테스트, Cloudflare 경유 신뢰 체인 테스트, 전체 지도 데이터의 품질 검증은 수행하지 않았다. 프록시 파싱·버킷 격리·키 제한은 로컬 테스트로 확인했다.

## 공개 게시 범위

- [공개 설계문서](https://github.com/JO-HEEJIN/b2d_geo_public/blob/main/docs/design/temporal-data-publication.md): 242줄, Mermaid 다이어그램 5개, 기존 README 연결.
- 공개 커밋: `7a800191da380fb15f05da2f7053648b0176e35a`.
- [공개 저장소 CI 성공](https://github.com/JO-HEEJIN/b2d_geo_public/actions/runs/34711582769).
- 실제 업로드된 문서를 다시 조회해 제안 상태 표시와 다이어그램 5개를 확인했다.
- 원천별 갱신 정책, 비동기 ETL, 시간 의미, 불변 버전, manifest, A/B·canary, 온라인 과거 조회, 캐시·요청 제한, 검증·보존 범위만 공개했다.
- 운영 주소·접속 경로·키·실제 배포 식별자·사적 대화·사업 문서·원자료는 공개본에 포함하지 않았다. 이 배포 기록 자체는 비공개 저장소용이다.

## 후속 작업

1. 실제 Cloudflare 계정·요금제·기존 룰을 확인하고 별도 승인된 변경으로 앞단 보호·정적 캐시를 구성한다. 인증·과금 API/MCP는 우선 BYPASS한다.
2. [상세 설계](../docs/design/etl-temporal-architecture.md)에 따라 보유 과거 원본과 coverage를 확정한 후 시범 지역의 버전 적재·조회부터 구현한다.
3. 프록시 주소 변경, 캐시 규칙, 원본 우회 방어, 전역 제한·정확한 과금 요구는 각각 검증한다. 이번 브랜드 배포가 이 후속 작업을 대신하지 않는다.
