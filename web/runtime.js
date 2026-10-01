import { readWithProgress, downloadStatus } from './download.mjs';
import { installDrop } from './drop.mjs';
import { saveFile } from './save.mjs';
import { bootConfig, emptied } from './boot.mjs';
import { openLibrary } from './library.mjs';
import { pickFiles, pickFolder, inputPicker } from './pickers.mjs';
import { fetchDocument } from './remote.mjs';

const config = bootConfig(document.querySelector('#gloss-boot')?.textContent, location.href);
const $ = selector => document.querySelector(selector);
const status = $('#status');
function message(text) {
  status.replaceChildren(document.createTextNode(text));
  const retry = document.createElement('button');
  retry.textContent = config.mode === 'app' ? 'Restart' : 'Restart demo';
  retry.addEventListener('click', () => location.reload());
  status.append(retry);
  status.hidden = false;
}
const hint = $('#drop');
const invitation = hint.textContent;
let focus = () => {};
function notice(text) {
  hint.textContent = text;
  hint.hidden = false;
  setTimeout(() => { hint.hidden = true; hint.textContent = invitation; }, 4000);
}

// In app mode what is opened is kept in the browser's storage for this
// site, and a later visit finds it again. The demo keeps nothing.
const library = config.mode === 'app' ? await openLibrary() : null;
async function tellLibrary() {
  const line = $('#library');
  if (!line || !library) return;
  const kept = await library.list();
  const size = kept.reduce((sum, f) => sum + f.size, 0);
  line.textContent = kept.length ? `${kept.length} file${kept.length === 1 ? '' : 's'} kept in this browser (${sizeText(size)})` : 'Nothing kept yet';
  $('#forget').hidden = kept.length === 0;
}
function sizeText(bytes) {
  return bytes >= 2 ** 20 ? `${(bytes / 2 ** 20).toFixed(1)} MiB` : `${Math.max(1, Math.round(bytes / 1024))} KiB`;
}
async function open(names, contents, { keep = true } = {}) {
  if (typeof window.gloss_drop !== 'function') return notice('gloss is still loading.');
  window.gloss_drop(names, contents);
  focus();
  if (keep && library) {
    for (let i = 0; i < names.length; i++) await library.keep(names[i], contents[i]);
    await tellLibrary();
  }
}
installDrop(window, hint, open, notice);
window.gloss_save = (name, bytes) => saveFile(name, bytes);
window.addEventListener('gloss-exit', () => message(config.mode === 'app' ? 'You quit gloss. Restart to open your files again.' : 'You quit gloss. Restart to explore again.'));

if (config.mode === 'app') {
  const chosen = async ([names, contents, skipped]) => {
    if (skipped.length) notice(`Skipped ${skipped.join(', ')}`);
    if (names.length) await open(names, contents);
  };
  const input = inputPicker(document);
  $('#open-files')?.addEventListener('click', () => pickFiles(window, input).then(chosen, error => notice(error.message)));
  $('#open-folder')?.addEventListener('click', () => pickFolder(window, input).then(chosen, error => notice(error.message)));
  // Forgetting is of everything: gloss holds what it was given in its own
  // memory, so the page starts again, with nothing to open.
  $('#forget')?.addEventListener('click', async () => { await library.forget(); location.replace(emptied(location.href)); });
  await tellLibrary();
  if (config.src && $('#notice')) {
    // The bar takes its room before the terminal measures what is left.
    $('#notice').textContent = `Fetching ${config.src}…`;
    $('#notice').hidden = false;
  }
}

try {
  const { BoobaTerminal } = await import('./booba/booba.js');
  await import('./booba-shim/pdfium/pdfium-shim.js');
  // PDF initialization is asynchronous; its Go bridge waits for readiness.
  window.boobaShim.pdfium.ready.catch(error => console.error('PDF renderer:', error));
  const go = new Go();
  go.argv = config.argv;
  status.textContent = 'Connecting to download gloss…';
  // The build records the decoded size, which a compressed response hides.
  const sizeText = fetch('app.wasm.size').then(r => (r.ok ? r.text() : ''), () => '');
  const response = await fetch('app.wasm');
  if (!response.ok) throw new Error(`Download failed: HTTP ${response.status}`);
  const bytes = await readWithProgress(response, (loaded, total) => {
    status.textContent = downloadStatus(loaded, total);
  }, Number(await sizeText));
  status.textContent = 'Download complete. Compiling WebAssembly…';
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  status.textContent = 'Starting the terminal…';
  const terminal = new BoobaTerminal('terminal');
  await terminal.init();
  go.run(instance).catch(error => message(`Unable to run gloss: ${error.message || error}`));
  terminal.connectWasm();
  status.hidden = true;
  focus = () => terminal.focus();
  focus();
  if (config.mode === 'app') {
    // What was kept comes back first; then what the link asks for.
    const [names, contents] = await library.load();
    if (names.length) await open(names, contents, { keep: false });
    if (config.src) {
      const where = $('#notice');
      try {
        const [name, data] = await fetchDocument(config.src);
        // Shown, and not kept: a link must not be able to fill the visitor's library,
        // pushing out the files they chose to keep.
        await open([name], [data], { keep: false });
        if (where) { where.textContent = `Opened ${name} from ${new URL(config.src).host}: fetched by your browser at this link's request, and not kept. Nothing else was fetched, and nothing was uploaded.`; where.hidden = false; }
      } catch (error) {
        if (where) { where.textContent = `${error.message} (${config.src})`; where.hidden = false; }
      }
    }
  }
} catch (error) {
  console.error(error);
  message(`Unable to start gloss: ${error.message || error}`);
}
