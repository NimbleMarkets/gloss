import { readWithProgress, downloadStatus } from './download.mjs';
import { installDrop } from './drop.mjs';
import { saveFile } from './save.mjs';

const status = document.querySelector('#status');
function message(text) {
  status.replaceChildren(document.createTextNode(text));
  const retry = document.createElement('button');
  retry.textContent = 'Restart demo';
  retry.addEventListener('click', () => location.reload());
  status.append(retry);
  status.hidden = false;
}
// Dropped files stay in this tab's memory and are gone on reload.
const hint = document.querySelector('#drop');
const invitation = hint.textContent;
let focus = () => {};
function notice(text) {
  hint.textContent = text;
  hint.hidden = false;
  setTimeout(() => { hint.hidden = true; hint.textContent = invitation; }, 4000);
}
installDrop(window, hint, (names, contents) => {
  if (typeof window.gloss_drop !== 'function') return notice('gloss is still loading.');
  window.gloss_drop(names, contents);
  focus();
}, notice);
window.gloss_save = (name, bytes) => saveFile(name, bytes);
window.addEventListener('gloss-exit', () => message('You quit gloss. Restart to explore again.'));
try {
  const { BoobaTerminal } = await import('./booba/booba.js');
  await import('./booba-shim/pdfium/pdfium-shim.js');
  // PDF initialization is asynchronous; its Go bridge waits for readiness.
  window.boobaShim.pdfium.ready.catch(error => console.error('PDF renderer:', error));
  const go = new Go();
  const sample = new URL(location.href).searchParams.get('sample');
  const names = new Set(['landscape.png','landscape.heic','shapes.svg','field-guide.pdf','gloss.stl','readme.md']);
  if (sample && !names.has(sample)) throw new Error('Unknown embedded sample.');
  go.argv = ['gloss-demo', ...(sample ? ['--sample', sample] : [])];
  status.textContent = 'Connecting to download gloss…';
  const response = await fetch('app.wasm');
  if (!response.ok) throw new Error(`Demo download failed: HTTP ${response.status}`);
  const bytes = await readWithProgress(response, (loaded, total) => {
    status.textContent = downloadStatus(loaded, total);
  });
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
} catch (error) {
  console.error(error);
  message(`Unable to start gloss: ${error.message || error}`);
}
