import test from 'node:test';
import assert from 'node:assert/strict';
import { bootConfig } from './boot.mjs';

test('demo mode takes a sample from the query and passes it to the wasm', () => {
  const config = bootConfig('{"mode":"demo"}', 'https://example.test/term.html?sample=gloss.stl');
  assert.equal(config.mode, 'demo');
  assert.deepEqual(config.argv, ['gloss-demo', '--sample', 'gloss.stl']);
  assert.equal(config.src, null);
  assert.deepEqual(bootConfig('{"mode":"demo"}', 'https://example.test/term.html').argv, ['gloss-demo']);
});

test('app mode starts empty and may name a document to fetch', () => {
  const config = bootConfig('{"mode":"app"}', 'https://example.test/app.html?src=https://files.test/report.pdf');
  assert.equal(config.mode, 'app');
  assert.deepEqual(config.argv, ['gloss-demo', '--app']);
  assert.equal(config.src, 'https://files.test/report.pdf');
  // A sample means nothing to the app, and a src nothing to the demo.
  assert.deepEqual(bootConfig('{"mode":"app"}', 'https://example.test/app.html?sample=gloss.stl').argv, ['gloss-demo', '--app']);
  assert.equal(bootConfig('{"mode":"demo"}', 'https://example.test/term.html?src=https://files.test/x.pdf').src, null);
});

test('the query may set the mode, and bad config falls back to the demo', () => {
  assert.equal(bootConfig('{"mode":"demo"}', 'https://example.test/term.html?mode=app').mode, 'app');
  assert.equal(bootConfig('not json', 'https://example.test/term.html').mode, 'demo');
  assert.equal(bootConfig('{"mode":"other"}', 'https://example.test/term.html').mode, 'demo');
  assert.equal(bootConfig(null, 'https://example.test/term.html').mode, 'demo');
});

test('only web addresses are fetched for src', () => {
  for (const src of ['file:///etc/passwd', 'javascript:alert(1)', 'data:text/plain,hi', 'ftp://x/y']) {
    assert.equal(bootConfig('{"mode":"app"}', `https://example.test/app.html?src=${encodeURIComponent(src)}`).src, null, src);
  }
});
