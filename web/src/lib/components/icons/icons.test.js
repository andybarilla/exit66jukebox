import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const read = (rel) => readFileSync(fileURLToPath(new URL(rel, import.meta.url)), 'utf8');

// Glyphs the player/cast surface previously used as stock/emoji controls.
const FORBIDDEN = ['📡', '⏮', '⏭', '▶', '❚❚', '◼', '♪'];

const PLAYER_FILES = [
  '../NowPlayingBar.svelte',
  '../MobilePlayer.svelte',
  '../CastPanel.svelte',
  '../TrackRow.svelte',
];

describe('player/cast icon sweep', () => {
  for (const file of PLAYER_FILES) {
    it(`${file} has no leftover emoji/stock glyphs`, () => {
      const src = read(file);
      const found = FORBIDDEN.filter((g) => src.includes(g));
      expect(found).toEqual([]);
    });
  }

  it('TrackRow uses IconPlus instead of a "+" glyph button', () => {
    const src = read('../TrackRow.svelte');
    expect(src).toContain('IconPlus');
    // The add button no longer renders a bare "+" as its text content.
    expect(src).not.toMatch(/>\s*\+\s*</);
  });
});
