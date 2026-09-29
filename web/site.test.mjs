import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';

test('active tabs preserve the session while Restart explicitly reloads it', async () => {
  function button(sample) {
    return { dataset: { sample }, listeners: {}, addEventListener(name, fn) { this.listeners[name] = fn; }, setAttribute() {} };
  }
  const tabs = [button(''), button('landscape.heic')];
  const restart = button();
  const urls = [];
  const frame = { set src(value) { urls.push(value); } };
  const link = {};
  const document = {
    querySelector: selector => ({ '#demo': frame, '#restart': restart, '#standalone': link })[selector],
    querySelectorAll: () => tabs,
  };
  vm.runInNewContext(await readFile(new URL('./site.js', import.meta.url), 'utf8'), { document });
  tabs[0].listeners.click();
  assert.equal(urls.length, 0);
  tabs[1].listeners.click();
  assert.deepEqual(urls, ['demo.html?sample=landscape.heic']);
  tabs[1].listeners.click();
  assert.equal(urls.length, 1);
  restart.listeners.click();
  assert.equal(urls.length, 2);
  assert.equal(urls[1], urls[0]);
  assert.equal(link.href, urls[1]);
});
