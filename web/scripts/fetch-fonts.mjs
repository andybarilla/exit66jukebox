// Run once: node scripts/fetch-fonts.mjs
// Downloads latin .woff2 for the three brand families into public/fonts/.
import { mkdir, writeFile } from 'node:fs/promises';

const FAMILIES = [
  { css: 'Chakra+Petch:wght@400;500;600;700', slug: 'chakra-petch' },
  { css: 'Space+Grotesk:wght@400;500;600;700', slug: 'space-grotesk' },
  { css: 'Space+Mono:wght@400;700', slug: 'space-mono' },
];
const UA =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36';

await mkdir('public/fonts', { recursive: true });
const faces = [];
for (const f of FAMILIES) {
  const css = await (
    await fetch(`https://fonts.googleapis.com/css2?family=${f.css}&display=swap`, {
      headers: { 'User-Agent': UA },
    })
  ).text();
  // Each @font-face block: capture weight + the latin woff2 url.
  const blocks = css.split('@font-face').slice(1);
  for (const b of blocks) {
    const url = b.match(/url\((https:[^)]+\.woff2)\)/)?.[1];
    const weight = b.match(/font-weight:\s*(\d+)/)?.[1] ?? '400';
    if (!url) continue;
    // One file per (family,weight); later unicode-range blocks overwrite with
    // the same name, leaving a working latin glyph set.
    const name = `${f.slug}-${weight}.woff2`;
    const buf = Buffer.from(await (await fetch(url)).arrayBuffer());
    await writeFile(`public/fonts/${name}`, buf);
    faces.push(name);
  }
}
console.log('saved', [...new Set(faces)].sort().join(', '));
