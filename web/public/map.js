// 지도 화면: 주소 검색 또는 지도 클릭 -> 그 필지의 공시지가·용도지역·토지특성·최근
// 실거래·지가변동률을 모아 4열 리포트로 보여주고 A4로 출력한다.
//   [사진+지도 1/3 | 거래내역 | 지가변동률 | 판단 요약 + 출력]  (2026-10 컨설팅 화이트보드)
// 좌표는 모두 [lon, lat](GeoJSON 순서); Leaflet에는 [lat, lon]으로 넘긴다.
// 모든 값은 공공데이터 사실이며, 요약 문장도 그 사실을 이어 붙인 것뿐이다
// (감정평가·가격 판단 없음).

// Leaflet 기본 마커 아이콘 경로를 vendor로. mergeOptions로 전체 URL을 주면
// Leaflet이 CSS에서 자동 감지한 imagePath를 그 앞에 또 붙여 경로가 중복된다
// (/vendor/images/vendor/images/... 404). imagePath만 지정하면 기본 파일명
// (marker-icon.png 등)에 이 경로가 붙는다.
L.Icon.Default.imagePath = "/vendor/images/";

const map = L.map("map", { zoomControl: true }).setView([37.5665, 126.978], 11);
const osmLayer = L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
  maxZoom: 19,
  attribution: '© OpenStreetMap',
}).addTo(map);
let aerialLayer = null; // 메인 지도용 V-World 영상+라벨. 키가 있을 때만 만든다.
let photoMap = null;    // "해당 토지 사진" 칸의 항공사진 지도
let photoParcel = null;

const app = document.getElementById("app");
const results = document.getElementById("results");
let pinMarker = null;
let parcelLayer = null;
let poiLayer = L.layerGroup().addTo(map);
let lotsLayer = L.layerGroup().addTo(map);
let salesLayer = L.layerGroup().addTo(map);
let salesMarkers = new Map(); // 법정동명 -> marker
let lastPoint = null; // {lat, lon}

// 조회 1회의 상태. 새 조회가 시작되면 queryId가 바뀌어, 늦게 도착한 이전 응답이
// 화면을 덮어쓰지 않는다.
let queryId = 0;
let R = null;

const EXAMPLES = [
  "경기도 하남시 대청로 10",
  "서울특별시 중구 세종대로 110",
  "경기도 성남시 분당구 판교역로 235",
];
const SALES_MONTHS = 3;
const SPIN = `<span class="spin"></span>`;

// ---------- 조회 진입점 ----------
async function queryPoint(lat, lon, opts = {}) {
  const my = ++queryId;
  lastPoint = { lat, lon };
  R = { address: null, parcel: null, umd: "", sgg: "", price: null, zones: [], features: null,
        sales: null, index: null, done: { price: false, sales: false, index: false } };
  setPin(lat, lon);
  enterReportLayout();
  if (!opts.keepView) map.setView([lat, lon], Math.max(map.getZoom(), 17));
  results.innerHTML =
    colShell("col-sales", "거래내역", "", `<div class="loading">${SPIN}</div>`) +
    colShell("col-index", "지가변동률", "", `<div class="loading">${SPIN}</div>`) +
    colShell("col-summary", "판단 요약", "", `<div class="loading">${SPIN}토지 정보 조회 중…</div>`);

  // 1) 역지오코딩 + 필지(폴리곤 포함)를 병렬로.
  const [rev, parcel] = await Promise.allSettled([
    api("/reverse", { lat, lon }),
    api("/parcel/by-point", { lat, lon, geometry: "true" }),
  ]);
  if (my !== queryId) return;

  R.address = addressOf(rev, parcel);
  const parcelData = parcel.status === "fulfilled" ? parcel.value : null;
  if (parcelData && parcelData.geometry) drawParcel(parcelData.geometry);
  else clearParcel();
  updatePhoto(parcelData && parcelData.geometry, lat, lon);
  renderPrintHead();

  if (!parcelData || !parcelData.pnu) {
    const none = `<p class="muted">이 지점에서 필지를 찾지 못해 조회하지 않았습니다.</p>`;
    results.innerHTML =
      colShell("col-sales", "거래내역", "", none) +
      colShell("col-index", "지가변동률", "", none) +
      colShell("col-summary", "판단 요약", "", addrBlock() +
        `<p class="muted">이 지점에서 필지를 찾지 못했습니다. 건물·도로·하천 위이거나 아직 데이터가 없는 지역일 수 있습니다.</p>`);
    salesLayer.clearLayers();
    refreshOverlays();
    return;
  }

  R.parcel = parcelData;
  R.sgg = (parcelData.bcode || "").slice(0, 5);
  R.umd = lastToken(parcelData.address_name || "");
  results.innerHTML = colSales() + colIndex() + colSummary();
  renderSummary();
  wireDeep(my);

  // 2) 감정평가 필드들을 병렬 조회.
  const pnu = parcelData.pnu;
  const [price, landUse, features] = await Promise.allSettled([
    api("/appraisal/official-price", { pnu }),
    api("/appraisal/land-use", { pnu }),
    api("/appraisal/land-features", { pnu }),
  ]);
  if (my !== queryId) return;
  R.price = price.status === "fulfilled" ? price.value : null;
  R.done.price = true;
  R.zones = landUse.status === "fulfilled" ? landUse.value.zones || [] : [];
  R.features = features.status === "fulfilled" ? features.value : null;

  const facts = document.getElementById("facts");
  if (facts) {
    facts.innerHTML = renderPrice(price) + renderLandUse(landUse) + renderFeatures(features) + renderParcelCard(parcelData);
  }
  // 대표 용도지역이 정해진 뒤에 지가변동률을 한 번만 부른다. 기본값(주거지역)과 실제
  // 용도지역을 동시에 부르면 늦게 온 응답이 이겨 다른 분류 값이 표시된다(QA MED-1).
  const ziClass = document.getElementById("ziClass");
  if (ziClass) {
    ziClass.value = zoneClassOf(mainZone(R.zones));
    loadPriceIndex(R.sgg, ziClass.value, my);
  }
  renderSummary();
  refreshOverlays();
}

async function searchAddress(q) {
  if (!q || q.length < 5) return;
  enterReportLayout();
  results.innerHTML = colShell("col-summary", "판단 요약", "", `<div class="loading">${SPIN}주소 검색 중…</div>`);
  try {
    const r = await api("/geocode", { q });
    if (r.match === "ambiguous") {
      results.innerHTML = colShell("col-summary", "검색 결과", "",
        `<div class="muted">여러 후보가 있습니다. 더 구체적으로 입력하세요.</div>` +
        (r.candidates || []).map((c) => `<div class="poi-item"><span class="pname">${esc(c)}</span></div>`).join(""));
      return;
    }
    if (!r.point) throw new Error("좌표를 찾지 못했습니다");
    const [lon, lat] = r.point;
    await queryPoint(lat, lon);
  } catch (e) {
    results.innerHTML = colShell("col-summary", "판단 요약", "", `<div class="err-line">검색 실패: ${esc(errText(e))}</div>`);
  }
}

// 결과가 생기면 4열 리포트 배치로 바꾼다. 그리드가 바뀌므로 Leaflet에 크기 변경을 알린다.
function enterReportLayout() {
  if (app.classList.contains("has-report")) return;
  app.classList.add("has-report");
  setTimeout(() => { map.invalidateSize(); if (photoMap) photoMap.invalidateSize(); }, 60);
}

// ---------- 열(column) 골격 ----------
function colShell(cls, title, sub, body) {
  return `<div class="col ${cls}"><div class="col-title">${title}${sub ? ` <small>${sub}</small>` : ""}</div>${body}</div>`;
}

function colSales() {
  return colShell("col-sales", "거래내역", `주변 지역 · 최근 ${SALES_MONTHS}개월 · 시간순`,
    `<div class="seg filter no-print" role="group" aria-label="거래 범위">
       <button type="button" class="seg-btn on" data-filter="all">주변 전체</button>
       <button type="button" class="seg-btn" data-filter="same">같은 동만</button>
     </div>
     <div id="salesOut">${SPIN}최근 거래를 모으는 중…</div>
     <p class="note">국토교통부 실거래 공개 자료는 지번 일부가 비공개입니다. 지도에는 법정동 단위로 표시하고, 거래를 누르면 지도가 그 동으로 이동합니다.
       거래 신고 기한이 계약 후 30일이라 가장 최근 달은 아직 덜 집계됐을 수 있습니다.</p>`);
}

const ZONE_CLASSES = ["주거지역", "상업지역", "공업지역", "녹지지역"];
const RANGES = [["1년", 12], ["3년", 36], ["5년", 60], ["전체", 0]];
let ziSeries = [];
let ziRange = 36;

function colIndex() {
  const zopts = ZONE_CLASSES.map((z) => `<option value="${z}">${z}</option>`).join("");
  return colShell("col-index", "지가변동률", "월별 · 전월 대비 %",
    `<div class="deep-ctl no-print">
       <select id="ziClass" aria-label="용도지역">${zopts}</select>
       <button type="button" class="btn ghost sm no-print" id="ziExpand">크게 보기</button>
     </div>
     <div class="deep-ctl no-print"><div class="seg" role="group" aria-label="기간">${segButtons()}</div></div>
     <div class="range-label" id="ziRangeLabel">&nbsp;</div>
     <div id="ziOut" class="chart-box">${SPIN}</div>
     <div id="ziTable"></div>`);
}

function segButtons() {
  return RANGES.map(([l, n]) => `<button type="button" class="seg-btn${n === ziRange ? " on" : ""}" data-range="${n}">${l}</button>`).join("");
}

function colSummary() {
  return colShell("col-summary", "판단 요약", "공공데이터 사실 기반",
    addrBlock() +
    `<div id="summary"></div>
     <div id="facts"><div class="loading">${SPIN}</div></div>
     <p class="unit-note">면적은 ㎡(헤베)와 평을 함께 표시합니다. 1평 = 3.3058㎡</p>
     ${disclaimerHTML()}
     <div class="print-box no-print">
       <button class="btn" id="printBtn" type="button">보고서 자동 작성 · 다운로드</button>
       <small>A4 · 인쇄 창에서 "PDF로 저장"을 고르면 파일로 받습니다</small>
     </div>`);
}

function addrBlock() {
  const a = R && R.address ? R.address : { road: "주소 미상", sub: "" };
  const pnu = R && R.parcel ? `<div class="pnu">PNU ${esc(R.parcel.pnu)}</div>` : "";
  return `<div class="addr-block"><div class="addr-line">${esc(a.road)}</div><div class="addr-sub">${esc(a.sub)}</div>${pnu}</div>`;
}

function disclaimerHTML() {
  return `<p class="disclaimer">이 화면과 리포트는 국토교통부·한국부동산원·국가공간정보 공공데이터를 모아 보여주는 참고 자료입니다.
    감정평가(감정평가법에 따른 가격 판단)가 아니며, 공식 확인은 각 기관의 원본 자료로 하시기 바랍니다.</p>`;
}

function renderPrintHead() {
  const el = document.getElementById("printHead");
  if (!el) return;
  const d = new Date();
  const ds = `${d.getFullYear()}.${String(d.getMonth() + 1).padStart(2, "0")}.${String(d.getDate()).padStart(2, "0")}`;
  el.innerHTML = `<div class="ph-title">올맵 토지 정보 보고서</div>
    <div class="ph-meta">${esc(R && R.address ? R.address.road : "")} · 출력일 ${ds}</div>`;
}

// ---------- 세부 카드 (판단 요약 열) ----------
function addressOf(rev, parcel) {
  const road =
    (rev.status === "fulfilled" && rev.value.road_address) ||
    (parcel.status === "fulfilled" && parcel.value.address_name) ||
    "주소 미상";
  const sub =
    (rev.status === "fulfilled" && rev.value.address_name) ||
    (parcel.status === "fulfilled" &&
      `${parcel.value.address_name || ""} ${parcel.value.jibun || ""}`) ||
    "";
  return { road, sub: sub.trim() };
}

function renderParcelCard(p) {
  return `<div class="card"><header>필지</header><div class="body">
    <dl class="kv">
      <dt>지번</dt><dd>${esc(p.jibun || "-")}</dd>
      <dt>지목</dt><dd>${esc(p.land_category || "-")}</dd>
      <dt>법정동</dt><dd>${esc(p.address_name || "-")}</dd>
      <dt>기준</dt><dd class="mono">${esc(p.source_version || "-")}</dd>
    </dl>
  </div></div>`;
}

function priceTypeLabel(p) {
  // STANDARD는 이 필지 자체가 표준지라는 뜻이다(서버가 standard_lots에서 pnu로 찾음). 추정값 아님.
  if (p.price_type === "STANDARD") return "표준지공시지가";
  if (p.price_type === "INDIVIDUAL") return "개별공시지가";
  return "공시지가"; // 모르는 값은 원문을 내보내지 않는다(innerHTML로 들어가는 자리)
}

function renderPrice(price) {
  if (price.status !== "fulfilled") return card404("공시지가", "이 지역은 아직 공시지가 데이터가 없습니다");
  const p = price.value;
  return `<div class="card"><header>공시지가 <span class="tag">${esc(p.base_year)}년</span></header><div class="body">
    <div class="price">${formatWon(p.price_per_sqm)}<span class="unit">원/㎡</span></div>
    <div class="price-sub">평당 약 ${fmtManwonNum(perPyeongManwon(p.price_per_sqm))}</div>
    <div class="price-type">${priceTypeLabel(p)}</div>
  </div></div>`;
}

function renderFeatures(f) {
  if (f.status !== "fulfilled") return "";
  const v = f.value;
  return `<div class="card"><header>토지특성 <span class="tag">${esc(v.base_year || "")}</span></header><div class="body">
    <dl class="kv">
      <dt>이용상황</dt><dd>${esc(v.land_use || "-")}</dd>
      <dt>지형고저</dt><dd>${esc(v.terrain_height || "-")}</dd>
      <dt>형상</dt><dd>${esc(v.terrain_shape || "-")}</dd>
      <dt>도로접면</dt><dd>${esc(v.road_side || "-")}</dd>
    </dl>
  </div></div>`;
}

function renderLandUse(lu) {
  if (lu.status !== "fulfilled" || !lu.value.zones || !lu.value.zones.length) return "";
  const zones = lu.value.zones;
  const shown = zones.slice(0, 6);
  const rows = shown
    .map(
      (z) => `<div class="zone">
        <span class="rel ${relClass(z.relation)}">${esc(z.relation || "")}</span>
        <span class="zname">${esc(z.zone_name)}</span>
        <span class="zcode">${esc(z.zone_code)}</span>
      </div>`
    )
    .join("");
  const more = zones.length > shown.length ? `<div class="zone-more">외 ${zones.length - shown.length}개 지역·지구</div>` : "";
  return `<div class="card"><header>용도지역·지구 <span class="tag">${zones.length}건</span></header><div class="body">
    <div class="zones">${rows}${more}</div>
  </div></div>`;
}

function card404(title, msg) {
  return `<div class="card"><header>${esc(title)}</header><div class="body muted">${esc(msg)}</div></div>`;
}

// ---------- 판단 요약 (타일 + 문장) ----------
// 대표 용도지역: 원자료에서 "포함" 관계이고 이름이 "지역"으로 끝나는 첫 항목.
function mainZone(zones) {
  const z = zones.find((x) => x.relation === "포함" && /지역$/.test(x.zone_name || "")) ||
            zones.find((x) => /지역$/.test(x.zone_name || ""));
  return z ? z.zone_name : "";
}

function zoneClassOf(zoneName) {
  if (/주거/.test(zoneName)) return "주거지역";
  if (/상업/.test(zoneName)) return "상업지역";
  if (/공업/.test(zoneName)) return "공업지역";
  if (/녹지/.test(zoneName)) return "녹지지역";
  return "주거지역";
}

function tile(label, value, sub, cls = "") {
  return `<div class="tile ${cls}"><div class="t-label">${label}</div><div class="t-value">${value}</div><div class="t-sub">${sub}</div></div>`;
}

function sameSales() {
  return R && R.sales ? R.sales.items.filter((x) => x.same) : [];
}

function renderSummary() {
  const el = document.getElementById("summary");
  if (!el || !R || !R.parcel) return;

  const zoneName = mainZone(R.zones);
  const cat = R.parcel.land_category || "";
  const landUse = R.features && R.features.land_use ? R.features.land_use : "";

  let priceTile;
  if (!R.done.price) priceTile = tile("공시지가", SPIN, "&nbsp;");
  else if (!R.price) priceTile = tile("공시지가", "-", "데이터 없음");
  else priceTile = tile("공시지가", `${formatWon(R.price.price_per_sqm)}<small>원/㎡</small>`,
    `평당 약 ${fmtManwonNum(perPyeongManwon(R.price.price_per_sqm))} · ${esc(R.price.base_year)}년`, "gold");

  let salesTile;
  if (!R.done.sales) salesTile = tile("같은 동 최근 거래", SPIN, "&nbsp;", "wide");
  else if (!R.sales) salesTile = tile("같은 동 최근 거래", "-", "조회 실패", "wide");
  else {
    const same = sameSales();
    const med = median(same.filter((x) => !x.share).map((x) => x.perPy));
    salesTile = tile(`${esc(R.umd || "같은 동")} ${SALES_MONTHS}개월 거래`, `${same.length}<small>건</small>`,
      same.length ? (isFinite(med) ? `평당 중위 ${fmtManwonNum(med)} (지목 구분 없음, 지분거래 제외)` : "평당가 계산 불가") : `주변 전체 ${R.sales.items.length}건`, "wide");
  }

  let idxTile;
  if (!R.done.index) idxTile = tile("지가변동률", SPIN, "&nbsp;");
  else if (!R.index) idxTile = tile("지가변동률", "-", "데이터 없음");
  else {
    const v = R.index.last.rate_pct;
    idxTile = tile("지가변동률 (월)", `${v > 0 ? "+" : ""}${v}<small>%</small>`,
      `${esc(fmtMonth(R.index.last.month))} · ${esc(R.index.cls)}`, v >= 0 ? "up" : "down");
  }

  const zoneTile = tile("용도지역", zoneName ? esc(zoneName) : (R.done.price ? "-" : SPIN),
    `지목 ${esc(cat || "-")}${landUse ? " · " + esc(landUse) : ""}`, "wide");

  // 좁은 열에서도 긴 용도지역명이 잘리지 않도록 용도지역·거래 칸은 가로 전체를 쓴다.
  el.innerHTML = `<div class="tiles">${zoneTile}${priceTile}${idxTile}${salesTile}</div>
    <p class="summary-text">${summarySentences(zoneName, cat)}</p>`;
}

// 사실만 이어 붙인 요약 문장. 평가·전망 표현 금지.
function summarySentences(zoneName, cat) {
  const out = [];
  out.push(`${zoneName ? esc(zoneName) + "에 속한 " : ""}지목 ${esc(cat || "미상")} 토지입니다.`);
  if (R.price) {
    out.push(`${esc(R.price.base_year)}년 공시지가는 ㎡당 ${formatWon(R.price.price_per_sqm)}원(평당 약 ${fmtManwonNum(perPyeongManwon(R.price.price_per_sqm))})이며 ${priceTypeLabel(R.price)} 값입니다.`);
  }
  if (R.sales) {
    const same = sameSales();
    const med = median(same.filter((x) => !x.share).map((x) => x.perPy));
    if (same.length) {
      out.push(`같은 동(${esc(R.umd)})에서 최근 ${SALES_MONTHS}개월간 토지 거래 ${same.length}건이 신고됐고` +
        (isFinite(med) ? `, 평당 거래가 중위값(지목 구분 없음, 지분거래 제외)은 약 ${fmtManwonNum(med)}입니다.` : " 평당가는 계산할 수 없습니다."));
    } else {
      out.push(`같은 동(${esc(R.umd)})의 최근 ${SALES_MONTHS}개월 토지 거래 신고는 없고, 주변(같은 시군구) 전체로는 ${R.sales.items.length}건입니다.`);
    }
  }
  if (R.index) {
    const v = R.index.last.rate_pct;
    out.push(`${esc(sggName())} ${esc(R.index.cls)} 지가는 ${esc(fmtMonth(R.index.last.month))} 기준 전월 대비 ${v > 0 ? "+" : ""}${v}% 변동했습니다.`);
  }
  return out.join(" ");
}

// ---------- 거래내역 열 ----------
function wireDeep(my) {
  loadSalesRecent(R.sgg, my);
  const ziClass = document.getElementById("ziClass");
  if (ziClass) {
    ziClass.addEventListener("change", () => loadPriceIndex(R.sgg, ziClass.value, queryId));
  }
  document.querySelectorAll(".col-index .seg-btn").forEach((b) =>
    b.addEventListener("click", () => setRange(Number(b.dataset.range)))
  );
  document.querySelectorAll(".col-sales .seg-btn").forEach((b) =>
    b.addEventListener("click", () => {
      salesFilter = b.dataset.filter;
      document.querySelectorAll(".col-sales .seg-btn").forEach((x) => x.classList.toggle("on", x === b));
      renderSalesList();
    })
  );
  const ex = document.getElementById("ziExpand");
  if (ex) ex.addEventListener("click", openChartModal);
}

function isSameUmd(umdNm) {
  if (!R || !R.parcel || !umdNm) return false;
  const full = R.parcel.address_name || "";
  return full === umdNm || full.endsWith(" " + umdNm);
}

function normSale(it) {
  const amount = num(it.deal_amount_manwon);
  const area = num(it.deal_area_m2);
  const py = area > 0 ? area / SQM_PER_PYEONG : NaN;
  const ym = `${it.deal_year || ""}${String(it.deal_month || "").padStart(2, "0")}`;
  return {
    raw: it,
    umd: it.umd_nm || "",
    amount, area,
    perPy: isFinite(py) && py > 0 ? amount / py : NaN,
    share: /지분/.test(it.share_dealing_type || ""),
    ym,
    date: `${it.deal_year || ""}.${String(it.deal_month || "").padStart(2, "0")}.${String(it.deal_day || "").padStart(2, "0")}`,
    sortKey: Number(`${ym}${String(it.deal_day || 0).padStart(2, "0")}`) || 0,
    same: isSameUmd(it.umd_nm),
  };
}

let salesFilter = "all";

async function loadSalesRecent(sgg, my) {
  const box = document.getElementById("salesOut");
  salesFilter = "all";
  try {
    const r = await api("/appraisal/sales/recent", { lawd_cd: sgg, months: SALES_MONTHS });
    if (my !== queryId) return;
    // 시간순(최신 먼저). 같은 날이면 같은 동을 위로.
    const items = (r.items || []).map(normSale)
      .sort((a, b) => (b.sortKey - a.sortKey) || (b.same - a.same));
    R.sales = { items, months: r.months || [], failed: r.failed_months || [],
                truncated: r.truncated_months || [], points: r.umd_points || [] };
  } catch (e) {
    if (my !== queryId) return;
    R.sales = null;
    const msg = e.status === 503 ? "실거래 조회가 지금은 꺼져 있습니다. 잠시 후 다시 시도해 주세요."
      : e.status === 404 ? "이 지역 코드로는 실거래를 조회할 수 없습니다."
      : errText(e);
    if (box) box.innerHTML = `<span class="err-line">${esc(msg)}</span>`;
  }
  R.done.sales = true;
  if (R.sales) renderSalesList();
  drawSalesMarkers();
  renderSummary();
}

function saleRow(x) {
  const r = x.raw;
  return `<div class="sale${x.same ? " same" : ""}" data-umd="${esc(x.umd)}" role="button" tabindex="0">
    <div class="sale-main"><span class="amt">${formatManwon(r.deal_amount_manwon)}</span>
      <span class="ppy">${isFinite(x.perPy) ? "평당 " + fmtManwonNum(x.perPy) : ""}</span>
      <span class="sale-date">${esc(x.date)}</span></div>
    <div class="sale-sub">${x.same ? '<span class="badge same">같은 동</span> ' : ""}${esc(x.umd)} ${esc(r.jibun_masked || "")} · 지목 ${esc(r.jimok || "-")} · 면적 ${fmtArea(x.area)}${x.share ? ' · <span class="badge">지분거래</span>' : ""}</div>
  </div>`;
}

function renderSalesList() {
  const box = document.getElementById("salesOut");
  if (!box || !R.sales) return;
  const { items, months, failed } = R.sales;
  const monthsTxt = months.map(fmtMonth).join(", ");
  if (!items.length) {
    box.innerHTML = `<span class="muted">최근 ${SALES_MONTHS}개월(${esc(monthsTxt)}) 같은 시군구 안 토지 거래 신고가 없습니다.</span>`;
    return;
  }
  const same = items.filter((x) => x.same);
  const med = median(same.filter((x) => !x.share).map((x) => x.perPy));
  const rows = salesFilter === "same" ? same : items;
  const stat = `<div class="sales-stat">
      <span><b>${esc(R.umd || "같은 동")}</b> ${same.length}건${isFinite(med) ? ` · 평당 중위 ${fmtManwonNum(med)}` : ""}</span>
      <span class="muted">주변(같은 시군구) 전체 ${items.length}건 · ${esc(monthsTxt)}</span>
    </div>`;
  const warn = (failed.length ? `<div class="muted">일부 월(${esc(failed.map(fmtMonth).join(", "))})은 공공데이터 응답이 없어 빠졌습니다.</div>` : "") +
    (R.sales.truncated.length ? `<div class="muted">${esc(R.sales.truncated.map(fmtMonth).join(", "))}은 거래가 많아 앞쪽 1,000건만 표시합니다.</div>` : "");
  let list = "";
  let cur = "";
  rows.forEach((x) => {
    if (x.ym !== cur) {
      cur = x.ym;
      const n = rows.filter((y) => y.ym === cur).length;
      list += `<div class="month-head"><span>${esc(fmtMonth(cur))}</span><span>${n}건</span></div>`;
    }
    list += saleRow(x);
  });
  if (!rows.length) list = `<p class="muted">같은 동 거래가 없습니다.</p>`;
  box.innerHTML = stat + warn + list;
  box.querySelectorAll(".sale").forEach((el) => {
    const go = () => focusUmd(el.dataset.umd);
    el.addEventListener("click", go);
    el.addEventListener("keydown", (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); go(); } });
  });
}

function focusUmd(umd) {
  const m = salesMarkers.get(umd);
  if (!m) return;
  map.setView(m.getLatLng(), Math.max(map.getZoom(), 15));
  m.openPopup();
}

// 법정동 중심점 마커(건수 배지). 지번이 비공개라 필지 단위로는 찍지 않는다.
function drawSalesMarkers() {
  salesLayer.clearLayers();
  salesMarkers = new Map();
  if (!R || !R.sales || !document.getElementById("salesToggle").checked) return;
  const groups = new Map();
  R.sales.items.forEach((x) => {
    if (!groups.has(x.umd)) groups.set(x.umd, []);
    groups.get(x.umd).push(x);
  });
  groups.forEach((rows, umd) => {
    const pt = R.sales.points.find((p) => p.name === umd || p.name.endsWith(" " + umd));
    if (!pt) return;
    const same = rows[0].same;
    const icon = L.divIcon({
      className: "umd-pin-wrap",
      html: `<div class="umd-pin${same ? " same" : ""}"><b>${rows.length}</b>건</div>`,
      iconSize: null,
    });
    const list = rows.slice(0, 5).map((x) =>
      `<div class="pop-row"><b>${formatManwon(x.raw.deal_amount_manwon)}</b> ${fmtArea(x.area)}<br><span class="muted">${esc(x.raw.jimok || "")} · ${esc(x.date)}</span></div>`
    ).join("");
    const mk = L.marker([pt.lat, pt.lng], { icon, keyboard: true, title: `${umd} 거래 ${rows.length}건` })
      .bindPopup(`<b>${esc(umd)}</b> 최근 ${SALES_MONTHS}개월 ${rows.length}건${list}` +
        (rows.length > 5 ? `<div class="muted">외 ${rows.length - 5}건은 거래내역 열에서 확인</div>` : "") +
        `<div class="muted" style="margin-top:4px">위치는 동 중심입니다 (지번 비공개).</div>`)
      .addTo(salesLayer);
    salesMarkers.set(umd, mk);
  });
}

// ---------- 지가변동률 열 ----------
// 지가변동률 요청 순번. 같은 조회 안에서 용도지역을 빠르게 바꿔도 마지막 요청만 반영한다.
let ziSeq = 0;

async function loadPriceIndex(sgg, cls, my) {
  const seq = ++ziSeq;
  const box = document.getElementById("ziOut");
  if (box) box.innerHTML = SPIN;
  try {
    const r = await api("/appraisal/price-index", { sigungu: sgg, use_zone_class: cls });
    if (my !== queryId || seq !== ziSeq) return;
    // 변동률은 숫자로 고정해 둔다: 차트 계산이 안전해지고, 화면에 넣을 때 문자열이 섞일 일이 없다.
    ziSeries = (r.series || [])
      .filter((s) => s.rate_pct != null)
      .map((s) => ({ month: s.month, rate_pct: Number(s.rate_pct) }))
      .filter((s) => isFinite(s.rate_pct));
    R.index = ziSeries.length ? { cls, last: ziSeries[ziSeries.length - 1] } : null;
    if (!ziSeries.length) {
      if (box) box.innerHTML = `<span class="muted">해당 용도지역 데이터가 없습니다.</span>`;
      document.getElementById("ziRangeLabel").innerHTML = "&nbsp;";
      document.getElementById("ziTable").innerHTML = "";
    } else drawZi();
  } catch (e) {
    if (my !== queryId || seq !== ziSeq) return;
    R.index = null;
    if (box) box.innerHTML = `<span class="err-line">${esc(errText(e))}</span>`;
  }
  R.done.index = true;
  renderSummary();
}

function setRange(n) {
  ziRange = n;
  document.querySelectorAll(".seg-btn[data-range]").forEach((x) => x.classList.toggle("on", Number(x.dataset.range) === n));
  drawZi();
  const mc = document.getElementById("modalChart");
  if (mc) mountChart(mc, rangeSlice(ziSeries, ziRange), 960, 380);
}

function rangeSlice(series, n) {
  return n > 0 ? series.slice(-n) : series;
}

// 범위 명시: 표시 중인 기간을 숫자로 적는다 (화이트보드 3번 "범위 명시").
function rangeText(s) {
  if (!s.length) return "";
  const cls = (R && R.index && R.index.cls) || "";
  return `표시 범위 <b>${esc(fmtMonth(s[0].month))} ~ ${esc(fmtMonth(s[s.length - 1].month))}</b> · ${s.length}개월 · ${esc(sggName())} ${esc(cls)}`;
}

function drawZi() {
  const box = document.getElementById("ziOut");
  if (!box || !ziSeries.length) return;
  const s = rangeSlice(ziSeries, ziRange);
  document.getElementById("ziRangeLabel").innerHTML = rangeText(s);
  mountChart(box, s, 320, 270);
  const rows = s.slice().reverse().map((x) => {
    const v = x.rate_pct;
    return `<tr><td>${esc(fmtMonth(x.month))}</td><td class="${v >= 0 ? "up" : "down"}">${v > 0 ? "+" : ""}${v}%</td></tr>`;
  }).join("");
  document.getElementById("ziTable").innerHTML =
    `<table class="zi-table"><thead><tr><th>월</th><th>전월 대비 변동률</th></tr></thead><tbody>${rows}</tbody></table>`;
}

// ---------- 의존성 없는 인터랙티브 SVG 라인 차트 ----------
// x축 = 월, y축 = 변동률(%), 축 제목과 0선을 표시하고, 마우스/터치를 올리면 해당 월 값을 보여준다.
function niceStep(raw) {
  const p = Math.pow(10, Math.floor(Math.log10(raw)));
  const f = raw / p;
  return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * p;
}

function lineChartSVG(series, W, H) {
  const m = { l: 50, r: 14, t: 22, b: 40 };
  const vals = series.map((s) => s.rate_pct);
  let lo = Math.min(0, ...vals), hi = Math.max(0, ...vals);
  if (hi === lo) { hi += 0.1; lo -= 0.1; }
  const step = niceStep((hi - lo) / 4);
  lo = Math.floor(lo / step) * step;
  hi = Math.ceil(hi / step) * step;
  const n = series.length;
  const x = (i) => m.l + (i * (W - m.l - m.r)) / Math.max(1, n - 1);
  const y = (v) => m.t + ((hi - v) / (hi - lo)) * (H - m.t - m.b);
  const dec = step < 0.1 ? 2 : step < 1 ? 1 : 0;

  let grid = "";
  for (let t = lo; t <= hi + step / 2; t += step) {
    const yy = y(t).toFixed(1);
    const zero = Math.abs(t) < step / 1000;
    grid += `<line class="g${zero ? " zero" : ""}" x1="${m.l}" x2="${W - m.r}" y1="${yy}" y2="${yy}"/>` +
      `<text class="yl" x="${m.l - 6}" y="${yy}" dy="0.32em" text-anchor="end">${(zero ? 0 : t).toFixed(dec)}</text>`;
  }
  const ticks = n > 2 ? [0, Math.floor((n - 1) / 2), n - 1] : [...Array(n).keys()];
  const xl = ticks.map((i) =>
    `<text class="xl" x="${x(i).toFixed(1)}" y="${H - m.b + 16}" text-anchor="${i === 0 ? "start" : i === n - 1 ? "end" : "middle"}">${esc(fmtMonth(series[i].month))}</text>`).join("");
  const axes = `<text class="axt" x="2" y="12" text-anchor="start">Y: 변동률(%)</text>` +
    `<text class="axt" x="${W - m.r}" y="${H - 4}" text-anchor="end">X: 월</text>`;
  const pts = series.map((s, i) => `${x(i).toFixed(1)},${y(s.rate_pct).toFixed(1)}`);
  const y0 = y(0).toFixed(1);
  const area = `M${x(0).toFixed(1)},${y0} L${pts.join(" L")} L${x(n - 1).toFixed(1)},${y0} Z`;
  const last = series[n - 1];
  return `<svg class="lchart" viewBox="0 0 ${W} ${H}" role="img"
      aria-label="지가변동률 추이, ${esc(fmtMonth(series[0].month))}부터 ${esc(fmtMonth(last.month))}까지, 최근 ${last.rate_pct}%">
    ${grid}${xl}${axes}
    <path class="ar" d="${area}"/>
    <polyline class="ln" points="${pts.join(" ")}"/>
    <circle class="dot" cx="${x(n - 1).toFixed(1)}" cy="${y(last.rate_pct).toFixed(1)}" r="3"/>
    <line class="guide" x1="0" x2="0" y1="${m.t}" y2="${H - m.b}" visibility="hidden"/>
    <circle class="hdot" r="4" cx="0" cy="0" visibility="hidden"/>
    <rect class="hit" x="${m.l}" y="${m.t}" width="${W - m.l - m.r}" height="${H - m.t - m.b}"
      data-ml="${m.l}" data-mr="${m.r}"/>
  </svg>`;
}

function mountChart(box, series, W, H) {
  box.innerHTML = lineChartSVG(series, W, H) + `<div class="ctip" hidden></div>`;
  const svg = box.querySelector("svg");
  const hit = svg.querySelector(".hit");
  const guide = svg.querySelector(".guide");
  const hdot = svg.querySelector(".hdot");
  const tip = box.querySelector(".ctip");
  const n = series.length;
  const ml = Number(hit.dataset.ml), mr = Number(hit.dataset.mr);
  const pts = svg.querySelector(".ln").getAttribute("points").split(" ").map((p) => p.split(",").map(Number));

  function show(clientX) {
    const rect = svg.getBoundingClientRect();
    const vx = ((clientX - rect.left) / rect.width) * W;
    const i = Math.max(0, Math.min(n - 1, Math.round(((vx - ml) / (W - ml - mr)) * (n - 1))));
    const [px, py] = pts[i];
    guide.setAttribute("x1", px); guide.setAttribute("x2", px); guide.setAttribute("visibility", "visible");
    hdot.setAttribute("cx", px); hdot.setAttribute("cy", py); hdot.setAttribute("visibility", "visible");
    const v = series[i].rate_pct;
    tip.textContent = `${fmtMonth(series[i].month)}  ${v > 0 ? "+" : ""}${v}%`;
    tip.hidden = false;
    const leftPct = (px / W) * 100;
    tip.style.left = `${leftPct}%`;
    tip.style.transform = leftPct > 70 ? "translateX(-100%)" : leftPct < 30 ? "none" : "translateX(-50%)";
  }
  function hide() {
    guide.setAttribute("visibility", "hidden");
    hdot.setAttribute("visibility", "hidden");
    tip.hidden = true;
  }
  hit.addEventListener("mousemove", (e) => show(e.clientX));
  hit.addEventListener("mouseleave", hide);
  hit.addEventListener("touchstart", (e) => show(e.touches[0].clientX), { passive: true });
  hit.addEventListener("touchmove", (e) => show(e.touches[0].clientX), { passive: true });
}

function openChartModal() {
  if (!ziSeries.length) return;
  const cls = (R && R.index && R.index.cls) || "";
  const wrap = document.createElement("div");
  wrap.className = "modal";
  wrap.setAttribute("role", "dialog");
  wrap.setAttribute("aria-modal", "true");
  wrap.setAttribute("aria-label", "지가변동률 크게 보기");
  wrap.innerHTML = `<div class="modal-card">
      <div class="modal-head"><b>지가변동률</b><span class="muted">${esc(cls)} · 월별 · 전월 대비 %</span>
        <div class="seg" role="group" aria-label="기간">${segButtons()}</div>
        <button type="button" class="btn ghost sm" id="modalClose">닫기</button></div>
      <div class="range-label" id="modalRange"></div>
      <div class="chart-box" id="modalChart"></div>
    </div>`;
  document.body.appendChild(wrap);
  const draw = () => {
    const s = rangeSlice(ziSeries, ziRange);
    document.getElementById("modalRange").innerHTML = rangeText(s);
    mountChart(document.getElementById("modalChart"), s, 960, 380);
  };
  draw();
  wrap.querySelectorAll(".seg-btn").forEach((b) =>
    b.addEventListener("click", () => { setRange(Number(b.dataset.range)); draw(); })
  );
  const opener = document.activeElement;
  const close = () => {
    wrap.remove();
    document.removeEventListener("keydown", onKey);
    if (opener && opener.focus) opener.focus(); // 닫은 뒤 포커스를 "크게 보기" 버튼으로 되돌린다
  };
  const onKey = (e) => { if (e.key === "Escape") close(); };
  document.addEventListener("keydown", onKey);
  wrap.addEventListener("click", (e) => { if (e.target === wrap) close(); });
  document.getElementById("modalClose").addEventListener("click", close);
  document.getElementById("modalClose").focus();
}

function fmtMonth(m) { const s = String(m); return s.length === 6 ? `${s.slice(0, 4)}.${s.slice(4)}` : s; }

// ---------- 해당 토지 사진 (항공사진) ----------
// V-World 영상 타일만 쓴다(지시서 3.3 허용 출처). 거리뷰·로드뷰는 지시서 1.1(상용 지도
// 자산 금지)에 따라 넣지 않는다. 키는 서버 env(B2D_VWORLD_KEY)에서 /config.json으로만 받는다.
function vworldLayers(key) {
  const k = encodeURIComponent(key);
  const attribution = "항공사진 © 국토교통부 공간정보 오픈플랫폼(V-World)";
  return [
    L.tileLayer(`https://api.vworld.kr/req/wmts/1.0.0/${k}/Satellite/{z}/{y}/{x}.jpeg`, { minZoom: 6, maxZoom: 19, attribution }),
    L.tileLayer(`https://api.vworld.kr/req/wmts/1.0.0/${k}/Hybrid/{z}/{y}/{x}.png`, { minZoom: 6, maxZoom: 19 }),
  ];
}

async function updatePhoto(geometry, lat, lon) {
  const cfg = await B2D_CONFIG;
  const key = cfg && cfg.vworldKey;
  const el = document.getElementById("photoMap");
  const empty = document.getElementById("photoEmpty");
  if (!key) { el.hidden = true; empty.hidden = false; return; }
  el.hidden = false; empty.hidden = true;
  if (!photoMap) {
    photoMap = L.map("photoMap", { zoomControl: false, scrollWheelZoom: false });
    vworldLayers(key).forEach((l) => l.addTo(photoMap));
  }
  photoMap.invalidateSize();
  if (photoParcel) { photoMap.removeLayer(photoParcel); photoParcel = null; }
  if (geometry) {
    photoParcel = L.geoJSON(geometry, { style: { color: "#facc15", weight: 3, fill: false } }).addTo(photoMap);
    try { photoMap.fitBounds(photoParcel.getBounds(), { maxZoom: 19, padding: [18, 18] }); }
    catch (e) { photoMap.setView([lat, lon], 18); }
  } else {
    photoMap.setView([lat, lon], 18);
  }
}

function setupAerialToggle(key) {
  aerialLayer = L.layerGroup(vworldLayers(key));
  document.getElementById("aerialWrap").hidden = false;
  document.getElementById("aerialToggle").addEventListener("change", (e) => setAerial(e.target.checked));
}

function setAerial(on) {
  if (!aerialLayer) return;
  if (on) { map.removeLayer(osmLayer); aerialLayer.addTo(map); }
  else { map.removeLayer(aerialLayer); osmLayer.addTo(map); }
}

B2D_CONFIG.then((cfg) => { if (cfg && cfg.vworldKey) setupAerialToggle(cfg.vworldKey); });

// ---------- 리포트 출력 ----------
// print-mode로 A4 배치를 먼저 화면에 적용해 지도들이 인쇄 크기로 타일을 다시 받게 하고,
// 잠시 뒤 인쇄 대화상자를 연다. "PDF로 저장"을 고르면 파일이 된다. 브라우저 메뉴로
// 인쇄해도(beforeprint) 같은 배치를 쓴다.
function enterPrintMode() {
  document.body.classList.add("print-mode");
  map.invalidateSize();
  if (photoMap) photoMap.invalidateSize();
  if (parcelLayer) {
    try { map.fitBounds(parcelLayer.getBounds(), { maxZoom: 17, padding: [24, 24] }); } catch (e) {}
  }
  if (photoMap && photoParcel) {
    try { photoMap.fitBounds(photoParcel.getBounds(), { maxZoom: 19, padding: [18, 18] }); } catch (e) {}
  }
}
function printReport() {
  enterPrintMode();
  setTimeout(() => window.print(), 900);
}
window.addEventListener("beforeprint", () => {
  if (!document.body.classList.contains("print-mode")) enterPrintMode();
});
window.addEventListener("afterprint", () => {
  document.body.classList.remove("print-mode");
  setTimeout(() => { map.invalidateSize(); if (photoMap) photoMap.invalidateSize(); }, 60);
});

// ---------- 오버레이(주변 상가 / 표준지) ----------
async function refreshOverlays() {
  if (!lastPoint) return;
  const { lat, lon } = lastPoint;
  poiLayer.clearLayers();
  lotsLayer.clearLayers();
  if (document.getElementById("poiToggle").checked) loadPOI(lat, lon);
  if (document.getElementById("lotsToggle").checked) loadLots(lat, lon);
}

async function loadPOI(lat, lon) {
  try {
    // q 없이 lat/lon만 주면 서버가 반경 내 근접순 브라우즈 모드로 응답한다.
    const r = await api("/places/search", { lat, lon, radius: 500, limit: 50 });
    (r.items || []).slice(0, 60).forEach((it) => {
      if (!it.point) return;
      const [plon, plat] = it.point;
      L.circleMarker([plat, plon], { radius: 4, color: "#0f766e", weight: 1, fillOpacity: 0.7 })
        .bindPopup(`<b>${esc(it.name)}</b><br><span class="mono">${esc(it.category_code || "")}</span><br>${esc(it.road_address || "")}`)
        .addTo(poiLayer);
    });
  } catch (e) {
    /* 상가 검색은 q가 필요할 수 있음 — 실패해도 조용히 무시 */
  }
}

async function loadLots(lat, lon) {
  try {
    const r = await api("/appraisal/standard-lots", { lat, lng: lon, radius: 1000 });
    (r.standard_lots || []).forEach((s) => {
      L.circleMarker([s.lat, s.lng], { radius: 5, color: "#a45a09", weight: 1.5, fillColor: "#eab34d", fillOpacity: 0.8 })
        .bindPopup(
          `<b>표준지</b> <span class="mono">${esc(s.pnu)}</span><br>` +
            `${formatWon(s.price_per_sqm)}원/㎡ (평당 약 ${fmtManwonNum(perPyeongManwon(s.price_per_sqm))})<br>${esc(s.use_zone || "")} · ${esc(s.land_use || "")}<br>` +
            `<span class="mono">${esc(s.distance_m)}m</span>`
        )
        .addTo(lotsLayer);
    });
  } catch (e) {
    /* 미적재 지역이면 조용히 무시 */
  }
}

// ---------- 지도 요소 ----------
function setPin(lat, lon) {
  if (pinMarker) pinMarker.setLatLng([lat, lon]);
  else pinMarker = L.marker([lat, lon]).addTo(map);
}
function drawParcel(geometry) {
  clearParcel();
  parcelLayer = L.geoJSON(geometry, {
    style: { color: "#0f766e", weight: 2, fillColor: "#0f766e", fillOpacity: 0.12 },
  }).addTo(map);
  try {
    map.fitBounds(parcelLayer.getBounds(), { maxZoom: 18, padding: [30, 30] });
  } catch (e) {}
}
function clearParcel() {
  if (parcelLayer) { map.removeLayer(parcelLayer); parcelLayer = null; }
}

// ---------- 유틸 ----------
function esc(s) {
  return String(s == null ? "" : s).replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])
  );
}
function relClass(rel) {
  if (rel === "포함") return "포함";
  if (rel === "저촉") return "저촉";
  return "접함";
}
// 시군구 이름: 법정동 전체명에서 마지막(읍면동)만 뺀다. 예) "경기도 하남시 신장동" -> "경기도 하남시"
function sggName() {
  const full = (R && R.parcel && R.parcel.address_name) || "";
  const parts = full.trim().split(/\s+/);
  return parts.length > 1 ? parts.slice(0, -1).join(" ") : (R ? R.sgg : "");
}
function lastToken(s) {
  const parts = String(s).trim().split(/\s+/);
  return parts[parts.length - 1] || "";
}

// ---------- 이벤트 ----------
document.getElementById("go").addEventListener("click", () => searchAddress(document.getElementById("q").value.trim()));
document.getElementById("q").addEventListener("keydown", (e) => {
  if (e.key === "Enter") searchAddress(e.target.value.trim());
});
map.on("click", (e) => queryPoint(e.latlng.lat, e.latlng.lng, { keepView: true }));
document.getElementById("poiToggle").addEventListener("change", refreshOverlays);
document.getElementById("lotsToggle").addEventListener("change", refreshOverlays);
document.getElementById("salesToggle").addEventListener("change", drawSalesMarkers);
// 판단 요약 열은 데이터가 올 때마다 다시 그려지므로 출력 버튼은 위임으로 받는다.
results.addEventListener("click", (e) => { if (e.target.closest("#printBtn")) printReport(); });

const exWrap = document.getElementById("examples");
EXAMPLES.forEach((ex) => {
  const b = document.createElement("button");
  b.className = "chip";
  b.textContent = ex;
  b.onclick = () => { document.getElementById("q").value = ex; searchAddress(ex); };
  exWrap.appendChild(b);
});

// 공유 가능한 딥링크: /map.html?q=주소  또는  ?lat=..&lon=..
(function fromURL() {
  const p = new URLSearchParams(location.search);
  const q = p.get("q");
  const lat = parseFloat(p.get("lat"));
  const lon = parseFloat(p.get("lon"));
  if (q) { document.getElementById("q").value = q; searchAddress(q); }
  else if (isFinite(lat) && isFinite(lon)) queryPoint(lat, lon);
})();
