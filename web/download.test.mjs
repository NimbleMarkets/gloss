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

test('an encoded download uses the expected decoded size for progress, never a plain length', async () => {
  const gz = { 'content-length': '1', 'content-encoding': 'gzip' };
  const events = [];
  await readWithProgress(new Response('decoded', { headers: gz }), (...event) => events.push(event), 7);
  assert.deepEqual(events, [[0, 7], [7, 7]]);
  events.length = 0;
  await readWithProgress(new Response('decoded', { headers: { 'content-length': '7' } }), (...event) => events.push(event), 99);
  assert.deepEqual(events.at(-1), [7, 7]);
  for (const bad of [0, NaN, -1]) {
    events.length = 0;
    await readWithProgress(new Response('decoded', { headers: gz }), (...event) => events.push(event), bad);
    assert.deepEqual(events.at(-1), [7, null]);
  }
});

test('a broken download rejects rather than compiling incomplete bytes', async () => {
  const body = new ReadableStream({ pull(controller) { controller.error(new Error('connection lost')); } });
  await assert.rejects(readWithProgress(new Response(body), () => {}), /connection lost/);
  assert.equal(body.locked, false);
});
