<script>
  import { Virtualizer } from 'virtua/svelte';
  import AlbumCard from './AlbumCard.svelte';
  import { columnsForWidth } from '../grid.js';
  let { cards = [], onOpen, onRequest } = $props();

  const MIN_COL = 150, GAP = 14;
  let width = $state(0);
  const cols = $derived(columnsForWidth(width, MIN_COL, GAP));
  // Chunk cards into rows of `cols`; one virtual item == one grid row.
  const rows = $derived.by(() => {
    const out = [];
    for (let i = 0; i < cards.length; i += cols) out.push(cards.slice(i, i + cols));
    return out;
  });
</script>
<!-- zero-height sentinel measures the grid's available width without affecting
     the scroll container Virtualizer attaches to (its parent) -->
<div bind:clientWidth={width} style="height:0;"></div>
<Virtualizer data={rows} getKey={(row) => row[0].id}>
  {#snippet children(row)}
    <div style="display:grid; grid-template-columns:repeat({cols}, minmax(0, 1fr)); gap:{GAP}px; padding-bottom:{GAP}px;">
      {#each row as a (a.id)}
        <AlbumCard title={a.name} artist={a.artistName} meta={a.meta} initial={a.initial}
          cover={a.cover} gradient={a.gradient}
          onOpen={() => onOpen(a)} onRequest={() => onRequest(a)} />
      {/each}
    </div>
  {/snippet}
</Virtualizer>
