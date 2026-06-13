import { tick } from 'svelte';

// makeInfiniteScroll wires a virtua Virtualizer handle to a loadMore callback:
// it loads the next page when the user scrolls near the end, and tops the list
// up until the viewport is full so a short first page can't stall scrolling
// (when content doesn't overflow, virtua's onscroll never fires again).
//
// getHandle() returns the Virtualizer handle (or undefined before mount);
// getLoadMore() returns the async loadMore (resolving to whether it grew).
export function makeInfiniteScroll(getHandle, getLoadMore, threshold = 400) {
  let filling = false;

  const nearEnd = (h) =>
    h.getScrollOffset() + h.getViewportSize() >= h.getScrollSize() - threshold;
  const notScrollable = (h) => h.getScrollSize() <= h.getViewportSize() + 1;

  async function fill() {
    const load = getLoadMore();
    let h = getHandle();
    if (filling || !load || !h) return;
    filling = true;
    try {
      let guard = 0;
      while (h && notScrollable(h) && guard++ < 100) {
        const grew = await load();
        if (!grew) break;
        await tick();
        h = getHandle();
      }
    } finally {
      filling = false;
    }
  }

  function onScroll() {
    const h = getHandle();
    const load = getLoadMore();
    if (h && load && nearEnd(h)) load();
  }

  return { fill, onScroll };
}
