// The library keeps the files the app was given, in the browser's own
// storage for this site, so that a visit later finds them again. Nothing
// leaves the browser.
export const libraryLimit = 256 * 2 ** 20;
const indexName = 'library.json';

export class Library {
  // directory is an OPFS directory handle, or null where there is none.
  constructor(directory, { limit = libraryLimit } = {}) {
    this.directory = directory;
    this.limit = limit;
    this.index = null; // [{name, size, added}], oldest first.
  }

  async list() {
    if (!this.directory) return [];
    if (this.index) return this.index.map(f => ({ ...f }));
    try {
      const handle = await this.directory.getFileHandle(indexName);
      const file = await handle.getFile();
      const parsed = JSON.parse(new TextDecoder().decode(await file.arrayBuffer()));
      this.index = Array.isArray(parsed) ? parsed.filter(f => typeof f.name === 'string' && Number.isFinite(f.size)) : [];
    } catch {
      this.index = [];
    }
    return this.index.map(f => ({ ...f }));
  }

  // keep stores bytes under name, in place of any file of that name, and
  // lets the oldest go to make room. It reports whether the file was kept.
  async keep(rawName, bytes) {
    if (!this.directory) return false;
    const name = safeName(rawName);
    if (!name || bytes.length > this.limit) return false;
    await this.list();
    await this.remove(name);
    let total = this.index.reduce((sum, f) => sum + f.size, 0) + bytes.length;
    while (total > this.limit && this.index.length) {
      const oldest = this.index[0];
      await this.remove(oldest.name);
      total -= oldest.size;
    }
    try {
      const handle = await this.directory.getFileHandle(fileName(name), { create: true });
      const writable = await handle.createWritable();
      await writable.write(bytes);
      await writable.close();
    } catch {
      return false;
    }
    this.index.push({ name, size: bytes.length, added: Date.now() });
    await this.save();
    return true;
  }

  // load reads every kept file, oldest first, as names and contents.
  async load() {
    const names = [];
    const contents = [];
    for (const entry of await this.list()) {
      try {
        const handle = await this.directory.getFileHandle(fileName(entry.name));
        const file = await handle.getFile();
        names.push(entry.name);
        contents.push(new Uint8Array(await file.arrayBuffer()));
      } catch {
        // A file gone from under the index is left out.
      }
    }
    return [names, contents];
  }

  async forget() {
    await this.list();
    for (const entry of [...this.index]) await this.remove(entry.name);
    await this.save();
  }

  async remove(name) {
    const i = this.index.findIndex(f => f.name === name);
    if (i < 0) return;
    this.index.splice(i, 1);
    try { await this.directory.removeEntry(fileName(name)); } catch { /* already gone */ }
  }

  async save() {
    try {
      const handle = await this.directory.getFileHandle(indexName, { create: true });
      const writable = await handle.createWritable();
      await writable.write(new TextEncoder().encode(JSON.stringify(this.index)));
      await writable.close();
    } catch {
      // Storage that cannot be written is as good as none.
    }
  }
}

// safeName keeps only the last element of a name, so that nothing reaches
// outside the library's folder.
export function safeName(name) {
  return String(name ?? '').split(/[\\/]/).pop().trim();
}

// Files are stored under a prefix, so that the index cannot be taken for one.
function fileName(name) { return `f-${name}`; }

// openLibrary finds the browser's storage for this site, or gives an
// empty library where there is none (older browsers, private windows).
export async function openLibrary(storage = globalThis.navigator?.storage) {
  try {
    const root = await storage.getDirectory();
    return new Library(await root.getDirectoryHandle('library', { create: true }));
  } catch {
    return new Library(null);
  }
}
