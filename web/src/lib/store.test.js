import { describe, it, expect, vi, afterEach } from 'vitest';
import { createStore } from './store.svelte.js';

// createStore reads localStorage for the saved display name; stub it (node env).
global.localStorage = { getItem: () => null, setItem: () => {} };

afterEach(() => { vi.restoreAllMocks(); });

// openAlbum maps the album's enriched track rows for the dialog. The backend
// serves the track position as `track_no`; the dialog renders it, so mapTrack
// must carry it through as `trackNo` (issue #68).
describe('openAlbum track mapping', () => {
  it('carries track_no through as trackNo for the album dialog', async () => {
    global.fetch = vi.fn(async () => ({
      json: async () => [
        { id: 1, title: 'Come Together', track_no: 1, duration: 200 },
        { id: 2, title: 'Something', track_no: 2, duration: 180 },
      ],
    }));
    const store = createStore();
    await store.openAlbum({ id: 10, name: 'Abbey Road', artistName: 'The Beatles' });
    expect(store.detailAlbum.tracks.map((t) => t.trackNo)).toEqual([1, 2]);
  });

  it('defaults a missing/zero track_no to 0 so the row degrades gracefully', async () => {
    global.fetch = vi.fn(async () => ({
      json: async () => [{ id: 3, title: 'Untitled', duration: 0 }],
    }));
    const store = createStore();
    await store.openAlbum({ id: 11, name: 'X', artistName: 'Y' });
    expect(store.detailAlbum.tracks[0].trackNo).toBe(0);
  });
});
