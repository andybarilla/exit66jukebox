import { describe, expect, test, vi } from 'vitest';
import {
  beforeUnloadIfDirty,
  buildEditableSettingsSnapshot,
  hasEditableSettingsChanges,
  loadPathBrowserLocation,
} from './settingsPanelState.js';

describe('buildEditableSettingsSnapshot', () => {
  test('includes editable access, library, and federation fields', () => {
    const snapshot = buildEditableSettingsSnapshot({
      signupEnabled: true,
      guestAccess: false,
      libraries: [
        {
          id: 'library-a',
          name: 'Main Library',
          path: '/srv/music',
          enabled: true,
          last_scan_at: '2026-01-01T00:00:00Z',
        },
      ],
      federation: {
        enabled: true,
        role: 'hub',
        hub_addr: 'https://hub.example.test',
        listen: ':9443',
        token: 'new-token',
        peer_id: 'peer-a',
        token_configured: true,
        restart_required: true,
      },
    });

    expect(JSON.parse(snapshot)).toEqual({
      signupEnabled: true,
      guestAccess: false,
      libraries: [
        {
          id: 'library-a',
          name: 'Main Library',
          path: '/srv/music',
          enabled: true,
        },
      ],
      federation: {
        enabled: true,
        role: 'hub',
        hub_addr: 'https://hub.example.test',
        listen: ':9443',
        token: 'new-token',
        peer_id: 'peer-a',
      },
    });
  });

  test('excludes transient admin panel data from the snapshot', () => {
    const snapshot = buildEditableSettingsSnapshot({
      signupEnabled: false,
      guestAccess: true,
      loading: true,
      error: 'failed',
      invites: [{ id: 1, email: 'invite@example.test' }],
      users: [{ id: 2, email: 'user@example.test' }],
      federationPeers: [{ peer_id: 'peer-b' }],
      libraryWarnings: [{ path: '/srv/music', message: 'missing' }],
      libraryBusy: true,
      libraryMessage: 'Saved.',
      libraryError: 'oops',
      libraries: [
        {
          id: 'library-a',
          name: 'Main Library',
          path: '/srv/music',
          enabled: false,
          missing: true,
          scan_status: 'running',
          last_scan_at: '2026-01-01T00:00:00Z',
        },
      ],
      federation: {
        enabled: false,
        role: 'member',
        hub_addr: 'https://hub.example.test',
        listen: ':9443',
        token: '',
        peer_id: 'peer-a',
        token_configured: false,
        restart_required: true,
      },
    });

    expect(snapshot).not.toContain('loading');
    expect(snapshot).not.toContain('invite@example.test');
    expect(snapshot).not.toContain('user@example.test');
    expect(snapshot).not.toContain('peer-b');
    expect(snapshot).not.toContain('restart_required');
    expect(snapshot).not.toContain('token_configured');
    expect(snapshot).not.toContain('libraryWarnings');
    expect(snapshot).not.toContain('scan_status');
    expect(snapshot).not.toContain('last_scan_at');
  });

  test('produces stable normalized JSON when transient fields differ', () => {
    const baseState = {
      signupEnabled: true,
      guestAccess: true,
      libraries: [{ id: 3, name: 'Archive', path: '/mnt/archive', enabled: true }],
      federation: { enabled: true, role: 'peer', hub_addr: '', listen: ':9443', token: '', peer_id: 'peer-a' },
    };

    const firstSnapshot = buildEditableSettingsSnapshot({
      ...baseState,
      loading: true,
      libraries: [{ ...baseState.libraries[0], last_scan_at: 'first' }],
      federation: { ...baseState.federation, restart_required: true },
    });
    const secondSnapshot = buildEditableSettingsSnapshot({
      ...baseState,
      loading: false,
      libraries: [{ ...baseState.libraries[0], last_scan_at: 'second' }],
      federation: { ...baseState.federation, restart_required: false },
    });

    expect(firstSnapshot).toBe(secondSnapshot);
  });
});

describe('hasEditableSettingsChanges', () => {
  test('ignores transient differences', () => {
    const cleanSnapshot = buildEditableSettingsSnapshot({
      signupEnabled: true,
      guestAccess: false,
      libraries: [{ id: 'library-a', name: 'Main', path: '/srv/music', enabled: true }],
      federation: { enabled: false, role: '', hub_addr: '', listen: '', token: '', peer_id: '' },
    });

    expect(hasEditableSettingsChanges(cleanSnapshot, {
      signupEnabled: true,
      guestAccess: false,
      libraries: [{ id: 'library-a', name: 'Main', path: '/srv/music', enabled: true, last_scan_at: 'changed' }],
      federation: { enabled: false, role: '', hub_addr: '', listen: '', token: '', peer_id: '', token_configured: false, restart_required: true },
      invites: [{ id: 1 }],
      users: [{ id: 2 }],
      federationPeers: [{ peer_id: 'peer-a' }],
    })).toBe(false);
  });

  test('detects editable library path changes', () => {
    const cleanSnapshot = buildEditableSettingsSnapshot({
      signupEnabled: true,
      guestAccess: false,
      libraries: [{ id: 'library-a', name: 'Main', path: '/srv/music', enabled: true }],
      federation: { enabled: false, role: '', hub_addr: '', listen: '', token: '', peer_id: '' },
    });

    expect(hasEditableSettingsChanges(cleanSnapshot, {
      signupEnabled: true,
      guestAccess: false,
      libraries: [{ id: 'library-a', name: 'Main', path: '/srv/changed', enabled: true }],
      federation: { enabled: false, role: '', hub_addr: '', listen: '', token: '', peer_id: '' },
    })).toBe(true);
  });
});

describe('beforeUnloadIfDirty', () => {
  test('does nothing when settings are clean', () => {
    const event = { preventDefault: vi.fn(), returnValue: undefined };

    beforeUnloadIfDirty(false, event);

    expect(event.preventDefault).not.toHaveBeenCalled();
    expect(event.returnValue).toBeUndefined();
  });

  test('requests a native prompt when settings are dirty', () => {
    const event = { preventDefault: vi.fn(), returnValue: undefined };

    beforeUnloadIfDirty(true, event);

    expect(event.preventDefault).toHaveBeenCalledOnce();
    expect(event.returnValue).toBe('');
  });
});

describe('loadPathBrowserLocation', () => {
  test('returns a normalized successful location', async () => {
    const listLibraryPaths = vi.fn(async () => ({
      path: '/srv/music',
      parent: '/srv',
      directories: [{ name: 'Albums', path: '/srv/music/Albums' }],
    }));

    await expect(loadPathBrowserLocation(listLibraryPaths, '/srv/music', true)).resolves.toEqual({
      path: '/srv/music',
      parent: '/srv',
      directories: [{ name: 'Albums', path: '/srv/music/Albums' }],
      error: '',
      requestedError: '',
    });
    expect(listLibraryPaths).toHaveBeenCalledWith('/srv/music');
  });

  test('defaults optional location fields when successful response omits them', async () => {
    const listLibraryPaths = vi.fn(async () => ({ path: '/srv/music' }));

    await expect(loadPathBrowserLocation(listLibraryPaths, '/srv/music', true)).resolves.toEqual({
      path: '/srv/music',
      parent: '',
      directories: [],
      error: '',
      requestedError: '',
    });
    expect(listLibraryPaths).toHaveBeenCalledWith('/srv/music');
  });

  test('retries the default start and preserves the requested error when fallback is enabled', async () => {
    const listLibraryPaths = vi
      .fn()
      .mockRejectedValueOnce(new Error('path is not a directory'))
      .mockResolvedValueOnce({ path: '/srv', parent: '/', directories: [] });

    await expect(loadPathBrowserLocation(listLibraryPaths, '/bad/file', true)).resolves.toEqual({
      path: '/srv',
      parent: '/',
      directories: [],
      error: '',
      requestedError: 'path is not a directory',
    });
    expect(listLibraryPaths).toHaveBeenNthCalledWith(1, '/bad/file');
    expect(listLibraryPaths).toHaveBeenNthCalledWith(2);
  });

  test('returns an error without retry when fallback is disabled', async () => {
    const listLibraryPaths = vi.fn(async () => {
      throw new Error('permission denied');
    });

    await expect(loadPathBrowserLocation(listLibraryPaths, '/root', false)).resolves.toEqual({
      path: '',
      parent: '',
      directories: [],
      error: 'permission denied',
      requestedError: '',
    });
    expect(listLibraryPaths).toHaveBeenCalledOnce();
    expect(listLibraryPaths).toHaveBeenCalledWith('/root');
  });
});
