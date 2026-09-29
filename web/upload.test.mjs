import test from 'node:test';
import assert from 'node:assert/strict';
import { upload } from './upload.mjs';

function server(status, body) {
  const calls = [];
  const send = async (url, init) => {
    calls.push({ url, init });
    return { ok: status >= 200 && status < 300, status, json: async () => body };
  };
  return { calls, send };
}

test('dropped files are posted together, each under its own name', async () => {
  const { calls, send } = server(200, { paths: ['/tmp/gloss-1/a.png', '/tmp/gloss-1/b.svg'] });
  const paths = await upload(['a.png', 'b.svg'], [new Uint8Array([1, 2, 3]), new Uint8Array([4])], send);
  assert.deepEqual(paths, ['/tmp/gloss-1/a.png', '/tmp/gloss-1/b.svg']);
  assert.equal(calls.length, 1);
  // Relative, so that it carries the page's token.
  assert.equal(calls[0].url, 'drop');
  assert.equal(calls[0].init.method, 'POST');
  const files = calls[0].init.body.getAll('file');
  assert.deepEqual(files.map(f => f.name), ['a.png', 'b.svg']);
  assert.deepEqual([...new Uint8Array(await files[0].arrayBuffer())], [1, 2, 3]);
});

test('a refusal is explained', async () => {
  for (const [status, words] of [[413, /128 MiB/], [400, /nothing/i], [403, /refused/i], [500, /500/]]) {
    const { send } = server(status, {});
    await assert.rejects(upload(['a.png'], [new Uint8Array([1])], send), words);
  }
  await assert.rejects(upload(['a.png'], [new Uint8Array([1])], async () => { throw new Error('connection lost'); }), /connection lost/);
});
