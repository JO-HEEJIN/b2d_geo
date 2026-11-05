const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const code = fs.readFileSync(path.join(__dirname, '../public/b2c.js'), 'utf8');
const start = code.indexOf('function renderMarketCoverage(');
const end = code.indexOf('\nfunction renderRecentlySold(', start);
assert.ok(start >= 0 && end > start);
const render = vm.runInNewContext(code.slice(start, end) + '\nrenderMarketCoverage', {
  esc: (s) => String(s).replaceAll('<', '&lt;').replaceAll('>', '&gt;'),
  formatManwon: (value) => String(value),
});
test('unknown market inventory is never presented as zero', () => {
  assert.match(render({ status: 'not_connected', listing_count: null }), /남은 매물 수는 확인할 수 없습니다/);
  assert.doesNotMatch(render({ status: 'not_connected', listing_count: null }), /0건/);
  assert.match(render({ status: 'unavailable', sources: [], listing_count: null }), /남은 매물 수를 확정할 수 없습니다/);
  assert.doesNotMatch(render({ status: 'partial', sources: [], listing_count: null }), /0건/);
});
test('only a known complete count displays zero, and missing is not sold', () => {
  const html = render({ status: 'ready', listing_count: 0, sources: [{ name: '<supplier>', observed_at: new Date().toISOString() }], changes: [{ kind: 'missing', external_id: '<id>' }] });
  assert.match(html, /0건/);
  assert.match(html, /판매 확인 안 됨/);
  assert.doesNotMatch(html, /<supplier>|<id>/);
});
