import { describe, it, expect } from 'vitest';
import { createPager } from './pager.js';

// A controllable fetcher: each call returns a promise we resolve by hand, so we
// can interleave responses and assert ordering/race behavior.
function deferredFetcher() {
  const calls = [];
  const fn = (search, offset, limit) => {
    let resolve;
    const promise = new Promise((r) => { resolve = r; });
    calls.push({ search, offset, limit, resolve });
    return promise;
  };
  return { fn, calls };
}

function page(items, total) { return { items, total }; }

describe('createPager', () => {
  it('reset loads the first page and records the total', async () => {
    const { fn, calls } = deferredFetcher();
    const p = createPager(fn, 2);
    const done = p.reset('');
    expect(p.loading).toBe(true);
    calls[0].resolve(page([{ id: 1 }, { id: 2 }], 5));
    await done;
    expect(p.items.map((x) => x.id)).toEqual([1, 2]);
    expect(p.total).toBe(5);
    expect(p.loading).toBe(false);
    expect(p.done).toBe(false);
  });

  it('loadMore appends the next page and stops at the total', async () => {
    const { fn, calls } = deferredFetcher();
    const p = createPager(fn, 2);
    const r = p.reset('');
    calls[0].resolve(page([{ id: 1 }, { id: 2 }], 3));
    await r;

    const grew = await loadMoreResolving(p, calls, page([{ id: 3 }], 3));
    expect(grew).toBe(true);
    expect(p.items.map((x) => x.id)).toEqual([1, 2, 3]);
    expect(p.done).toBe(true);

    // Already at total: loadMore is a no-op that fetches nothing and returns false.
    const before = calls.length;
    const grew2 = await p.loadMore();
    expect(grew2).toBe(false);
    expect(calls.length).toBe(before);
  });

  it('clears loading even when a stale reset response arrives, and never appends it', async () => {
    const { fn, calls } = deferredFetcher();
    const p = createPager(fn, 2);
    const first = p.reset('a');   // call 0
    const second = p.reset('b');  // call 1 — supersedes 'a'

    // Resolve the superseding search first, then the stale one.
    calls[1].resolve(page([{ id: 9 }], 1));
    await second;
    calls[0].resolve(page([{ id: 1 }, { id: 2 }], 5));
    await first;

    expect(p.items.map((x) => x.id)).toEqual([9]); // stale 'a' did not append
    expect(p.total).toBe(1);
    expect(p.loading).toBe(false);                  // not wedged
  });

  it('ignores a concurrent loadMore while a load is already in flight', async () => {
    const { fn, calls } = deferredFetcher();
    const p = createPager(fn, 2);
    const r = p.reset('');
    calls[0].resolve(page([{ id: 1 }, { id: 2 }], 10));
    await r;

    const a = p.loadMore();       // starts fetch (call 1)
    const b = p.loadMore();       // should no-op: a load is in flight
    expect(calls.length).toBe(2);
    calls[1].resolve(page([{ id: 3 }, { id: 4 }], 10));
    await Promise.all([a, b]);
    expect(p.items.map((x) => x.id)).toEqual([1, 2, 3, 4]);
  });

  it('falls back to items.length as total when the fetcher omits it, so paging terminates', async () => {
    const { fn, calls } = deferredFetcher();
    const p = createPager(fn, 2);
    const r = p.reset('');
    calls[0].resolve(page([{ id: 1 }], undefined));
    await r;
    expect(p.total).toBe(1);
    expect(p.done).toBe(true);
  });
});

// Helper: drive a loadMore() whose fetch we resolve with the given page.
async function loadMoreResolving(p, calls, result) {
  const before = calls.length;
  const promise = p.loadMore();
  calls[before].resolve(result);
  return promise;
}
