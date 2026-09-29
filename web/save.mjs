// The browser has no working directory, so an export becomes a download.
export function saveFile(name, bytes, page = globalThis) {
  const url = page.URL.createObjectURL(new page.Blob([bytes.slice()], { type: 'image/png' }));
  const link = page.document.createElement('a');
  link.href = url;
  link.download = name;
  link.click();
  page.setTimeout(() => page.URL.revokeObjectURL(url), 1000);
}
