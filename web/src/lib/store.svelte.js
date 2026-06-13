import {
  listTracks, listAlbums, listArtists, albumTracks, getQueue, requestTo, removeRequest,
  setShuffle, subscribeEvents, coverURL, albumCoverURL, HOUSE,
  discoverGenres, discoverRediscover, discoverRecent,
  getStation, startStation as apiStartStation, stopStation as apiStopStation,
  scanStatus,
} from './api.js';
import { gradientFor } from './format.js';
import { createPager } from './pager.js';

const PAGE_SIZE = 100;

export function createStore() {
  let tab = $state('albums');
  let query = $state('');
  let stream = $state('house');           // 'house' (shared) | 'me' (personal)
  let isPhone = $state(false);
  let lineupOpen = $state(false);
  let detailAlbum = $state(null);          // {id,name,artistName,tracks:[...]} | null
  let shuffle = $state({ house: false, me: false }); // per-stream, mirrors backend
  let displayName = $state(localStorage.getItem('e66.name') || 'You');
  let toasts = $state([]);

  // Browse state per tab. Slot codes/tones/names are carried by the backend on
  // each row (#53), so the client no longer holds or groups the whole library.
  // Each tab fetches windowed pages on demand and appends as the user scrolls.
  let view = $state({
    albums: { items: [], total: 0, loading: false },
    artists: { items: [], total: 0, loading: false },
    tracks: { items: [], total: 0, loading: false },
  });
  let scan = $state(null);      // /api/scan snapshot {running,added,...} | null

  // per-stream live state
  let nowPlaying = $state({ house: null, me: null });
  let progress = $state({ house: 0, me: 0 });   // seconds
  let queues = $state({ house: [], me: [] });
  let listeners = $state({ house: 0, me: 1 });

  // discover state
  let discoverGenreList = $state([]);     // [{genre, count}]
  let discoverSelectedGenre = $state('');
  let discoverRediscoverRows = $state([]);
  let discoverRecentRows = $state([]);
  let discoverStation = $state(null);     // {stream_id, genre, threshold, batch} | null

  let _uid = 0;
  let _esHouse = null;

  const pagers = {
    albums: createPager((q, off, lim) => listAlbums(q, off, lim), PAGE_SIZE),
    artists: createPager((q, off, lim) => listArtists(q, off, lim), PAGE_SIZE),
    tracks: createPager((q, off, lim) => listTracks(q, off, lim), PAGE_SIZE),
  };
  // The search a tab's loaded items currently reflect, so switching tabs only
  // refetches when the query changed since that tab last loaded.
  const loadedQuery = { albums: null, artists: null, tracks: null };
  let _searchTimer = null;

  function isBrowseTab(t) { return t === 'albums' || t === 'artists' || t === 'tracks'; }

  // sync copies a pager's snapshot into reactive state so getters update.
  function sync(t) {
    const p = pagers[t];
    view[t] = { items: p.items, total: p.total, loading: p.loading };
  }

  // reloadActive resets the active browse tab to page 0 for the current query.
  async function reloadActive() {
    clearTimeout(_searchTimer); _searchTimer = null;
    if (!isBrowseTab(tab)) return;
    const t = tab;
    const p = pagers[t];
    sync(t);                       // reflect loading=true immediately
    const start = p.reset(query);
    sync(t);
    await start;
    loadedQuery[t] = query;
    sync(t);
  }

  // ensureLoaded loads the active tab if its data does not match the current
  // query yet (first visit or a search happened while it was inactive).
  function ensureLoaded() {
    if (tab === 'discover') return;
    if (loadedQuery[tab] !== query) reloadActive();
    else sync(tab);
  }

  function scheduleSearch() {
    clearTimeout(_searchTimer);
    _searchTimer = setTimeout(reloadActive, 250);
  }

  // loadMore appends the next page of the active tab; returns whether it grew
  // (so the list component's viewport-fill loop knows when to stop).
  async function loadMore() {
    if (!isBrowseTab(tab)) return false;
    const t = tab;
    const grew = await pagers[t].loadMore();
    sync(t);
    return grew;
  }

  // ---- display mappers: backend rows -> the shape the components expect ----
  function mapTrack(t) {
    return {
      id: t.id, title: t.title, duration: t.duration || 0,
      code: t.code || '··', tone: t.tone || 'magenta',
      artistName: t.artist_name || 'Unknown', albumName: t.album_name || '',
      cover: coverURL(t.id), gradient: gradientFor(t.id),
    };
  }
  function mapAlbum(a) {
    return {
      id: a.id, name: a.name, artistName: a.artist_name || 'Unknown',
      letter: a.letter, tone: a.tone || 'magenta',
      meta: `${a.track_count} track${a.track_count === 1 ? '' : 's'}`,
      initial: (a.name?.[0] || '?').toUpperCase(),
      cover: albumCoverURL(a.id), gradient: gradientFor(a.id),
    };
  }
  function mapArtist(a) {
    return {
      id: a.id, name: a.name,
      albumCount: a.album_count, trackCount: a.track_count,
      meta: `${a.album_count} album${a.album_count === 1 ? '' : 's'} · ${a.track_count} track${a.track_count === 1 ? '' : 's'}`,
      initial: (a.name?.[0] || '?').toUpperCase(),
      gradient: gradientFor(1000 + a.id),
    };
  }

  // Poll /api/scan while a scan is in flight. When it finishes (running flips
  // true→false) reload the active tab so new tracks/counts appear without a
  // manual refresh. Idle when no scan is running, so it's cheap to start.
  let _scanTimer = null;
  let _scanWasRunning = false;
  async function pollScan() {
    let snap = null;
    try { snap = await scanStatus(); } catch { snap = null; }
    scan = snap;
    const running = !!(snap && snap.running);
    if (_scanWasRunning && !running) {
      // Force the active tab to refetch (its loadedQuery is stale post-scan).
      loadedQuery.albums = loadedQuery.artists = loadedQuery.tracks = null;
      ensureLoaded();
    }
    _scanWasRunning = running;
    _scanTimer = running ? setTimeout(pollScan, 1500) : null;
  }
  function startScanPolling() {
    if (_scanTimer) return;
    pollScan();
  }

  async function refreshQueue(s) {
    const r = await getQueue(s);
    queues[s] = (r.queue || []).map(normalizeQueued);
    if (typeof r.listeners === 'number') listeners[s] = r.listeners;
  }

  // Queue items are {track:{...enriched...}, requested_by} for /me; the house
  // SSE/queue may send a bare enriched track. Both carry backend code/tone/names.
  function normalizeQueued(item) {
    const t = item.track || item;
    return {
      uid: ++_uid, id: t.id, title: t.title,
      artistName: t.artist_name || 'Unknown', albumName: t.album_name || '',
      code: t.code || '··', tone: t.tone || 'magenta', requester: item.requested_by || '',
      cover: coverURL(t.id), gradient: gradientFor(t.id),
    };
  }

  async function loadDiscoverLists(genre) {
    const [rd, rc] = await Promise.all([
      discoverRediscover(genre), discoverRecent(genre),
    ]);
    discoverRediscoverRows = (Array.isArray(rd) ? rd : []).map(mapTrack);
    discoverRecentRows = (Array.isArray(rc) ? rc : []).map(mapTrack);
  }

  async function loadStation() {
    const r = await getStation(stream);
    discoverStation = r?.genre ? r : null;
  }

  function pushToast(tone, title, message) {
    const id = ++_uid;
    toasts = [...toasts, { id, tone, title, message }];
    setTimeout(() => { toasts = toasts.filter((t) => t.id !== id); }, 3400);
  }

  return {
    // primitive state accessors
    get tab() { return tab; },
    set tab(v) { tab = v; ensureLoaded(); },
    get query() { return query; },
    set query(v) { query = v; scheduleSearch(); },
    get stream() { return stream; },
    get isPhone() { return isPhone; }, set isPhone(v) { isPhone = v; },
    get lineupOpen() { return lineupOpen; }, set lineupOpen(v) { lineupOpen = v; },
    get shuffle() { return shuffle[stream]; },
    get displayName() { return displayName; },
    set displayName(v) { displayName = v; localStorage.setItem('e66.name', v); },
    get toasts() { return toasts; },

    get scan() { return scan; },

    // Personal is always "just you": the `me` stream has no broadcast hub, so
    // the backend reports 0 listeners for it. Never let that 0 surface here.
    get listeners() { return stream === 'me' ? 1 : (listeners.house || 0); },
    get queue() { return queues[stream]; },
    get nowPlaying() { return nowPlaying[stream]; },
    get progress() { return progress[stream]; },

    // browse views (already filtered server-side; just map to display shape)
    get albumCards() { return view.albums.items.map(mapAlbum); },
    get artistRows() { return view.artists.items.map(mapArtist); },
    get trackRows() { return view.tracks.items.map(mapTrack); },

    // currentCount is the server total ("N in the crate"); loading distinguishes
    // "still fetching" from "genuinely empty" so the empty state doesn't flash.
    get currentCount() {
      if (tab === 'discover') return discoverRediscoverRows.length + discoverRecentRows.length;
      return isBrowseTab(tab) ? view[tab].total : 0;
    },
    get loading() { return isBrowseTab(tab) ? view[tab].loading : false; },

    loadMore() { return loadMore(); },

    // discover accessors
    get discoverGenres() { return discoverGenreList; },
    get discoverSelectedGenre() { return discoverSelectedGenre; },
    get discoverRediscover() { return discoverRediscoverRows; },
    get discoverRecent() { return discoverRecentRows; },
    get discoverStation() { return discoverStation; },
    get detailAlbum() { return detailAlbum; },

    // ----- actions -----
    async init() {
      // Seed scan state before the initial load so a scan that finishes *during*
      // the first fetch is still seen as a true→false transition by the first
      // poll, triggering the reload that pulls in the last tracks.
      const s0 = await scanStatus().catch(() => null);
      scan = s0;
      _scanWasRunning = !!(s0 && s0.running);
      await reloadActive();        // first page of the active (albums) tab
      startScanPolling();
      await Promise.all([refreshQueue('house'), refreshQueue('me')]);
      discoverGenres().then((g) => { discoverGenreList = Array.isArray(g) ? g : []; }).catch(() => {});
      _esHouse = subscribeEvents(HOUSE, (e) => {
        if (e.type === 'now-playing') {
          nowPlaying.house = e.data ? normalizeNP(e.data) : null;
          progress.house = 0;
        } else if (e.type === 'queue-changed') {
          refreshQueue('house');
        }
      });
    },
    teardown() {
      if (_esHouse) { _esHouse(); _esHouse = null; }
      if (_scanTimer) { clearTimeout(_scanTimer); _scanTimer = null; }
      if (_searchTimer) { clearTimeout(_searchTimer); _searchTimer = null; }
    },

    setStream(s) {
      if (s === stream) return;
      stream = s;
      pushToast('cyan', 'Stream', s === 'house'
        ? 'Tuned in to the house stream — everyone hears this.'
        : 'Switched to your personal stream.');
    },
    toggleStream() { this.setStream(stream === 'house' ? 'me' : 'house'); },

    async toggleShuffle(v) {
      shuffle[stream] = typeof v === 'boolean' ? v : !shuffle[stream];
      await setShuffle(stream, shuffle[stream]);
    },

    refreshQueue(s) { return refreshQueue(s); },

    // Open the album dialog, fetching its tracks on demand. A sequence guard
    // drops a stale response if a different album is opened before it resolves.
    async openAlbum(card) {
      const id = card.id;
      detailAlbum = { id, name: card.name, artistName: card.artistName, tracks: [] };
      const rows = await albumTracks(id);
      if (detailAlbum && detailAlbum.id === id) {
        detailAlbum = { ...detailAlbum, tracks: rows.map(mapTrack) };
      }
    },
    closeAlbum() { detailAlbum = null; },
    // openArtist jumps to the Tracks tab filtered by the artist name. Set state
    // directly and reload immediately (no debounce) for a snappy jump.
    openArtist(a) { tab = 'tracks'; query = a.name; reloadActive(); },
    openLineup() { lineupOpen = true; },
    closeLineup() { lineupOpen = false; },
    onResize() { const ph = window.innerWidth < 760; isPhone = ph; if (!ph) lineupOpen = false; },

    // The backend rejects duplicates / recently-played tracks. Branch on the
    // real `queued` count so we never claim a success that didn't happen.
    async requestTrack(t) {
      const r = await requestTo(stream, t.id, { kind: 'track', by: displayName });
      await refreshQueue(stream);
      if (r.queued > 0) pushToast('success', 'Queued', `${t.title} joined the lineup.`);
      else pushToast('amber', 'Not queued', r.message || 'That track is already in the lineup.');
    },
    async requestAlbum(al) {
      const r = await requestTo(stream, al.id, { kind: 'album', by: displayName });
      await refreshQueue(stream);
      if (r.queued > 0) pushToast('success', 'Queued', `${al.name} — ${r.queued} track${r.queued === 1 ? '' : 's'} on the way.`);
      else pushToast('amber', 'Nothing new', `${al.name} is already in the lineup.`);
    },
    async requestArtist(a) {
      const r = await requestTo(stream, a.id, { kind: 'artist', by: displayName });
      await refreshQueue(stream);
      if (r.queued > 0) pushToast('success', 'Queued', `${a.name} — ${r.queued} track${r.queued === 1 ? '' : 's'} on the way.`);
      else pushToast('amber', 'Nothing new', `${a.name} is already in the lineup.`);
    },
    async removeFromQueue(item) {
      await removeRequest(stream, item.id);
      await refreshQueue(stream);
    },
    // discover actions
    async loadDiscover() {
      const genres = await discoverGenres();
      discoverGenreList = Array.isArray(genres) ? genres : [];
      await Promise.all([loadDiscoverLists(discoverSelectedGenre), loadStation()]);
    },
    async setDiscoverGenre(genre) {
      discoverSelectedGenre = genre;
      await loadDiscoverLists(genre);
    },
    async startStation(genre) {
      await apiStartStation(stream, genre);
      await Promise.all([loadStation(), refreshQueue(stream)]);
      pushToast('cyan', 'Station started', `${genre} radio is now filling the queue.`);
    },
    async stopStation() {
      await apiStopStation(stream);
      discoverStation = null;
      await refreshQueue(stream);
      pushToast('amber', 'Station stopped', 'Genre radio stopped.');
    },

    dismissToast(id) { toasts = toasts.filter((t) => t.id !== id); },

    // progress tick (called once/sec by App for the active stream's now-playing)
    tick(seconds) { if (nowPlaying[stream]) progress[stream] = seconds; },
    setProgress(s, sec) { progress[s] = sec; },
    setNowPlaying(s, np) { nowPlaying[s] = np; },

    // npMeta maps an enriched track (from /next) to now-playing display fields.
    npMeta(t) {
      return {
        code: t.code || '··', artistName: t.artist_name || 'Unknown',
        albumName: t.album_name || '', tone: t.tone || 'magenta',
        cover: coverURL(t.id), gradient: gradientFor(t.id),
      };
    },
  };

  // normalizeNP maps an enriched now-playing track (SSE/`/next`) to display shape.
  function normalizeNP(t) {
    return {
      id: t.id, title: t.title, code: t.code || '··',
      artistName: t.artist_name || 'Unknown', albumName: t.album_name || '',
      tone: t.tone || 'magenta', duration: t.duration || 0,
      cover: coverURL(t.id), gradient: gradientFor(t.id),
    };
  }
}
