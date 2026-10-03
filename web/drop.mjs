// The same limit as gloss on the command line (document.MaxFileBytes; a test
// in limits.test.mjs holds the two together), and how messages name it.
export const maxBytes = 128 * 2 ** 20;
export const maxLabel = `${maxBytes / 2 ** 20} MiB`;

// Files dropped on target are read in the page and handed to send(names,
// contents). Every drag is cancelled so the browser never navigates away from
// the session; file drags and enabled URL drags show the hint.
export function installDrop(target, hint, send, report, openURL = null) {
  let depth = 0;
  const carriesFiles = event => [...(event.dataTransfer?.types ?? [])].includes('Files');
  const carriesURL = event => openURL && [...(event.dataTransfer?.types ?? [])].some(type => type === 'text/uri-list' || type === 'text/plain');
  const show = visible => { hint.hidden = !visible; };
  target.addEventListener('dragenter', event => {
    event.preventDefault();
    if (!carriesFiles(event) && !carriesURL(event)) return;
    depth++;
    show(true);
  });
  target.addEventListener('dragover', event => {
    event.preventDefault();
    if (carriesFiles(event) || carriesURL(event)) show(true);
  });
  target.addEventListener('dragleave', event => {
    if (!carriesFiles(event) && !carriesURL(event)) return;
    depth = Math.max(0, depth - 1);
    if (depth === 0) show(false);
  });
  target.addEventListener('drop', async event => {
    event.preventDefault();
    depth = 0;
    show(false);
    if (!carriesFiles(event)) {
      if (!openURL) return;
      const text = event.dataTransfer?.getData('text/uri-list') || event.dataTransfer?.getData('text/plain') || '';
      const lines = text.split(/\r?\n/).map(line => line.trim()).filter(line => line && !line.startsWith('#'));
      if (text.length > 65536 || lines.length !== 1) return report('Drop one http or https URL at a time.');
      try { await openURL(lines[0]); } catch (error) { report(error.message || String(error)); }
      return;
    }
    const names = [];
    const contents = [];
    const skipped = [];
    for (const file of event.dataTransfer.files) {
      if (file.size > maxBytes) {
        skipped.push(`${file.name} (over ${maxLabel})`);
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
