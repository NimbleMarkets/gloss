import test from 'node:test';
import assert from 'node:assert/strict';
import { File } from 'node:buffer';
import { createPickAPI, fileProblem, maxPickBytes, maxPickFiles, messageProblem, maxMessageCharacters, messageDraft } from './pick-api.mjs';

test('confirmation names exact uploaded IDs and stays inside the token URL', async () => {
  const calls = [];
  const api = createPickAPI(async (path, options) => {
    calls.push({ path, ...options });
    return { ok: true, json: async () => ({ state: 'waiting', files: [] }) };
  });
  await api.list();
  await api.remove('id/with/slashes');
  await api.confirm(['opaque-a', 'opaque-b']);
  await api.decline();
  assert.deepEqual(calls.map(c => [c.path, c.method]), [
    ['files', 'GET'], ['files/id%2Fwith%2Fslashes', 'DELETE'], ['confirm', 'POST'], ['decline', 'POST'],
  ]);
  assert.deepEqual(JSON.parse(calls[2].body), { ids: ['opaque-a', 'opaque-b'] });
  for (const call of calls) {
    assert.equal(call.credentials, 'omit');
    assert.equal(call.referrerPolicy, 'no-referrer');
    assert.equal(call.cache, 'no-store');
  }
});

test('upload uses the original File and reports actual transfer progress', async () => {
  let xhr;
  const values = [];
  const api = createPickAPI(undefined, () => (xhr = {
    upload: {}, open(method, path) { this.method = method; this.path = path; },
    send(body) { this.body = body; },
  }));
  const file = new File(['receipt'], 'receipt.txt', { type: 'text/plain' });
  const promise = api.upload(file, value => values.push(value));
  assert.equal(xhr.path, 'files');
  assert.equal(xhr.method, 'POST');
  assert.equal(xhr.timeout, 120000);
  assert.equal(await xhr.body.get('file').text(), 'receipt');
  assert.equal(xhr.body.get('file').name, 'receipt.txt');
  xhr.upload.onprogress({ lengthComputable: false });
  xhr.upload.onprogress({ lengthComputable: true, loaded: 7, total: 10 });
  assert.deepEqual(values, [70]);
  xhr.status = 200;
  xhr.responseText = JSON.stringify({ files: [{ id: 'abc', name: 'receipt.txt', size: 7 }] });
  xhr.onload();
  assert.deepEqual(await promise, [{ id: 'abc', name: 'receipt.txt', size: 7 }]);
});

test('format refusals, interruption, and malformed replies never report success', async () => {
  for (const reason of ['refused', 'network', 'timeout', 'abort', 'malformed']) {
    let xhr;
    const api = createPickAPI(undefined, () => (xhr = { upload: {}, open() {}, send() {} }));
    const promise = api.upload(new File(['hi'], 'a.txt'));
    if (reason === 'refused') {
      xhr.status = 415; xhr.responseText = 'received text; required image/*'; xhr.onload();
    } else if (reason === 'malformed') {
      xhr.status = 200; xhr.responseText = '<html>'; xhr.onload();
    } else {
      xhr[{ network: 'onerror', timeout: 'ontimeout', abort: 'onabort' }[reason]]();
    }
    await assert.rejects(promise);
  }
  const api = createPickAPI(async () => ({ ok: false, status: 409, text: async () => 'Already finished' }));
  await assert.rejects(api.confirm(['a']), /Already finished/);
});

test('client limits catch empty, oversized, and excess files before uploading', () => {
  assert.match(fileProblem({ name: 'empty', size: 0 }, 0), /empty/);
  assert.match(fileProblem({ name: 'huge', size: maxPickBytes + 1 }, 0), /128 MiB/);
  assert.match(fileProblem({ name: 'extra', size: 1 }, maxPickFiles), /200/);
  assert.equal(fileProblem({ name: 'last', size: maxPickBytes }, maxPickFiles - 1), '');
});

test('confirmation preserves Unicode messages and omits blank optional messages', async () => {
  const bodies = [];
  const api = createPickAPI(async (_path, options) => {
    bodies.push(JSON.parse(options.body));
    return { ok: true, json: async () => ({ state: 'picked' }) };
  });
  const message = '  First page is the receipt.\n東京 ☕ <script>literal</script>  ';
  await api.confirm(['a'], message);
  await api.confirm(['b'], ' \n\t');
  assert.deepEqual(bodies, [{ ids: ['a'], message }, { ids: ['b'] }]);
  assert.equal(messageProblem('😀'.repeat(maxMessageCharacters)), '');
  await assert.rejects(api.confirm(['c'], '😀'.repeat(maxMessageCharacters + 1)), /2,000/);
  assert.equal(bodies.length, 2, 'over-limit message was not sent');
});

test('message drafts are session-scoped and storage failures do not block sending', () => {
  const values = new Map();
  const storage = { getItem: k => values.get(k), setItem: (k, v) => values.set(k, v), removeItem: k => values.delete(k) };
  const draft = messageDraft(storage, '/token-a/');
  draft.save('My notes');
  assert.equal(messageDraft(storage, '/token-a/').load(), 'My notes');
  assert.equal(messageDraft(storage, '/token-b/').load(), '');
  draft.clear();
  assert.equal(draft.load(), '');
  const blocked = messageDraft(undefined, 'blocked');
  blocked.save('Still sendable');
  assert.equal(blocked.load(), '');
  blocked.clear();
});
