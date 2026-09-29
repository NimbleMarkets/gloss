import test from 'node:test';
import assert from 'node:assert/strict';
import { readWithProgress, downloadStatus } from './download.mjs';

test('reports streamed bytes and reconstructs the binary without corruption', async () => {
  const body = new ReadableStream({ start(controller) {
    controller.enqueue(new Uint8Array([0, 97, 115]));
    controller.enqueue(new Uint8Array([109, 255]));
    controller.close();
  } });
  const events = [];
  const result = await readWithProgress(new Response(body, { headers: { 'content-length': '5' } }), (...event) => events.push(event));
  assert.deepEqual([...result], [0, 97, 115, 109, 255]);
  assert.deepEqual(events, [[0, 5], [3, 5], [5, 5]]);
});

test('compressed or missing lengths show byte counts without a false percentage', async () => {
  for (const headers of [{}, { 'content-length': '1', 'content-encoding': 'gzip' }]) {
    const events = [];
    await readWithProgress(new Response('decoded', { headers }), (...event) => events.push(event));
    assert.deepEqual(events.at(-1), [7, null]);
  }
  assert.match(downloadStatus(1048576, null), /1.0 MiB received/);
  assert.match(downloadStatus(1048576, 2097152), /50%/);
  assert.doesNotMatch(downloadStatus(3, 2), /150%/);
});

test('a broken download rejects rather than compiling incomplete bytes', async () => {
  const body = new ReadableStream({ pull(controller) { controller.error(new Error('connection lost')); } });
  await assert.rejects(readWithProgress(new Response(body), () => {}), /connection lost/);
  assert.equal(body.locked, false);
});
