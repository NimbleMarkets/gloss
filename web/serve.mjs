import { BoobaTerminal, resolveBoobaURLs } from './static/booba/booba.js';
import { parseRendererFromURL } from './static/ghostty-web/ghostty-web.js';
import { installDrop } from './drop.mjs';
import { upload } from './upload.mjs';

const status = document.querySelector('#status');
const hint = document.querySelector('#drop');
const invitation = hint.textContent;
let terminal;
function notice(text) {
  hint.textContent = text;
  hint.hidden = false;
  setTimeout(() => { hint.hidden = true; hint.textContent = invitation; }, 4000);
}
installDrop(window, hint, async (names, contents) => {
  try {
    await upload(names, contents);
  } catch (error) {
    notice(error.message || String(error));
  }
  terminal?.focus();
}, notice);
try {
  terminal = new BoobaTerminal('terminal', { renderer: parseRendererFromURL() });
  let connected = false;
  terminal.onStatusChange = state => {
    if (state === 'connected') {
      connected = true;
      status.hidden = true;
    } else if (connected && state === 'disconnected') {
      // gloss ends with its viewer, and its server with it.
      status.textContent = 'gloss has finished. You can close this tab.';
      status.hidden = false;
    }
  };
  terminal.onTitleChange = title => { document.title = title || 'gloss'; };
  await terminal.init();
  const urls = resolveBoobaURLs(document.baseURI);
  terminal.connectAuto(urls.wsUrl, null, null);
  terminal.focus();
} catch (error) {
  console.error(error);
  status.textContent = `Unable to start gloss: ${error.message || error}`;
  status.hidden = false;
}
