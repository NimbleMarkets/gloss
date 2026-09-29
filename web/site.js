const frame = document.querySelector('#demo');
const buttons = [...document.querySelectorAll('[data-sample]')];
let sample = '';
function openSample(value, restart = false) {
  if (value === sample && !restart) return;
  sample = value;
  const url = `demo.html${sample ? `?sample=${encodeURIComponent(sample)}` : ''}`;
  frame.src = url;
  document.querySelector('#standalone').href = url;
  for (const button of buttons) button.setAttribute('aria-pressed', String(button.dataset.sample === sample));
}
for (const button of buttons) button.addEventListener('click', () => openSample(button.dataset.sample));
document.querySelector('#restart').addEventListener('click', () => openSample(sample, true));
// A drop that misses the terminal must not replace the page with the file.
for (const name of ['dragover', 'drop']) window.addEventListener(name, event => event.preventDefault());
