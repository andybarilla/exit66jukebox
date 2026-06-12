import { describe, it, expect } from 'vitest';
import { scanIndicator } from './scan.js';

describe('scanIndicator', () => {
  it('is hidden when there is no status', () => {
    expect(scanIndicator(null).visible).toBe(false);
    expect(scanIndicator(undefined).visible).toBe(false);
  });

  it('is hidden when a scan is not running', () => {
    expect(scanIndicator({ running: false, added: 10 }).visible).toBe(false);
  });

  it('is visible with a count of indexed tracks while running', () => {
    const ind = scanIndicator({ running: true, added: 7, updated: 3, skipped: 0, failed: 0 });
    expect(ind.visible).toBe(true);
    expect(ind.text).toContain('10'); // added + updated
  });

  it('counts only newly indexed tracks (added + updated), not skipped', () => {
    const ind = scanIndicator({ running: true, added: 2, updated: 0, skipped: 500, failed: 0 });
    expect(ind.text).toContain('2');
    expect(ind.text).not.toContain('500');
  });
});
