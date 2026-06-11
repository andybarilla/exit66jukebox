<script>
  let { tab = 'albums', onTab } = $props();
  const TABS = [['artists', 'Artists'], ['albums', 'Albums'], ['tracks', 'Tracks']];
</script>
<div style="display:inline-flex; flex:none; border:1px solid var(--border-strong); border-radius:var(--radius-md); overflow:hidden; height:36px;">
  {#each TABS as [key, label]}
    <button onclick={() => onTab(key)}
      style="padding:0 16px; font-family:var(--font-display); font-weight:600; font-size:12px; letter-spacing:0.08em; text-transform:uppercase; border:none; cursor:pointer; background:{tab === key ? 'var(--neon-amber)' : 'transparent'}; color:{tab === key ? 'var(--text-on-accent)' : 'var(--text-muted)'};">{label}</button>
  {/each}
</div>
