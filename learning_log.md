# learning_log

이 파일은 b2d_geo 구축 전 과정의 학습 기록이다. 각 Phase 완료 시 상세히 기록하며,
실패한 시도와 그 원인을 성공한 결과보다 우선해서 남긴다.

---

## [2026-07-03] Phase 0 — 프로젝트 골격 구축 (메인 오케스트레이터)

### 무엇을 했는가

개발 환경을 구성하고 프로젝트 골격을 세웠다. 구체적으로:

1. 개발 도구 설치: Go 1.26.4, colima 0.10.3(컨테이너 런타임), docker CLI 29.6.1,
   docker compose 5.3.0. 모두 Homebrew로 설치했다.
2. `b2d_geo`를 독립 git 저장소로 초기화했다. 이전에는 홈 디렉터리에 있는 다른
   저장소의 하위 디렉터리(untracked)였기 때문에, 프로젝트 자체의 커밋 이력을
   가질 수 없는 상태였다.
3. Go 모듈(`github.com/JO-HEEJIN/b2d_geo`)과 지시서 5절의 디렉터리 구조를 만들었다.
   각 internal 패키지에는 역할을 설명하는 doc.go만 두었다 (실제 코드는 담당 Phase에서).
4. docker-compose.yml로 PostGIS 16-3.4 컨테이너를 정의하고 기동했다.
5. UML 문서의 ERD를 그대로 옮긴 초기 스키마(migrations/001_init.sql)와,
   Go 표준 라이브러리 embed 기반의 자체 마이그레이션 러너를 작성해 실제로 적용했다.
6. 서브에이전트 4종(b2d-data, b2d-engine, b2d-api, b2d-qa) 정의,
   GitHub Actions CI(go vet, go test, staticcheck), Makefile, README를 작성했다.

### 왜 이렇게 설계했는가 (대안과 기각 사유)

**colima vs Docker Desktop.** macOS에서 docker 컨테이너를 돌리려면 리눅스 VM이
필요하다(컨테이너는 리눅스 커널 기능이기 때문). Docker Desktop은 GUI 앱이라
설치에 수동 개입이 필요하고 기업 환경에서는 유료 라이선스 문제도 있다. colima는
brew 명령만으로 설치와 기동이 끝나는 CLI 도구라서 1인 운영 원칙(BR-07)에 더 맞다.

**마이그레이션 러너를 직접 작성한 이유.** golang-migrate 같은 기성 도구가 있지만,
지시서의 의존성 최소화 원칙(직접 의존성 5개 이하)에 따라 stdlib의 `embed`로 SQL
파일을 바이너리에 내장하고 60줄짜리 러너를 직접 작성했다. 우리에게 필요한 기능은
"번호 순서로 적용, 적용 이력 기록, 트랜잭션 보장" 세 가지뿐이고, 이는 기성 도구의
수천 줄을 들여올 이유가 되지 못한다. 롤백(down migration) 기능은 의도적으로 뺐다.
운영 원칙이 "적용된 파일은 수정하지 않고 신규 번호로 추가"이므로 down이 필요 없다.

**스키마에서 buildings의 자연키를 확정하지 않은 이유.** juso.go.kr 파일 명세서를
아직 실물로 확인하지 않았다. 건물관리번호(25자리)가 유니크 키일 가능성이 높지만,
명세 확인 전에 코드에 박으면 R-04(좌표계 오판)와 같은 부류의 사고가 된다.
Phase 1에서 명세를 확보한 뒤 별도 마이그레이션(002_)으로 추가한다.

**parcels.geom을 Polygon이 아닌 MultiPolygon으로 한 이유.** ERD 초안에는 Polygon으로
되어 있었으나, 실제 지적 데이터에는 하나의 필지가 도로 등으로 분리된 다중 폴리곤인
경우가 존재한다. Polygon으로 잡으면 적재 단계에서 해당 행이 전부 실패한다.
MultiPolygon은 Polygon을 포함하는 상위 타입이라 안전한 쪽으로 선택했다.

### 막힌 지점과 해결 과정 (에러 메시지 원문 포함)

**1. 마이그레이션 첫 실행이 인증 오류로 실패했다.**

```
migrate failed: create schema_migrations: failed to connect to `user=b2d database=b2d_geo`:
	[::1]:5432 (localhost): tls error: server refused TLS connection
	127.0.0.1:5432 (localhost): server error: FATAL: role "b2d" does not exist (SQLSTATE 28000)
```

컨테이너는 healthy 상태였는데 "role b2d does not exist"라니 이상했다. `lsof`로
5432 포트를 확인하니 원인이 드러났다: 이 머신에는 Homebrew로 설치된
postgresql@17이 서비스로 돌면서 이미 127.0.0.1:5432를 점유하고 있었다.
컨테이너의 포트 포워딩(ssh, 와일드카드 `*:5432`)도 함께 listen 중이었지만,
운영체제는 더 구체적인 바인딩(127.0.0.1)을 우선하므로 localhost로 접속하면
컨테이너가 아니라 로컬 postgres 17에 연결됐던 것이다. 그 서버에는 당연히 b2d
롤이 없다. 해결: 기존 서비스를 중단시키는 대신(다른 프로젝트가 쓸 수 있다)
컨테이너를 호스트 포트 5433으로 옮겼다. 교훈: "connection refused"가 아니라
"role does not exist"처럼 서버가 응답은 하는데 내용이 이상한 오류는,
엉뚱한 서버에 연결됐을 가능성부터 의심해야 한다.

**2. 과거 Docker Desktop의 잔재 설정이 남아 있었다.**

`~/.docker/config.json`에 `"credsStore": "desktop"`이 남아 있었는데, 해당 자격증명
헬퍼 바이너리(docker-credential-desktop)는 Docker Desktop 제거와 함께 사라진
상태였다. 이 상태로는 이미지 pull 시 자격증명 조회 단계에서 실패할 수 있어
해당 항목을 제거하고, brew로 설치한 compose 플러그인 경로(cliPluginsExtraDirs)를
추가했다. `docker compose version`과 hello-world pull로 정상 동작을 확인했다.

**3. PostGIS 이미지가 arm64 미지원이라 에뮬레이션으로 돈다.**

```
postgis The requested image's platform (linux/amd64) does not match the detected
host platform (linux/arm64/v8) and no specific platform was requested
```

postgis/postgis:16-3.4 공식 이미지는 amd64만 제공한다. Apple Silicon에서는
에뮬레이션으로 동작하므로 기능상 문제는 없지만 성능이 떨어진다. Phase 4의
성능 측정(NFR-01: p95 < 50ms)은 이 환경에서 측정하면 불리하게 나올 수 있다는
점을 기록해 둔다. 지시서가 이미지 버전을 명시하고 있으므로 임의로 바꾸지 않고,
성능 측정 시점에 재논의한다.

**4. caveman 도구가 이 머신에 없다.**

지시서 1.4절의 caveman 모드(에이전트 통신 토큰 압축)는 별도 도구를 전제하는
것으로 보이나 `which caveman`, Claude 플러그인 목록 모두에서 찾지 못했다.
도구 없이도 서브에이전트 보고를 "짧은 문장, 결과 중심"으로 강제하는 방식으로
취지(토큰 절약)는 지키되, 도구 설치가 필요하면 사용자 확인이 필요하다.

### 이 작업에서 배울 수 있는 핵심 개념

**PostGIS와 공간 데이터베이스.** PostgreSQL은 범용 관계형 DB이고, PostGIS는 여기에
"공간 타입"을 추가하는 확장이다. `geometry(Point, 4326)` 같은 컬럼 선언에서
Point는 도형의 종류(점), 4326은 좌표계 식별번호(SRID)다. EPSG:4326은 흔히 아는
WGS84 경위도 좌표계(GPS가 쓰는 그것)이고, 한국 공공데이터가 자주 쓰는 EPSG:5179는
미터 단위의 평면 좌표계다. 서로 다른 좌표계의 값을 섞으면 좌표가 수백 km씩
어긋나므로, 적재 시점에 `ST_Transform`으로 4326으로 통일하는 것이 이 프로젝트의
원칙이다.

**GiST 인덱스와 pg_trgm.** 일반 B-tree 인덱스는 "크다/작다"로 정렬 가능한 값에만
쓸 수 있다. 공간 데이터의 "이 점이 이 폴리곤 안에 있는가" 같은 질의는 GiST
(Generalized Search Tree)라는 별도 인덱스 구조가 필요하다. pg_trgm은 문자열을
3글자 단위 조각(trigram)으로 쪼개 유사도를 계산하는 확장으로, "미사강변데로"
같은 오타 입력에서도 "미사강변대로"를 찾게 해 준다. 둘 다 이번 스키마에
미리 만들어 두었다.

**Go의 embed.** 다음 코드가 마이그레이션 내장의 전부다.

```go
//go:embed *.sql
var FS embed.FS
```

첫 줄은 주석처럼 보이지만 컴파일러 지시자다. 빌드 시점에 같은 디렉터리의 *.sql
파일들을 바이너리 안에 복사해 넣고, FS 변수로 파일시스템처럼 읽을 수 있게 한다.
덕분에 배포 시 SQL 파일을 따로 나를 필요가 없다. 단일 바이너리 배포라는
프로젝트 목표(온프레미스 설치 단순화)와 직결되는 기능이다.

**마이그레이션 러너의 핵심 로직** (internal/store/migrate.go):

```go
tx, err := pool.Begin(ctx)                                  // 트랜잭션 시작
if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {   // SQL 파일 전체 실행
    tx.Rollback(ctx)                                        // 실패 시 전부 되돌림
    return fmt.Errorf("apply %s: %w", name, err)
}
if _, err := tx.Exec(ctx,
    `INSERT INTO schema_migrations (filename) VALUES ($1)`, name,
); err != nil {                                             // 적용 이력 기록
    tx.Rollback(ctx)
    return fmt.Errorf("record %s: %w", name, err)
}
if err := tx.Commit(ctx); err != nil { ... }                // 둘 다 성공해야 확정
```

스키마 변경과 이력 기록이 하나의 트랜잭션 안에 있다는 점이 중요하다. 중간에
실패하면 절반만 적용된 상태가 남지 않고, 재실행하면 처음부터 다시 시도한다.
`$1` 자리표시자는 pgx의 파라미터 바인딩으로, 문자열 조합 없이 값을 전달해
SQL 인젝션을 원천 차단한다 (보안 체크리스트 1번 항목의 실례).

### 검증 방법과 결과 (실제 수치)

- `go build ./...`, `go vet ./...`: 통과. 직접 의존성은 jackc/pgx/v5 하나
  (상한 5개 대비 1개 사용).
- `docker compose up -d --wait` 후 psql 직접 조회:
  `SELECT postgis_version()` → `3.4 USE_GEOS=1 USE_PROJ=1 USE_STATS=1`.
  pg_trgm 확장 활성 확인.
- 마이그레이션 적용 후 `\dt`: public 스키마에 buildings, places, parcels,
  admin_areas, api_keys, usage_log, schema_migrations 7개 테이블 확인.
- 멱등성: `b2d-etl migrate` 재실행 후에도 schema_migrations는 1행 유지
  (001_init.sql, 2026-07-03 04:47 UTC 적용). 중복 적용 없음.
- 커밋 author 확인: `git log --format='%an <%ae>'` → `JO-HEEJIN <midmost44@gmail.com>`.

### 관련 커밋 해시

- 0a883ec feat: bootstrap project skeleton with PostGIS schema and migration runner
