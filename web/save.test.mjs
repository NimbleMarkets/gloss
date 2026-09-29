import test from 'node:test';
import assert from 'node:assert/strict';
import { saveFile } from './save.mjs';

test('an export is offered as a PNG download, then released', () => {
  const links = [];
  const released = [];
  const blobs = [];
  const page = {
    Blob: class { constructor(parts, options) { blobs.push({ parts, options }); } },
    URL: { createObjectURL: () => 'blob:0', revokeObjectURL: url => released.push(url) },
    document: { createElement: tag => { const link = { tag, clicks: 0, click() { this.clicks++; } }; links.push(link); return link; } },
    setTimeout: fn => fn(),
  };
  const bytes = new Uint8Array([137, 80, 78, 71]);
  saveFile('field-guide-page-2.png', bytes, page);
  assert.equal(links.length, 1);
  assert.equal(links[0].tag, 'a');
  assert.equal(links[0].download, 'field-guide-page-2.png');
  assert.equal(links[0].href, 'blob:0');
  assert.equal(links[0].clicks, 1);
  assert.deepEqual(blobs[0].options, { type: 'image/png' });
  assert.deepEqual([...blobs[0].parts[0]], [137, 80, 78, 71]);
  // Go reuses its memory; the download must own a copy.
  assert.notEqual(blobs[0].parts[0].buffer, bytes.buffer);
  assert.deepEqual(released, ['blob:0']);
});
