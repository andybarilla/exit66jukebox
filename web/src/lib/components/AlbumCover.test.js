import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const read = (rel) => readFileSync(fileURLToPath(new URL(rel, import.meta.url)), 'utf8');

// Clicking the cover in the now-playing bar, the Lineup block, a queue item, or
// the mobile player opens that track's album (#71). The cover becomes a real
// <button> (keyboard-accessible) only when an albumId + onOpenAlbum are present;
// otherwise it stays a plain <div> so a missing album_id is an inert no-op.
const COMPONENTS = [
  'NowPlayingBar.svelte',
  'Lineup.svelte',
  'QueueItem.svelte',
  'MobilePlayer.svelte',
];

describe.each(COMPONENTS)('album-cover affordance in %s', (file) => {
  const src = read(`./${file}`);

  it('gates the affordance on a truthy albumId (album_id 0 = no album)', () => {
    expect(src).toMatch(/\$derived\([^)]*!!\s*[\w.?]*albumId/);
  });

  it('renders a keyboard-accessible button labelled "Open album" when openable', () => {
    expect(src).toContain('<button type="button"');
    expect(src).toContain('aria-label="Open album"');
  });

  it('falls back to a non-interactive div when not openable', () => {
    expect(src).toMatch(/\{:else\}\s*<div[^>]*>\{@render/);
  });
});

describe('Lineup forwards album fields to queue items', () => {
  const src = read('./Lineup.svelte');
  it('passes albumId and an onOpenAlbum handler to each QueueItem', () => {
    expect(src).toMatch(/albumId=\{q\.albumId\}/);
    expect(src).toContain('onOpenAlbum(q)');
  });
});
