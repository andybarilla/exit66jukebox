import { describe, it, expect, beforeEach, vi } from 'vitest';
import { createStore } from './store.svelte.js';

beforeEach(() => {
  vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {} });
});

describe('cast-active state', () => {
  it('defaults to false and is toggled by setCastActive', () => {
    const s = createStore();
    expect(s.castActive).toBe(false);
    s.setCastActive(true);
    expect(s.castActive).toBe(true);
    s.setCastActive(false);
    expect(s.castActive).toBe(false);
  });

  it('exposes muteLocalOnCast, defaulting true before config loads', () => {
    const s = createStore();
    expect(s.muteLocalOnCast).toBe(true);
  });
});
