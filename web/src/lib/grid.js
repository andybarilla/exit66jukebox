// Number of columns a `repeat(auto-fill, minmax(minCol, 1fr))` grid with the
// given gap would show at `width`: the largest C with C*minCol + (C-1)*gap <= width.
export function columnsForWidth(width, minCol, gap) {
  return Math.max(1, Math.floor((width + gap) / (minCol + gap)));
}
