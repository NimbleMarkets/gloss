import { maxBytes, maxLabel } from './drop.mjs';

const extensions = { 'image/png': '.png', 'image/jpeg': '.jpg', 'image/gif': '.gif', 'image/webp': '.webp', 'image/svg+xml': '.svg', 'application/pdf': '.pdf', 'text/markdown': '.md', 'text/html': '.html', 'text/plain': '.txt', 'text/csv': '.csv', 'application/json': '.json', 'model/stl': '.stl', 'model/3mf': '.3mf' };

// nameFor names a fetched document after the last part of its address,
// with an extension from its content type where the address gives none.
export function nameFor(address, contentType) {
  let name = '';
  try { name = decodeURIComponent(new URL(address).pathname.split('/').filter(Boolean).pop() ?? ''); } catch { /* no name */ }
  name = name.split(/[\\/]/).pop() || 'document';
  const type = (contentType || '').split(';')[0].trim().toLowerCase();
  if (!/\.[a-z0-9]{1,6}$/i.test(name) && extensions[type]) name += extensions[type];
  return name;
}

// fetchDocument fetches the address, in the browser, and gives its name
// and bytes. The address's server must allow it (CORS); the size limit is
// gloss's own.
export async function fetchDocument(address, send = fetch) {
  let response;
  try {
    response = await send(address, { mode: 'cors', credentials: 'omit', redirect: 'follow' });
  } catch {
    throw new Error('The document could not be fetched: the address may be wrong, or its server may not allow pages to read it.');
  }
  if (!response.ok) throw new Error(`The document could not be fetched: HTTP ${response.status}.`);
  const length = Number(response.headers.get('content-length'));
  if (length > maxBytes) throw new Error(`The document is over ${maxLabel}.`);
  const bytes = new Uint8Array(await response.arrayBuffer());
  if (bytes.length > maxBytes) throw new Error(`The document is over ${maxLabel}.`);
  if (bytes.length === 0) throw new Error('The document is empty.');
  return [nameFor(address, response.headers.get('content-type')), bytes];
}
