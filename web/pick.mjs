import { createPickAPI, fileProblem, messageProblem, messageDraft, maxMessageCharacters } from './pick-api.mjs';

const api = createPickAPI();
const el = id => document.getElementById(id);
const previews = new Map();
let files = [], busy = true, finished = false;
let storage;
try { storage = window.sessionStorage; } catch {}
const draft = messageDraft(storage, `gloss-pick-message:${window.location.pathname}`);
el('reply').value = draft.load();

function notice(text, error = false) {
  el('message').textContent = text;
  el('message').classList.toggle('error', error);
}

function controls() {
  const problem = messageProblem(el('reply').value);
  const count = Array.from(el('reply').value).length;
  el('reply').disabled = busy || finished;
  el('reply').setCustomValidity(problem);
  el('reply').setAttribute('aria-invalid', String(Boolean(problem)));
  el('reply-count').textContent = `${count.toLocaleString()} / ${maxMessageCharacters.toLocaleString()} characters${problem ? ' — please shorten your message.' : ''}`;
  el('reply-count').classList.toggle('error', Boolean(problem));
  el('choose').disabled = busy || finished;
  el('cancel').disabled = busy || finished;
  el('send').disabled = busy || finished || !files.length || Boolean(problem);
  el('send').textContent = files.length ? `Send ${files.length} file${files.length === 1 ? '' : 's'}` : 'Send files';
  for (const button of el('files').querySelectorAll('button')) button.disabled = busy || finished;
}

function size(bytes) {
  return bytes < 1024 * 1024 ? `${Math.max(1, Math.round(bytes / 1024))} KiB` : `${(bytes / 1024 / 1024).toFixed(1)} MiB`;
}

function release(id) {
  if (previews.has(id)) URL.revokeObjectURL(previews.get(id));
  previews.delete(id);
}

function render() {
  el('files').replaceChildren();
  for (const file of files) {
    const row = document.createElement('li');
    const picture = document.createElement(previews.has(file.id) ? 'img' : 'span');
    if (previews.has(file.id)) {
      picture.src = previews.get(file.id);
      picture.alt = '';
      picture.className = 'thumb';
      picture.onerror = () => { release(file.id); render(); };
    } else {
      picture.className = 'file-icon';
      picture.textContent = '▧';
      picture.setAttribute('aria-hidden', 'true');
    }
    const details = document.createElement('div');
    details.className = 'details';
    const name = document.createElement('span');
    name.className = 'filename';
    name.textContent = file.name;
    const info = document.createElement('span');
    info.className = 'size';
    info.textContent = `${size(file.size)} · Ready to send`;
    details.append(name, info);
    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'remove';
    remove.textContent = 'Remove';
    remove.setAttribute('aria-label', `Remove ${file.name}`);
    remove.addEventListener('click', () => removeFile(file.id));
    row.append(picture, details, remove);
    el('files').append(row);
  }
  controls();
}

function finish(state, count = files.length, message = '') {
  finished = true;
  draft.clear();
  for (const id of previews.keys()) release(id);
  el('picker').hidden = true;
  el('result').hidden = false;
  el('result-title').textContent = state === 'picked' ? 'Files sent.' : 'Request declined.';
  el('result-detail').textContent = state === 'picked'
    ? `${count} file${count === 1 ? ' is' : 's are'} ready for the requester. You can close this tab.`
    : 'No files were handed over. You can close this tab.';
  el('result-note').textContent = state === 'picked' ? message : '';
  el('result-note').hidden = !el('result-note').textContent;
  notice('');
  el('result').focus();
}

async function refresh() {
  const result = await api.list();
  files = result.files;
  for (const id of previews.keys()) if (!files.some(file => file.id === id)) release(id);
  if (result.state !== 'waiting') finish(result.state, files.length, result.message);
  render();
}

async function removeFile(id) {
  if (busy || finished) return;
  busy = true;
  controls();
  try {
    const result = await api.remove(id);
    files = result.files;
    release(id);
    notice('File removed.');
  } catch (error) { notice(error.message, true); }
  finally { busy = false; render(); el('choose').focus(); }
}

async function addFiles(chosen) {
  if (busy || finished || !chosen.length) return;
  busy = true;
  controls();
  const problems = [];
  el('progress').hidden = false;
  try {
    for (const [index, file] of Array.from(chosen).entries()) {
      const problem = fileProblem(file, files.length);
      if (problem) { problems.push(problem); continue; }
      notice(`Uploading ${index + 1} of ${chosen.length}: ${file.name}`);
      el('progress').value = 0;
      try {
        const uploaded = await api.upload(file, value => { el('progress').value = value; });
        files.push(...uploaded);
        // Local previews only. The request API exposes no file download route.
        if (/^image\/(png|jpeg|gif|webp|bmp)$/.test(file.type) && file.size <= 16 * 1024 * 1024 && uploaded[0]) {
          previews.set(uploaded[0].id, URL.createObjectURL(file));
        }
        render();
      } catch (error) {
        problems.push(`${file.name}: ${error.message}`);
        // A response may have been lost after the server stored the file.
        // Reconcile first, and stop this batch rather than retrying blindly.
        await refresh();
        break;
      }
    }
    notice(problems.length ? problems.join(' ') : 'Ready when you are. Check the list, then send your files.', problems.length > 0);
  } catch {
    notice('Connection lost. Reload this page to recover completed uploads while the request is still open.', true);
  } finally {
    busy = false;
    el('progress').hidden = true;
    controls();
  }
}

async function settle(decline = false) {
  if (busy || finished || (!decline && !files.length)) return;
  if (!decline && messageProblem(el('reply').value)) { el('reply').reportValidity(); return; }
  busy = true;
  controls();
  notice(decline ? 'Declining request…' : 'Sending your files…');
  try {
    const result = decline ? await api.decline() : await api.confirm(files.map(file => file.id), el('reply').value);
    finish(result.state, files.length, result.message);
  } catch {
    // Do not claim failure: the confirmation might have reached the server.
    try { await refresh(); }
    catch { notice('Could not confirm the result. The request may have finished; ask the requester to check its status.', true); }
    if (!finished && !el('message').classList.contains('error')) notice('The request is still open. Check your files and try again.', true);
  } finally { busy = false; controls(); }
}

el('choose').addEventListener('click', () => el('input').click());
el('input').addEventListener('change', event => {
  const chosen = Array.from(event.target.files);
  event.target.value = '';
  addFiles(chosen);
});
el('send').addEventListener('click', () => settle());
el('cancel').addEventListener('click', () => settle(true));
el('reply').addEventListener('input', () => { draft.save(el('reply').value); controls(); });
window.addEventListener('dragover', event => {
  event.preventDefault();
  if (!busy && !finished) el('drop').classList.add('over');
});
window.addEventListener('dragleave', event => { if (!event.relatedTarget) el('drop').classList.remove('over'); });
window.addEventListener('drop', event => {
  event.preventDefault();
  el('drop').classList.remove('over');
  addFiles(Array.from(event.dataTransfer?.files ?? []));
});
window.addEventListener('pagehide', () => { for (const id of previews.keys()) release(id); });

try {
  await refresh();
  if (!finished) notice(files.length ? 'Your uploaded files are still here. Check the list, then send them.' : '');
  busy = false;
  controls();
} catch {
  el('reply').disabled = true;
  notice('This request is unavailable or has expired. Ask the requester for a new link.', true);
}
