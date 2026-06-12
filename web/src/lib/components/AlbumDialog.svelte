<script>
  import TrackRow from './TrackRow.svelte';
  import { fmt, keyActivate } from '../format.js';
  let { album = null, nowPlayingId = null, onClose, onRequestAll, onAddTrack } = $props();
</script>
{#if album}
  <div role="button" tabindex="-1" aria-label="Close" onclick={(e) => { if (e.target === e.currentTarget) onClose(); }} onkeydown={keyActivate(onClose)} style="position:fixed; inset:0; z-index:100; display:flex; align-items:center; justify-content:center; padding:24px; background:rgba(6,6,11,0.72); backdrop-filter:blur(6px); -webkit-backdrop-filter:blur(6px);">
    <div role="dialog" aria-modal="true" tabindex="-1"
      style="width:100%; max-width:460px; background:var(--bg-surface-raised); background-image:var(--scanline); border:1px solid var(--border-strong); border-radius:var(--radius-xl); box-shadow:var(--shadow-xl); overflow:hidden;">
      <div style="height:3px; background:linear-gradient(90deg, var(--neon-magenta), var(--neon-cyan));"></div>
      <div style="padding:var(--space-7);">
        <div style="display:flex; align-items:flex-start; justify-content:space-between; gap:16px; margin-bottom:14px;">
          <div>
            <div style="font-family:var(--font-mono); font-size:11px; letter-spacing:0.22em; text-transform:uppercase; color:var(--neon-cyan); margin-bottom:6px;">{album.artistName}</div>
            <div style="font-family:var(--font-display); font-weight:700; font-size:24px; letter-spacing:0.02em; text-transform:uppercase; color:var(--text-strong);">{album.name}</div>
          </div>
          <button onclick={onClose} aria-label="Close" style="background:none; border:none; color:var(--text-faint); font-size:20px; cursor:pointer; line-height:1;">✕</button>
        </div>
        <div style="display:flex; flex-direction:column; gap:14px;">
          <div style="display:flex; align-items:center; justify-content:space-between; gap:12px;">
            <span style="font-family:var(--font-mono); font-size:11px; letter-spacing:0.16em; text-transform:uppercase; color:var(--text-faint);">{album.tracks.length} tracks</span>
            <button class="qall" onclick={onRequestAll} style="height:38px; padding:0 18px; border:1.5px solid var(--neon-magenta); background:var(--neon-magenta); color:var(--text-on-accent); border-radius:var(--radius-md); cursor:pointer; font-family:var(--font-display); font-weight:600; font-size:13px; letter-spacing:0.08em; text-transform:uppercase;">Queue all</button>
          </div>
          <div style="display:flex; flex-direction:column; gap:2px; max-height:336px; overflow-y:auto; margin-right:-6px; padding-right:6px;">
            {#each album.tracks as t (t.id)}
              <TrackRow code={t.code} title={t.title} artist={t.artistName} duration={fmt(t.duration)}
                cover={t.cover} gradient={t.gradient} tone={t.tone}
                playing={t.id === nowPlayingId} onAdd={() => onAddTrack(t)} />
            {/each}
          </div>
        </div>
      </div>
    </div>
  </div>
{/if}
<style>
  .qall:hover { background: var(--neon-magenta-bright); border-color: var(--neon-magenta-bright); }
</style>
