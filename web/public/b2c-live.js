// One authenticated stream per open listing view. Notifications reload the
// authoritative DB snapshot; no external listing provider is required.
class ListingLiveView {
  constructor(name, box, apply) {
    this.name = name;
    this.box = box;
    this.apply = apply;
    this.connected = false;
    this.closed = false;
    this.loading = false;
    this.pending = false;
    this.lastRead = 0;
    this.onVisibility = () => {
      if (document.hidden) this.controller?.abort();
      else { this.connect(); this.refresh(); }
    };
    document.addEventListener("visibilitychange", this.onVisibility);
    this.observer = new MutationObserver(() => {
      if (!box.isConnected) this.stop();
    });
    this.observer.observe(document.body, { childList: true, subtree: true });
    // Recheck freshness metadata and recover if a notification is lost.
    this.fallback = setInterval(() => {
      if (!document.hidden && (!this.connected || Date.now() - this.lastRead > 60000)) this.refresh();
    }, 15000);
    this.connect();
  }

  showStatus() {
    const el = this.box.querySelector(".live-status");
    if (el) el.textContent = this.connected
      ? "매물 변동 실시간 연결됨 · 등록·호가 변경·판매/내림을 자동 반영합니다."
      : "실시간 연결을 확인하고 있습니다. 연결이 지연되면 15초마다 다시 확인합니다.";
  }

  refresh() {
    if (this.closed || document.hidden || !this.box.isConnected) return;
    this.pending = true;
    if (this.loading || this.refreshTimer) return;
    // Coalesce transaction bursts and stay below the public key's request rate.
    this.refreshTimer = setTimeout(async () => {
      this.refreshTimer = null;
      if (this.closed || document.hidden) return;
      this.pending = false;
      this.loading = true;
      try {
        const data = await api("/b2c/listings", { complex: this.name, limit: 50, offset: 0 });
        if (!this.closed && this.box.isConnected) {
          this.lastRead = Date.now();
          this.apply(data);
        }
      } catch (e) {
        const status = this.box.querySelector(".live-status");
        if (status && !this.closed) status.textContent = "매물 갱신을 확인하지 못했습니다. 자동으로 다시 확인합니다. " + errText(e);
      } finally {
        this.loading = false;
        if (this.pending) this.refresh();
      }
    }, Math.max(200, 1000 - (Date.now() - this.lastRead)));
  }

  async connect() {
    if (this.closed || document.hidden || this.connecting) return;
    clearTimeout(this.reconnectTimer);
    this.connecting = true;
    this.controller = new AbortController();
    try {
      const cfg = await B2D_CONFIG;
      if (this.closed || document.hidden) return;
      const response = await fetch(cfg.apiBase + "/b2c/listings/stream", {
        headers: cfg.demoKey ? { Authorization: "Bearer " + cfg.demoKey } : {},
        signal: this.controller.signal,
      });
      if (!response.ok || !response.headers.get("Content-Type")?.startsWith("text/event-stream") || !response.body) {
        throw new Error("live stream unavailable");
      }
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, "\n");
        if (buffer.length > 65536) throw new Error("invalid live event");
        let end;
        while ((end = buffer.indexOf("\n\n")) >= 0) {
          const frame = buffer.slice(0, end);
          buffer = buffer.slice(end + 2);
          const event = /^event: (\w+)$/m.exec(frame)?.[1];
          if (event === "ready" || event === "changed") {
            this.connected = true;
            this.showStatus();
            this.refresh();
          } else if (event === "unavailable") {
            this.connected = false;
            this.showStatus();
          }
        }
      }
    } catch (e) {
      if (e.name !== "AbortError") console.debug("[b2c] live connection will retry");
    } finally {
      this.controller.abort();
      this.connecting = false;
      this.connected = false;
      if (!this.closed) {
        this.showStatus();
        if (!document.hidden) this.reconnectTimer = setTimeout(() => this.connect(), 1500 + Math.random() * 1500);
      }
    }
  }

  stop() {
    this.closed = true;
    this.controller?.abort();
    clearTimeout(this.refreshTimer);
    clearTimeout(this.reconnectTimer);
    clearInterval(this.fallback);
    this.observer.disconnect();
    document.removeEventListener("visibilitychange", this.onVisibility);
  }
}
