import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const read = (rel) => readFileSync(fileURLToPath(new URL(rel, import.meta.url)), 'utf8');

describe('TrackRow add-button feedback (#36)', () => {
  const src = read('./TrackRow.svelte');

  it('imports a check icon for the confirmation glyph', () => {
    expect(src).toContain('IconCheck');
  });

  it('tracks a transient justAdded confirmation flag', () => {
    expect(src).toMatch(/let\s+justAdded\s*=\s*\$state\(/);
  });

  it('sets justAdded on click and reverts via a ~600ms timeout', () => {
    expect(src).toMatch(/justAdded\s*=\s*true/);
    expect(src).toMatch(/setTimeout\([\s\S]*?,\s*600\s*\)/);
    expect(src).toMatch(/justAdded\s*=\s*false/);
  });

  it('still stops propagation so row navigation is unaffected', () => {
    expect(src).toContain('e.stopPropagation()');
  });

  it('still calls onAdd for the authoritative toast', () => {
    expect(src).toContain('onAdd()');
  });

  it('swaps the glyph between IconPlus and IconCheck on the flag', () => {
    expect(src).toMatch(/justAdded[\s\S]*IconCheck/);
  });
});

describe('IconCheck component', () => {
  const src = read('./icons/IconCheck.svelte');

  it('renders an svg using currentColor stroke', () => {
    expect(src).toContain('<svg');
    expect(src).toContain('currentColor');
  });

  it('accepts a size prop like the other icons', () => {
    expect(src).toMatch(/size\s*=\s*\d+/);
  });
});
