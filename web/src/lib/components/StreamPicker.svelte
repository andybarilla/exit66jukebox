<script>
  // The stream selector: every shared stream in one list, with the personal
  // stream pinned underneath as a separate control rather than an entry in it —
  // it is a different thing (your own queue, played locally), not another room.
  // Admin affordances (new / rename / delete) show only for admins; the server
  // gates them on the stream's kind regardless of what is rendered here.
  import { keyActivate } from '../format.js';

  let {
    streams = [],
    current,
    personalId,
    canManage = false,
    atCap = false,
    onSelect,
    onCreate,
    onRename,
    onDelete,
  } = $props();

  let creating = $state(false);
  let newName = $state('');
  let renamingId = $state('');
  let renameName = $state('');

  const label = 'font-family:var(--font-mono); font-size:9px; letter-spacing:0.22em; text-transform:uppercase; color:var(--text-faint);';
  const rowBase = 'display:flex; align-items:center; gap:6px; width:100%; padding:5px 9px; border:none; cursor:pointer; text-align:left; font-family:var(--font-mono); font-size:11px; font-weight:700; letter-spacing:0.08em; text-transform:uppercase;';

  function rowStyle(active) {
    return `${rowBase} background:${active ? 'var(--neon-cyan)' : 'transparent'}; color:${active ? 'var(--text-on-accent)' : 'var(--text-muted)'};`;
  }

  async function submitCreate() {
    const name = newName.trim();
    if (!name) return;
    creating = false;
    newName = '';
    await onCreate?.(name);
  }

  function startRename(st) {
    renamingId = st.id;
    renameName = st.name;
  }

  async function submitRename() {
    const name = renameName.trim();
    const id = renamingId;
    if (!name || !id) { renamingId = ''; return; }
    renamingId = '';
    await onRename?.(id, name);
  }
</script>

<div style="display:flex; flex-direction:column; gap:8px;">
  <span style={label}>Stream</span>

  <div role="listbox" aria-label="Shared streams"
       style="display:flex; flex-direction:column; border:1px solid var(--border-strong); border-radius:var(--radius-sm); overflow:hidden;">
    {#each streams as st (st.id)}
      {#if renamingId === st.id}
        <!-- svelte-ignore a11y_autofocus -->
        <input
          value={renameName}
          oninput={(e) => (renameName = e.currentTarget.value)}
          onblur={submitRename}
          onkeydown={(e) => { if (e.key === 'Enter') submitRename(); if (e.key === 'Escape') renamingId = ''; }}
          autofocus
          aria-label="Stream name"
          maxlength="60"
          style="width:100%; box-sizing:border-box; padding:5px 9px; border:none; background:var(--bg-surface-raised); color:var(--text-body); font-family:var(--font-mono); font-size:11px;" />
      {:else}
        <div style="display:flex; align-items:center;">
          <button role="option" aria-selected={current === st.id}
                  onclick={() => onSelect?.(st.id)} style={rowStyle(current === st.id)}>
            <span style="flex:1; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">{st.name || st.id}</span>
            <span style="font-weight:400; opacity:0.7;">{st.listeners ?? 0}</span>
          </button>
          {#if canManage}
            <span role="button" tabindex="0" aria-label={`Rename ${st.name}`}
                  onclick={() => startRename(st)} onkeydown={keyActivate(() => startRename(st))}
                  style="padding:0 6px; cursor:pointer; color:var(--text-faint); font-size:11px;">✎</span>
            {#if !st.house}
              <span role="button" tabindex="0" aria-label={`Delete ${st.name}`}
                    onclick={() => onDelete?.(st.id)} onkeydown={keyActivate(() => onDelete?.(st.id))}
                    style="padding:0 8px 0 2px; cursor:pointer; color:var(--text-faint); font-size:11px;">✕</span>
            {/if}
          {/if}
        </div>
      {/if}
    {/each}
  </div>

  {#if canManage}
    {#if creating}
      <!-- svelte-ignore a11y_autofocus -->
      <input
        value={newName}
        oninput={(e) => (newName = e.currentTarget.value)}
        onblur={submitCreate}
        onkeydown={(e) => { if (e.key === 'Enter') submitCreate(); if (e.key === 'Escape') { creating = false; newName = ''; } }}
        autofocus
        placeholder="Stream name"
        aria-label="New stream name"
        maxlength="60"
        style="width:100%; box-sizing:border-box; padding:5px 9px; border:1px solid var(--border-strong); border-radius:var(--radius-sm); background:var(--bg-surface-raised); color:var(--text-body); font-family:var(--font-mono); font-size:11px;" />
    {:else if !atCap}
      <button onclick={() => (creating = true)}
              style="align-self:flex-start; padding:3px 9px; border:1px dashed var(--border-strong); border-radius:var(--radius-sm); background:transparent; color:var(--text-faint); cursor:pointer; font-family:var(--font-mono); font-size:10px; letter-spacing:0.1em; text-transform:uppercase;">+ New stream</button>
    {:else}
      <span style="font-family:var(--font-mono); font-size:9px; letter-spacing:0.1em; color:var(--text-faint);">Stream limit reached</span>
    {/if}
  {/if}

  <!-- The personal stream is pinned here, deliberately outside the list above. -->
  <button onclick={() => onSelect?.(personalId)}
          style="{rowStyle(current === personalId)} border:1px solid var(--border-strong); border-radius:var(--radius-sm);">Personal</button>
</div>
