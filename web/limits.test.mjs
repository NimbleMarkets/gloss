import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile, readdir } from 'node:fs/promises';
import { maxBytes, maxLabel } from './drop.mjs';
import { maxPicked } from './pickers.mjs';

// The page repeats limits that gloss enforces in Go. These tests read the Go
// source and the guide, so a change to one side fails here until the other
// follows.
const read = path => readFile(new URL(path, import.meta.url), 'utf8');

test('the page limit is the loader limit', async () => {
  const go = await read('../internal/document/loader.go');
  const m = go.match(/^const MaxFileBytes = (\d+) << (\d+)$/m);
  assert.ok(m, 'MaxFileBytes not found in internal/document/loader.go');
  assert.equal(maxBytes, Number(m[1]) * 2 ** Number(m[2]));
});

test('the guide names the limit the page applies', async () => {
  assert.match(await read('../docs/hugo/content/guide/formats.md'), new RegExp(`limited to ${maxLabel}`));
});

test('the guide names how many files one pick reads', async () => {
  assert.match(await read('../docs/hugo/content/guide/handoff.md'), new RegExp(`more than ${maxPicked} files`));
});

test('no page script spells the limit out', async () => {
  for (const name of (await readdir(new URL('.', import.meta.url))).filter(n => n.endsWith('.mjs') && !n.endsWith('.test.mjs'))) {
    const text = await read(name);
    assert.ok(!/\b\d+ MiB/.test(text.replace(/\$\{[^}]*\}/g, '')) || name === 'download.mjs', `${name} writes a size in MiB; use maxLabel`);
  }
});
