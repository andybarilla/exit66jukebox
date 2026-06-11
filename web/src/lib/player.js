import { nextTrack, audioURL } from './api.js';

// createPlayer wires an <audio> element to the private queue (local mode): when a
// track ends it fetches the next. Returns { start, stop }; stop() tears down the
// poll loop and listener so the player can be replaced when switching modes.
export function createPlayer(audio, onNowPlaying) {
  let stopped = false;
  let timer = null;

  async function playNext() {
    if (stopped) return;
    try {
      const res = await nextTrack();
      if (stopped) return;
      if (res.ok) {
        audio.src = audioURL(res.track.id);
        audio.play().catch(() => onNowPlaying(null));
        onNowPlaying(res.track);
      } else {
        onNowPlaying(null);
        timer = setTimeout(playNext, 2000);
      }
    } catch (_) {
      timer = setTimeout(playNext, 5000);
    }
  }

  audio.addEventListener('ended', playNext);

  function stop() {
    stopped = true;
    if (timer) clearTimeout(timer);
    audio.removeEventListener('ended', playNext);
  }

  return { start: playNext, stop };
}
