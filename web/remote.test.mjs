import test from 'node:test';
import assert from 'node:assert/strict';
import { fetchDocument, nameFor } from './remote.mjs';

test('a document is fetched by the browser and named after its address', async () => {
  const send = async () => new Response(new Uint8Array([1, 2, 3]), { headers: { 'content-type': 'image/png' } });
  const [name, bytes] = await fetchDocument('https://files.test/photos/holiday.png?x=1', send);
  assert.equal(name, 'holiday.png');
  assert.deepEqual([...bytes], [1, 2, 3]);
});

test('a name comes from the address, or the content type when the address has none', () => {
  assert.equal(nameFor('https://files.test/a/b/report.pdf', ''), 'report.pdf');
  assert.equal(nameFor('https://files.test/', 'image/png'), 'document.png');
  assert.equal(nameFor('https://files.test/view', 'application/pdf; charset=x'), 'view.pdf');
  assert.equal(nameFor('https://files.test/odd', 'application/octet-stream'), 'odd');
});

test('failures are said plainly: not found, too large, refused', async () => {
  await assert.rejects(fetchDocument('https://files.test/x', async () => new Response('', { status: 404 })), /HTTP 404/);
  const huge = async () => new Response(new Uint8Array(1), { headers: { 'content-length': String(200 * 2 ** 20) } });
  await assert.rejects(fetchDocument('https://files.test/x', huge), /128 MiB/);
  await assert.rejects(fetchDocument('https://files.test/x', async () => { throw new TypeError('Failed to fetch'); }), /could not be fetched/);
  await assert.rejects(fetchDocument('https://files.test/x', async () => new Response('')), /empty/);
});
