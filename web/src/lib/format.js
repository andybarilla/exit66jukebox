// keyActivate wraps a click handler so Enter/Space also fire it, for
// role="button" divs that can't be real <button>s (they nest buttons).
export function keyActivate(fn) {
  return (e) => {
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); fn(e); }
  };
}

// fmt formats seconds as m:ss (matches the design's NowPlayingBar).
export function fmt(sec) {
  if (sec == null || isNaN(sec)) return '0:00';
  const m = Math.floor(sec / 60);
  const s = Math.floor(sec % 60);
  return `${m}:${s < 10 ? '0' : ''}${s}`;
}

// compareNames orders artist/album names by library alphabetization rules:
// a leading "The"/"A"/"An" article and any leading punctuation are dropped from
// the sort key, and comparison is case- and accent-insensitive with natural
// numeric ordering ("Album 2" before "Album 10").
const ARTICLE = /^(the|a|an)\s+/i;
function sortName(s) {
  return (s || '').replace(/^[^\p{L}\p{N}]+/u, '').replace(ARTICLE, '');
}
export function compareNames(a, b) {
  return sortName(a).localeCompare(sortName(b), undefined, { sensitivity: 'base', numeric: true });
}

// gradientFor returns a deterministic cover gradient for an id (fallback art).
export function gradientFor(id) {
  const pairs = [
    ['#0b2e3a', '#1aa6c2'], ['#2a0f1f', '#c41e6b'], ['#2a1d08', '#c98a1e'],
    ['#17112e', '#5e3fd6'], ['#2a1208', '#b8431e'], ['#141033', '#3b5bd6'],
    ['#0a2230', '#3a6fb0'], ['#241024', '#8a1f4a'],
  ];
  const [a, b] = pairs[Math.abs(Number(id)) % pairs.length];
  return `radial-gradient(circle at 28% 22%, rgba(255,255,255,0.16), transparent 46%), linear-gradient(150deg, ${a} 0%, ${b} 100%)`;
}

// toneGradient maps an art-tone name to the TrackRow/QueueItem tile gradient.
export function toneGradient(tone) {
  return {
    magenta: 'linear-gradient(135deg, #2a0f1f, #ff2e88 280%)',
    cyan: 'linear-gradient(135deg, #08252b, #1fe0ff 280%)',
    amber: 'linear-gradient(135deg, #2a1d08, #ffb02e 280%)',
    violet: 'linear-gradient(135deg, #17112e, #8a6cff 280%)',
  }[tone] || 'linear-gradient(135deg, #2a0f1f, #ff2e88 280%)';
}
