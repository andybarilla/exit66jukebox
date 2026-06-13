<script>
  import {
    listSonos, castSonos, stopSonos, getSonosVolume, setSonosVolume,
    addManualSonos, nextHouse,
  } from '../api.js';
  let { onToast = () => {} } = $props();

  let open = $state(false);
  let searching = $state(false);
  let devices = $state([]);        // [{name, ip}]
  let activeIp = $state(null);     // device currently being cast to
  let volume = $state(70);
  let manualIp = $state('');
  let adding = $state(false);

  let volTimer;                    // debounce handle for setSonosVolume

  async function search() {
    searching = true;
    try {
      const list = await listSonos();
      devices = Array.isArray(list) ? list : [];
      if (devices.length === 0) onToast('amber', 'No Sonos found', 'Nothing answered on the LAN. SSDP may be blocked — try a manual IP.');
    } catch (_) {
      onToast('amber', 'Search failed', 'Could not reach the Sonos discovery endpoint.');
    } finally {
      searching = false;
    }
  }

  async function cast(d) {
    try {
      await castSonos(d.ip);
      activeIp = d.ip;
      onToast('success', 'Casting', `House stream → ${d.name}.`);
      try { const r = await getSonosVolume(d.ip); if (typeof r?.volume === 'number') volume = r.volume; } catch (_) {}
    } catch (_) {
      onToast('amber', 'Cast failed', `Could not cast to ${d.name}.`);
    }
  }

  async function stop() {
    if (!activeIp) return;
    const ip = activeIp;
    try {
      await stopSonos(ip);
      activeIp = null;
      onToast('cyan', 'Stopped', 'Sonos playback stopped.');
    } catch (_) {
      onToast('amber', 'Stop failed', 'Could not stop Sonos playback.');
    }
  }

  function setVolFrom(e) {
    if (!activeIp) return;
    const r = e.currentTarget.getBoundingClientRect();
    volume = Math.round(Math.max(0, Math.min(1, (e.clientX - r.left) / r.width)) * 100);
    clearTimeout(volTimer);
    const ip = activeIp;
    volTimer = setTimeout(() => { setSonosVolume(ip, volume).catch(() => {}); }, 180);
  }
  function onVolPointerDown(e) {
    if (!activeIp) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    setVolFrom(e);
  }
  function onVolPointerMove(e) {
    if (e.buttons !== 1) return; // only while dragging
    setVolFrom(e);
  }

  async function next() {
    try { await nextHouse(); onToast('cyan', 'Skipped', 'Advanced the house queue.'); }
    catch (_) { onToast('amber', 'Skip failed', 'Could not advance the house queue.'); }
  }

  async function addManual() {
    const ip = manualIp.trim();
    if (!ip) return;
    adding = true;
    try {
      const d = await addManualSonos(ip);
      if (!devices.some((x) => x.ip === d.ip)) devices = [...devices, d];
      manualIp = '';
      onToast('success', 'Added', `${d.name} added manually.`);
    } catch (_) {
      onToast('amber', 'Not a Sonos', `${ip} did not answer as a Sonos device.`);
    } finally {
      adding = false;
    }
  }
</script>

<div style="position:relative; flex:none;">
  <button onclick={() => (open = !open)} aria-label="Cast to Sonos" aria-expanded={open}
    style="display:inline-flex; align-items:center; gap:7px; padding:7px 12px; border:1px solid {activeIp ? 'var(--neon-cyan)' : 'var(--border-default)'}; border-radius:var(--radius-sm); background:{activeIp ? 'rgba(34,224,238,0.06)' : 'transparent'}; font-family:var(--font-mono); font-size:11px; letter-spacing:0.1em; text-transform:uppercase; color:{activeIp ? 'var(--neon-cyan)' : 'var(--text-muted)'}; cursor:pointer; white-space:nowrap;">
    <span style="font-size:13px; line-height:1;">📡</span>Cast
  </button>

  {#if open}
    <div role="button" tabindex="-1" aria-label="Close" onclick={() => (open = false)} onkeydown={(e) => { if (e.key === 'Escape') open = false; }} style="position:fixed; inset:0; z-index:80;"></div>
    <div style="position:absolute; right:0; top:calc(100% + 8px); z-index:81; width:296px; max-width:calc(100vw - 28px); background:var(--bg-surface); background-image:var(--scanline); border:1.5px solid var(--neon-cyan); border-radius:var(--radius-md); box-shadow:var(--shadow-lg); padding:14px; box-sizing:border-box; display:flex; flex-direction:column; gap:12px;">
      <div style="display:flex; align-items:center; justify-content:space-between;">
        <span style="font-family:var(--font-display); font-weight:700; font-size:13px; letter-spacing:0.08em; text-transform:uppercase; color:var(--text-strong);">Cast to Sonos</span>
        <button onclick={search} disabled={searching} style="font-family:var(--font-mono); font-size:10px; letter-spacing:0.08em; text-transform:uppercase; padding:5px 9px; border:1px solid var(--border-strong); border-radius:var(--radius-sm); background:transparent; color:var(--text-body); cursor:pointer;">{searching ? 'Searching…' : 'Search'}</button>
      </div>

      {#if devices.length > 0}
        <div style="display:flex; flex-wrap:wrap; gap:7px;">
          {#each devices as d (d.ip)}
            <button onclick={() => cast(d)}
              style="padding:6px 11px; border:1px solid {activeIp === d.ip ? 'var(--neon-cyan)' : 'var(--border-strong)'}; border-radius:var(--radius-pill); background:{activeIp === d.ip ? 'var(--neon-cyan)' : 'transparent'}; color:{activeIp === d.ip ? 'var(--text-on-accent)' : 'var(--text-body)'}; font-family:var(--font-mono); font-size:11px; letter-spacing:0.04em; cursor:pointer; white-space:nowrap;">{d.name}</button>
          {/each}
        </div>
      {:else}
        <span style="font-family:var(--font-mono); font-size:11px; color:var(--text-faint);">No devices yet — search the LAN or add an IP below.</span>
      {/if}

      {#if activeIp}
        <div style="display:flex; align-items:center; gap:10px;">
          <span style="color:var(--text-muted); font-size:15px;">♪</span>
          <div onpointerdown={onVolPointerDown} onpointermove={onVolPointerMove} role="slider" tabindex="0" aria-label="Sonos volume" aria-valuenow={volume} style="position:relative; flex:1; height:14px; display:flex; align-items:center; cursor:pointer; touch-action:none;">
            <div style="position:absolute; left:0; right:0; height:4px; border-radius:var(--radius-pill); background:var(--ink-700);"></div>
            <div style="position:absolute; left:0; width:{volume}%; height:4px; border-radius:var(--radius-pill); background:var(--neon-cyan);"></div>
            <div style="position:absolute; left:calc({volume}% - 6px); width:12px; height:12px; border-radius:50%; background:var(--paper-100); border:2px solid var(--neon-cyan);"></div>
          </div>
          <span style="font-family:var(--font-mono); font-size:10px; color:var(--text-faint); width:26px; text-align:right;">{volume}</span>
        </div>
        <div style="display:flex; gap:8px;">
          <button onclick={next} style="flex:1; padding:7px 0; border:1px solid var(--border-strong); border-radius:var(--radius-sm); background:transparent; color:var(--text-body); font-family:var(--font-mono); font-size:11px; letter-spacing:0.08em; text-transform:uppercase; cursor:pointer;">Next ⏭</button>
          <button onclick={stop} style="flex:1; padding:7px 0; border:1px solid var(--neon-magenta); border-radius:var(--radius-sm); background:transparent; color:var(--neon-magenta-bright); font-family:var(--font-mono); font-size:11px; letter-spacing:0.08em; text-transform:uppercase; cursor:pointer;">Stop ◼</button>
        </div>
      {/if}

      <form onsubmit={(e) => { e.preventDefault(); addManual(); }} style="display:flex; gap:8px;">
        <input bind:value={manualIp} placeholder="SSDP blocked? Sonos IP…" aria-label="Manual Sonos IP"
          style="flex:1; min-width:0; padding:7px 10px; border:1px solid var(--border-default); border-radius:var(--radius-sm); background:var(--ink-950); color:var(--text-body); font-family:var(--font-mono); font-size:11px;" />
        <button type="submit" disabled={adding} style="padding:7px 11px; border:1px solid var(--border-strong); border-radius:var(--radius-sm); background:transparent; color:var(--text-body); font-family:var(--font-mono); font-size:11px; letter-spacing:0.06em; text-transform:uppercase; cursor:pointer;">{adding ? '…' : 'Add'}</button>
      </form>
    </div>
  {/if}
</div>
