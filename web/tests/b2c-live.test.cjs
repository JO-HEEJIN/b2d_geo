const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');
const { setTimeout: delay } = require('node:timers/promises');

test('fragmented notifications refresh committed snapshots, coalesce bursts and stop on navigation', async () => {
  let stream, reads = 0, price = 80000, observerCallback;
  const status = { textContent: '' };
  const box = { isConnected: true, querySelector: () => status };
  const applied = [];
  const handlers = new Map();
  const document = {
    hidden: false, body: {},
    addEventListener: (name, cb) => handlers.set(name, cb),
    removeEventListener: (name) => handlers.delete(name),
  };
  const context = vm.createContext({
    document, box, console, AbortController, TextDecoder, setTimeout, clearTimeout, setInterval, clearInterval,
    B2D_CONFIG: Promise.resolve({ apiBase: '/v1', demoKey: 'test-only' }),
    MutationObserver: class { constructor(cb) { observerCallback = cb; } observe() {} disconnect() {} },
    api: async () => { reads++; return { price }; },
    errText: (e) => e.message,
    fetch: async (url, init) => {
      assert.equal(url, '/v1/b2c/listings/stream');
      assert.equal(init.headers.Authorization, 'Bearer test-only');
      return {
        ok: true, headers: { get: () => 'text/event-stream' },
        body: new ReadableStream({
          start(controller) {
            stream = controller;
            init.signal.addEventListener('abort', () => controller.close(), { once: true });
          },
        }),
      };
    },
    apply: (data) => applied.push(data.price),
  });
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../public/b2c-live.js'), 'utf8') + '\nthis.view = new ListingLiveView("complex", box, apply);', context);
  try {
    await delay(20);
    stream.enqueue(new TextEncoder().encode('event: rea'));
    stream.enqueue(new TextEncoder().encode('dy\ndata: {}\n\n'));
    await delay(300);
    assert.deepEqual(applied, [80000]);
    assert.match(status.textContent, /실시간 연결됨/);
    price = 80500;
    for (let i = 0; i < 20; i++) stream.enqueue(new TextEncoder().encode('event: changed\ndata: {}\n\n'));
    await delay(1100);
    assert.equal(reads, 2, 'a burst must produce one latest-snapshot reload');
    assert.deepEqual(applied, [80000, 80500]);
    box.isConnected = false;
    observerCallback();
    await delay(30);
    assert.equal(context.view.closed, true);
    assert.equal(handlers.size, 0, 'navigation must remove visibility listeners');
    assert.equal(reads, 2);
  } finally { context.view.stop(); }
});
