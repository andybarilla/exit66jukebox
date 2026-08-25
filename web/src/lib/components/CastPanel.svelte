<script>
  import {
    listSonos, castSonos, stopSonos, getSonosVolume, setSonosVolume,
    addManualSonos, nextShared, HOUSE,
  } from '../api.js';
  import IconCast from './icons/IconCast.svelte';
  import IconNext from './icons/IconNext.svelte';
  import IconStop from './icons/IconStop.svelte';
  import IconVolume from './icons/IconVolume.svelte';
  // streams is the shared streams a speaker can be sent to. Personal streams are
  // deliberately absent: a private stream never gets a broadcast pipeline, so
  // there is nothing for a speaker to fetch (#130).
  let {
    streams = [], currentStream = HOUSE, onToast = () => {}, onCastChange = () => {},
  } = $props();

  let open = $state(false);
  let searching = $state(false);
  // devices come back from the server carrying what each one is playing, read
  // off the speaker itself. Nothing here is stored server-side, so this list is
  // the whole device-to-stream mapping and a re-search re-reads it.
  let devices = $state([]);        // [{name, ip, stream}]
  let picked = $state({});         // ip -> the stream id its dropdown is on
  let volumes = $state({});        // ip -> last-read volume
  let busy = $state(null);         // ip currently mid-request
  let manualIp = $state('');
  let adding = $state(false);

  let volTimers = {};              // ip -> debounce handle for setSonosVolume

  // A stream a speaker is playing that has since been deleted would otherwise
  // leave its dropdown blank; fall back the same way an unplayed speaker does.
  function defaultPick(d) {
    const known = (id) => streams.some((st) => st.id === id);
    if (known(d.stream)) return d.stream;
    if (known(currentStream)) return currentStream;
    return streams[0]?.id ?? HOUSE;
  }

  function streamName(id) {
    return streams.find((st) => st.id === id)?.name ?? id;
  }

  // Report the set of streams speakers are playing, which is what the local
  // <audio> mute keys off — muting only when a speaker has this listener's own
  // stream, not whenever any cast is running.
  function publish() {
    onCastChange([...new Set(devices.map((d) => d.stream).filter(Boolean))]);
  }

  function apply(list) {
    devices = Array.isArray(list) ? list : [];
    const next = {};
    for (const d of devices) next[d.ip] = picked[d.ip] ?? defaultPick(d);
    picked = next;
    publish();
  }

  async function search() {
    searching = true;
    try {
      apply(await listSonos());
      if (devices.length === 0) onToast('amber', 'No Sonos found', 'Nothing answered on the LAN. SSDP may be blocked — try a manual IP.');
    } catch (_) {
      onToast('amber', 'Search failed', 'Could not reach the Sonos discovery endpoint.');
    } finally {
      searching = false;
    }
  }

  async function cast(d) {
    const id = picked[d.ip] ?? defaultPick(d);
    busy = d.ip;
    try {
      await castSonos(d.ip, id);
      devices = devices.map((x) => (x.ip === d.ip ? { ...x, stream: id } : x));
      publish();
      onToast('success', 'Casting', `${streamName(id)} → ${d.name}.`);
      try { const r = await getSonosVolume(d.ip); if (typeof r?.volume === 'number') volumes[d.ip] = r.volume; } catch (_) {}
    } catch (_) {
      onToast('amber', 'Cast failed', `Could not cast to ${d.name}.`);
    } finally {
      busy = null;
    }
  }

  async function stop(d) {
    busy = d.ip;
    try {
      await stopSonos(d.ip);
      devices = devices.map((x) => (x.ip === d.ip ? { ...x, stream: null } : x));
      publish();
      onToast('cyan', 'Stopped', `${d.name} stopped.`);
    } catch (_) {
      onToast('amber', 'Stop failed', `Could not stop ${d.name}.`);
    } finally {
      busy = null;
    }
  }

  // Next advances the queue of the stream this speaker is playing, which since
  // #130 need not be house.
  async function next(d) {
    try {
      await nextShared(d.stream);
      onToast('cyan', 'Skipped', `Advanced the ${streamName(d.stream)} queue.`);
    } catch (_) {
      onToast('amber', 'Skip failed', `Could not advance the ${streamName(d.stream)} queue.`);
    }
  }

  function setVolFrom(d, e) {
    const r = e.currentTarget.getBoundingClientRect();
    const v = Math.round(Math.max(0, Math.min(1, (e.clientX - r.left) / r.width)) * 100);
    volumes[d.ip] = v;
    clearTimeout(volTimers[d.ip]);
    volTimers[d.ip] = setTimeout(() => { setSonosVolume(d.ip, v).catch(() => {}); }, 180);
  }
  function onVolPointerDown(d, e) {
    e.currentTarget.setPointerCapture(e.pointerId);
    setVolFrom(d, e);
  }
  function onVolPointerMove(d, e) {
    if (e.buttons !== 1) return; // only while dragging
    setVolFrom(d, e);
  }

  async function addManual() {
    const ip = manualIp.trim();
    if (!ip) return;
    adding = true;
    try {
      const d = await addManualSonos(ip);
      if (!devices.some((x) => x.ip === d.ip)) apply([...devices, { ...d, stream: null }]);
      manualIp = '';
      onToast('success', 'Added', `${d.name} added manually.`);
    } catch (_) {
      onToast('amber', 'Not a Sonos', `${ip} did not answer as a Sonos device.`);
    } finally {
      adding = false;
    }
  }

  let anyCasting = $derived(devices.some((d) => d.stream));
</script>

<div style="position:relative; flex:none;">
  <button onclick={() => (open = !open)} aria-label="Cast to Sonos" aria-expanded={open}
    style="display:inline-flex; align-items:center; gap:7px; padding:7px 12px; border:1px solid {anyCasting ? 'var(--neon-cyan)' : 'var(--border-default)'}; border-radius:var(--radius-sm); background:{anyCasting ? 'rgba(34,224,238,0.06)' : 'transparent'}; font-family:var(--font-mono); font-size:11px; letter-spacing:0.1em; text-transform:uppercase; color:{anyCasting ? 'var(--neon-cyan)' : 'var(--text-muted)'}; cursor:pointer; white-space:nowrap;">
    <span style="display:inline-flex; line-height:1;"><IconCast size={15} /></span>Cast
  </button>

  {#if open}
    <div role="button" tabindex="-1" aria-label="Close" onclick={() => (open = false)} onkeydown={(e) => { if (e.key === 'Escape') open = false; }} style="position:fixed; inset:0; z-index:80;"></div>
    <div style="position:absolute; right:0; top:calc(100% + 8px); z-index:81; width:320px; max-width:calc(100vw - 28px); background:var(--bg-surface); background-image:var(--scanline); border:1.5px solid var(--neon-cyan); border-radius:var(--radius-md); box-shadow:var(--shadow-lg); padding:14px; box-sizing:border-box; display:flex; flex-direction:column; gap:12px;">
      <div style="display:flex; align-items:center; justify-content:space-between;">
        <span style="font-family:var(--font-display); font-weight:700; font-size:13px; letter-spacing:0.08em; text-transform:uppercase; color:var(--text-strong);">Cast to Sonos</span>
        <button onclick={search} disabled={searching} style="font-family:var(--font-mono); font-size:10px; letter-spacing:0.08em; text-transform:uppercase; padding:5px 9px; border:1px solid var(--border-strong); border-radius:var(--radius-sm); background:transparent; color:var(--text-body); cursor:pointer;">{searching ? 'Searching…' : 'Search'}</button>
      </div>

      {#if devices.length > 0}
        <div style="display:flex; flex-direction:column; gap:10px;">
          {#each devices as d (d.ip)}
            <div style="display:flex; flex-direction:column; gap:7px; padding:9px; border:1px solid {d.stream ? 'var(--neon-cyan)' : 'var(--border-default)'}; border-radius:var(--radius-sm);">
              <div style="display:flex; align-items:center; justify-content:space-between; gap:8px;">
                <span style="font-family:var(--font-mono); font-size:11px; color:var(--text-body); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{d.name}</span>
                <span style="font-family:var(--font-mono); font-size:10px; color:{d.stream ? 'var(--neon-cyan)' : 'var(--text-faint)'}; white-space:nowrap;">{d.stream ? streamName(d.stream) : 'idle'}</span>
              </div>
              <div style="display:flex; gap:7px;">
                <select bind:value={picked[d.ip]} aria-label="Stream for {d.name}"
                  style="flex:1; min-width:0; padding:5px 7px; border:1px solid var(--border-default); border-radius:var(--radius-sm); background:var(--ink-950); color:var(--text-body); font-family:var(--font-mono); font-size:11px;">
                  {#each streams as st (st.id)}<option value={st.id}>{st.name}</option>{/each}
                </select>
                <button onclick={() => cast(d)} disabled={busy === d.ip}
                  style="padding:5px 11px; border:1px solid var(--border-strong); border-radius:var(--radius-sm); background:transparent; color:var(--text-body); font-family:var(--font-mono); font-size:11px; letter-spacing:0.06em; text-transform:uppercase; cursor:pointer; white-space:nowrap;">Cast</button>
              </div>

              {#if d.stream}
                <div style="display:flex; align-items:center; gap:10px;">
                  <span style="color:var(--text-muted); display:inline-flex;"><IconVolume size={15} /></span>
                  <div onpointerdown={(e) => onVolPointerDown(d, e)} onpointermove={(e) => onVolPointerMove(d, e)} role="slider" tabindex="0" aria-label="Volume for {d.name}" aria-valuenow={volumes[d.ip] ?? 70} style="position:relative; flex:1; height:14px; display:flex; align-items:center; cursor:pointer; touch-action:none;">
                    <div style="position:absolute; left:0; right:0; height:4px; border-radius:var(--radius-pill); background:var(--ink-700);"></div>
                    <div style="position:absolute; left:0; width:{volumes[d.ip] ?? 70}%; height:4px; border-radius:var(--radius-pill); background:var(--neon-cyan);"></div>
                    <div style="position:absolute; left:calc({volumes[d.ip] ?? 70}% - 6px); width:12px; height:12px; border-radius:50%; background:var(--paper-100); border:2px solid var(--neon-cyan);"></div>
                  </div>
                  <span style="font-family:var(--font-mono); font-size:10px; color:var(--text-faint); width:26px; text-align:right;">{volumes[d.ip] ?? 70}</span>
                </div>
                <div style="display:flex; gap:8px;">
                  <button onclick={() => next(d)} style="flex:1; padding:6px 0; border:1px solid var(--border-strong); border-radius:var(--radius-sm); background:transparent; color:var(--text-body); font-family:var(--font-mono); font-size:11px; letter-spacing:0.08em; text-transform:uppercase; cursor:pointer; display:inline-flex; align-items:center; justify-content:center; gap:6px;">Next <IconNext size={14} /></button>
                  <button onclick={() => stop(d)} disabled={busy === d.ip} style="flex:1; padding:6px 0; border:1px solid var(--neon-magenta); border-radius:var(--radius-sm); background:transparent; color:var(--neon-magenta-bright); font-family:var(--font-mono); font-size:11px; letter-spacing:0.08em; text-transform:uppercase; cursor:pointer; display:inline-flex; align-items:center; justify-content:center; gap:6px;">Stop <IconStop size={13} /></button>
                </div>
              {/if}
            </div>
          {/each}
        </div>
      {:else}
        <span style="font-family:var(--font-mono); font-size:11px; color:var(--text-faint);">No devices yet — search the LAN or add an IP below.</span>
      {/if}

      <form onsubmit={(e) => { e.preventDefault(); addManual(); }} style="display:flex; gap:8px;">
        <input bind:value={manualIp} placeholder="SSDP blocked? Sonos IP…" aria-label="Manual Sonos IP"
          style="flex:1; min-width:0; padding:7px 10px; border:1px solid var(--border-default); border-radius:var(--radius-sm); background:var(--ink-950); color:var(--text-body); font-family:var(--font-mono); font-size:11px;" />
        <button type="submit" disabled={adding} style="padding:7px 11px; border:1px solid var(--border-strong); border-radius:var(--radius-sm); background:transparent; color:var(--text-body); font-family:var(--font-mono); font-size:11px; letter-spacing:0.06em; text-transform:uppercase; cursor:pointer;">{adding ? '…' : 'Add'}</button>
      </form>
    </div>
  {/if}
</div>
