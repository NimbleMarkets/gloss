import { maxBytes, maxLabel } from './drop.mjs';

// A pick reads at most this many files, so that a folder of thousands
// does not stall the page.
export const maxPicked = 200;

// readChosen reads files chosen by whatever means into names and contents,
// as a drop is read, and says what it left out.
export async function readChosen(files) {
  const names = [];
  const contents = [];
  const skipped = [];
  for (const f of files) {
    if (names.length >= maxPicked) {
      skipped.push(`${files.length - maxPicked} more (at most ${maxPicked} at once)`);
      break;
    }
    const name = f.name ?? '';
    const base = name.split('/').pop();
    if (base.startsWith('.')) { skipped.push(`${base} (hidden)`); continue; }
    if (f.size > maxBytes) { skipped.push(`${base} (over ${maxLabel})`); continue; }
    if (f.size === 0) { skipped.push(`${base} (empty)`); continue; }
    try {
      names.push(name);
      contents.push(new Uint8Array(await f.arrayBuffer()));
    } catch {
      names.pop();
      skipped.push(`${base} (unreadable)`);
    }
  }
  return [names, contents, skipped];
}

// pickFiles asks for files with the browser's own picker where there is
// one, else through a file input that fallback provides.
export async function pickFiles(win, fallback) {
  try {
    if (typeof win.showOpenFilePicker === 'function') {
      const handles = await win.showOpenFilePicker({ multiple: true });
      return readChosen(await Promise.all(handles.map(h => h.getFile())));
    }
    return readChosen(await fallback({ multiple: true }));
  } catch (error) {
    if (error?.name === 'AbortError') return [[], [], []];
    throw error;
  }
}

// pickFolder asks for a folder and reads the files in it and below, files
// before folders at each level, hidden ones left out, up to the pick's
// limit. Where there is no folder picker, a file input over a folder does.
export async function pickFolder(win, fallback) {
  try {
    if (typeof win.showDirectoryPicker === 'function') {
      const root = await win.showDirectoryPicker();
      const files = [];
      await walk(root, '', files);
      return readChosen(files);
    }
    return readChosen(await fallback({ directory: true }));
  } catch (error) {
    if (error?.name === 'AbortError') return [[], [], []];
    throw error;
  }
}

async function walk(dir, prefix, files, depth = 0) {
  if (depth > 6 || files.length > maxPicked) return;
  const folders = [];
  const here = [];
  for await (const entry of dir.values()) {
    if (entry.name.startsWith('.')) continue;
    if (entry.kind === 'directory') folders.push(entry);
    else here.push(entry);
  }
  here.sort((a, b) => a.name.localeCompare(b.name));
  for (const entry of here) {
    const f = await entry.getFile();
    files.push({ name: prefix + entry.name, size: f.size, arrayBuffer: () => f.arrayBuffer() });
  }
  for (const folder of folders) await walk(folder, `${prefix}${folder.name}/`, files, depth + 1);
}

// inputPicker makes the fallback: a file input clicked for the visitor,
// whose files are handed back once chosen.
export function inputPicker(document) {
  return ({ multiple = false, directory = false } = {}) => new Promise(resolve => {
    const input = document.createElement('input');
    input.type = 'file';
    input.hidden = true;
    input.multiple = multiple;
    if (directory) input.webkitdirectory = true;
    // Keep the input attached while the native dialog is open. Safari may
    // otherwise discard it before delivering the chosen files.
    const finish = files => { input.remove(); resolve(files); };
    input.addEventListener('change', () => finish([...input.files].map(f => ({ name: f.webkitRelativePath || f.name, size: f.size, arrayBuffer: () => f.arrayBuffer() }))));
    input.addEventListener('cancel', () => finish([]));
    document.body.append(input);
    input.click();
  });
}
