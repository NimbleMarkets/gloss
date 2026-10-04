import { BoobaTerminal, resolveBoobaURLs } from './static/booba/booba.js';
import { parseRendererFromURL } from './static/ghostty-web/ghostty-web.js';
import { installDrop } from './drop.mjs';
import { fetchDocument } from './remote.mjs';
import { upload } from './upload.mjs';
import { pickFiles, inputPicker } from './pickers.mjs';

const status = document.querySelector('#status');
const hint = document.querySelector('#drop');
const invitation = hint.textContent;
const choose = document.querySelector('#choose-files');
const message = document.querySelector('#message');
let terminal;
function notice(text) {
	message.textContent = text;
  hint.textContent = text;
  hint.hidden = false;
  setTimeout(() => { hint.hidden = true; hint.textContent = invitation; }, 4000);
}
async function chosen(names, contents, skipped = []) {
  if (!names.length) {
    if (skipped.length) notice(skipped.join('; '));
    return;
  }
  choose.disabled = true;
  try {
    await upload(names, contents);
    message.textContent = `${names.length} file${names.length === 1 ? '' : 's'} added. ${skipped.join('; ')}`;
  } catch (error) {
    notice(error.message || String(error));
  } finally {
    choose.disabled = false;
  }
  terminal?.focus();
}
let fetching = false;
installDrop(window, hint, chosen, notice, document.body.dataset.fetch === 'true' ? async address => {
  if (fetching) throw new Error('A document is already being fetched.');
  fetching = true;
  try {
    const [name, data] = await fetchDocument(address);
    await chosen([name], [data]);
  } finally { fetching = false; }
} : null);
choose.addEventListener('click', async () => {
  try {
    await chosen(...await pickFiles(window, inputPicker(document)));
  } catch (error) {
    notice(error.message || String(error));
  }
});
// An export made in the viewer is kept by the server for this page, which
// saves it as a download: where the browser saves, not where gloss runs.
async function takeExports(open) {
  let after = 0;
  while (open()) {
    let list;
    try {
      const response = await fetch(`exports?after=${after}`, { cache: 'no-store' });
      if (!response.ok) return;
      list = await response.json();
    } catch {
      return; // The server has gone with the viewer.
    }
    for (const { id, name } of list) {
      const link = document.createElement('a');
      link.href = `export/${id}`;
      link.download = name;
      link.click();
      after = id + 1;
    }
  }
}
try {
  terminal = new BoobaTerminal('terminal', { renderer: parseRendererFromURL() });
  let connected = false, ended = false;
  terminal.onStatusChange = state => {
    if (state === 'connected') {
      connected = true;
      choose.disabled = false;
      status.hidden = true;
    } else if (connected && state === 'disconnected') {
      // gloss ends with its viewer, and its server with it.
      ended = true;
      status.textContent = 'gloss has finished. You can close this tab.';
      status.hidden = false;
      choose.disabled = true;
    }
  };
  terminal.onTitleChange = title => { document.title = title || 'gloss'; };
  takeExports(() => !ended);
  await terminal.init();
  const urls = resolveBoobaURLs(document.baseURI);
  terminal.connectAuto(urls.wsUrl, null, null);
  terminal.focus();
} catch (error) {
  console.error(error);
  status.textContent = `Unable to start gloss: ${error.message || error}`;
  status.hidden = false;
  choose.disabled = true;
}
