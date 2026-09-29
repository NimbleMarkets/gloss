// Files dropped on the served page are handed to gloss, which keeps them for
// whoever started it. The address is relative: it carries the page's token.
export async function upload(names, contents, send = fetch) {
  const form = new FormData();
  names.forEach((name, i) => form.append('file', new Blob([contents[i]]), name));
  const response = await send('drop', { method: 'POST', body: form });
  if (!response.ok) {
    const reasons = { 400: 'Nothing in the drop could be used.', 403: 'The drop was refused.', 413: 'The drop is over 128 MiB.' };
    throw new Error(reasons[response.status] ?? `The drop failed (HTTP ${response.status}).`);
  }
  return (await response.json()).paths;
}
