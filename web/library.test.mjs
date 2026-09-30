import test from 'node:test';
import assert from 'node:assert/strict';
import { Library } from './library.mjs';

// An in-memory stand-in for an OPFS directory handle.
function memoryDirectory() {
  const files = new Map();
  return {
    files,
    async getFileHandle(name, { create } = {}) {
      if (!files.has(name)) {
        if (!create) throw new Error('NotFoundError');
        files.set(name, new Uint8Array());
      }
      return {
        async getFile() { return { size: files.get(name).length, async arrayBuffer() { return files.get(name).slice().buffer; } }; },
        async createWritable() { return { async write(data) { files.set(name, new Uint8Array(data)); }, async close() {} }; },
      };
    },
    async removeEntry(name) { files.delete(name); },
  };
}

const bytes = n => new Uint8Array(n).fill(7);

test('files kept come back in the order they came, and by name once', async () => {
  const dir = memoryDirectory();
  const lib = new Library(dir, { limit: 1000 });
  await lib.keep('a.png', bytes(10));
  await lib.keep('b.svg', bytes(20));
  await lib.keep('a.png', bytes(30));
  const kept = await lib.list();
  assert.deepEqual(kept.map(f => f.name), ['b.svg', 'a.png']);
  assert.deepEqual(kept.map(f => f.size), [20, 30]);
  const [names, contents] = await lib.load();
  assert.deepEqual(names, ['b.svg', 'a.png']);
  assert.equal(contents[1].length, 30);
  // The index survives a fresh Library over the same directory.
  const again = new Library(dir, { limit: 1000 });
  assert.deepEqual((await again.list()).map(f => f.name), ['b.svg', 'a.png']);
});

test('the oldest files go when the library is full, and one too large is not kept', async () => {
  const dir = memoryDirectory();
  const lib = new Library(dir, { limit: 100 });
  await lib.keep('one', bytes(40));
  await lib.keep('two', bytes(40));
  await lib.keep('three', bytes(40));
  assert.deepEqual((await lib.list()).map(f => f.name), ['two', 'three']);
  assert.ok(!dir.files.has('one'));
  assert.equal(await lib.keep('huge', bytes(101)), false);
  assert.deepEqual((await lib.list()).map(f => f.name), ['two', 'three']);
});

test('forgetting empties the library, and names are kept safe', async () => {
  const dir = memoryDirectory();
  const lib = new Library(dir, { limit: 1000 });
  await lib.keep('../escape.png', bytes(1));
  await lib.keep('sub/dir/file.png', bytes(1));
  assert.deepEqual((await lib.list()).map(f => f.name), ['escape.png', 'file.png']);
  await lib.forget();
  assert.deepEqual(await lib.list(), []);
  assert.equal([...dir.files.keys()].filter(k => k.startsWith("f-")).length, 0);
});

test('a missing directory means no library, without failing the page', async () => {
  const lib = new Library(null, { limit: 1000 });
  assert.deepEqual(await lib.list(), []);
  assert.equal(await lib.keep('a', bytes(1)), false);
  assert.deepEqual(await lib.load(), [[], []]);
});
