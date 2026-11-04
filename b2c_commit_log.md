# b2c 커밋 로그

b2c 브랜치에 합쳐진 feat 브랜치의 커밋 메시지와 PR description을 합친 순서대로 기록한다.

## 1. `feat/b2c-keywords` ([#9](https://github.com/JO-HEEJIN/b2d_geo/pull/9))

### 커밋

- `51a960a` feat(b2c): add module keyword dictionary

  Map each B2C module number to an English name so file names and code read
  the same for anyone, and record the shared browsing principle in the package
  doc.

### Description

Adds src/b2c/keywords.go.

- Maps module numbers 1-5 to English names: district, floor_plan, recent_trades, infra_map, remaining_listings. The original Korean keyword is kept as a comment next to each name.
- Module files are prefixed with their key number (1_district.go = Keywords[1]).
- The package doc records the browsing principle shared by every module: show many homes, sort them, never prune them for the user.

## 2. `feat/b2c-district` ([#10](https://github.com/JO-HEEJIN/b2d_geo/pull/10))

### 커밋

- `5f01802` feat(b2c): classify and cluster the first seven districts

  Covers Songdo-dong, Dongtan 1, Dongtan 2, Hanam, Yongin Suji, Suwon and
  Jamsil. Addresses are matched by city/district plus legal dong, so Dongtan 1
  and 2 are split by dong name. A centroid fallback handles inputs without a
  dong, and k-means groups complexes inside a zone into neighborhoods.

### Description

Module 1 (`district`, 동이나 구): `src/b2c/1_district.go`

#### What it does
`DistrictClassifier` handles the first seven zones only: 인천 송도동, 1동탄, 2동탄, 하남, 용인 수지, 수원, 잠실.

- `Classify(address)`: finds the zone for a lot-number address or a road address with its dong in parentheses. Administrative dongs such as 잠실2동 are folded into the legal dong (잠실동).
- `NearestZone(lon, lat)`: falls back to the nearest zone center when an address has no dong. The centers are approximate.
- `Cluster(points, k)`: deterministic k-means that groups complexes inside a zone into neighborhoods.
- `Zones()`: lists the supported zones.

#### Notes
- Dongtan 1 and 2 are split by legal dong (1: 반송·석우·능동, 2: 영천·청계·오산·목·산척·장지·송·방교·중·신·금곡동). A road address with no dong cannot be split by text alone.
- Jamsil covers 잠실동 and 신천동.
- Zone boundary polygons are not used yet. Zone centers only back up the address match.

#### Verification
`go test ./src/b2c`: classification cases (including 양천구 목동 vs 화성 목동), nearest zone, and clustering all pass.


## 3. `feat/b2c-floor-plan` ([#11](https://github.com/JO-HEEJIN/b2d_geo/pull/11))

### 커밋

- `01b4742` feat(b2c): compare floor plan types and plan in-person viewings

  The same complex can have different bathroom positions and living room
  layouts under one size, so each type is kept as its own variant. The catalog
  compares two types field by field, lists the units built with each layout,
  and builds a tour that visits every type without dropping any.

### Description

Module 2 (`floor_plan`, 아파트의 구조): `src/b2c/2_floor_plan.go`

#### Problem
Units of the same size in the same complex can have a different bathroom position or living room layout. A floor plan image alone is not enough, so buyers need to go and see the unit.

#### What it does
`FloorPlanCatalog` stores floor plan types for each complex (84A, 84B, ...).

- `AddVariant`: registers a type with rooms, bathrooms, bays, bathroom position, living room layout, orientation, and the units built with that layout.
- `Variants`: returns every type, sorted by area and then name. Nothing is filtered out, because browsing many layouts is part of the fun.
- `Compare(a, b)`: lists only the fields that differ between two types.
- `UnitsWithLayout`: lists the actual units with that layout, so the buyer knows which door to knock on.
- `TourPlan`: one stop per type. A type with no viewable unit yet still stays on the plan.
- `ViewingChecklist`: things to check in person (bathroom, living room, light, noise, storage), followed by the remodeling and maintenance items a buyer pays for after signing.

#### Notes
- Floor plan images should only come from sources with clear rights, such as an official sales notice. Where the floor plan and unit data will come from still needs approval.

#### Verification
`go test ./src/b2c`: ordering, field diffs, unknown type error, tour stops and checklist all pass.


## 4. `feat/b2c-recent-trades` ([#12](https://github.com/JO-HEEJIN/b2d_geo/pull/12))

### 커밋

- `80a885e` feat(b2c): judge neighborhood liveliness from six months of trades

  Counts non-canceled apartment trades in the last six months, the average
  gap between them and the trend between the two three-month halves. A fast
  trade cycle is treated as proof the area is alive, and the verdict text
  follows how buyers actually feel: fear of missing a home, relief that
  others bought there, and the remodeling bill that comes after signing.

### Description

Module 3 (`recent_trades`, 최근 실거래): `src/b2c/3_recent_trades.go`

#### Why
How many times an area traded in the last six months is the key signal. In Dongtan, for example, many SK hynix employees look for homes at once, so listings sell as soon as they appear and buyers sometimes cannot find one at all. A fast trade cycle like that proves the area is far from dead.

#### What it does
`RecentTradeAnalyzer`
- `Fetch(ctx, lawdCode)`: pulls the last six months from a `TradeSource`. The source is an interface; connecting the MOLIT apartment trade API needs approval first.
- `Activity(trades, complex)`: counts trades with canceled ones excluded and listed separately, plus average gap, days since the last trade, and the recent 3 months vs the earlier 3 months.
  - hot: 24 or more trades (about one a week or faster)
  - active: 6 or more (about one a month)
  - slow: 1 to 5
  - dormant: none
- `Verdict(activity, buildYear)`: verdict text based on buyer emotion.
  - hot: the home you like may be gone while you hesitate, so book a second viewing right away, and still fill in the checklist.
  - active: other people decided to buy here, so the area is a sound basis for a decision.
  - Cautions: rising demand, canceled trades, and the remodeling cost that comes with building age (15+ and 25+ years).

#### Verification
`go test ./src/b2c`: level thresholds, window/complex/canceled filtering, verdict cautions and the fetch window all pass.


## 5. `feat/b2c-infra-map` ([#13](https://github.com/JO-HEEJIN/b2d_geo/pull/13))

### 커밋

- `1b3f085` style(b2c): gofmt the trade activity struct

  Realigns the TradeActivity fields after an empty trailing comment was
  removed.

- `ae37181` feat(b2c): show walking time to subway, bus, convenience store and hospital

  Every summary lists the four facility kinds in a fixed order with the
  nearest one and its walking time, estimated from straight-line distance
  with a detour factor at 80 m per minute. Convenience stores and hospitals
  reuse the existing places nearby search; kinds without a loaded source are
  shown as pending instead of disappearing.

### Description

Module 4 (`infra_map`, 인프라맵): `src/b2c/4_infra_map.go`

#### What it does
`InfraMap` shows how many minutes on foot it takes to reach each nearby facility.

- `Summary(lon, lat)`: one line each for subway station, bus stop, convenience store and hospital, always in that order. A line shows the nearest place, or why there is none: "데이터 준비 중" (no data yet) or "not within N minutes on foot".
- `Nearest` / `Around`: the nearest place, or a list sorted by distance, each with walking minutes.
- `WalkMinutes(distance)`: straight-line distance × 1.3 detour factor ÷ 80 m per minute, rounded up, minimum 1 minute.

#### Reuse of existing b2d_geo code
`PlacesSource` wraps `internal/places.Nearby`, the KNN search behind the existing map's "주변 상가" overlay, and uses it for kinds stored in the small-business store data (convenience stores, hospitals).

#### Needs approval / follow-up
- The store-data category codes are left as constructor arguments. They get filled in only after checking them against the loaded data.
- Subway stations and bus stops are not loaded in b2d_geo yet. They need an ETL from public data such as the national bus stop locations and urban rail station data. Until then they show as "데이터 준비 중".
- Walking time is a straight-line estimate. Real walking routes would need a road network.

#### Also in this PR
- `style(b2c)`: gofmt fix for the TradeActivity struct from module 3.

#### Verification
`go vet`, `go test ./src/b2c` and `go build ./...` pass. The walking-minute table, summary order and missing-data lines, distance sorting and error propagation are covered.


## 6. `feat/b2c-remaining-listings` ([#14](https://github.com/JO-HEEJIN/b2d_geo/pull/14))

### 커밋

- `e5239eb` feat(b2c): keep remaining listings fresh with a persistent systemd timer

  Each refresh compares the current listings with the previous run, so new
  listings and the ones that just sold are both visible. State is saved to a
  JSON file atomically between runs, and the module renders the oneshot
  service and the every-minute timer with Persistent=true so a missed run is
  caught up right after the server comes back.

### Description

Module 5 (`remaining_listings`, 남은 매물): `src/b2c/5_remaining_listings.go`

#### Why
Listings sell very fast, so buyers need to see what is still left right now. Naver offers this in real time. This module is our answer.

#### What it does
`RemainingListings`
- `Refresh(ctx)`: compares the source's current listings with the previous state and returns new listings (Added) and gone ones (Gone, sold or taken down). If the source fails, the state is left untouched.
- `Remaining(complex)`: every listing still on the market. Nothing is filtered out (eye-shopping principle).
- `AddedSince(t)`: new listings, for alerts.
- `RecentlyGone(t)`: listings that just sold, kept for 7 days. Seeing "just sold" lets buyers feel the area is actually trading.
- `LoadState` / `SaveState`: carries state between timer runs in a JSON file. Writes go to a temp file followed by a rename, so an interrupted run never leaves half a file.
- `SystemdUnits(execStart)`: renders `b2c-listings.service` (oneshot) and `b2c-listings.timer` (`OnCalendar=*:*:00`, `AccuracySec=1s`, `Persistent=true`).

#### Why a systemd timer instead of cron
- `Persistent=true` runs a missed refresh immediately after boot or downtime.
- Every run is logged in `journalctl -u b2c-listings`.
- The unit files are placed on the AWS Lightsail server and edited with `ned`.

#### Data source rule
`ListingSource` only accepts licensed listing feeds or listings registered directly by agents and owners. Scraping portals such as Naver is forbidden by project rule 1.1, so this PR contains no crawler code. Which source to use needs approval.

#### Needs approval / follow-up
- The CLI entry point for `execStart` (one Refresh then exit) and the server install steps.
- Choosing and contracting a listing source.

#### Verification
`go test -race ./src/b2c`: added/gone tracking, first-seen preservation, state kept on fetch error, state file round trip, and unit file contents all pass. The local machine is macOS, so `systemd-analyze verify` has not been run yet. Run it on the server before enabling the timer.


