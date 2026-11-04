// 올맵 매물 등록 (중개사·집주인). 조회(동네·단지 목록)는 공개 데모 키(shared.js api()),
// 등록·내 매물·팔림 처리는 사용자가 넣은 b2c_write 키로 /v1/b2c/write/*를 부른다.

const KEY_STORE = "b2c-write-key";
const $ = (id) => document.getElementById(id);
let complexes = [];

function esc(s) {
  return String(s == null ? "" : s).replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])
  );
}

function getKey() {
  try {
    return localStorage.getItem(KEY_STORE) || "";
  } catch (e) {
    return "";
  }
}

function setMsg(el, text, ok) {
  el.textContent = text;
  el.className = "reg-msg " + (ok ? "ok" : "bad");
}

// writeApi는 등록 키로 /v1/b2c/write/*를 부른다. body가 문자열이 아니면 JSON으로 보낸다.
async function writeApi(method, path, body, contentType) {
  const headers = { "X-API-Key": getKey() };
  let payload;
  if (body !== undefined) {
    headers["Content-Type"] = contentType || "application/json";
    payload = contentType ? body : JSON.stringify(body);
  }
  const res = await fetch("/v1/b2c/write" + path, { method, headers, body: payload });
  const data = await res.json().catch(() => null);
  if (!res.ok) {
    const msg =
      res.status === 401
        ? "키가 올바르지 않습니다."
        : res.status === 403
        ? "이 키에는 매물 등록 권한이 없습니다."
        : (data && data.error && (data.error.message || data.error)) || res.statusText;
    throw Object.assign(new Error(typeof msg === "string" ? msg : "요청 실패"), { status: res.status, body: data });
  }
  return data;
}

// ---------- 키 ----------

async function checkKey() {
  const key = getKey();
  const on = Boolean(key);
  ["formCard", "csvCard", "mineCard"].forEach((id) => ($(id).hidden = true));
  $("keyClear").hidden = !on;
  if (!on) return;
  try {
    await loadMine();
    setMsg($("keyMsg"), "확인됐습니다. 매물을 등록할 수 있습니다.", true);
    ["formCard", "csvCard", "mineCard"].forEach((id) => ($(id).hidden = false));
  } catch (e) {
    setMsg($("keyMsg"), e.message, false);
  }
}

$("keySave").addEventListener("click", () => {
  const v = $("key").value.trim();
  if (!v) return;
  try {
    localStorage.setItem(KEY_STORE, v);
  } catch (e) {
    setMsg($("keyMsg"), "브라우저에 키를 저장할 수 없습니다(사생활 보호 모드 등).", false);
    return;
  }
  $("key").value = "";
  checkKey();
});

$("keyClear").addEventListener("click", () => {
  try {
    localStorage.removeItem(KEY_STORE);
  } catch (e) {
    /* 저장소를 못 쓰면 지울 것도 없다 */
  }
  setMsg($("keyMsg"), "키를 지웠습니다.", true);
  checkKey();
});

// ---------- 동네·단지 ----------

async function loadZones() {
  try {
    const { zones } = await api("/b2c/districts");
    $("zone").insertAdjacentHTML("beforeend", zones.map((z) => `<option value="${esc(z.key)}">${esc(z.name)}</option>`).join(""));
  } catch (e) {
    setMsg($("formMsg"), "동네 목록을 불러오지 못했습니다. " + errText(e), false);
  }
}

$("zone").addEventListener("change", async () => {
  const key = $("zone").value;
  complexes = [];
  renderComplexes();
  $("complex").disabled = $("complexFilter").disabled = true;
  if (!key) return;
  setMsg($("formMsg"), "단지 목록을 불러오는 중입니다...", true);
  try {
    const d = await api(`/b2c/zones/${encodeURIComponent(key)}/complexes`);
    complexes = (d.complexes || []).slice().sort((a, b) => a.name.localeCompare(b.name, "ko"));
    $("complex").disabled = $("complexFilter").disabled = false;
    renderComplexes();
    setMsg($("formMsg"), `단지 ${complexes.length}곳`, true);
  } catch (e) {
    setMsg($("formMsg"), "단지 목록을 불러오지 못했습니다. " + errText(e), false);
  }
});

$("complexFilter").addEventListener("input", renderComplexes);

function renderComplexes() {
  const q = $("complexFilter").value.trim();
  $("complex").innerHTML = complexes
    .map((c, i) => ({ c, i }))
    .filter(({ c }) => !q || c.name.includes(q))
    .map(({ c, i }) => `<option value="${i}">${esc(c.name)} (${esc(c.umd_nm)})</option>`)
    .join("");
}

$("price").addEventListener("input", () => {
  const v = Number($("price").value);
  $("pricePreview").textContent = v > 0 ? formatManwon(v) : "";
});

$("submit").addEventListener("click", async () => {
  const c = complexes[Number($("complex").value)];
  const price = Number($("price").value);
  const floor = $("floor").value === "" ? 0 : Number($("floor").value);
  if (!c) return setMsg($("formMsg"), "단지를 골라 주세요.", false);
  if (!(price > 0) || !Number.isInteger(price)) return setMsg($("formMsg"), "호가를 만원 단위 정수로 넣어 주세요.", false);
  if (!Number.isInteger(floor)) return setMsg($("formMsg"), "층은 정수로 넣어 주세요.", false);
  const body = {
    complex_name: c.name,
    type_name: $("type").value.trim(),
    asking_manwon: price,
    floor,
    lon: c.place ? c.place.lon : 0,
    lat: c.place ? c.place.lat : 0,
  };
  $("submit").disabled = true;
  try {
    await writeApi("POST", "/listings", body);
    setMsg($("formMsg"), `${c.name} ${body.type_name} ${formatManwon(price)} 등록했습니다.`, true);
    $("price").value = $("floor").value = $("type").value = "";
    $("pricePreview").textContent = "";
    loadMine();
  } catch (e) {
    setMsg($("formMsg"), e.message, false);
  } finally {
    $("submit").disabled = false;
  }
});

// ---------- CSV ----------

$("csvUpload").addEventListener("click", async () => {
  const f = $("csvFile").files[0];
  if (!f) return setMsg($("csvMsg"), "CSV 파일을 골라 주세요.", false);
  if (f.size > 1 << 20) return setMsg($("csvMsg"), "1MB 이하 파일만 올릴 수 있습니다.", false);
  $("csvUpload").disabled = true;
  try {
    // 바이트 그대로 보낸다. 엑셀 기본 저장(CP949)도 서버가 읽는다.
    const d = await writeApi("POST", "/listings/import", await f.arrayBuffer(), "text/csv");
    setMsg($("csvMsg"), `${d.inserted}건 등록했습니다.`, true);
    $("csvFile").value = "";
    loadMine();
  } catch (e) {
    const rows = e.body && e.body.rows;
    setMsg(
      $("csvMsg"),
      rows ? "아무것도 등록하지 않았습니다. " + rows.map((r) => `${r.line}번째 줄: ${r.message}`).join(" / ") : e.message,
      false
    );
  } finally {
    $("csvUpload").disabled = false;
  }
});

// ---------- 내 매물 ----------

const STATUS = { active: "판매 중", sold: "팔림", withdrawn: "내림" };

async function loadMine() {
  const { listings } = await writeApi("GET", "/listings/mine");
  const box = $("mine");
  if (!listings.length) {
    box.innerHTML = `<p class="muted">아직 등록한 매물이 없습니다.</p>`;
    return;
  }
  box.innerHTML = `<table class="mini reg-table"><tbody>${listings
    .map(
      (l) => `<tr class="${l.status === "active" ? "" : "closed"}">
        <td>${esc(l.complex_name)}<div class="muted">${esc(l.type_name || "")}${l.floor ? " · " + l.floor + "층" : ""}</div></td>
        <td class="r">${formatManwon(l.asking_manwon)}<div class="muted">${STATUS[l.status] || esc(l.status)}</div></td>
        <td class="r">${
          l.status === "active"
            ? `<button class="btn ghost reg-small" data-price-id="${esc(l.id)}" data-price="${l.asking_manwon}">호가 변경</button> <button class="btn ghost reg-small" data-id="${esc(l.id)}" data-s="sold">팔림</button> <button class="btn ghost reg-small" data-id="${esc(l.id)}" data-s="withdrawn">내림</button>`
            : ""
        }</td></tr>`
    )
    .join("")}</tbody></table>`;
  box.querySelectorAll("button[data-price-id]").forEach((b) =>
    b.addEventListener("click", async () => {
      const value = prompt("새 호가를 만원 단위 정수로 입력해 주세요.", b.dataset.price);
      if (value === null) return;
      const price = Number(value.replace(/,/g, ""));
      if (!Number.isSafeInteger(price) || price <= 0 || price > 2147483647) return alert("호가를 올바른 만원 단위 정수로 입력해 주세요.");
      b.disabled = true;
      try {
        await writeApi("PATCH", `/listings/${encodeURIComponent(b.dataset.priceId)}/price`, { asking_manwon: price });
        await loadMine();
      } catch (e) {
        b.disabled = false;
        alert(e.message);
      }
    })
  );
  box.querySelectorAll("button[data-id]").forEach((b) =>
    b.addEventListener("click", async () => {
      b.disabled = true;
      try {
        await writeApi("POST", `/listings/${encodeURIComponent(b.dataset.id)}/close`, { status: b.dataset.s });
        loadMine();
      } catch (e) {
        b.disabled = false;
        alert(e.message);
      }
    })
  );
}

loadZones();
checkKey();
