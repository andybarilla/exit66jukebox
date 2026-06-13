<script>
  import { Virtualizer } from 'virtua/svelte';
  import TrackRow from './TrackRow.svelte';
  import { fmt } from '../format.js';
  let { tracks = [], nowPlayingId = null, onAdd } = $props();
</script>
<!-- itemSize hints the fixed row height so virtua skips its unmeasured-item
     estimation pass and mounts only the visible window from the first frame -->
<Virtualizer data={tracks} getKey={(t) => t.id} itemSize={68}>
  {#snippet children(t)}
    <div style="padding-bottom:2px;">
      <TrackRow code={t.code} title={t.title} artist={t.artistName} duration={fmt(t.duration)}
        cover={t.cover} gradient={t.gradient} tone={t.tone}
        playing={t.id === nowPlayingId} onAdd={() => onAdd(t)} />
    </div>
  {/snippet}
</Virtualizer>
