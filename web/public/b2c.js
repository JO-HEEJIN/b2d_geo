// 올맵 집 구경 (B2C). 동네 -> 단지 목록(거래 많은 순, 전부) -> 단지 상세(판단, 실거래, 도보시간, 구조, 남은 매물).
// API는 /v1/b2c/* (shared.js의 api()가 공개 데모 키를 붙인다). 후보를 추려 줄이지 않는 것이 원칙이다.

const panel = document.getElementById("panel");
const zonesEl = document.getElementById("zones");
const legend = document.getElementById("legend");

const map = L.map("map", { zoomControl: true }).setView([37.35, 127.0], 10);
L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
  maxZoom: 19,
  attribution: "© OpenStreetMap",
}).addTo(map);
const complexLayer = L.layerGroup().addTo(map);
const infraLayer = L.layerGroup().addTo(map);

const LEVEL = {
  hot: "바로 팔림",
  active: "꾸준히 거래",
  slow: "거래 드묾",
  dormant: "거래 없음",
};
const INFRA = {
  subway_station: "지하철역",
  bus_stop: "버스정류장",
  convenience_store: "편의점",
  hospital: "병원",
};

let currentZone = null;
let currentList = [];

function esc(s) {
  return String(s == null ? "" : s).replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])
  );
}

function pyeong(m2) {
  const v = Number(m2);
  return isFinite(v) && v > 0 ? Math.round(v / SQM_PER_PYEONG) + "평" : "";
}

function cssVar(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

function levelColor(level) {
  return level === "hot" ? cssVar("--gold") : level === "active" ? cssVar("--accent") : cssVar("--neutral");
}

function days(n) {
  if (!isFinite(n) || n <= 0) return "-";
  return n < 1 ? "하루에 여러 건" : `${Math.round(n)}일마다`;
}

function dateStr(iso) {
  const d = new Date(iso);
  return isNaN(d) ? "" : `${d.getFullYear()}.${String(d.getMonth() + 1).padStart(2, "0")}.${String(d.getDate()).padStart(2, "0")}`;
}

// ---------- 동네 ----------

async function loadZones() {
  try {
    const { zones } = await api("/b2c/districts");
    zonesEl.innerHTML = zones
      .map((z) => `<button role="tab" aria-selected="false" data-key="${esc(z.key)}">${esc(z.name)}</button>`)
      .join("");
    zonesEl.querySelectorAll("button").forEach((b) =>
      b.addEventListener("click", () => selectZone(zones.find((z) => z.key === b.dataset.key)))
    );
    const fromHash = zones.find((z) => z.key === location.hash.slice(1));
    if (fromHash) selectZone(fromHash);
  } catch (e) {
    panel.innerHTML = `<div class="b2c-error">동네 목록을 불러오지 못했습니다.<br>${esc(errText(e))}</div>`;
  }
}

async function selectZone(zone) {
  currentZone = zone;
  history.replaceState(null, "", "#" + zone.key);
  zonesEl.querySelectorAll("button").forEach((b) => b.setAttribute("aria-selected", String(b.dataset.key === zone.key)));
  map.setView([zone.center[1], zone.center[0]], 14);
  complexLayer.clearLayers();
  infraLayer.clearLayers();
  panel.innerHTML = `<div class="b2c-loading">${esc(zone.name)}의 최근 6개월 실거래를 모으는 중입니다...</div>`;
  try {
    const data = await api(`/b2c/zones/${encodeURIComponent(zone.key)}/complexes`);
    if (currentZone !== zone) return;
    currentList = data.complexes || [];
    renderList(zone, currentList, data.disclaimer);
  } catch (e) {
    panel.innerHTML = `<div class="b2c-error">실거래를 불러오지 못했습니다.<br>${esc(errText(e))}</div>`;
  }
}

function renderList(zone, list, disclaimer) {
  const total = list.reduce((s, c) => s + c.activity.count, 0);
  panel.innerHTML =
    `<div class="list-head"><span><b>${esc(zone.name)}</b> 단지 ${list.length}곳</span><span>6개월 거래 ${total.toLocaleString("ko-KR")}건</span></div>` +
    (list.length ? "" : `<div class="b2c-empty">최근 6개월 동안 거래된 단지가 없습니다.</div>`) +
    list
      .map(
        (c, i) => `<button class="cx" data-i="${i}">
          <span class="cx-name">${esc(c.name)}<span class="lvl ${esc(c.activity.level)}">${LEVEL[c.activity.level] || ""}</span></span>
          <span class="cx-price">${c.latest_price_manwon ? formatManwon(c.latest_price_manwon) : ""}</span>
          <span class="cx-sub">${esc(c.umd_nm)} · ${c.build_year ? new Date().getFullYear() - c.build_year + "년차" : ""}</span>
          <span class="cx-meta">6개월 ${c.activity.count}건 · ${days(c.activity.avg_interval_days)}</span>
        </button>`
      )
      .join("") +
    `<p class="disclaimer" style="padding:12px 18px">${esc(disclaimer || "")}</p>`;
  panel.querySelectorAll(".cx").forEach((b) => b.addEventListener("click", () => openComplex(list[Number(b.dataset.i)])));
  drawComplexes(list);
  legend.hidden = false;
}

function drawComplexes(list) {
  complexLayer.clearLayers();
  const pts = [];
  list.forEach((c) => {
    if (!c.place) return;
    const ll = [c.place.lat, c.place.lon];
    pts.push(ll);
    L.circleMarker(ll, {
      radius: Math.min(16, 5 + Math.sqrt(c.activity.count) * 1.4),
      color: "#fff",
      weight: 1.5,
      fillColor: levelColor(c.activity.level),
      fillOpacity: 0.88,
    })
      .bindTooltip(`${esc(c.name)} · 6개월 ${c.activity.count}건`)
      .on("click", () => openComplex(c))
      .addTo(complexLayer);
  });
  if (pts.length) map.fitBounds(pts, { padding: [30, 30], maxZoom: 15 });
}

// ---------- 단지 상세 ----------

async function openComplex(c) {
  panel.scrollTop = 0;
  panel.innerHTML = `<div class="detail"><div class="detail-top"><button class="back">← ${esc(currentZone.name)} 단지 목록</button>
    <h2>${esc(c.name)}</h2><div class="sub">${esc(c.umd_nm)} ${esc(c.jibun)}</div></div>
    <div class="b2c-loading">이 단지의 거래, 주변 시설, 구조를 모으는 중입니다...</div></div>`;
  panel.querySelector(".back").addEventListener("click", backToList);
  if (c.place) map.setView([c.place.lat, c.place.lon], 16);
  try {
    const d = await api("/b2c/complex", { zone: currentZone.key, sgg: c.sgg_cd, umd: c.umd_nm, jibun: c.jibun, name: c.name });
    renderDetail(d);
  } catch (e) {
    panel.querySelector(".b2c-loading").outerHTML = `<div class="b2c-error">단지 정보를 불러오지 못했습니다.<br>${esc(errText(e))}</div>`;
  }
}

function backToList() {
  infraLayer.clearLayers();
  renderList(currentZone, currentList, "");
}

function renderDetail(d) {
  const c = d.complex;
  const a = c.activity;
  const v = d.verdict;
  const age = c.build_year ? `${c.build_year}년 준공 (${new Date().getFullYear() - c.build_year}년차)` : "";
  let html = `<div class="detail"><div class="detail-top"><button class="back">← ${esc(currentZone.name)} 단지 목록</button>
    <h2>${esc(c.name)}<span class="lvl ${esc(a.level)}">${LEVEL[a.level] || ""}</span></h2>
    <div class="sub">${esc(c.umd_nm)} ${esc(c.jibun)}${age ? " · " + esc(age) : ""}</div></div>`;

  html += `<div class="sec"><h3>이 단지는 살아 있나</h3>
    <div class="verdict ${esc(a.level)}"><div class="msg">${esc(v.message)}</div>
    ${v.cautions && v.cautions.length ? `<ul>${v.cautions.map((x) => `<li>${esc(x)}</li>`).join("")}</ul>` : ""}</div>
    <div class="stats">
      <div class="stat"><b>${a.count}건</b><span>6개월 거래</span></div>
      <div class="stat"><b>${days(a.avg_interval_days)}</b><span>평균 거래 간격</span></div>
      <div class="stat"><b>${a.days_since_last >= 0 ? a.days_since_last + "일 전" : "-"}</b><span>마지막 거래</span></div>
    </div>
    <p class="disclaimer">${esc(v.disclaimer)}</p></div>`;

  html += `<div class="sec"><h3>걸어서 몇 분 <small>직선거리 기준 추정</small></h3>`;
  if (d.infra) {
    html += `<div class="walk">${d.infra
      .map((l) =>
        l.found
          ? `<span class="k">${INFRA[l.kind]}</span><span>${esc(l.facility.name)}${l.kind === "subway_station" && l.facility.address ? " " + esc(l.facility.address) : ""}</span><span class="min">도보 ${l.facility.walk_minutes}분</span>`
          : `<span class="k">${INFRA[l.kind]}</span><span class="none">${esc(l.note)}</span><span></span>`
      )
      .join("")}</div>`;
  } else {
    html += `<p class="muted">이 단지의 위치를 찾지 못해 주변 시설을 계산하지 못했습니다.</p>`;
  }
  html += `</div>`;

  html += `<div class="sec"><h3>같은 아파트, 다른 구조 <small>평면도만으로는 모릅니다. 직접 보세요</small></h3><div id="floorPlans"></div></div>`;

  html += `<div class="sec"><h3>최근 실거래 <small>해제된 거래는 줄을 그었습니다</small></h3>`;
  html += d.trades.length
    ? `<table class="mini">${d.trades
        .map(
          (t) => `<tr class="${t.canceled ? "cancel" : ""}"><td>${dateStr(t.deal_date)}</td><td>${t.area_m2}㎡ (${pyeong(t.area_m2)})</td><td>${t.floor}층</td><td class="r">${formatManwon(t.price_manwon)}</td></tr>`
        )
        .join("")}</table>`
    : `<p class="muted">최근 6개월 거래가 없습니다.</p>`;
  html += `</div>`;

  html += `<div class="sec"><h3>지금 남은 매물 <small>전부 보여드립니다</small></h3><div id="listings"></div></div>`;
  html += `<div class="sec" id="recentlySold"></div>`;
  html += `</div>`;
  panel.innerHTML = html;
  panel.querySelector(".back").addEventListener("click", backToList);

  renderFloorPlans(d.floor_plans);
  renderListings(c.name, d.listings);
  renderRecentlySold(d.recently_sold || []);
  drawInfra(c, d.infra);
}

// 구조 칸: 준비돼 있으면 타입 버튼과 상세, 만드는 중이면 5초마다 다시 묻는다(최대 2분).
function renderFloorPlans(fp, tries = 0) {
  const box = document.getElementById("floorPlans");
  if (!box) return;
  if (!fp) {
    box.innerHTML = `<p class="muted">이 단지의 위치를 찾지 못해 구조 정보를 불러오지 못했습니다.</p>`;
    return;
  }
  if (fp.status === "building" || (!fp.variants.length && fp.status !== "none" && tries > 0)) {
    box.innerHTML = `<p class="muted">건축물대장에서 이 단지의 모든 세대를 읽어 구조를 정리하는 중입니다. 잠시 후 자동으로 나타납니다.</p>`;
    if (tries < 24) {
      setTimeout(async () => {
        try {
          const next = await api("/b2c/floor-plans", { complex_id: fp.complex_id });
          if (document.getElementById("floorPlans") === box) renderFloorPlans(next, tries + 1);
        } catch (e) {
          box.innerHTML = `<p class="muted">구조 정보를 불러오지 못했습니다. ${esc(errText(e))}</p>`;
        }
      }, 5000);
    }
    return;
  }
  if (!fp.variants.length) {
    box.innerHTML = `<p class="muted">아직 구조 정보가 없습니다. 건축물대장 조회는 하루 한도가 있어 내일 다시 보이거나, 등록되면 나타납니다.</p>`;
    return;
  }
  box.innerHTML =
    `<div class="types">${fp.variants
      .map((t, i) => `<button aria-pressed="${i === 0}" data-i="${i}">${esc(t.type_name)} · ${t.units.length}세대</button>`)
      .join("")}</div><div class="type-box" id="typeBox"></div>`;
  const showType = (i) => {
    const t = fp.variants[i];
    box.querySelectorAll(".types button").forEach((b) => b.setAttribute("aria-pressed", String(Number(b.dataset.i) === i)));
    const units = t.units.slice(0, 12).join(", ") + (t.units.length > 12 ? ` 외 ${t.units.length - 12}세대` : "");
    document.getElementById("typeBox").innerHTML =
      `전용 ${t.exclusive_area_m2}㎡ (${pyeong(t.exclusive_area_m2)})` +
      (t.bathroom_position ? ` · 화장실 ${esc(t.bathroom_position)}` : "") +
      (t.living_room ? ` · 거실 ${esc(t.living_room)}` : "") +
      `<div style="margin-top:6px">이 구조로 지어진 집: <span class="units">${esc(units || "등록된 세대 없음")}</span></div>` +
      `<ol>${(fp.checklists[t.type_name] || []).map((x) => `<li>${esc(x)}</li>`).join("")}</ol>`;
  };
  box.querySelectorAll(".types button").forEach((b) => b.addEventListener("click", () => showType(Number(b.dataset.i))));
  showType(0);
}

let listingLiveView;
let listingViewVersion = 0;

function listingRows(items) {
  return items
    .filter((l) => l.source !== "sample")
    .map(
      (l) =>
        `<tr><td><small class="muted">${l.source?.startsWith("market/") ? "제휴 공급 매물" : l.source === "naver" ? "네이버" : "직접 등록"}</small> ${esc(l.type_name || "")}</td><td>${l.floor ? l.floor + "층" : ""}</td><td class="r">${formatManwon(l.asking_manwon)}</td></tr>`
    )
    .join("");
}

function naverSyncNotice(statuses) {
  if (!statuses?.length) return "중개사·집주인이 직접 등록한 실제 매물입니다. 전체 시장의 매물 수와는 다를 수 있습니다.";
  const latest = statuses.map((s) => s.last_success_at).sort((a, b) => Date.parse(a) - Date.parse(b)).at(-1);
  const when = new Date(latest).toLocaleString("ko-KR", { timeZone: "Asia/Seoul" });
  if (statuses.some((s) => !s.fresh)) return `네이버 갱신이 지연되고 있습니다. 마지막 정상 수집: ${when}. 현재 실시간 정보로 볼 수 없습니다.`;
  return `네이버 실제 매물 · 최근 수집 ${when} · 1분마다 변동을 확인합니다.`;
}

function renderListings(name, page, statuses, changes = [], market) {
  const version = ++listingViewVersion;
  const box = document.getElementById("listings");
  if (!box) return;
  const total = page?.listings?.some((l) => l.source === "sample")
    ? page.listings.filter((l) => l.source !== "sample").length : page?.total || 0;
  const counts = { added: 0, gone: 0, price_changed: 0 };
  changes.forEach((c) => { if (Object.hasOwn(counts, c.kind)) counts[c.kind]++; });
  box.innerHTML =
    `<p class="muted live-status" role="status"></p>` +
    renderMarketCoverage(market) +
    (statuses?.length ? `<p class="muted naver-status">${esc(naverSyncNotice(statuses))}</p>` : "") +
    (changes.length ? `<p class="muted">최근 변동(최대 100건): 신규 ${counts.added} · 사라짐 ${counts.gone} · 호가 변경 ${counts.price_changed}. 목록에서 사라진 매물은 거래 완료가 확인된 것은 아닙니다.</p>` : "") +
    (total ? `<p class="muted" style="margin:0 0 6px">서비스에서 확인 가능한 판매 중 게시매물 ${total}건</p><table class="mini" id="listingTable">${listingRows(page.listings)}</table>` : "");
  let next = page?.next_offset;
  const addMore = () => {
    if (next == null) return;
    const btn = document.createElement("button");
    btn.className = "btn ghost more";
    btn.textContent = "더 보기";
    btn.addEventListener("click", async () => {
      btn.disabled = true;
      try {
        const d = await api("/b2c/listings", { complex: name, limit: 50, offset: next });
        if (version !== listingViewVersion || !box.isConnected) return;
        document.getElementById("listingTable").insertAdjacentHTML("beforeend", listingRows(d.remaining.listings));
        next = d.remaining.next_offset;
        btn.remove();
        addMore();
      } catch (e) {
        btn.disabled = false;
        btn.textContent = "다시 시도 (" + errText(e) + ")";
      }
    });
    box.appendChild(btn);
  };
  addMore();
  if (!listingLiveView || listingLiveView.box !== box || listingLiveView.name !== name || listingLiveView.closed) {
    listingLiveView?.stop();
    listingLiveView = new ListingLiveView(name, box, (data) => {
      renderListings(name, data.remaining, data.naver_sync || [], data.naver_changes || [], data.market);
      renderRecentlySold(data.recently_sold || []);
    });
  }
  listingLiveView.showStatus();
  if (statuses === undefined) listingLiveView.refresh();
}

function renderMarketCoverage(market) {
  if (!market) return `<p class="muted">시장 매물 연결 상태를 확인하고 있습니다.</p>`;
  if (market.status === "not_connected") return `<p class="muted">시장 매물 공급원이 아직 연결되지 않았습니다. 현재 남은 매물 수는 확인할 수 없습니다.</p>`;
  const sources = market.sources || [];
  const names = sources.map((s) => esc(s.name)).join(", ");
  const times = sources.map((s) => s.observed_at).filter(Boolean).sort((a, b) => Date.parse(a) - Date.parse(b));
  const asOf = times.length ? new Date(times[0]).toLocaleString("ko-KR", { timeZone: "Asia/Seoul" }) : "확인 전";
  const state = market.status === "ready" ? "수집 정상" : market.status === "partial" ? "일부 공급원 확인 불가" : market.status === "stale" ? "수집 지연 · 마지막 확인 목록" : "공급원 응답 확인 불가";
  let html = `<p class="muted">${names} · ${state} · 기준 ${esc(asOf)}</p>`;
  if (market.listing_count !== null && market.listing_count !== undefined) html += `<p>공급원에서 확인한 판매 중 게시매물 <strong>${market.listing_count}건</strong> <small>연결한 공급 범위 기준, 전체 시장 또는 중복 제거한 주택 수가 아닙니다.</small></p>`;
  else html += `<p class="muted">남은 매물 수를 확정할 수 없습니다.</p>`;
  const kinds = { added: "신규", relisted: "재등장", price_changed: "호가 변경", sold: "판매 완료", withdrawn: "매물 내림", missing: "목록에서 사라짐(판매 확인 안 됨)" };
  const events = market.changes || [];
  if (events.length) html += `<p class="muted">최근 변동(최대 100건)</p><table class="mini">${events.map((c) => `<tr><td>${esc(kinds[c.kind] || c.kind)}</td><td>${esc(c.external_id)}</td><td class="r">${c.previous_manwon ? formatManwon(c.previous_manwon) : ""}${c.previous_manwon && c.current_manwon ? " → " : ""}${c.current_manwon ? formatManwon(c.current_manwon) : ""}</td></tr>`).join("")}</table>`;
  return html;
}

function renderRecentlySold(listings) {
  const box = document.getElementById("recentlySold");
  if (!box) return;
  const real = listings.filter((l) => l.source !== "sample");
  box.innerHTML = real.length ? `<h3>최근 7일 사이 팔린 등록 매물</h3><table class="mini">${listingRows(real)}</table>` : "";
}

function drawInfra(c, lines) {
  infraLayer.clearLayers();
  if (!c.place) return;
  const home = [c.place.lat, c.place.lon];
  L.circleMarker(home, { radius: 9, color: "#fff", weight: 2, fillColor: levelColor(c.activity.level), fillOpacity: 1 })
    .bindTooltip(esc(c.name), { permanent: true, direction: "top", offset: [0, -8] })
    .addTo(infraLayer);
  const pts = [home];
  (lines || []).forEach((l) => {
    if (!l.found) return;
    const ll = [l.facility.lat, l.facility.lon];
    pts.push(ll);
    L.polyline([home, ll], { color: cssVar("--ink-3"), weight: 1.5, dashArray: "4 4" }).addTo(infraLayer);
    L.circleMarker(ll, { radius: 6, color: cssVar("--accent"), weight: 2, fillColor: cssVar("--surface"), fillOpacity: 1 })
      .bindTooltip(`${INFRA[l.kind]} ${esc(l.facility.name)} · 도보 ${l.facility.walk_minutes}분`)
      .addTo(infraLayer);
  });
  map.fitBounds(pts, { padding: [50, 50], maxZoom: 17 });
}

loadZones();
