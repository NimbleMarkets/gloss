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
  vm.runInNewContext(await readFile(new URL('./site.js', import.meta.url), 'utf8'), { document, window: { addEventListener() {} } });
  tabs[0].listeners.click();
  assert.equal(urls.length, 0);
  tabs[1].listeners.click();
  assert.deepEqual(urls, ['term.html?sample=landscape.heic']);
  tabs[1].listeners.click();
  assert.equal(urls.length, 1);
  restart.listeners.click();
  assert.equal(urls.length, 2);
  assert.equal(urls[1], urls[0]);
  assert.equal(link.href, urls[1]);
});

test('a phone-width screen leads with a sample image instead of the menu', async () => {
  function button(sample) {
    return { dataset: { sample }, pressed: null, listeners: {}, addEventListener(name, fn) { this.listeners[name] = fn; }, setAttribute(name, value) { if (name === 'aria-pressed') this.pressed = value; } };
  }
  const tabs = [button(''), button('landscape.png')];
  const urls = [];
  const frame = { set src(value) { urls.push(value); } };
  const link = {};
  const document = {
    querySelector: selector => ({ '#demo': frame, '#restart': button(), '#standalone': link })[selector],
    querySelectorAll: () => tabs,
  };
  const window = { addEventListener() {}, matchMedia: () => ({ matches: true }) };
  vm.runInNewContext(await readFile(new URL('./site.js', import.meta.url), 'utf8'), { document, window });
  assert.deepEqual(urls, ['term.html?sample=landscape.png']);
  assert.equal(link.href, urls[0]);
  assert.equal(tabs[0].pressed, 'false');
  assert.equal(tabs[1].pressed, 'true');
});
