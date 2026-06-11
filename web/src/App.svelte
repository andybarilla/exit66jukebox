<script>
  import { onMount, onDestroy } from 'svelte';
  import { createStore } from './lib/store.svelte.js';
  import { audioURL, houseStreamURL, nextTrack } from './lib/api.js';
  import { fmt } from './lib/format.js';
  import TopBar from './lib/components/TopBar.svelte';
  import Tabs from './lib/components/Tabs.svelte';
  import AlbumGrid from './lib/components/AlbumGrid.svelte';
  import ArtistList from './lib/components/ArtistList.svelte';
  import TrackList from './lib/components/TrackList.svelte';
  import Lineup from './lib/components/Lineup.svelte';
  import NowPlayingBar from './lib/components/NowPlayingBar.svelte';
  import MobilePlayer from './lib/components/MobilePlayer.svelte';
  import AlbumDialog from './lib/components/AlbumDialog.svelte';
  import Toast from './lib/components/Toast.svelte';

  const s = createStore();
  let audio;
  let playing = $state(true);
  let volume = $state(68);
  let tickTimer, resizeHandler;

  // Active now-playing slice + derived progress %.
  const np = $derived(s.nowPlaying);
  const dur = $derived(np?.duration || 0);
  const cur = $derived(Math.min(s.progress, dur || s.progress));
  const npPct = $derived((dur ? Math.min(100, (cur / dur) * 100) : 0) + '%');
  const streamLabel = $derived(s.stream === 'house' ? 'House' : 'Personal');
  const chip = $derived(`${streamLabel} · ${s.listeners}`);

  // ---- personal (me) playback: client audio drives now-playing ----
  async function advancePersonal() {
    const r = await nextTrack(); // GET /api/streams/me/next
    if (r && r.ok && r.track) {
      s.setNowPlaying('me', normalize(r.track));
      s.setProgress('me', 0);
      audio.src = audioURL(r.track.id);
      if (playing) audio.play().catch(() => {});
    } else {
      s.setNowPlaying('me', null);
      playing = false;
    }
  }
  function normalize(t) {
    // mirror store.normalizeNP via exported helper if present; minimal inline:
    return { id: t.id, title: t.title, duration: t.duration || 0, ...s.npMeta(t) };
  }

  function applyStreamAudio() {
    if (!audio) return;
    if (s.stream === 'house') {
      audio.src = houseStreamURL();
      if (playing) audio.play().catch(() => {});
    } else if (!s.nowPlaying) {
      advancePersonal();
    } else {
      audio.src = audioURL(s.nowPlaying.id);
      if (playing) audio.play().catch(() => {});
    }
  }

  function togglePlay() {
    playing = !playing;
    if (!audio) return;
    if (playing) audio.play().catch(() => {}); else audio.pause();
  }
  function onNext() {
    if (s.stream === 'me') advancePersonal();
    // house: next is server-driven; SSE will update now-playing.
  }
  function onPrev() { s.setProgress(s.stream, 0); if (audio && s.stream === 'me') audio.currentTime = 0; }
  function onSeek(frac) {
    const t = Math.round(frac * dur);
    s.setProgress(s.stream, t);
    if (audio && s.stream === 'me') audio.currentTime = t;
  }

  onMount(async () => {
    await s.init();
    s.onResize();
    resizeHandler = () => s.onResize();
    window.addEventListener('resize', resizeHandler);
    applyStreamAudio();
    // 1s tick: personal reads exact audio time; house approximates.
    tickTimer = setInterval(() => {
      if (!playing || !s.nowPlaying) return;
      if (s.stream === 'me' && audio && !audio.paused) {
        s.setProgress('me', audio.currentTime);
      } else {
        s.setProgress(s.stream, s.progress + 1);
      }
    }, 1000);
    if (audio) audio.addEventListener('ended', () => { if (s.stream === 'me') advancePersonal(); });
  });
  onDestroy(() => {
    clearInterval(tickTimer);
    window.removeEventListener('resize', resizeHandler);
    s.teardown();
    if (audio) { audio.pause(); audio.src = ''; }
  });

  // re-apply audio when the user switches streams
  let lastStream = 'house';
  $effect(() => {
    if (s.stream !== lastStream) { lastStream = s.stream; playing = true; applyStreamAudio(); }
  });
</script>

<div style="position:relative; height:100vh; width:100%; display:flex; flex-direction:column; overflow:hidden; box-sizing:border-box; background:var(--grid-glow), var(--bg-base); font-family:var(--font-sans); color:var(--text-body);">

  <TopBar isPhone={s.isPhone} query={s.query} onSearch={(v) => (s.query = v)}
    streamChipLabel={chip} onToggleStream={() => s.toggleStream()} />

  <!-- BODY -->
  <div style="display:flex; flex:1; min-height:0;">
    <main style="flex:1; min-width:0; display:flex; flex-direction:column; padding:18px 22px; box-sizing:border-box;">
      <div style="display:flex; align-items:center; justify-content:space-between; gap:14px; margin-bottom:16px;">
        <Tabs tab={s.tab} onTab={(t) => (s.tab = t)} />
        <span style="font-family:var(--font-mono); font-size:11px; letter-spacing:0.16em; text-transform:uppercase; color:var(--text-faint); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{s.currentCount} in the crate</span>
      </div>

      <div style="flex:1; min-height:0; overflow-y:auto; margin-right:-8px; padding-right:8px;">
        {#if s.currentCount === 0}
          <div style="display:flex; flex-direction:column; align-items:center; justify-content:center; gap:10px; text-align:center; padding:70px 20px;">
            <div style="font-family:var(--font-display); font-weight:700; font-size:18px; letter-spacing:0.04em; text-transform:uppercase; color:var(--text-muted);">No matches on this side</div>
            <div style="font-family:var(--font-sans); font-size:14px; color:var(--text-faint); max-width:300px;">Nothing in the crate matches that. Try another artist, album, or slot code.</div>
          </div>
        {:else if s.tab === 'albums'}
          <AlbumGrid cards={s.albumCards} onOpen={(a) => s.openAlbum(a.id)} onRequest={(a) => s.requestAlbum(a)} />
        {:else if s.tab === 'artists'}
          <ArtistList rows={s.artistRows} onOpen={(a) => s.openArtist(a)} onRequest={(a) => s.requestArtist(a)} />
        {:else}
          <TrackList tracks={s.trackRows} nowPlayingId={np?.id} onAdd={(t) => s.requestTrack(t)} />
        {/if}
      </div>
    </main>

    {#if !s.isPhone}
      <aside style="width:328px; flex:none; padding:18px 18px 18px 0; min-height:0; display:flex;">
        <div style="flex:1; min-height:0; display:flex; background:var(--bg-surface); background-image:var(--scanline); border:1.5px solid var(--neon-magenta); box-shadow:var(--shadow-lg), var(--glow-soft-magenta); border-radius:var(--radius-lg); padding:16px; box-sizing:border-box;">
          <Lineup streamLabel={streamLabel} listeners={s.listeners} shuffle={s.shuffle}
            onToggleShuffle={(v) => s.toggleShuffle(v)} np={np} npPct={npPct}
            queue={s.queue} isPhone={false} onRemove={(q) => s.removeFromQueue(q)} />
        </div>
      </aside>
    {/if}
  </div>

  <!-- DESKTOP PLAYER -->
  {#if !s.isPhone}
    <div style="flex:none; display:flex; align-items:stretch;">
      <div style="flex:1; min-width:0;">
        <NowPlayingBar title={np?.title || 'Nothing playing'} artist={np?.artistName || '—'}
          code={np?.code || 'A6'} cover={np?.cover} gradient={np?.gradient} tone={np?.tone || 'magenta'}
          current={cur} duration={dur} {playing} {volume}
          onPlayPause={togglePlay} onPrev={onPrev} onNext={onNext} onSeek={onSeek}
          onVolume={(v) => { volume = v; if (audio) audio.volume = v / 100; }} />
      </div>
      <div style="width:220px; flex:none; border-top:1px solid var(--border-strong); border-left:1px solid var(--border-default); background:var(--bg-surface-raised); background-image:var(--scanline); display:flex; flex-direction:column; justify-content:center; gap:8px; padding:0 18px; box-sizing:border-box;">
        <span style="font-family:var(--font-mono); font-size:9px; letter-spacing:0.22em; text-transform:uppercase; color:var(--text-faint);">Stream</span>
        <div style="display:inline-flex; border:1px solid var(--border-strong); border-radius:var(--radius-sm); overflow:hidden; width:fit-content;">
          <button onclick={() => s.setStream('house')} style="padding:5px 12px; border:none; cursor:pointer; font-family:var(--font-mono); font-size:11px; font-weight:700; letter-spacing:0.08em; text-transform:uppercase; background:{s.stream === 'house' ? 'var(--neon-cyan)' : 'transparent'}; color:{s.stream === 'house' ? 'var(--text-on-accent)' : 'var(--text-muted)'};">House</button>
          <button onclick={() => s.setStream('me')} style="padding:5px 12px; border:none; cursor:pointer; font-family:var(--font-mono); font-size:11px; font-weight:700; letter-spacing:0.08em; text-transform:uppercase; background:{s.stream === 'me' ? 'var(--neon-cyan)' : 'transparent'}; color:{s.stream === 'me' ? 'var(--text-on-accent)' : 'var(--text-muted)'};">Personal</button>
        </div>
        <span style="display:inline-flex; align-items:center; gap:7px; font-family:var(--font-mono); font-size:10px; letter-spacing:0.1em; color:var(--text-faint);"><span style="width:6px; height:6px; border-radius:50%; background:var(--neon-cyan);"></span>{s.listeners} TUNED IN</span>
      </div>
    </div>
  {/if}

  <!-- MOBILE PLAYER -->
  {#if s.isPhone}
    <MobilePlayer {np} {npPct} {playing} onPlayPause={togglePlay} onNext={onNext} />
  {/if}

  <!-- PHONE LINEUP FAB -->
  {#if s.isPhone && !s.lineupOpen}
    <button onclick={() => s.openLineup()} style="position:absolute; right:16px; bottom:86px; z-index:60; height:46px; padding:0 16px; border-radius:var(--radius-md); background:var(--neon-magenta); color:var(--text-on-accent); border:none; box-shadow:var(--shadow-lg), var(--glow-soft-magenta); font-family:var(--font-display); font-weight:600; font-size:13px; letter-spacing:0.08em; text-transform:uppercase; display:inline-flex; align-items:center; gap:9px; cursor:pointer;">The Lineup<span style="font-family:var(--font-mono); font-weight:700; font-size:12px; padding:2px 7px; border-radius:var(--radius-sm); background:rgba(11,11,20,0.25);">{s.queue.length}</span></button>
  {/if}

  <!-- PHONE LINEUP SHEET -->
  {#if s.isPhone && s.lineupOpen}
    <div style="position:absolute; inset:0; z-index:75; display:flex; flex-direction:column; justify-content:flex-end;">
      <div onclick={() => s.closeLineup()} style="position:absolute; inset:0; background:rgba(6,6,11,0.72); backdrop-filter:blur(6px);"></div>
      <div style="position:relative; height:74vh; background:var(--bg-surface); background-image:var(--scanline); border-top:1.5px solid var(--neon-magenta); border-radius:var(--radius-lg) var(--radius-lg) 0 0; padding:18px; box-shadow:var(--shadow-xl); display:flex; box-sizing:border-box;">
        <Lineup streamLabel={streamLabel} listeners={s.listeners} shuffle={s.shuffle}
          onToggleShuffle={(v) => s.toggleShuffle(v)} np={np} npPct={npPct}
          queue={s.queue} isPhone={true} onClose={() => s.closeLineup()} onRemove={(q) => s.removeFromQueue(q)} />
      </div>
    </div>
  {/if}

  <!-- ALBUM DIALOG -->
  <AlbumDialog album={s.detailAlbum} nowPlayingId={np?.id}
    onClose={() => s.closeAlbum()} onRequestAll={() => { s.requestAlbum(s.detailAlbum); s.closeAlbum(); }}
    onAddTrack={(t) => s.requestTrack(t)} />

  <!-- TOASTS -->
  <div style="position:absolute; top:16px; right:16px; display:flex; flex-direction:column; gap:10px; z-index:95; max-width:320px;">
    {#each s.toasts as t (t.id)}
      <div style="animation:e66-toast-in .28s var(--ease-out);">
        <Toast tone={t.tone} title={t.title} message={t.message} onClose={() => s.dismissToast(t.id)} />
      </div>
    {/each}
  </div>

  <audio bind:this={audio} style="display:none;"></audio>
</div>
