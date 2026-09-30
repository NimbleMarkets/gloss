import test from 'node:test';
import assert from 'node:assert/strict';
import { readChosen, pickFiles, pickFolder, maxPicked } from './pickers.mjs';

const file = (name, size = 3) => ({ name, size, async arrayBuffer() { return new Uint8Array(size).fill(1).buffer; } });

test('chosen files are read in the page, the empty, the huge, and the hidden left out', async () => {
  const [names, contents, skipped] = await readChosen([file('a.png'), file('empty.png', 0), file('big.png', 129 * 2 ** 20), file('.DS_Store'), file('b.svg')]);
  assert.deepEqual(names, ['a.png', 'b.svg']);
  assert.equal(contents[0].length, 3);
  assert.deepEqual(skipped, ['empty.png (empty)', 'big.png (over 128 MiB)', '.DS_Store (hidden)']);
});

test('no more than a sensible number are read from one pick', async () => {
  const many = Array.from({ length: maxPicked + 5 }, (_, i) => file(`f${i}.png`));
  const [names, , skipped] = await readChosen(many);
  assert.equal(names.length, maxPicked);
  assert.ok(skipped.at(-1).includes('more'));
});

test('the native picker is used where there is one, else a file input', async () => {
  const handles = [{ async getFile() { return file('picked.png'); } }];
  const win = { showOpenFilePicker: async () => handles };
  assert.deepEqual((await pickFiles(win, () => { throw new Error('input used'); }))[0], ['picked.png']);
  const input = { files: [file('typed.png')] };
  assert.deepEqual((await pickFiles({}, async () => input.files))[0], ['typed.png']);
  // A cancelled picker picks nothing, and is no error.
  const cancelled = { showOpenFilePicker: async () => { throw new DOMException('cancelled', 'AbortError'); } };
  assert.deepEqual(await pickFiles(cancelled, () => []), [[], [], []]);
});

test('a folder is read through, files first at each level, folders within not hidden', async () => {
  const dir = (name, entries) => ({ kind: 'directory', name, async *values() { yield* entries; } });
  const fh = (name) => ({ kind: 'file', name, async getFile() { return file(name); } });
  const root = dir('root', [fh('z.png'), dir('.git', [fh('config')]), dir('sub', [fh('y.png')]), fh('a.png')]);
  const win = { showDirectoryPicker: async () => root };
  const [names] = await pickFolder(win, () => []);
  assert.deepEqual(names, ['a.png', 'z.png', 'sub/y.png']);
});
