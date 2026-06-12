<script>
  import TrackList from './TrackList.svelte';
  let {
    genres = [],
    selectedGenre = '',
    onGenre,
    rediscover = [],
    recent = [],
    nowPlayingId = null,
    onAdd,
    station = null,
    onStartStation,
    onStopStation,
  } = $props();

  let stationGenre = $state('');
</script>

<div style="display:flex; flex-direction:column; gap:24px;">

  <!-- Genre filter -->
  <div style="display:flex; align-items:center; gap:12px; flex-wrap:wrap;">
    <span style="font-family:var(--font-mono); font-size:10px; letter-spacing:0.18em; text-transform:uppercase; color:var(--text-faint);">Genre</span>
    <select
      value={selectedGenre}
      onchange={(e) => onGenre(e.currentTarget.value)}
      style="background:var(--bg-surface-raised); color:var(--text-body); border:1px solid var(--border-strong); border-radius:var(--radius-sm); padding:5px 10px; font-family:var(--font-mono); font-size:12px; letter-spacing:0.06em; cursor:pointer; outline:none;"
    >
      <option value="">All genres</option>
      {#each genres as g}
        <option value={g.genre}>{g.genre} ({g.count})</option>
      {/each}
    </select>
  </div>

  <!-- Genre station -->
  <div style="background:var(--bg-surface); background-image:var(--scanline); border:1.5px solid {station?.genre ? 'var(--neon-cyan)' : 'var(--border-strong)'}; border-radius:var(--radius-md); padding:14px 16px; box-shadow:{station?.genre ? 'var(--glow-soft-cyan)' : 'none'}; display:flex; align-items:center; gap:14px; flex-wrap:wrap;">
    <span style="font-family:var(--font-mono); font-size:10px; letter-spacing:0.18em; text-transform:uppercase; color:var(--text-faint); flex:none;">Station</span>
    {#if station?.genre}
      <span style="font-family:var(--font-display); font-weight:700; font-size:14px; letter-spacing:0.06em; text-transform:uppercase; color:var(--neon-cyan-bright, var(--neon-cyan)); flex:1;">
        {station.genre}
      </span>
      <span style="font-family:var(--font-mono); font-size:10px; color:var(--neon-cyan); letter-spacing:0.1em;">LIVE</span>
      <button
        onclick={onStopStation}
        style="padding:5px 14px; border-radius:var(--radius-sm); background:transparent; color:var(--neon-magenta); border:1px solid var(--neon-magenta); font-family:var(--font-mono); font-size:11px; font-weight:700; letter-spacing:0.1em; text-transform:uppercase; cursor:pointer;"
      >Stop</button>
    {:else}
      <select
        bind:value={stationGenre}
        style="flex:1; min-width:120px; background:var(--bg-surface-raised); color:var(--text-body); border:1px solid var(--border-strong); border-radius:var(--radius-sm); padding:5px 10px; font-family:var(--font-mono); font-size:12px; letter-spacing:0.06em; cursor:pointer; outline:none;"
      >
        <option value="">Pick a genre…</option>
        {#each genres as g}
          <option value={g.genre}>{g.genre}</option>
        {/each}
      </select>
      <button
        onclick={() => stationGenre && onStartStation(stationGenre)}
        disabled={!stationGenre}
        style="padding:5px 14px; border-radius:var(--radius-sm); background:{stationGenre ? 'var(--neon-cyan)' : 'transparent'}; color:{stationGenre ? 'var(--text-on-accent)' : 'var(--text-faint)'}; border:1px solid {stationGenre ? 'var(--neon-cyan)' : 'var(--border-default)'}; font-family:var(--font-mono); font-size:11px; font-weight:700; letter-spacing:0.1em; text-transform:uppercase; cursor:{stationGenre ? 'pointer' : 'default'};"
      >Start station</button>
    {/if}
  </div>

  <!-- Rediscover -->
  <div>
    <div style="font-family:var(--font-mono); font-size:10px; letter-spacing:0.18em; text-transform:uppercase; color:var(--text-faint); margin-bottom:10px;">Rediscover</div>
    {#if rediscover.length === 0}
      <div style="font-family:var(--font-sans); font-size:14px; color:var(--text-faint); padding:16px 0;">Nothing to rediscover{selectedGenre ? ` in ${selectedGenre}` : ''}.</div>
    {:else}
      <TrackList tracks={rediscover} {nowPlayingId} {onAdd} />
    {/if}
  </div>

  <!-- Recently Added -->
  <div>
    <div style="font-family:var(--font-mono); font-size:10px; letter-spacing:0.18em; text-transform:uppercase; color:var(--text-faint); margin-bottom:10px;">Recently Added</div>
    {#if recent.length === 0}
      <div style="font-family:var(--font-sans); font-size:14px; color:var(--text-faint); padding:16px 0;">No recent tracks{selectedGenre ? ` in ${selectedGenre}` : ''}.</div>
    {:else}
      <TrackList tracks={recent} {nowPlayingId} {onAdd} />
    {/if}
  </div>

</div>
