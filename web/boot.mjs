// The page says which mode it boots in; the query may add a sample (demo)
// or a document to fetch (app). Everything else the wasm decides.
export function bootConfig(json, href) {
  let mode = 'demo';
  try {
    const page = JSON.parse(json ?? '');
    if (page.mode === 'app' || page.mode === 'demo') mode = page.mode;
  } catch {
    // The page's config is missing or malformed: the demo is the safe mode.
  }
  const query = new URL(href).searchParams;
  if (query.get('mode') === 'app' || query.get('mode') === 'demo') mode = query.get('mode');
  const argv = ['gloss-demo'];
  let src = null;
  if (mode === 'app') {
    argv.push('--app');
    src = webAddress(query.get('src'));
  } else if (query.get('sample')) {
    argv.push('--sample', query.get('sample'));
  }
  return { mode, argv, src };
}

// emptied is the page's address to start again from once the library is
// forgotten: the same page, without the document a link asked it to fetch,
// which would otherwise come straight back.
export function emptied(href) {
  const url = new URL(href);
  url.searchParams.delete('src');
  return url.href;
}

// webAddress is the address as given when it is one the browser may fetch
// a document from, else null.
export function webAddress(value) {
  if (!value) return null;
  try {
    const url = new URL(value);
    return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : null;
  } catch {
    return null;
  }
}
