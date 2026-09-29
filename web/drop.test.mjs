import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';
import { installDrop, maxBytes } from './drop.mjs';

function page() {
  const target = { listeners: {}, addEventListener(name, fn) { this.listeners[name] = fn; } };
  const hint = { hidden: true };
  const sent = [];
  const reports = [];
  installDrop(target, hint, (names, contents) => { sent.push({ names, contents }); return names.length; }, text => reports.push(text));
  return { target, hint, sent, reports };
}

function drag(files, types = ['Files']) {
  return { prevented: 0, preventDefault() { this.prevented++; }, dataTransfer: { types, files } };
}

function file(name, bytes, size = bytes.length) {
  return { name, size, reads: 0, async arrayBuffer() { this.reads++; return Uint8Array.from(bytes).buffer; } };
}

test('the hint follows a file drag without letting the browser navigate', () => {
  const { target, hint } = page();
  for (const name of ['dragenter', 'dragover']) {
    const event = drag([]);
    target.listeners[name](event);
    assert.equal(event.prevented, 1, name);
    assert.equal(hint.hidden, false, name);
  }
  // Moving between child elements must not flicker the hint.
  target.listeners.dragenter(drag([]));
  target.listeners.dragleave(drag([]));
  assert.equal(hint.hidden, false);
  target.listeners.dragleave(drag([]));
  assert.equal(hint.hidden, true);
});

test('dragged text is refused without offering to open it', async () => {
  const { target, hint, sent } = page();
  const over = drag([], ['text/plain']);
  target.listeners.dragover(over);
  assert.equal(over.prevented, 1);
  assert.equal(hint.hidden, true);
  const drop = drag([], ['text/uri-list']);
  await target.listeners.drop(drop);
  assert.equal(drop.prevented, 1);
  assert.equal(sent.length, 0);
});

test('dropped files reach the pager in one message', async () => {
  const { target, hint, sent, reports } = page();
  target.listeners.dragenter(drag([]));
  const event = drag([file('a.png', [1, 2, 3]), file('b.svg', [4])]);
  await target.listeners.drop(event);
  assert.equal(event.prevented, 1);
  assert.equal(hint.hidden, true);
  assert.equal(sent.length, 1);
  assert.deepEqual(sent[0].names, ['a.png', 'b.svg']);
  assert.ok(sent[0].contents[0] instanceof Uint8Array);
  assert.deepEqual(sent[0].contents.map(bytes => [...bytes]), [[1, 2, 3], [4]]);
  assert.deepEqual(reports, []);
  // The next drag starts from a clean count.
  target.listeners.dragenter(drag([]));
  target.listeners.dragleave(drag([]));
  assert.equal(hint.hidden, true);
});

test('oversized and unreadable drops are skipped, not read', async () => {
  const { target, sent, reports } = page();
  const huge = file('huge.pdf', [1], maxBytes + 1);
  const folder = { name: 'photos', size: 0, async arrayBuffer() { throw new Error('not a file'); } };
  const empty = file('empty.png', []);
  await target.listeners.drop(drag([huge, folder, empty, file('ok.png', [9])]));
  assert.equal(huge.reads, 0);
  assert.deepEqual(sent[0].names, ['ok.png']);
  assert.equal(reports.length, 1);
  assert.match(reports[0], /huge\.pdf.*128 MiB/);
  assert.match(reports[0], /photos/);
  assert.match(reports[0], /empty\.png/);
});

test('a drop with nothing usable sends nothing', async () => {
  const { target, sent, reports } = page();
  await target.listeners.drop(drag([file('huge.stl', [1], maxBytes + 1)]));
  assert.equal(sent.length, 0);
  assert.equal(reports.length, 1);
});

test('the gallery page refuses stray drops beside the terminal', async () => {
  const listeners = {};
  const window = { addEventListener(name, fn) { listeners[name] = fn; } };
  const document = {
    querySelector: () => ({ addEventListener() {} }),
    querySelectorAll: () => [],
  };
  vm.runInNewContext(await readFile(new URL('./site.js', import.meta.url), 'utf8'), { document, window });
  for (const name of ['dragover', 'drop']) {
    const event = drag([]);
    listeners[name](event);
    assert.equal(event.prevented, 1, name);
  }
});
