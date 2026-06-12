// Derive the library-scan indicator's display state from a /api/scan snapshot
// ({running, added, updated, skipped, failed}). Pure so it can be unit-tested
// and reused by the TopBar component.
export function scanIndicator(status) {
  if (!status || !status.running) return { visible: false, text: '' };
  const indexed = (status.added || 0) + (status.updated || 0);
  return { visible: true, text: `Scanning library… ${indexed} tracks` };
}
