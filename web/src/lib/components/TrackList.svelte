<script>
  import TrackRow from './TrackRow.svelte';
  import { fmt } from '../format.js';
  let { tracks = [], nowPlayingId = null, onAdd } = $props();
</script>
<div style="display:flex; flex-direction:column; gap:2px;">
  {#each tracks as t (t.code)}
    <TrackRow code={t.code} title={t.title} artist={t.artistName} duration={fmt(t.duration)}
      cover={t.cover} gradient={t.gradient} tone={t.tone}
      playing={t.id === nowPlayingId} onAdd={() => onAdd(t)} />
  {/each}
</div>
