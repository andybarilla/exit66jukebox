import { describe, it, expect } from 'vitest';
import { columnsForWidth } from './grid.js';

// Mirrors CSS `repeat(auto-fill, minmax(minCol, 1fr))` with a fixed gap:
// the most columns C such that C*minCol + (C-1)*gap <= width.
describe('columnsForWidth', () => {
  it('fits as many minCol columns as the width allows', () => {
    // 150*4 + 14*3 = 642 <= 660; a 5th needs 800.
    expect(columnsForWidth(660, 150, 14)).toBe(4);
    expect(columnsForWidth(806, 150, 14)).toBe(5);
  });

  it('returns 1 just below the two-column threshold', () => {
    // two columns need 150+14+150 = 314.
    expect(columnsForWidth(313, 150, 14)).toBe(1);
    expect(columnsForWidth(314, 150, 14)).toBe(2);
  });

  it('never returns less than 1, even at zero/unmeasured width', () => {
    expect(columnsForWidth(0, 150, 14)).toBe(1);
    expect(columnsForWidth(100, 150, 14)).toBe(1);
  });

  it('ignores the gap when it is zero', () => {
    expect(columnsForWidth(450, 150, 0)).toBe(3);
    expect(columnsForWidth(449, 150, 0)).toBe(2);
  });
});
