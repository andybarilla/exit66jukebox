<script>
  import { onMount, onDestroy } from 'svelte';
  import {
    listTracks, requestTrack, requestTo, getQueue,
    houseStreamURL, coverURL, subscribeEvents, HOUSE,
    listSonos, castSonos, stopSonos,
    discoverRediscover, discoverRecent, discoverGenres,
    getStation, startStation, stopStation,
  } from './lib/api.js';
  import { createPlayer } from './lib/player.js';

  let mode = 'local';        // 'local' | 'house'
  let search = '';
  let tracks = [];
  let nowPlaying = null;
  let houseQueue = [];
  let audio;
  let localPlayer = null;
  let closeEvents = null;

  async function doSearch() { tracks = await listTracks(search); }

  async function add(t) {
    if (mode === 'house') await requestTo(HOUSE, t.id);
    else await requestTrack(t.id);
  }

  function teardown() {
    if (localPlayer) { localPlayer.stop(); localPlayer = null; }
    if (closeEvents) { closeEvents(); closeEvents = null; }
    if (audio) { audio.pause(); audio.src = ''; }
    nowPlaying = null;
  }

  function startLocal() {
    localPlayer = createPlayer(audio, (t) => (nowPlaying = t));
    localPlayer.start();
  }

  async function startHouse() {
    audio.src = houseStreamURL();
    audio.play().catch(() => {});
    const q = await getQueue(HOUSE);
    houseQueue = q.queue || [];
    closeEvents = subscribeEvents(HOUSE, (e) => {
      if (e.type === 'now-playing') nowPlaying = e.data;
      else if (e.type === 'queue-changed') getQueue(HOUSE).then((q) => (houseQueue = q.queue || []));
    });
  }

  function setMode(m) {
    if (m === mode) return;
    teardown();
    mode = m;
    if (m === 'house') startHouse();
    else startLocal();
  }

  // Discover
  let showDiscover = false;
  let genres = [];
  let discoverGenre = '';
  let stationGenre = '';
  let rediscoverTracks = [];
  let recentTracks = [];
  let station = null; // {stream_id, genre, ...} or null

  $: streamId = mode === 'house' ? HOUSE : 'me';

  async function loadDiscover() {
    [genres, rediscoverTracks, recentTracks] = await Promise.all([
      discoverGenres(),
      discoverRediscover(discoverGenre),
      discoverRecent(discoverGenre),
    ]);
    station = await getStation(streamId);
    if (!station || !station.stream_id) station = null;
    if (!stationGenre && genres.length) stationGenre = genres[0].genre;
  }

  async function applyGenreFilter() {
    [rediscoverTracks, recentTracks] = await Promise.all([
      discoverRediscover(discoverGenre),
      discoverRecent(discoverGenre),
    ]);
  }

  async function doStartStation() {
    station = await startStation(streamId, stationGenre);
    if (!station || !station.stream_id) { station = null; return; }
    if (mode === 'house') {
      const q = await getQueue(HOUSE);
      houseQueue = q.queue || [];
    }
  }

  async function doStopStation() {
    await stopStation(streamId);
    station = null;
  }

  function toggleDiscover() {
    showDiscover = !showDiscover;
    if (showDiscover) loadDiscover();
  }

  let sonosDevices = [];
  let castIP = null;
  let sonosBusy = false;

  async function loadSonos() {
    sonosBusy = true;
    try { sonosDevices = await listSonos(); }
    finally { sonosBusy = false; }
  }
  async function cast(ip) { const r = await castSonos(ip); if (r && r.ok) castIP = ip; }
  async function stopCast() { if (castIP) { await stopSonos(castIP); castIP = null; } }

  onMount(async () => {
    tracks = await listTracks('');
    startLocal();
  });
  onDestroy(teardown);
</script>

<main>
  <h1>Exit 66 Jukebox</h1>

  <div class="modes">
    <button class:active={mode === 'local'} on:click={() => setMode('local')}>Local</button>
    <button class:active={mode === 'house'} on:click={() => setMode('house')}>House</button>
  </div>

  <div class="sonos">
    <button on:click={loadSonos} disabled={sonosBusy}>
      {sonosBusy ? 'Searching…' : 'Cast to Sonos'}
    </button>
    {#each sonosDevices as d (d.ip)}
      <button class:active={castIP === d.ip} on:click={() => cast(d.ip)}>{d.name}</button>
    {/each}
    {#if castIP}
      <button on:click={stopCast}>Stop casting</button>
    {/if}
  </div>

  <div class="now">
    {#if nowPlaying}
      <img class="cover" src={coverURL(nowPlaying.id)} alt="" on:error={(e) => (e.target.style.visibility = 'hidden')} />
      <span>Now playing: {nowPlaying.title}</span>
    {:else}
      <span>{mode === 'house' ? 'House stream idle — request something' : 'Queue empty — request something'}</span>
    {/if}
  </div>

  <input bind:value={search} on:keydown={(e) => e.key === 'Enter' && doSearch()} placeholder="Search songs" />
  <button on:click={doSearch}>Search</button>

  <ul>
    {#each tracks as t (t.id)}
      <li><button on:click={() => add(t)}>＋</button> {t.title}</li>
    {/each}
  </ul>

  {#if mode === 'house' && houseQueue.length}
    <h2>Up next (house)</h2>
    <ol>
      {#each houseQueue as t (t.id)}<li>{t.title}</li>{/each}
    </ol>
  {/if}

  <audio bind:this={audio} controls></audio>

  <div class="discover-toggle">
    <button class:active={showDiscover} on:click={toggleDiscover}>Discover</button>
  </div>

  {#if showDiscover}
    <div class="discover">
      <div class="discover-filter">
        <label for="genre-filter">Genre</label>
        <select id="genre-filter" bind:value={discoverGenre} on:change={applyGenreFilter}>
          <option value="">All genres</option>
          {#each genres as g (g.genre)}
            <option value={g.genre}>{g.genre} ({g.count})</option>
          {/each}
        </select>
      </div>

      <div class="station-control">
        <h2>Genre Station</h2>
        {#if station}
          <span class="station-active">Active: {station.genre}</span>
          <button on:click={doStopStation}>Stop station</button>
        {:else}
          <select bind:value={stationGenre}>
            {#each genres as g (g.genre)}
              <option value={g.genre}>{g.genre}</option>
            {/each}
          </select>
          <button on:click={doStartStation} disabled={!stationGenre}>Start station</button>
        {/if}
      </div>

      <h2>Rediscover</h2>
      <ul>
        {#each rediscoverTracks as t (t.id)}
          <li><button on:click={() => add(t)}>＋</button> {t.title}</li>
        {/each}
        {#if !rediscoverTracks.length}<li class="empty">No tracks</li>{/if}
      </ul>

      <h2>Recently Added</h2>
      <ul>
        {#each recentTracks as t (t.id)}
          <li><button on:click={() => add(t)}>＋</button> {t.title}</li>
        {/each}
        {#if !recentTracks.length}<li class="empty">No tracks</li>{/if}
      </ul>
    </div>
  {/if}
</main>

<style>
  main { font-family: system-ui; max-width: 640px; margin: 2rem auto; color: #eee; background: #181818; padding: 1.5rem; border-radius: 8px; }
  .modes button { margin-right: .5rem; }
  .modes button.active { background: #6cf; color: #000; }
  .now { color: #6cf; display: flex; align-items: center; gap: .5rem; min-height: 40px; }
  .cover { width: 40px; height: 40px; object-fit: cover; border-radius: 4px; }
  li { list-style: none; margin: .25rem 0; }
  ol li { list-style: decimal; margin-left: 1.5rem; }
  button { cursor: pointer; }
  .sonos { margin: .5rem 0; display: flex; flex-wrap: wrap; gap: .5rem; align-items: center; }
  .sonos button.active { background: #6cf; color: #000; }
  .discover-toggle { margin: 1rem 0 0; }
  .discover-toggle button.active { background: #6cf; color: #000; }
  .discover { margin-top: .75rem; border-top: 1px solid #333; padding-top: .75rem; }
  .discover-filter { display: flex; align-items: center; gap: .5rem; margin-bottom: .75rem; }
  .discover-filter select { background: #222; color: #eee; border: 1px solid #444; border-radius: 4px; padding: .25rem .5rem; }
  .station-control { display: flex; align-items: center; gap: .5rem; margin-bottom: .75rem; flex-wrap: wrap; }
  .station-control h2 { margin: 0; font-size: 1rem; }
  .station-control select { background: #222; color: #eee; border: 1px solid #444; border-radius: 4px; padding: .25rem .5rem; }
  .station-active { color: #6cf; }
  .discover h2 { margin: .75rem 0 .25rem; font-size: 1rem; color: #aaa; }
  .empty { color: #555; font-style: italic; }
</style>
