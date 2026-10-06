import test from 'node:test';
import assert from 'node:assert/strict';

// Exercise the actual page controller and event handlers with a small DOM.
class Element {
  value = ''; textContent = ''; children = []; handlers = {}; attributes = {};
  hidden = false; disabled = false;
  classes = new Set();
  classList = {
    toggle: (name, on) => on ? this.classes.add(name) : this.classes.delete(name),
    contains: name => this.classes.has(name),
    add: name => this.classes.add(name), remove: name => this.classes.delete(name),
  };
  append(...children) { this.children.push(...children); }
  replaceChildren() { this.children = []; }
  querySelectorAll() { return this.children.flatMap(row => row.children.filter(c => c.tag === 'button')); }
  setAttribute(name, value) { this.attributes[name] = value; }
  setCustomValidity(value) { this.validity = value; }
  reportValidity() { return !this.validity; }
  addEventListener(name, handler) { this.handlers[name] = handler; }
  focus() {}
}

let imports = 0;
async function page(t, { values = new Map(), reject = false } = {}) {
  const ids = ['choose', 'cancel', 'send', 'files', 'reply', 'reply-count', 'picker', 'result', 'result-title', 'result-detail', 'result-note', 'message', 'input', 'drop', 'progress'];
  const elements = Object.fromEntries(ids.map(id => [id, new Element()]));
  const calls = [];
  const original = { document: globalThis.document, window: globalThis.window, fetch: globalThis.fetch };
  t.after(() => Object.assign(globalThis, original));
  globalThis.document = {
    getElementById: id => elements[id],
    createElement: tag => Object.assign(new Element(), { tag }),
  };
  globalThis.window = {
    location: { pathname: '/token/' }, addEventListener() {},
    sessionStorage: { getItem: k => values.get(k), setItem: (k, v) => values.set(k, v), removeItem: k => values.delete(k) },
  };
  globalThis.fetch = async (path, options) => {
    calls.push({ path, options });
    if (path === 'confirm' && reject) return { ok: false, status: 503, text: async () => 'Try again' };
    const message = options.body ? JSON.parse(options.body).message : undefined;
    return { ok: true, json: async () => ({
      state: path === 'confirm' ? 'picked' : path === 'decline' ? 'declined' : 'waiting',
      files: [{ id: 'file-a', name: 'receipt.pdf', size: 42 }], message,
    }) };
  };
  await import(`./pick.mjs?test=${++imports}`);
  return { elements, calls, values };
}

test('page validates, submits, and displays the exact optional message as text', async t => {
  const { elements: e, calls, values } = await page(t);
  e.reply.value = 'x'.repeat(2001);
  e.reply.handlers.input();
  assert.equal(e.send.disabled, true);
  const message = 'Page 2 is missing.\n東京 ☕ <script>literal text</script>';
  e.reply.value = message;
  e.reply.handlers.input();
  assert.equal(e.send.disabled, false);
  assert.equal(values.get('gloss-pick-message:/token/'), message);
  await e.send.handlers.click();
  const sent = JSON.parse(calls.find(c => c.path === 'confirm').options.body);
  assert.deepEqual(sent, { ids: ['file-a'], message });
  assert.equal(e['result-note'].textContent, message);
  assert.equal(e['result-note'].hidden, false);
  assert.equal(e.send.disabled, true);
  assert.equal(values.size, 0);
});

test('drafts survive refresh and failed confirmation; Cancel does not send them', async t => {
  const values = new Map([['gloss-pick-message:/token/', 'Saved draft ☕']]);
  const { elements: e, calls } = await page(t, { values, reject: true });
  assert.equal(e.reply.value, 'Saved draft ☕');
  await e.send.handlers.click();
  assert.equal(e.reply.value, 'Saved draft ☕');
  assert.equal(values.get('gloss-pick-message:/token/'), 'Saved draft ☕');
  assert.equal(e.send.disabled, false);
  await e.cancel.handlers.click();
  assert.equal(calls.find(c => c.path === 'decline').options.body, undefined);
  assert.equal(e['result-note'].textContent, '');
  assert.equal(values.size, 0);
});
