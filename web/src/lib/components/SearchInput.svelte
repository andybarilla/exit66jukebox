<script>
  let { value = '', onInput, placeholder = '', height = '42px' } = $props();
</script>
<div class="e66-input" style="display:flex; align-items:center; gap:10px; height:{height}; padding:0 14px; background:var(--bg-inset); border:1.5px solid var(--border-strong); border-radius:var(--radius-md); transition:all var(--dur) var(--ease-out);">
  <input {value} oninput={(e) => onInput && onInput(e.target.value)} {placeholder}
    style="flex:1; min-width:0; height:100%; background:transparent; border:none; outline:none; color:var(--text-strong); font-family:var(--font-sans); font-size:15px; letter-spacing:0.01em;" />
</div>
<style>
  .e66-input:focus-within { border-color: var(--neon-cyan); box-shadow: 0 0 0 2px rgba(31,224,255,0.5); }
</style>
