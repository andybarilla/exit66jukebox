import { afterEach, describe, expect, test, vi } from 'vitest';
import { getLibraries, setLibraries } from './auth.js';

afterEach(() => vi.restoreAllMocks());

describe('library admin api', () => {
  test('reads library settings from admin endpoint', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ json: async () => ({ local_libraries: [] }) })));
    await expect(getLibraries()).resolves.toEqual({ local_libraries: [] });
    expect(fetch).toHaveBeenCalledWith('/api/admin/libraries');
  });

  test('saves library settings as json', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ ok: true }) })));
    await expect(setLibraries({ save_and_scan: true })).resolves.toEqual({ ok: true });
    expect(fetch).toHaveBeenCalledWith('/api/admin/libraries', expect.objectContaining({ method: 'POST' }));
  });
});
