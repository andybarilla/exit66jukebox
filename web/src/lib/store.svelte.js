import {
  listTracks, listAlbums, listArtists, getQueue, requestTo, removeRequest,
  setShuffle, subscribeEvents, coverURL, albumCoverURL, HOUSE,
  discoverGenres, discoverRediscover, discoverRecent,
  getStation, startStation as apiStartStation, stopStation as apiStopStation,
} from './api.js';
import { albumLetter, toneFor, gradientFor, compareNames } from './format.js';

const ME = 'me';

export function createStore() {
  let tab = $state('albums');
  let query = $state('');
  let stream = $state('house');           // 'house' (shared) | 'me' (personal)
  let isPhone = $state(false);
  let lineupOpen = $state(false);
  let detailAlbumId = $state(null);
  let shuffle = $state({ house: false, me: false }); // per-stream, mirrors backend
  let displayName = $state(localStorage.getItem('e66.name') || 'You');
  let toasts = $state([]);

  // library
  let albums = $state([]);      // {id,name,artistId,artistName,letter,tone,tracks:[{...,code}]}
  let artists = $state([]);     // {id,name,albumCount,trackCount,tone,tracks:[...]}
  let tracksByCode = $state({});

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
  let tracksById = {}; // id -> enriched track, so the lineup/now-playing reuse
                       // the exact same slot code + tone the crate computed.

  async function loadLibrary() {
    const [rawTracks, rawAlbums, rawArtists] = await Promise.all([
      listTracks(''), listAlbums(''), listArtists(''),
    ]);
    // group tracks by album, assigning crate-wall letters in alphabetical
    // order — sort first so the A/B/C… codes run alphabetically too.
    rawAlbums.sort((x, y) => compareNames(x.name, y.name));
    const albumById = new Map();
    rawAlbums.forEach((al, i) => albumById.set(al.id, {
      id: al.id, name: al.name, artistId: al.artist_id,
      letter: albumLetter(i), tone: toneFor(i), tracks: [],
    }));
    const artistName = new Map(rawArtists.map((a) => [a.id, a.name]));
    const codeMap = {};
    for (const t of rawTracks) {
      const al = albumById.get(t.album_id);
      if (!al) continue;
      const code = al.letter + (t.track_no || al.tracks.length + 1);
      const enriched = {
        ...t, code, tone: al.tone, albumName: al.name,
        artistName: artistName.get(t.artist_id) || 'Unknown',
        cover: coverURL(t.id), gradient: gradientFor(t.id),
      };
      al.tracks.push(enriched);
      codeMap[code] = enriched;
      tracksById[t.id] = enriched;
    }
    const albumList = [...albumById.values()].map((al) => ({
      ...al, artistName: artistName.get(al.artistId) || 'Unknown',
      cover: albumCoverURL(al.id), gradient: gradientFor(al.id),
      initial: (al.name[0] || '?').toUpperCase(),
    }));
    // artist groupings
    const byArtist = new Map();
    for (const al of albumList) {
      if (!byArtist.has(al.artistId)) byArtist.set(al.artistId, {
        id: al.artistId, name: al.artistName, tone: al.tone,
        gradient: gradientFor(1000 + al.artistId), albums: [], tracks: [],
      });
      const g = byArtist.get(al.artistId);
      g.albums.push(al); g.tracks.push(...al.tracks);
    }
    albums = albumList;
    artists = [...byArtist.values()].map((a) => ({
      ...a, albumCount: a.albums.length, trackCount: a.tracks.length,
      initial: (a.name[0] || '?').toUpperCase(),
    })).sort((x, y) => compareNames(x.name, y.name));
    tracksByCode = codeMap;
  }

  async function refreshQueue(s) {
    const r = await getQueue(s);
    queues[s] = (r.queue || []).map(normalizeQueued);
    if (typeof r.listeners === 'number') listeners[s] = r.listeners;
  }

  // backend queue items are {track:{...}, requested_by} for /me, but the house
  // SSE/queue may send bare tracks; normalize both.
  function normalizeQueued(item) {
    const t = item.track || item;
    const code = codeForTrack(t);
    return {
      uid: ++_uid, id: t.id, title: t.title,
      artistName: nameForArtist(t.artist_id), albumName: albumNameFor(t.album_id),
      code, tone: toneForTrack(t), requester: item.requested_by || '',
      cover: coverURL(t.id), gradient: gradientFor(t.id),
    };
  }
  function codeForTrack(t) {
    if (tracksById[t.id]) return tracksById[t.id].code;
    const al = albums.find((a) => a.id === t.album_id);
    return al ? al.letter + (t.track_no || 1) : '··';
  }
  function toneForTrack(t) {
    if (tracksById[t.id]) return tracksById[t.id].tone;
    const al = albums.find((a) => a.id === t.album_id);
    return al ? al.tone : 'magenta';
  }
  function nameForArtist(id) {
    const a = artists.find((x) => x.id === id);
    return a ? a.name : 'Unknown';
  }
  function albumNameFor(id) {
    const al = albums.find((a) => a.id === id);
    return al ? al.name : '';
  }

  // Enrich a raw discover track (id,title,artist_id,album_id,track_no,duration…)
  // with the same display fields TrackList/TrackRow expect.
  function enrichDiscover(t) {
    return {
      id: t.id, title: t.title, duration: t.duration || 0,
      code: codeForTrack(t), artistName: nameForArtist(t.artist_id),
      albumName: albumNameFor(t.album_id), tone: toneForTrack(t),
      cover: coverURL(t.id), gradient: gradientFor(t.id),
    };
  }

  async function loadDiscoverLists(genre) {
    const [rd, rc] = await Promise.all([
      discoverRediscover(genre), discoverRecent(genre),
    ]);
    discoverRediscoverRows = (Array.isArray(rd) ? rd : []).map(enrichDiscover);
    discoverRecentRows = (Array.isArray(rc) ? rc : []).map(enrichDiscover);
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

  // ----- derived (getters) -----
  function match(s) { const q = query.trim().toLowerCase(); return !q || String(s).toLowerCase().includes(q); }

  return {
    // primitive state accessors
    get tab() { return tab; }, set tab(v) { tab = v; },
    get query() { return query; }, set query(v) { query = v; },
    get stream() { return stream; },
    get isPhone() { return isPhone; }, set isPhone(v) { isPhone = v; },
    get lineupOpen() { return lineupOpen; }, set lineupOpen(v) { lineupOpen = v; },
    get detailAlbumId() { return detailAlbumId; },
    get shuffle() { return shuffle[stream]; },
    get displayName() { return displayName; },
    set displayName(v) { displayName = v; localStorage.setItem('e66.name', v); },
    get toasts() { return toasts; },

    get albums() { return albums; },
    get artists() { return artists; },

    // Personal is always "just you": the `me` stream has no broadcast hub, so
    // the backend reports 0 listeners for it. Never let that 0 surface here.
    get listeners() { return stream === 'me' ? 1 : (listeners.house || 0); },
    get queue() { return queues[stream]; },
    get nowPlaying() { return nowPlaying[stream]; },
    get progress() { return progress[stream]; },

    // filtered library
    get albumCards() {
      return albums.filter((a) => match(a.name) || match(a.artistName))
        .map((a) => ({ ...a, meta: `${a.tracks.length} tracks` }));
    },
    get artistRows() {
      return artists.filter((a) => match(a.name))
        .map((a) => ({ ...a, meta: `${a.albumCount} albums · ${a.trackCount} tracks` }));
    },
    get trackRows() {
      const all = albums.flatMap((a) => a.tracks);
      return all.filter((t) => match(t.title) || match(t.artistName) || match(t.albumName) || match(t.code));
    },
    get currentCount() {
      if (this.tab === 'albums') return this.albumCards.length;
      if (this.tab === 'artists') return this.artistRows.length;
      if (this.tab === 'discover') return discoverRediscoverRows.length + discoverRecentRows.length;
      return this.trackRows.length;
    },

    // discover accessors
    get discoverGenres() { return discoverGenreList; },
    get discoverSelectedGenre() { return discoverSelectedGenre; },
    get discoverRediscover() { return discoverRediscoverRows; },
    get discoverRecent() { return discoverRecentRows; },
    get discoverStation() { return discoverStation; },
    get detailAlbum() { return albums.find((a) => a.id === detailAlbumId) || null; },

    // ----- actions -----
    async init() {
      await loadLibrary();
      await Promise.all([refreshQueue('house'), refreshQueue('me')]);
      // Pre-load discover genres (lists load on tab activation via loadDiscover).
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
    teardown() { if (_esHouse) { _esHouse(); _esHouse = null; } },

    setStream(s) {
      if (s === stream) return;
      stream = s;
      // Each stream keeps its own shuffle flag (UI + backend); switching just
      // reveals the target stream's flag — don't push the old one onto it.
      pushToast('cyan', 'Stream', s === 'house'
        ? 'Tuned in to the house stream — everyone hears this.'
        : 'Switched to your personal stream.');
    },
    toggleStream() { this.setStream(stream === 'house' ? 'me' : 'house'); },

    async toggleShuffle(v) {
      shuffle[stream] = typeof v === 'boolean' ? v : !shuffle[stream];
      await setShuffle(stream, shuffle[stream]);
    },

    // Re-fetch a stream's queue (+ listeners) from the backend.
    refreshQueue(s) { return refreshQueue(s); },

    openAlbum(id) { detailAlbumId = id; },
    closeAlbum() { detailAlbumId = null; },
    openArtist(a) { tab = 'tracks'; query = a.name; },
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

    npMeta(t) {
      return {
        code: codeForTrack(t), artistName: nameForArtist(t.artist_id),
        albumName: albumNameFor(t.album_id), tone: toneForTrack(t),
        cover: coverURL(t.id), gradient: gradientFor(t.id),
      };
    },
  };

  function normalizeNP(t) {
    return {
      id: t.id, title: t.title, code: codeForTrack(t),
      artistName: nameForArtist(t.artist_id), albumName: albumNameFor(t.album_id),
      tone: toneForTrack(t), duration: t.duration || 0,
      cover: coverURL(t.id), gradient: gradientFor(t.id),
    };
  }
}
