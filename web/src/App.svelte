<script>
  import { onMount, onDestroy } from 'svelte';
  import {
    listTracks, requestTrack, requestTo, getQueue,
    houseStreamURL, coverURL, subscribeEvents, HOUSE,
    listSonos, castSonos, stopSonos,
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
</style>
