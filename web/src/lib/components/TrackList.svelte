<script>
  import { Virtualizer } from 'virtua/svelte';
  import TrackRow from './TrackRow.svelte';
  import { fmt } from '../format.js';
  import { makeInfiniteScroll } from '../infinite.js';
  let { tracks = [], nowPlayingId = null, onAdd, onLoadMore } = $props();

  let vlist = $state();
  const inf = makeInfiniteScroll(() => vlist, () => onLoadMore);
  // Top up until the viewport is full so short first pages don't stall scrolling.
  $effect(() => { tracks.length; inf.fill(); });
</script>
<!-- itemSize hints the fixed row height so virtua skips its unmeasured-item
     estimation pass and mounts only the visible window from the first frame -->
<Virtualizer bind:this={vlist} data={tracks} getKey={(t) => t.id} itemSize={68} onscroll={inf.onScroll}>
  {#snippet children(t)}
    <div style="padding-bottom:2px;">
      <TrackRow code={t.code} title={t.title} artist={t.artistName} duration={fmt(t.duration)}
        cover={t.cover} gradient={t.gradient} tone={t.tone}
        playing={t.id === nowPlayingId} onAdd={() => onAdd(t)} />
    </div>
  {/snippet}
</Virtualizer>
