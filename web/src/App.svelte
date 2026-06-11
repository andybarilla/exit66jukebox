<script>
  import { onMount } from 'svelte';
  import { listTracks, requestTrack } from './lib/api.js';
  import { createPlayer } from './lib/player.js';

  let search = '';
  let tracks = [];
  let nowPlaying = null;
  let audio;

  async function doSearch() { tracks = await listTracks(search); }
  async function add(t) { await requestTrack(t.id); }

  onMount(async () => {
    tracks = await listTracks('');
    const player = createPlayer(audio, (t) => (nowPlaying = t));
    player.start();
  });
</script>

<main>
  <h1>Exit 66 Jukebox</h1>
  <p class="now">{nowPlaying ? `Now playing: ${nowPlaying.title}` : 'Queue empty — request something'}</p>

  <input bind:value={search} on:keydown={(e) => e.key === 'Enter' && doSearch()} placeholder="Search songs" />
  <button on:click={doSearch}>Search</button>

  <ul>
    {#each tracks as t (t.id)}
      <li><button on:click={() => add(t)}>＋</button> {t.title}</li>
    {/each}
  </ul>

  <audio bind:this={audio} controls></audio>
</main>

<style>
  main { font-family: system-ui; max-width: 640px; margin: 2rem auto; color: #eee; background: #181818; padding: 1.5rem; border-radius: 8px; }
  .now { color: #6cf; }
  li { list-style: none; margin: .25rem 0; }
  button { cursor: pointer; }
</style>
