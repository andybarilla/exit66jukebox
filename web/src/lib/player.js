import { nextTrack, audioURL } from './api.js';

// Player wires an <audio> element to the private queue: when a track ends, it
// fetches the next one. Returns a start() that kicks off playback.
export function createPlayer(audio, onNowPlaying) {
  async function playNext() {
    const res = await nextTrack();
    if (res.ok) {
      audio.src = audioURL(res.track.id);
      audio.play();
      onNowPlaying(res.track);
    } else {
      onNowPlaying(null);
      setTimeout(playNext, 2000); // queue empty: poll
    }
  }
  audio.addEventListener('ended', playNext);
  return { start: playNext };
}
