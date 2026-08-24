import {
  listTracks, listAlbums, listArtists, albumTracks, getQueue, requestTo, removeRequest,
  setShuffle, subscribeEvents, coverURL, albumCoverURL, HOUSE, PERSONAL,
  listStreams, createStream as apiCreateStream, renameStream as apiRenameStream,
  deleteStream as apiDeleteStream,
  discoverGenres, discoverRediscover, discoverRecent, discoverRecommended,
  getStation, startStation as apiStartStation, stopStation as apiStopStation,
  scanStatus, getConfig,
} from './api.js';
import { fetchMe, logout as apiLogout } from './auth.js';
import { gradientFor } from './format.js';
import { createPager } from './pager.js';

const PAGE_SIZE = 100;

export function createStore() {
  let tab = $state('albums');
  let query = $state('');
  // stream is the id currently tuned in: any shared stream, or PERSONAL for the
  // pinned personal one. PERSONAL is an alias the server resolves per caller,
  // and it is only offered when config.personalStream says one exists.
  // sharedStreams mirrors GET /api/streams.
  let stream = $state(HOUSE);
  let sharedStreams = $state([{ id: HOUSE, name: 'House', house: true, listeners: 0 }]);
  let isPhone = $state(false);
  let lineupOpen = $state(false);
  let detailAlbum = $state(null);          // {id,name,artistName,tracks:[...]} | null
  let shuffle = $state({}); // per-stream id -> bool, mirrors backend
  let displayName = $state(localStorage.getItem('e66.name') || 'You');
  let toasts = $state([]);

  // Browse state per tab. Slot codes/tones/names are carried by the backend on
  // each row (#53), so the client no longer holds or groups the whole library.
  // Each tab fetches windowed pages on demand and appends as the user scrolls.
  // Seed loading:true so the first paint (before init's scanStatus round-trip and
  // first fetch resolve) shows an empty list, not a flash of the "No matches"
  // empty state, which is gated on !loading.
  let view = $state({
    albums: { items: [], total: 0, loading: true },
    artists: { items: [], total: 0, loading: true },
    tracks: { items: [], total: 0, loading: true },
  });
  let scan = $state(null);      // /api/scan snapshot {running,added,...} | null

  // Runtime config + cast state. config loads once on init; muteLocalOnCast
  // defaults true so a cast that starts before config resolves still mutes.
  // castActive is lifted out of CastPanel so App can mute the local <audio>.
  let config = $state({
    muteLocalOnCast: true,
    fedPeers: [],
    securityMode: 'full_login',
    guestAccess: false,
    requiresProfile: false,
    requiresLogin: true,
    signupEnabled: false,
    needsBootstrap: false,
    authenticated: false,
    maxSharedStreams: 0,
    // Assume none until /api/config says otherwise, so the Personal control
    // never flashes in on a mode that has no personal stream.
    personalStream: false,
  });
  let castActive = $state(false);

  // Auth state. me is the current user ({id,email,display_name,is_admin}) or null.
  // authChecked is set true after the first /me round-trip so the UI can gate on
  // "have we checked yet" vs "definitely logged out".
  let me = $state(null);
  let authChecked = $state(false);

  // per-stream live state, keyed by stream id (shared ids are opaque, so these
  // are plain maps rather than a fixed house/me pair).
  let nowPlaying = $state({});
  let progress = $state({});   // seconds
  let queues = $state({});
  let listeners = $state({});

  // discover state
  let discoverGenreList = $state([]);     // [{genre, count}]
  let discoverSelectedGenre = $state('');
  let discoverRediscoverRows = $state([]);
  let discoverRecentRows = $state([]);
  let discoverRecommendedRows = $state([]); // external recs mapped to local tracks
  let discoverStation = $state(null);     // {stream_id, genre, threshold, batch} | null

  let _uid = 0;
  let _es = null;       // SSE for the tuned-in shared stream
  let _esStream = null; // which stream _es is attached to
  let _started = false;
  let lastShared = HOUSE; // the shared stream the personal toggle returns to

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
      trackNo: t.track_no || 0,
      code: t.code || '··', tone: t.tone || 'magenta',
      artistName: t.artist_name || 'Unknown', albumName: t.album_name || '',
      links: t.links || [],
      cover: coverURL(t.id), gradient: gradientFor(t.id),
      sourcePeer: t.source_peer || '',
      offline: !!t.source_peer && !config.fedPeers.includes(t.source_peer),
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

  // loadStreams refreshes the shared-stream list, keeping house first so the
  // selector's default is always in the same place. If the stream we are tuned
  // into has disappeared (deleted by an admin elsewhere), fall back to house.
  async function loadStreams() {
    const list = await listStreams().catch(() => null);
    if (!Array.isArray(list) || list.length === 0) return;
    sharedStreams = [...list].sort((a, b) => (b.house === true) - (a.house === true));
    for (const st of sharedStreams) listeners[st.id] = st.listeners || 0;
    if (stream !== PERSONAL && !sharedStreams.some((st) => st.id === stream)) {
      stream = HOUSE;
      lastShared = HOUSE;
      subscribeToStream(HOUSE);
      refreshQueue(HOUSE);
    }
  }

  function unsubscribeStream() {
    if (_es) { _es(); _es = null; _esStream = null; }
  }

  // subscribeToStream points the single SSE connection at the tuned-in shared
  // stream. Only one is open at a time: an idle shared stream is torn down
  // server-side once nobody is attached, and holding a subscription on every
  // stream would keep them all alive.
  function subscribeToStream(s) {
    if (s === PERSONAL) { unsubscribeStream(); return; }
    lastShared = s;
    if (_esStream === s) return;
    unsubscribeStream();
    _esStream = s;
    _es = subscribeEvents(s, (e) => {
      if (e.type === 'now-playing') {
        nowPlaying[s] = e.data ? normalizeNP(e.data) : null;
        progress[s] = 0;
      } else if (e.type === 'queue-changed') {
        refreshQueue(s);
      } else if (e.type === 'stream-closed') {
        // The stream was deleted out from under us; drop its state and go home.
        unsubscribeStream();
        delete queues[s]; delete nowPlaying[s]; delete progress[s]; delete listeners[s];
        pushToast('amber', 'Stream closed', 'That stream was removed.');
        if (stream === s) { stream = HOUSE; lastShared = HOUSE; subscribeToStream(HOUSE); refreshQueue(HOUSE); }
        loadStreams();
      }
    });
  }

  async function refreshQueue(s) {
    const r = await getQueue(s);
    queues[s] = (r.queue || []).map(normalizeQueued);
    if (typeof r.listeners === 'number') listeners[s] = r.listeners;
    // Seed now-playing on first load / after a reconnect so a client tuning in
    // mid-track shows the current track immediately instead of waiting for the
    // next SSE event. Only when local state is still empty, so we never fight a
    // live `now-playing` event or the running progress tick once they take over.
    if (r.now_playing && nowPlaying[s] == null) {
      nowPlaying[s] = normalizeNP(r.now_playing.track);
      progress[s] = r.now_playing.offset_seconds || 0;
    }
  }

  // Queue items are {track:{...enriched...}, requested_by} for the personal
  // stream; a shared stream's SSE/queue may send a bare enriched track. Both
  // carry backend code/tone/names.
  function normalizeQueued(item) {
    const t = item.track || item;
    return {
      uid: ++_uid, id: t.id, title: t.title, albumId: t.album_id,
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

  // Recommendations are externally sourced (not genre-filtered), so they load
  // once with the Discover tab rather than on every genre change.
  async function loadRecommended() {
    const rec = await discoverRecommended();
    discoverRecommendedRows = (Array.isArray(rec) ? rec : []).map(mapTrack);
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
    get shuffle() { return shuffle[stream] === true; },
    get displayName() { return displayName; },
    set displayName(v) { displayName = v; localStorage.setItem('e66.name', v); },
    get toasts() { return toasts; },
    pushToast,

    get scan() { return scan; },
    get config() { return config; },
    get muteLocalOnCast() { return config.muteLocalOnCast; },
    get fedPeers() { return config.fedPeers; },
    get castActive() { return castActive; },
    setCastActive(v) { castActive = v; },

    get me() { return me; },
    get authChecked() { return authChecked; },
    get isAdmin() { return me?.is_admin === true; },
    setMe(u) { me = u; },
    async signOut() { await apiLogout(); me = null; },

    // Personal is always "just you": it has no broadcast hub, so the backend
    // reports 0 listeners for it. Never let that 0 surface here.
    get listeners() { return stream === PERSONAL ? 1 : (listeners[stream] || 0); },

    // The shared streams a listener can tune into, house first. The personal
    // stream is deliberately not in here — the client pins it separately.
    // Listener counts come from the live per-stream state rather than the
    // list response, which is only refetched when the set of streams changes.
    get sharedStreams() {
      return sharedStreams.map((st) => ({ ...st, listeners: listeners[st.id] ?? st.listeners ?? 0 }));
    },
    get isSharedStream() { return stream !== PERSONAL; },
    // atSharedStreamCap mirrors the server's store-side limit, served via
    // /api/config so the constant lives in one place.
    get atSharedStreamCap() {
      return config.maxSharedStreams > 0 && sharedStreams.length >= config.maxSharedStreams;
    },
    // canManageStreams matches the server gate on create/rename/delete.
    get canManageStreams() { return this.isAdmin || config.securityMode === 'open'; },
    get streamName() {
      if (stream === PERSONAL) return 'Personal';
      return sharedStreams.find((st) => st.id === stream)?.name || 'Stream';
    },
    get queue() { return queues[stream] || []; },
    get nowPlaying() { return nowPlaying[stream] || null; },
    get progress() { return progress[stream] || 0; },

    // browse views (already filtered server-side; just map to display shape)
    get albumCards() { return view.albums.items.map(mapAlbum); },
    get artistRows() { return view.artists.items.map(mapArtist); },
    get trackRows() { return view.tracks.items.map(mapTrack); },

    // currentCount is the server total ("N in the crate"); loading distinguishes
    // "still fetching" from "genuinely empty" so the empty state doesn't flash.
    get currentCount() {
      if (tab === 'discover') return discoverRecommendedRows.length + discoverRediscoverRows.length + discoverRecentRows.length;
      return isBrowseTab(tab) ? view[tab].total : 0;
    },
    get loading() { return isBrowseTab(tab) ? view[tab].loading : false; },

    loadMore() { return loadMore(); },

    // discover accessors
    get discoverGenres() { return discoverGenreList; },
    get discoverSelectedGenre() { return discoverSelectedGenre; },
    get discoverRediscover() { return discoverRediscoverRows; },
    get discoverRecent() { return discoverRecentRows; },
    get discoverRecommended() { return discoverRecommendedRows; },
    get discoverStation() { return discoverStation; },
    get detailAlbum() { return detailAlbum; },

    // ----- actions -----
    // bootstrap loads only the lightweight auth/config state needed to decide
    // whether the user has access. Heavy data loads live in start(), gated on
    // access being granted, so they never 401 for a logged-out guest.
    async bootstrap() {
      await getConfig()
        .then((c) => {
          if (!c) return;
          config = {
            muteLocalOnCast: typeof c.mute_local_on_cast === 'boolean' ? c.mute_local_on_cast : config.muteLocalOnCast,
            fedPeers: Array.isArray(c.fed_peers) ? c.fed_peers : [],
            securityMode: c.security_mode || 'full_login',
            guestAccess: !!c.guest_access,
            requiresProfile: !!c.requires_profile,
            requiresLogin: !!c.requires_login,
            signupEnabled: !!c.signup_enabled,
            needsBootstrap: !!c.needs_bootstrap,
            authenticated: !!c.authenticated,
            maxSharedStreams: Number(c.max_shared_streams) || 0,
            personalStream: !!c.personal_stream,
          };
        })
        .catch(() => {});
      fetchMe().then((u) => { me = u; }).catch(() => {}).finally(() => { authChecked = true; });
    },
    // start runs the heavy loads once access is granted. Guarded so calling it
    // again (e.g. after login) is a no-op.
    async start() {
      if (_started) return;
      _started = true;
      // Seed scan state before the initial load so a scan that finishes *during*
      // the first fetch is still seen as a true→false transition by the first
      // poll, triggering the reload that pulls in the last tracks.
      const s0 = await scanStatus().catch(() => null);
      scan = s0;
      _scanWasRunning = !!(s0 && s0.running);
      await reloadActive();        // first page of the active (albums) tab
      startScanPolling();
      await loadStreams();
      // The personal queue is only fetched where there is one: in the open
      // modes that route 404s, and prefetching it would just log an error.
      const queuesToLoad = [refreshQueue(stream)];
      if (config.personalStream && stream !== PERSONAL) queuesToLoad.push(refreshQueue(PERSONAL));
      await Promise.all(queuesToLoad);
      discoverGenres().then((g) => { discoverGenreList = Array.isArray(g) ? g : []; }).catch(() => {});
      subscribeToStream(stream);
    },
    async init() { await this.bootstrap(); await this.start(); },
    teardown() {
      unsubscribeStream();
      if (_scanTimer) { clearTimeout(_scanTimer); _scanTimer = null; }
      if (_searchTimer) { clearTimeout(_searchTimer); _searchTimer = null; }
    },

    setStream(s) {
      if (s === stream) return;
      stream = s;
      subscribeToStream(s);
      refreshQueue(s);
      if (s === PERSONAL) {
        pushToast('cyan', 'Stream', 'Switched to your personal stream.');
      } else {
        const name = sharedStreams.find((st) => st.id === s)?.name || 'this stream';
        pushToast('cyan', 'Stream', `Tuned in to ${name} — everyone on it hears this.`);
      }
    },
    // hasPersonalStream mirrors the server: false in the two open security
    // modes, where a request may carry no user and so there is nobody to key a
    // personal stream on. The Personal control is hidden rather than offered
    // and refused.
    get hasPersonalStream() { return config.personalStream; },
    // toggleStream flips between the personal stream and the last shared one
    // (house when there wasn't one), which is what the compact chip does. A
    // no-op where there is no personal stream to flip to.
    toggleStream() {
      if (stream !== PERSONAL && !config.personalStream) return;
      this.setStream(stream === PERSONAL ? lastShared : PERSONAL);
    },

    loadStreams() { return loadStreams(); },

    async createStream(name) {
      const r = await apiCreateStream(name);
      if (!r.ok) { pushToast('amber', 'Not created', r.error); return null; }
      await loadStreams();
      pushToast('success', 'Stream created', `${name} is ready.`);
      return r.stream?.id || null;
    },
    async renameStream(id, name) {
      const r = await apiRenameStream(id, name);
      if (!r.ok) { pushToast('amber', 'Not renamed', r.error); return false; }
      await loadStreams();
      return true;
    },
    async deleteStream(id) {
      const r = await apiDeleteStream(id);
      if (!r.ok) { pushToast('amber', 'Not deleted', r.error); return false; }
      // Tuned into the stream that just went away: fall back to house.
      if (stream === id) this.setStream(HOUSE);
      await loadStreams();
      pushToast('success', 'Stream deleted', 'It is gone and its queue with it.');
      return true;
    },

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
      await Promise.all([loadDiscoverLists(discoverSelectedGenre), loadRecommended(), loadStation()]);
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
        code: t.code || '··', albumId: t.album_id, artistName: t.artist_name || 'Unknown',
        albumName: t.album_name || '', tone: t.tone || 'magenta',
        cover: coverURL(t.id), gradient: gradientFor(t.id),
      };
    },
  };

  // normalizeNP maps an enriched now-playing track (SSE/`/next`) to display shape.
  function normalizeNP(t) {
    return {
      id: t.id, title: t.title, code: t.code || '··', albumId: t.album_id,
      artistName: t.artist_name || 'Unknown', albumName: t.album_name || '',
      tone: t.tone || 'magenta', duration: t.duration || 0,
      cover: coverURL(t.id), gradient: gradientFor(t.id),
    };
  }
}
