export const maxBytes = 128 * 2 ** 20; // The same limit as gloss on the command line.

// Files dropped on target are read in the page and handed to send(names,
// contents). Every drag is cancelled so the browser never navigates away from
// the session; only file drags show the hint.
export function installDrop(target, hint, send, report) {
  let depth = 0;
  const carriesFiles = event => [...(event.dataTransfer?.types ?? [])].includes('Files');
  const show = visible => { hint.hidden = !visible; };
  target.addEventListener('dragenter', event => {
    event.preventDefault();
    if (!carriesFiles(event)) return;
    depth++;
    show(true);
  });
  target.addEventListener('dragover', event => {
    event.preventDefault();
    if (carriesFiles(event)) show(true);
  });
  target.addEventListener('dragleave', event => {
    if (!carriesFiles(event)) return;
    depth = Math.max(0, depth - 1);
    if (depth === 0) show(false);
  });
  target.addEventListener('drop', async event => {
    event.preventDefault();
    depth = 0;
    show(false);
    if (!carriesFiles(event)) return;
    const names = [];
    const contents = [];
    const skipped = [];
    for (const file of event.dataTransfer.files) {
      if (file.size > maxBytes) {
        skipped.push(`${file.name} (over 128 MiB)`);
        continue;
      }
      try {
        const bytes = new Uint8Array(await file.arrayBuffer());
        if (bytes.length === 0) {
          skipped.push(`${file.name} (empty)`);
          continue;
        }
        names.push(file.name);
        contents.push(bytes);
      } catch {
        // Folders arrive as entries that cannot be read.
        skipped.push(`${file.name} (not a file)`);
      }
    }
    if (skipped.length) report(`Skipped ${skipped.join(', ')}`);
    if (names.length) send(names, contents);
  });
}
