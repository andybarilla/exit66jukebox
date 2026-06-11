# Exit 66 Jukebox — Design System

**Neon-noir meets Route 66.** Exit 66 Jukebox is a social, venue-based digital jukebox: patrons "tap in" at a bar or club, browse the venue's crate, queue tracks, and **boost** their picks with credits so the crowd's favorites rise to the top of the lineup. The aesthetic is a deliberate collision — the warm Americana of a roadside diner jukebox (chrome, amber highway signage, Route 66 nostalgia) shot through a **cyberpunk** lens (deep blue-black surfaces, hot neon, scanline texture, terminal type).

> **Sources:** This system was authored from a one-line brand brief ("Exit 66 Jukebox: modern with a cyberpunk edge") with **no attached codebase, Figma, or decks**. Everything here — palette, type, components, the app UI kit — is an original interpretation of that brief. If you have real brand assets (logo files, fonts, product screens, a codebase), attach them and we'll reconcile this system to them.

---

## Content fundamentals — how Exit 66 talks

The voice is **late-night, confident, a little neon-lit.** It speaks to the patron as a player in the room, not a passive listener.

- **Person:** Second person, "you." The app is the venue talking to you: *"You're at The Last Offramp. Queue a track, boost it, hold the floor."*
- **Tone:** Punchy and physical. Short imperatives — *"Drop the needle." "Tap in." "Hold the floor."* Music and highway metaphors run throughout (Side A / Side B, the crate, the lineup, the offramp).
- **Casing:** **Display headings and labels are UPPERCASE**, tracked out like marquee lettering. Body copy is sentence case. Mono micro-labels (codes, counts, timers) are uppercase and widely tracked.
- **Jukebox vocabulary (use consistently):**
  - **The Crate** — the browsable library of tracks.
  - **The Lineup** — the play queue.
  - **Slot code** — every track has a 2-char code (`A6`, `B2`) like a real jukebox selector.
  - **Credits (◈)** — the currency you spend to **boost** a track up the lineup.
  - **Side A / Side B** — sections / the front and back of the queue.
  - **Tap in** — entering / joining a venue.
  - **On Air / Now serving** — the live status.
- **Emoji:** **None.** Never use emoji. The "icon" texture comes from the `◈` credit glyph, mono labels, and the Lucide line-icon set.
- **Numbers & data:** Lean. Show the slot code, the time, the credit count — nothing decorative. No vanity stats.

**Examples**
- Confirmation: *"Queued — Midnight Loop joined the lineup at #4."*
- Empty state: *"The floor is yours. Head to the crate and drop the first track."*
- Error: *"Out of credits. Top up to boost this track."*
- Marquee: *"EXIT 66 · SIDE A"*

---

## Visual foundations

**Color.** Deep, cool **blue-black** backgrounds (`--ink-980 → --ink-650`, e.g. `#06060B`, `#0B0B14`, `#10101B`) carry the whole system. Light comes from four **neons**: **magenta `#FF2E88`** (primary — actions, the brand badge, the now-playing accent), **cyan `#1FE0FF`** (secondary — focus, links, info, volume), **amber `#FFB02E`** (Route 66 warmth — credits, the tagline, explicit flags), and **violet `#8A6CFF`** (tertiary art accent). Text is **warm white** (`--paper-100 #F6F3EB`) stepping down to muted greys. Neons are used as *light*, not fill — small surfaces, edges, glows — never large flat fields.

**Type.** Three families:
- **Chakra Petch** (display/headings) — angular, semi-condensed, techy. Always uppercase + tracked for that neon-sign feel.
- **Space Grotesk** (UI/body) — modern geometric sans, reads clean on dark.
- **Space Mono** (data) — slot codes, timers, credit counts, micro-labels; uppercase, very wide tracking (0.24em).
Scale runs 11 → 88px. (Fonts load from Google Fonts CDN — see Caveats.)

**Backgrounds.** Never plain. Three signature treatments: `--grid-glow` (a faint magenta radial from the top), a **dual-neon wash** (magenta from one corner, cyan from another), and a faint **scanline** overlay (`--scanline`) applied to cards and bars for CRT texture. Full-bleed raster imagery is *not* used — album art is represented by neon **slot-code tiles** (a gradient chip stamped with the track's code), reinforcing the jukebox metaphor.

**HUD edges are the signature.** Feedback is delivered with **crisp flat neon strokes — no bloom.** Interactive, live, and selected elements get a 1.5px neon ring (`--glow-magenta/-cyan/-amber`, all zero-blur); the most important containers (the now-playing marquee, the brand badge, the active nav) get a **double frame** — an inner gap then an outer neon stroke (`--hud-double-*`). Selected/active states resolve to a **solid neon fill** with dark text. Headings stay flat (the `--text-glow-*` tokens are `none`). The texture comes from sharp corners, double frames, mono labels and scanlines — not light bloom. Stroke weights are tokenized: `--stroke` 1.5px, `--stroke-bold` 2px.

**Motion.** Quick and snappy. `--dur 200ms` default with `--ease-out`; toggles and jukebox buttons use `--ease-snap` (a slight overshoot). Fades + small translates for toasts and tooltips. The one ambient animation is the **equalizer** on the Now Playing screen (bars pulse while playing). Respect reduced-motion in production.

**States.**
- *Hover:* brighten the neon (magenta → magenta-bright) and add a crisp 1.5px ring; neutral surfaces lift to `--bg-surface-hover`.
- *Press:* `translateY(1px)`, no scale.
- *Focus:* crisp cyan double-ring, no blur (`--focus-shadow`).
- *Disabled:* 40% opacity, no ring.

**Borders, radii, shadows.** Crisp **1.5px** strokes (`--stroke`) carry the look; hairline borders (`--line` = white @ 12%) divide neutral content. **Sharp, machined radii** — 0/2/3/5/8px (terminal/HUD feel); pills are reserved for circular avatars and toggle knobs. Depth shadows are near-black and structural only (`--shadow-md/lg/xl`) — color never blurs.d deep (`--shadow-md/lg/xl`); they sit *under* the neon glow, never compete with it.

**Cards.** Surface `--bg-surface` (`#10101B`) + faint scanline + hairline border + `--radius-lg` (5px). Interactive cards lift 2px and snap a crisp neon ring on their edge on hover. No colored-left-border cards, no soft pastel gradients.

**Transparency & blur.** Used sparingly and with intent: modal scrims are `rgba(6,6,11,0.72)` + `backdrop-filter: blur(6px)`; soft neon fills are low-alpha tints of the accent (e.g. `rgba(255,46,136,0.10)`).

---

## Iconography

- **System:** [**Lucide**](https://lucide.dev) — clean 2px line icons that match the system's hairline weight. Loaded from CDN (`unpkg.com/lucide`) and rendered via `<i data-lucide="name"></i>` + `lucide.createIcons()`. *(Substitution flag: with no brand icon set provided, Lucide is the chosen stand-in — swap if you have a house set.)*
- **Common glyphs:** `play / pause / skip-back / skip-forward / shuffle` (transport), `search / disc-3 / list-music / radio` (nav), `zap` (boost), `plus / check-check / trash-2` (actions), `map-pin` (venue).
- **Credit glyph:** `◈` (a Unicode diamond) is the brand credit symbol — used in `CreditMeter`, queue boost badges, and copy.
- **Transport glyphs** inside `NowPlayingBar` use Unicode media chars (`▶ ❚❚ ⏮ ⏭`) so the component stays dependency-free.
- **Emoji:** never.
- **Logo/brand:** `assets/logo.svg` (full wordmark lockup) and `assets/mark.svg` (the `66` badge). Both are typographic SVGs referencing the brand fonts.

---

## Index — what's in this system

**Foundations (root)**
- `styles.css` — the single entry point consumers link. `@import`s everything below.
- `tokens/` — `colors.css`, `typography.css`, `spacing.css`, `effects.css` (radii, shadows, glows, textures, motion), `fonts.css` (webfont `@import`).
- `guidelines/` — foundation specimen cards (Colors, Type, Spacing, Brand) shown in the Design System tab.
- `assets/` — `logo.svg`, `mark.svg`.

**Components** (`components/<group>/` — `.jsx` + `.d.ts` + `.prompt.md` + a `*.card.html` per group)
- `core/` — **Button, IconButton, Badge, Avatar, Card**
- `forms/` — **Input, Select, Switch, Slider**
- `feedback/` — **Toast, ProgressBar, Tooltip, Dialog**
- `music/` — **TrackRow, QueueItem, NowPlayingBar, CreditMeter** *(the signature jukebox primitives)*

All components mount from the compiled bundle: `const { Button } = window.Exit66JukeboxDesignSystem_cf9d10`.

**UI kit** (`ui_kits/jukebox-app/`)
- `index.html` — interactive venue jukebox: tap in → browse the crate → queue & boost tracks → live transport. Screens: `NowPlayingScreen`, `BrowseScreen`, `QueueScreen`; chrome in `AppShell.jsx`; mock data in `data.js`.

**Meta**
- `SKILL.md` — makes this folder usable as a downloadable Agent Skill.
- `readme.md` — this file.

---

## Caveats

- **Fonts are loaded from Google Fonts CDN**, not self-hosted binaries, so the compiler reports 0 shipped webfonts. For production/offline use, download Chakra Petch / Space Grotesk / Space Mono `.woff2` files and replace the `@import` in `tokens/fonts.css` with local `@font-face` rules.
- **Lucide** stands in for a house icon set (none was provided).
- The **logo, palette, type, and product concept are invented** from the brief — they're a strong starting point, not an existing brand. Validate against any real Exit 66 assets you have.
