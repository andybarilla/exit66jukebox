<script>
  import { onMount, onDestroy } from 'svelte';
  import { createStore } from './lib/store.svelte.js';
  import { audioURL, houseStreamURL, nextHouse, nextTrack } from './lib/api.js';
  import { fmt, keyActivate } from './lib/format.js';
  import TopBar from './lib/components/TopBar.svelte';
  import Tabs from './lib/components/Tabs.svelte';
  import AlbumGrid from './lib/components/AlbumGrid.svelte';
  import ArtistList from './lib/components/ArtistList.svelte';
  import TrackList from './lib/components/TrackList.svelte';
  import Discover from './lib/components/Discover.svelte';
  import Lineup from './lib/components/Lineup.svelte';
  import NowPlayingBar from './lib/components/NowPlayingBar.svelte';
  import MobilePlayer from './lib/components/MobilePlayer.svelte';
  import AlbumDialog from './lib/components/AlbumDialog.svelte';
  import Toast from './lib/components/Toast.svelte';
  import Login from './lib/components/Login.svelte';
  import Signup from './lib/components/Signup.svelte';
  import InviteAccept from './lib/components/InviteAccept.svelte';
  import PasswordReset from './lib/components/PasswordReset.svelte';
  import VerifyEmail from './lib/components/VerifyEmail.svelte';
  import AdminPanel from './lib/components/AdminPanel.svelte';
  import ProfilePicker from './lib/components/ProfilePicker.svelte';

  const s = createStore();
  let audio;
  let playing = $state(true);
  let volume = $state(68);
  let showSignup = $state(false);
  let showAuth = $state(false);
  let adminPanelOpen = $state(false);
  let currentPath = $state(window.location.pathname);
  const onInvitePath = $derived(currentPath.startsWith('/invite/'));
  const onResetPath = $derived(currentPath.startsWith('/reset-password/'));
  const onVerifyPath = $derived(currentPath.startsWith('/verify/'));
  const onAdminPath = $derived(currentPath === '/admin');
  const needsProfileSelection = $derived(
    s.config.requiresProfile && !onAdminPath && !s.me?.is_passwordless_profile
  );
  const needsAccountLogin = $derived(
    s.config.requiresLogin && (!s.me || s.me?.is_passwordless_profile)
  );
  let tickTimer, resizeHandler, popstateHandler;

  // Active now-playing slice + derived progress %.
  const np = $derived(s.nowPlaying);
  const dur = $derived(np?.duration || 0);
  const cur = $derived(Math.min(s.progress, dur || s.progress));
  const npPct = $derived((dur ? Math.min(100, (cur / dur) * 100) : 0) + '%');
  const streamLabel = $derived(s.stream === 'house' ? 'House' : 'Personal');
  const chip = $derived(`${streamLabel} · ${s.listeners}`);
  // Queue controls show for admins on any stream, for open-mode house guests,
  // and for any guest on their own personal stream (which the server never gates).
  const canControlHouseQueue = $derived(
    s.isAdmin || (s.config.securityMode === 'open' && s.stream === 'house')
  );
  const canControl = $derived(canControlHouseQueue || s.stream === 'me');

  // Attempt playback and let the audio element's play/pause events drive the
  // `playing` flag. A blocked autoplay rejects without firing 'pause', so the
  // catch reconciles the transport (shows Play) instead of lying that it's on.
  function tryPlay() {
    audio?.play().catch(() => { playing = false; });
  }

  // ---- personal (me) playback: client audio drives now-playing ----
  let advancing = false; // guard: stream-switch and the queued-while-idle effect
                         // can both fire; one pop per advance.
  async function advancePersonal() {
    if (advancing) return;
    advancing = true;
    try {
      const r = await nextTrack(); // POST /api/streams/me/next (pops server-side)
      if (r && r.ok && r.track) {
        s.setNowPlaying('me', normalize(r.track));
        s.setProgress('me', 0);
        audio.src = audioURL(r.track.id);
        if (playing) tryPlay();
      } else {
        s.setNowPlaying('me', null);
        playing = false;
      }
      // The pop removed the track server-side; personal has no SSE, so refresh
      // the queue ourselves to keep "up next" in sync.
      s.refreshQueue('me');
    } finally {
      advancing = false;
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
      if (playing) tryPlay();
    } else if (!s.nowPlaying) {
      advancePersonal();
    } else {
      audio.src = audioURL(s.nowPlaying.id);
      if (playing) tryPlay();
    }
  }

  // Act on the element's real state, not a flag that autoplay may have
  // desynced — otherwise the first click after a blocked autoplay is inverted.
  function togglePlay() {
    if (!audio) return;
    if (audio.paused) { playing = true; tryPlay(); } else audio.pause();
  }
  function onNext() {
    if (s.stream === 'me') advancePersonal();
    if (s.stream === 'house') nextHouse();
    // house: next is server-driven; SSE will update now-playing.
  }
  function onPrev() { s.setProgress(s.stream, 0); if (audio && s.stream === 'me') audio.currentTime = 0; }
  function onSeek(frac) {
    const t = Math.round(frac * dur);
    s.setProgress(s.stream, t);
    if (audio && s.stream === 'me') audio.currentTime = t;
  }

  // Called after login/signup/invite: adopt the user, run heavy loads (start is
  // idempotent), and drop the auth overlay.
  function afterLogin(u) {
    s.setMe(u);
    s.start();
    showAuth = false;
  }

  function replaceRoute(path) {
    window.history.replaceState(null, '', path);
    currentPath = window.location.pathname;
  }

  function openAdminRoute() {
    replaceRoute('/admin');
    if (s.isAdmin) adminPanelOpen = true;
    else showAuth = true;
  }

  onMount(async () => {
    // Lightweight auth/config check first; only run heavy loads once access is
    // granted (logged in or guest access on), so they never 401 on the gate.
    await s.bootstrap();
    if ((s.me || s.config.guestAccess) && !s.config.requiresProfile && !needsAccountLogin) await s.start();
    s.onResize();
    resizeHandler = () => s.onResize();
    popstateHandler = () => { currentPath = window.location.pathname; };
    window.addEventListener('resize', resizeHandler);
    window.addEventListener('popstate', popstateHandler);
    if (audio) audio.volume = volume / 100;
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
    if (audio) {
      audio.addEventListener('ended', () => { if (s.stream === 'me') advancePersonal(); });
      // Reflect the element's real state so the transport never lies about
      // whether sound is coming out (autoplay block, stall, manual pause).
      audio.addEventListener('play', () => { playing = true; });
      audio.addEventListener('pause', () => { playing = false; });
    }
  });
  onDestroy(() => {
    clearInterval(tickTimer);
    window.removeEventListener('resize', resizeHandler);
    window.removeEventListener('popstate', popstateHandler);
    s.teardown();
    if (audio) { audio.pause(); audio.src = ''; }
  });

  // When a user logs in (any path), ensure heavy loads have run (start() is
  // guarded/idempotent) and dismiss the auth overlay.
  $effect(() => {
    if (s.me && !needsProfileSelection && !needsAccountLogin) { s.start(); showAuth = false; }
  });

  $effect(() => {
    if (onAdminPath && s.authChecked && s.isAdmin) adminPanelOpen = true;
  });

  // re-apply audio when the user switches streams
  let lastStream = 'house';
  $effect(() => {
    if (s.stream !== lastStream) { lastStream = s.stream; playing = true; applyStreamAudio(); }
  });

  // Personal is client-driven: a track queued while nothing is playing must
  // kick playback off itself. (House is server-driven and needs no nudge — the
  // hub pops the queue on its own.)
  $effect(() => {
    if (s.stream === 'me' && !s.nowPlaying && s.queue.length > 0) {
      playing = true; // queuing into an idle stream is an intent to play
      advancePersonal();
    }
  });

  // Mute the local <audio> while a Sonos cast is active (gated by config).
  // Muting, not pausing — the timeline keeps running so resuming is seamless and
  // the volume value is untouched.
  $effect(() => {
    if (audio) audio.muted = s.muteLocalOnCast && s.castActive;
  });

  // Load discover data when switching to the discover tab.
  let lastTab = s.tab;
  $effect(() => {
    if (s.tab === 'discover' && s.tab !== lastTab) s.loadDiscover();
    lastTab = s.tab;
  });
</script>

<div style="position:relative; height:100vh; width:100%; display:flex; flex-direction:column; overflow:hidden; box-sizing:border-box; background:var(--grid-glow), var(--bg-base); font-family:var(--font-sans); color:var(--text-body);">

{#if !s.authChecked}
  <!-- waiting for the /me round-trip — render a minimal splash -->
  <div style="flex:1;"></div>
{:else if onInvitePath}
  <InviteAccept onLoggedIn={(u) => { s.setMe(u); s.start(); replaceRoute('/'); }} />
{:else if onResetPath}
  <PasswordReset onComplete={() => replaceRoute('/')} />
{:else if onVerifyPath}
  <VerifyEmail onComplete={() => replaceRoute('/')} />
{:else if onAdminPath && !s.isAdmin}
  <Login canSignup={false} onSwitchToSignup={() => (showSignup = false)} onLoggedIn={afterLogin} />
{:else if needsProfileSelection}
  <ProfilePicker onLoggedIn={afterLogin} />
{:else if needsAccountLogin || showAuth}
  {#if showSignup}
    <Signup onLoggedIn={afterLogin} onSwitchToLogin={() => (showSignup = false)} />
  {:else}
    <Login canSignup={s.config.signupEnabled || s.config.needsBootstrap}
           onSwitchToSignup={() => (showSignup = true)}
           onLoggedIn={afterLogin} />
  {/if}
{:else}
  <TopBar isPhone={s.isPhone} query={s.query} onSearch={(v) => (s.query = v)}
    streamChipLabel={chip} onToggleStream={() => s.toggleStream()} scan={s.scan}
    onToast={(tone, title, msg) => s.pushToast(tone, title, msg)}
    onCastActive={(v) => s.setCastActive(v)}
    isAdmin={s.isAdmin} me={s.me} onLogout={() => s.signOut()}
    onOpenSettings={openAdminRoute} onLogin={() => (showAuth = true)} />

  <!-- BODY -->
  <div style="display:flex; flex:1; min-height:0;">
    <main style="flex:1; min-width:0; display:flex; flex-direction:column; padding:18px 22px; box-sizing:border-box;">
      <div style="display:flex; align-items:center; justify-content:space-between; gap:14px; margin-bottom:16px;">
        <Tabs tab={s.tab} onTab={(t) => (s.tab = t)} />
        {#if s.tab !== 'discover'}
          <span style="font-family:var(--font-mono); font-size:11px; letter-spacing:0.16em; text-transform:uppercase; color:var(--text-faint); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">{s.currentCount} in the crate</span>
        {/if}
      </div>

      <div style="flex:1; min-height:0; overflow-y:auto; margin-right:-8px; padding-right:8px;">
        {#if s.tab === 'discover'}
          <Discover
            genres={s.discoverGenres}
            selectedGenre={s.discoverSelectedGenre}
            onGenre={(g) => s.setDiscoverGenre(g)}
            rediscover={s.discoverRediscover}
            recent={s.discoverRecent}
            recommended={s.discoverRecommended}
            nowPlayingId={np?.id}
            onAdd={(t) => s.requestTrack(t)}
            station={s.discoverStation}
            onStartStation={(g) => s.startStation(g)}
            onStopStation={() => s.stopStation()}
          />
        {:else if !s.loading && s.currentCount === 0}
          <div style="display:flex; flex-direction:column; align-items:center; justify-content:center; gap:10px; text-align:center; padding:70px 20px;">
            <div style="font-family:var(--font-display); font-weight:700; font-size:18px; letter-spacing:0.04em; text-transform:uppercase; color:var(--text-muted);">No matches on this side</div>
            <div style="font-family:var(--font-sans); font-size:14px; color:var(--text-faint); max-width:300px;">Nothing in the crate matches that. Try another artist, album, or slot code.</div>
          </div>
        {:else if s.tab === 'albums'}
          <AlbumGrid cards={s.albumCards} onOpen={(a) => s.openAlbum(a)} onRequest={(a) => s.requestAlbum(a)} onLoadMore={() => s.loadMore()} />
        {:else if s.tab === 'artists'}
          <ArtistList rows={s.artistRows} onOpen={(a) => s.openArtist(a)} onRequest={(a) => s.requestArtist(a)} onLoadMore={() => s.loadMore()} />
        {:else}
          <TrackList tracks={s.trackRows} nowPlayingId={np?.id} onAdd={(t) => s.requestTrack(t)} onLoadMore={() => s.loadMore()} />
        {/if}
      </div>
    </main>

    {#if !s.isPhone}
      <aside style="width:328px; flex:none; padding:18px 18px 18px 0; min-height:0; display:flex;">
        <div style="flex:1; min-height:0; display:flex; background:var(--bg-surface); background-image:var(--scanline); border:1.5px solid var(--neon-magenta); box-shadow:var(--shadow-lg), var(--glow-soft-magenta); border-radius:var(--radius-lg); padding:16px; box-sizing:border-box;">
          <Lineup streamLabel={streamLabel} listeners={s.listeners} shuffle={s.shuffle}
            onToggleShuffle={(v) => s.toggleShuffle(v)} np={np} npPct={npPct}
            queue={s.queue} isPhone={false} canControl={canControl} onRemove={(q) => s.removeFromQueue(q)}
            onOpenAlbum={(item) => s.openAlbum({ id: item.albumId, name: item.albumName, artistName: item.artistName })} />
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
          albumId={np?.albumId} onOpenAlbum={np?.albumId ? () => s.openAlbum({ id: np.albumId, name: np.albumName, artistName: np.artistName }) : undefined}
          current={cur} duration={dur} {playing} {volume} canSkip={canControl}
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
    <MobilePlayer {np} {npPct} {playing} canSkip={canControl} onPlayPause={togglePlay} onNext={onNext}
      onOpenAlbum={np?.albumId ? () => s.openAlbum({ id: np.albumId, name: np.albumName, artistName: np.artistName }) : undefined} />
  {/if}

  <!-- PHONE LINEUP FAB -->
  {#if s.isPhone && !s.lineupOpen}
    <button onclick={() => s.openLineup()} style="position:absolute; right:16px; bottom:86px; z-index:60; height:46px; padding:0 16px; border-radius:var(--radius-md); background:var(--neon-magenta); color:var(--text-on-accent); border:none; box-shadow:var(--shadow-lg), var(--glow-soft-magenta); font-family:var(--font-display); font-weight:600; font-size:13px; letter-spacing:0.08em; text-transform:uppercase; display:inline-flex; align-items:center; gap:9px; cursor:pointer;">The Lineup<span style="font-family:var(--font-mono); font-weight:700; font-size:12px; padding:2px 7px; border-radius:var(--radius-sm); background:rgba(11,11,20,0.25);">{s.queue.length}</span></button>
  {/if}

  <!-- PHONE LINEUP SHEET -->
  {#if s.isPhone && s.lineupOpen}
    <div style="position:absolute; inset:0; z-index:75; display:flex; flex-direction:column; justify-content:flex-end;">
      <div role="button" tabindex="-1" aria-label="Close" onclick={() => s.closeLineup()} onkeydown={keyActivate(() => s.closeLineup())} style="position:absolute; inset:0; background:rgba(6,6,11,0.72); backdrop-filter:blur(6px);"></div>
      <div style="position:relative; height:74vh; background:var(--bg-surface); background-image:var(--scanline); border-top:1.5px solid var(--neon-magenta); border-radius:var(--radius-lg) var(--radius-lg) 0 0; padding:18px; box-shadow:var(--shadow-xl); display:flex; box-sizing:border-box;">
        <Lineup streamLabel={streamLabel} listeners={s.listeners} shuffle={s.shuffle}
          onToggleShuffle={(v) => s.toggleShuffle(v)} np={np} npPct={npPct}
          queue={s.queue} isPhone={true} canControl={canControl} onClose={() => s.closeLineup()} onRemove={(q) => s.removeFromQueue(q)}
          onOpenAlbum={(item) => s.openAlbum({ id: item.albumId, name: item.albumName, artistName: item.artistName })} />
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

  {#if adminPanelOpen}
    <AdminPanel onClose={() => { adminPanelOpen = false; if (onAdminPath) replaceRoute('/'); }} />
  {/if}
{/if}
</div>
