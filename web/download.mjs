// Fetch bodies are decoded by the browser. Content-Length describes compressed
// transfer bytes when Content-Encoding is set, so only use it for plain bodies.
export async function readWithProgress(response, progress) {
  const length = Number(response.headers.get('content-length'));
  const encoding = response.headers.get('content-encoding');
  const total = (!encoding || encoding === 'identity') && Number.isFinite(length) && length > 0 ? length : null;
  let loaded = 0;
  progress(loaded, total);
  if (!response.body?.getReader) {
    const bytes = new Uint8Array(await response.arrayBuffer());
    progress(bytes.length, total);
    return bytes;
  }
  const reader = response.body.getReader();
  const chunks = [];
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
      loaded += value.length;
      progress(loaded, total);
    }
  } finally {
    reader.releaseLock();
  }
  const bytes = new Uint8Array(loaded);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.length;
  }
  return bytes;
}

export function downloadStatus(loaded, total) {
  const mib = bytes => (bytes / 1048576).toFixed(1);
  if (total && loaded <= total) {
    return `Downloading gloss… ${mib(loaded)} / ${mib(total)} MiB (${Math.floor(loaded / total * 100)}%)`;
  }
  return `Downloading gloss… ${mib(loaded)} MiB received`;
}
