// createPager accumulates server pages for one browse list. fetchPage(search,
// offset, limit) resolves to { items, total }. The pager is plain (non-reactive)
// so it can be unit-tested; the store mirrors its snapshot into runed state after
// each call.
//
// A monotonic sequence guards against out-of-order responses: when reset() is
// called with a new search while an earlier fetch is still in flight, the stale
// response is dropped instead of appending to (or wedging) the newer query.
export function createPager(fetchPage, pageSize = 100) {
  let items = [];
  let total = 0;
  let loading = false;
  let done = false;
  let seq = 0;

  // load fetches one page for `mySeq`; returns whether it appended rows. A
  // response whose seq is stale (a newer reset happened) is discarded.
  async function load(mySeq) {
    loading = true;
    try {
      const res = await fetchPage(currentSearch, items.length, pageSize);
      if (mySeq !== seq) return false; // superseded by a newer reset
      const page = Array.isArray(res?.items) ? res.items : [];
      const t = Number(res?.total);
      items = items.concat(page);
      total = Number.isFinite(t) ? t : items.length;
      done = items.length >= total || page.length === 0;
      return page.length > 0;
    } finally {
      if (mySeq === seq) loading = false;
    }
  }

  let currentSearch = '';

  return {
    get items() { return items; },
    get total() { return total; },
    get loading() { return loading; },
    get done() { return done; },

    // reset starts a fresh search from page 0, abandoning any in-flight load.
    reset(search) {
      currentSearch = search;
      items = [];
      total = 0;
      done = false;
      return load(++seq);
    },

    // loadMore fetches the next page unless one is in flight or we've reached the
    // total. Resolves to true when rows were appended.
    loadMore() {
      if (loading || done) return Promise.resolve(false);
      return load(seq);
    },
  };
}
