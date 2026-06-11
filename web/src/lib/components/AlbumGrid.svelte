<script>
  import AlbumCard from './AlbumCard.svelte';
  let { cards = [], onOpen, onRequest } = $props();
</script>
<div style="display:grid; grid-template-columns:repeat(auto-fill, minmax(150px, 1fr)); gap:14px;">
  {#each cards as a (a.id)}
    <AlbumCard title={a.name} artist={a.artistName} meta={a.meta} initial={a.initial}
      cover={a.cover} gradient={a.gradient}
      onOpen={() => onOpen(a)} onRequest={() => onRequest(a)} />
  {/each}
</div>
