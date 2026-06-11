<script>
  let { name = '', size = 'md', ring = 'none' } = $props();
  const DIMS = { xs: 24, sm: 32, md: 40, lg: 56, xl: 80 };
  const RINGS = {
    none: 'none',
    magenta: '0 0 0 2px var(--bg-base), 0 0 0 4px var(--neon-magenta)',
    cyan: '0 0 0 2px var(--bg-base), 0 0 0 4px var(--neon-cyan)',
    amber: '0 0 0 2px var(--bg-base), 0 0 0 4px var(--neon-amber)',
  };
  const d = $derived(DIMS[size] || DIMS.md);
  const initials = $derived(name.split(' ').map((w) => w[0]).filter(Boolean).slice(0, 2).join('').toUpperCase());
</script>
<div style="width:{d}px; height:{d}px; flex:none; border-radius:var(--radius-pill); box-shadow:{RINGS[ring] || RINGS.none}; background:linear-gradient(140deg, var(--ink-700), var(--ink-850)); display:inline-flex; align-items:center; justify-content:center; overflow:hidden; font-family:var(--font-mono); font-weight:700; font-size:{d * 0.36}px; color:var(--paper-200); letter-spacing:0.02em;">{initials || '·'}</div>
