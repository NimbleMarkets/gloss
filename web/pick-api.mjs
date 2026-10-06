import { maxBytes, maxLabel } from './drop.mjs';
import { maxPicked } from './pickers.mjs';

// The upload page talks only in upload IDs; it never sends local host paths.
// These relative URLs preserve the request's token and work without a viewer.
export function createPickAPI(send = fetch, makeXHR = () => new XMLHttpRequest()) {
  async function request(path, method = 'GET', body) {
    const response = await send(path, {
      method, cache: 'no-store', credentials: 'omit', referrerPolicy: 'no-referrer',
      ...(body === undefined ? {} : { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }),
    });
    if (!response.ok) throw new Error((await response.text()).trim() || `Request failed (${response.status}).`);
    return response.json();
  }
  return {
    list: () => request('files'),
    remove: id => request(`files/${encodeURIComponent(id)}`, 'DELETE'),
    async confirm(ids, message = '') {
      const problem = messageProblem(message);
      if (problem) throw new Error(problem);
      return request('confirm', 'POST', { ids, ...(message.trim() ? { message } : {}) });
    },
    decline: () => request('decline', 'POST'),
    upload(file, progress = () => {}) {
      // A File goes straight into FormData: no arrayBuffer copy of large files.
      return new Promise((resolve, reject) => {
        const xhr = makeXHR();
        xhr.open('POST', 'files');
        xhr.timeout = 120000;
        xhr.upload.onprogress = event => {
          if (event.lengthComputable) progress(Math.round(event.loaded / event.total * 100));
        };
        xhr.onload = () => {
          if (xhr.status < 200 || xhr.status >= 300) {
            reject(new Error(xhr.responseText.trim() || `Upload failed (${xhr.status}).`));
            return;
          }
          try { resolve(JSON.parse(xhr.responseText).files); }
          catch { reject(new Error('The upload response could not be read.')); }
        };
        xhr.onerror = () => reject(new Error('The upload connection was lost.'));
        xhr.ontimeout = () => reject(new Error('The upload took too long.'));
        xhr.onabort = () => reject(new Error('The upload was interrupted.'));
        const body = new FormData();
        body.append('file', file, file.name);
        xhr.send(body);
      });
    },
  };
}

export const maxPickFiles = maxPicked;
export const maxPickBytes = maxBytes;
export const maxMessageCharacters = 2000;

export function messageProblem(message) {
  return Array.from(message).length > maxMessageCharacters ? 'Message must be at most 2,000 characters.' : '';
}

// Drafts stay in this browser tab, scoped to the session URL. Nothing sends
// the draft to the requester until confirmation. Blocked storage is harmless.
export function messageDraft(storage, key) {
  return {
    load() { try { return storage.getItem(key) ?? ''; } catch { return ''; } },
    save(value) { try { value ? storage.setItem(key, value) : storage.removeItem(key); } catch {} },
    clear() { try { storage.removeItem(key); } catch {} },
  };
}

export function fileProblem(file, count) {
  if (count >= maxPickFiles) return `At most ${maxPickFiles} files can be added.`;
  if (!file.size) return `${file.name} is empty.`;
  if (file.size > maxPickBytes) return `${file.name} is over ${maxLabel}.`;
  return '';
}
