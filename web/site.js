const frame = document.querySelector('#demo');
const buttons = [...document.querySelectorAll('[data-sample]')];
let sample = '';
function openSample(value, restart = false) {
  if (value === sample && !restart) return;
  sample = value;
  const url = `term.html${sample ? `?sample=${encodeURIComponent(sample)}` : ''}`;
  frame.src = url;
  document.querySelector('#standalone').href = url;
  for (const button of buttons) button.setAttribute('aria-pressed', String(button.dataset.sample === sample));
}
for (const button of buttons) button.addEventListener('click', () => openSample(button.dataset.sample));
document.querySelector('#restart').addEventListener('click', () => openSample(sample, true));
// A drop that misses the terminal must not replace the page with the file.
window.addEventListener('dragover', event => event.preventDefault());

window.addEventListener('drop', event => {
  event.preventDefault();
  if ([...(event.dataTransfer?.types ?? [])].includes('Files')) return;
  const text = event.dataTransfer?.getData('text/uri-list') || event.dataTransfer?.getData('text/plain') || '';
  const lines = text.split(/\r?\n/).map(line => line.trim()).filter(line => line && !line.startsWith('#'));
  if (text.length <= 65536 && lines.length === 1 && /^https?:\/\//i.test(lines[0])) {
    frame.contentWindow.postMessage({ type: 'gloss-url-drop', url: lines[0] }, location.origin);
  }
});
