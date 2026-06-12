/* @ds-bundle: {"format":3,"namespace":"Exit66JukeboxDesignSystem_cf9d10","components":[{"name":"Avatar","sourcePath":"components/core/Avatar.jsx"},{"name":"Badge","sourcePath":"components/core/Badge.jsx"},{"name":"Button","sourcePath":"components/core/Button.jsx"},{"name":"Card","sourcePath":"components/core/Card.jsx"},{"name":"IconButton","sourcePath":"components/core/IconButton.jsx"},{"name":"Dialog","sourcePath":"components/feedback/Dialog.jsx"},{"name":"ProgressBar","sourcePath":"components/feedback/ProgressBar.jsx"},{"name":"Toast","sourcePath":"components/feedback/Toast.jsx"},{"name":"Tooltip","sourcePath":"components/feedback/Tooltip.jsx"},{"name":"Input","sourcePath":"components/forms/Input.jsx"},{"name":"Select","sourcePath":"components/forms/Select.jsx"},{"name":"Slider","sourcePath":"components/forms/Slider.jsx"},{"name":"Switch","sourcePath":"components/forms/Switch.jsx"},{"name":"CreditMeter","sourcePath":"components/music/CreditMeter.jsx"},{"name":"NowPlayingBar","sourcePath":"components/music/NowPlayingBar.jsx"},{"name":"QueueItem","sourcePath":"components/music/QueueItem.jsx"},{"name":"TrackRow","sourcePath":"components/music/TrackRow.jsx"}],"sourceHashes":{"components/core/Avatar.jsx":"5a835a5e6b09","components/core/Badge.jsx":"a1fa627c1c46","components/core/Button.jsx":"c2d60d80806f","components/core/Card.jsx":"792d29a585fd","components/core/IconButton.jsx":"bb525055edb4","components/feedback/Dialog.jsx":"2a745564faa6","components/feedback/ProgressBar.jsx":"6f2366180263","components/feedback/Toast.jsx":"852204aba460","components/feedback/Tooltip.jsx":"44c9789442f7","components/forms/Input.jsx":"66891747f8d0","components/forms/Select.jsx":"7b437e778efa","components/forms/Slider.jsx":"40f97ac205f7","components/forms/Switch.jsx":"48e7de52bc78","components/music/CreditMeter.jsx":"bc59b81423f0","components/music/NowPlayingBar.jsx":"aa3c5fb08809","components/music/QueueItem.jsx":"f7b0b6849ff7","components/music/TrackRow.jsx":"e8fa2a31022e","ui_kits/jukebox-app/AppShell.jsx":"09646dd23bd1","ui_kits/jukebox-app/BrowseScreen.jsx":"0f14d2b14640","ui_kits/jukebox-app/NowPlayingScreen.jsx":"9a4922f9e50f","ui_kits/jukebox-app/QueueScreen.jsx":"8570f43d5ca2","ui_kits/jukebox-app/data.js":"54d380c66f00"},"inlinedExternals":[],"unexposedExports":[]} */

(() => {

const __ds_ns = (window.Exit66JukeboxDesignSystem_cf9d10 = window.Exit66JukeboxDesignSystem_cf9d10 || {});

const __ds_scope = {};

(__ds_ns.__errors = __ds_ns.__errors || []);

// components/core/Avatar.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Exit 66 Jukebox — Avatar
 * User / artist token. Neon ring optional. Falls back to mono initials.
 */
function Avatar({
  src = null,
  name = '',
  size = 'md',
  ring = 'none',
  square = false,
  style = {},
  ...rest
}) {
  const dims = {
    xs: 24,
    sm: 32,
    md: 40,
    lg: 56,
    xl: 80
  };
  const d = dims[size] || dims.md;
  const rings = {
    none: 'none',
    magenta: '0 0 0 2px var(--bg-base), 0 0 0 4px var(--neon-magenta)',
    cyan: '0 0 0 2px var(--bg-base), 0 0 0 4px var(--neon-cyan)',
    amber: '0 0 0 2px var(--bg-base), 0 0 0 4px var(--neon-amber)'
  };
  const initials = name.split(' ').map(w => w[0]).filter(Boolean).slice(0, 2).join('').toUpperCase();
  return /*#__PURE__*/React.createElement("div", _extends({
    style: {
      width: d,
      height: d,
      flex: 'none',
      borderRadius: square ? 'var(--radius-md)' : 'var(--radius-pill)',
      boxShadow: rings[ring] || rings.none,
      background: 'linear-gradient(140deg, var(--ink-700), var(--ink-850))',
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      overflow: 'hidden',
      fontFamily: 'var(--font-mono)',
      fontWeight: 700,
      fontSize: d * 0.36,
      color: 'var(--paper-200)',
      letterSpacing: '0.02em',
      ...style
    }
  }, rest), src ? /*#__PURE__*/React.createElement("img", {
    src: src,
    alt: name,
    style: {
      width: '100%',
      height: '100%',
      objectFit: 'cover'
    }
  }) : initials || '·');
}
Object.assign(__ds_scope, { Avatar });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Avatar.jsx", error: String((e && e.message) || e) }); }

// components/core/Badge.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Exit 66 Jukebox — Badge
 * Small status / category pill. Mono, tracked, neon-tinted.
 */
function Badge({
  tone = 'magenta',
  variant = 'soft',
  dot = false,
  children,
  style = {},
  ...rest
}) {
  const tones = {
    magenta: {
      c: 'var(--neon-magenta)',
      rgb: '255,46,136'
    },
    cyan: {
      c: 'var(--neon-cyan)',
      rgb: '31,224,255'
    },
    amber: {
      c: 'var(--neon-amber)',
      rgb: '255,176,46'
    },
    violet: {
      c: 'var(--neon-violet)',
      rgb: '138,108,255'
    },
    success: {
      c: 'var(--status-success)',
      rgb: '61,245,155'
    },
    danger: {
      c: 'var(--status-danger)',
      rgb: '255,77,94'
    },
    neutral: {
      c: 'var(--paper-300)',
      rgb: '171,168,189'
    }
  };
  const t = tones[tone] || tones.magenta;
  const variants = {
    soft: {
      background: `rgba(${t.rgb},0.12)`,
      color: t.c,
      border: `1.5px solid rgba(${t.rgb},0.45)`
    },
    solid: {
      background: t.c,
      color: 'var(--text-on-accent)',
      border: '1.5px solid transparent'
    },
    outline: {
      background: 'transparent',
      color: t.c,
      border: `1.5px solid ${t.c}`
    }
  };
  return /*#__PURE__*/React.createElement("span", _extends({
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      gap: 6,
      height: 22,
      padding: '0 10px',
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      fontWeight: 700,
      letterSpacing: '0.14em',
      textTransform: 'uppercase',
      borderRadius: 'var(--radius-sm)',
      whiteSpace: 'nowrap',
      ...(variants[variant] || variants.soft),
      ...style
    }
  }, rest), dot ? /*#__PURE__*/React.createElement("span", {
    style: {
      width: 6,
      height: 6,
      borderRadius: '50%',
      background: t.c
    }
  }) : null, children);
}
Object.assign(__ds_scope, { Badge });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Badge.jsx", error: String((e && e.message) || e) }); }

// components/core/Button.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const {
  useState
} = React;
/**
 * Exit 66 Jukebox — Button
 * Neon-noir action button. Primary = magenta fill that lights up on hover.
 */
function Button({
  variant = 'primary',
  size = 'md',
  disabled = false,
  fullWidth = false,
  icon = null,
  iconRight = null,
  type = 'button',
  onClick,
  children,
  style = {},
  ...rest
}) {
  const [hover, setHover] = useState(false);
  const [press, setPress] = useState(false);
  const sizes = {
    sm: {
      height: 'var(--control-h-sm)',
      padding: '0 14px',
      font: '12px',
      gap: '7px'
    },
    md: {
      height: 'var(--control-h-md)',
      padding: '0 20px',
      font: '14px',
      gap: '9px'
    },
    lg: {
      height: 'var(--control-h-lg)',
      padding: '0 28px',
      font: '15px',
      gap: '11px'
    }
  };
  const s = sizes[size] || sizes.md;
  const base = {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: s.gap,
    height: s.height,
    padding: s.padding,
    width: fullWidth ? '100%' : 'auto',
    fontFamily: 'var(--font-display)',
    fontWeight: 600,
    fontSize: s.font,
    letterSpacing: '0.08em',
    textTransform: 'uppercase',
    borderRadius: 'var(--radius-md)',
    cursor: disabled ? 'not-allowed' : 'pointer',
    border: '1.5px solid transparent',
    transition: 'all var(--dur) var(--ease-out)',
    transform: press && !disabled ? 'translateY(1px)' : 'none',
    opacity: disabled ? 0.4 : 1,
    whiteSpace: 'nowrap',
    userSelect: 'none',
    outline: 'none'
  };
  const variants = {
    primary: {
      background: hover && !disabled ? 'var(--neon-magenta-bright)' : 'var(--neon-magenta)',
      color: 'var(--text-on-accent)',
      borderColor: hover && !disabled ? 'var(--neon-magenta-bright)' : 'var(--neon-magenta)',
      boxShadow: 'none'
    },
    secondary: {
      background: hover && !disabled ? 'rgba(31,224,255,0.10)' : 'transparent',
      color: 'var(--neon-cyan)',
      borderColor: 'var(--neon-cyan)',
      boxShadow: hover && !disabled ? 'var(--glow-soft-cyan)' : 'none'
    },
    ghost: {
      background: hover && !disabled ? 'var(--bg-surface-hover)' : 'transparent',
      color: 'var(--text-body)',
      borderColor: 'var(--border-default)'
    },
    danger: {
      background: hover && !disabled ? '#ff6472' : 'var(--status-danger)',
      color: 'var(--text-on-accent)',
      borderColor: 'var(--status-danger)',
      boxShadow: 'none'
    }
  };
  return /*#__PURE__*/React.createElement("button", _extends({
    type: type,
    disabled: disabled,
    onClick: onClick,
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => {
      setHover(false);
      setPress(false);
    },
    onMouseDown: () => setPress(true),
    onMouseUp: () => setPress(false),
    style: {
      ...base,
      ...(variants[variant] || variants.primary),
      ...style
    }
  }, rest), icon ? /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'inline-flex',
      fontSize: '1.15em'
    }
  }, icon) : null, children, iconRight ? /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'inline-flex',
      fontSize: '1.15em'
    }
  }, iconRight) : null);
}
Object.assign(__ds_scope, { Button });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Button.jsx", error: String((e && e.message) || e) }); }

// components/core/Card.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const {
  useState
} = React;
/**
 * Exit 66 Jukebox — Card
 * Surface container. Optional neon edge that lights on hover (interactive cards).
 */
function Card({
  glow = 'none',
  interactive = false,
  padding = 'md',
  children,
  style = {},
  onClick,
  ...rest
}) {
  const [hover, setHover] = useState(false);
  const glows = {
    none: {
      border: '1px solid var(--border-default)',
      shadow: 'var(--shadow-md)',
      hoverShadow: 'var(--shadow-lg)'
    },
    magenta: {
      border: '1px solid rgba(255,46,136,0.35)',
      shadow: 'var(--shadow-md)',
      hoverShadow: 'var(--glow-magenta)'
    },
    cyan: {
      border: '1px solid rgba(31,224,255,0.35)',
      shadow: 'var(--shadow-md)',
      hoverShadow: 'var(--glow-cyan)'
    },
    amber: {
      border: '1px solid rgba(255,176,46,0.35)',
      shadow: 'var(--shadow-md)',
      hoverShadow: 'var(--glow-amber)'
    }
  };
  const g = glows[glow] || glows.none;
  const pads = {
    none: 0,
    sm: 'var(--space-4)',
    md: 'var(--space-6)',
    lg: 'var(--space-8)'
  };
  return /*#__PURE__*/React.createElement("div", _extends({
    onClick: onClick,
    onMouseEnter: () => interactive && setHover(true),
    onMouseLeave: () => interactive && setHover(false),
    style: {
      position: 'relative',
      background: 'var(--bg-surface)',
      backgroundImage: 'var(--scanline)',
      border: g.border,
      borderRadius: 'var(--radius-lg)',
      boxShadow: hover ? g.hoverShadow : g.shadow,
      padding: pads[padding] ?? pads.md,
      transition: 'box-shadow var(--dur) var(--ease-out), transform var(--dur) var(--ease-out)',
      transform: hover ? 'translateY(-2px)' : 'none',
      cursor: interactive ? 'pointer' : 'default',
      ...style
    }
  }, rest), children);
}
Object.assign(__ds_scope, { Card });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Card.jsx", error: String((e && e.message) || e) }); }

// components/core/IconButton.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const {
  useState
} = React;
/**
 * Exit 66 Jukebox — IconButton
 * Square/round icon-only control. Used for transport & toolbar actions.
 */
function IconButton({
  variant = 'ghost',
  size = 'md',
  shape = 'rounded',
  disabled = false,
  active = false,
  label,
  onClick,
  children,
  style = {},
  ...rest
}) {
  const [hover, setHover] = useState(false);
  const dims = {
    sm: 32,
    md: 42,
    lg: 54
  };
  const d = dims[size] || dims.md;
  const accentOn = active || hover && !disabled;
  const variants = {
    solid: {
      background: accentOn ? 'var(--neon-magenta-bright)' : 'var(--neon-magenta)',
      color: 'var(--text-on-accent)',
      boxShadow: 'none',
      border: '1.5px solid var(--neon-magenta)'
    },
    outline: {
      background: hover && !disabled ? 'rgba(31,224,255,0.10)' : 'transparent',
      color: active ? 'var(--neon-cyan)' : 'var(--text-body)',
      border: `1.5px solid ${active ? 'var(--neon-cyan)' : 'var(--border-strong)'}`,
      boxShadow: accentOn ? 'var(--glow-soft-cyan)' : 'none'
    },
    ghost: {
      background: hover && !disabled ? 'var(--bg-surface-hover)' : 'transparent',
      color: active ? 'var(--neon-cyan)' : 'var(--text-muted)',
      border: '1.5px solid transparent'
    }
  };
  return /*#__PURE__*/React.createElement("button", _extends({
    type: "button",
    "aria-label": label,
    title: label,
    disabled: disabled,
    onClick: onClick,
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    style: {
      width: d,
      height: d,
      flex: 'none',
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      borderRadius: shape === 'circle' ? 'var(--radius-pill)' : 'var(--radius-md)',
      cursor: disabled ? 'not-allowed' : 'pointer',
      opacity: disabled ? 0.4 : 1,
      fontSize: size === 'lg' ? 22 : size === 'sm' ? 15 : 18,
      transition: 'all var(--dur) var(--ease-out)',
      outline: 'none',
      ...(variants[variant] || variants.ghost),
      ...style
    }
  }, rest), children);
}
Object.assign(__ds_scope, { IconButton });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/IconButton.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Dialog.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Exit 66 Jukebox — Dialog
 * Centered modal over a blurred neon-dimmed scrim. Controlled via `open`.
 */
function Dialog({
  open = false,
  onClose,
  title,
  eyebrow,
  footer = null,
  width = 460,
  inline = false,
  children,
  style = {},
  ...rest
}) {
  if (!open) return null;
  const panel = /*#__PURE__*/React.createElement("div", _extends({
    role: "dialog",
    "aria-modal": "true",
    onClick: e => e.stopPropagation(),
    style: {
      width: '100%',
      maxWidth: width,
      background: 'var(--bg-surface-raised)',
      backgroundImage: 'var(--scanline)',
      border: '1px solid var(--border-strong)',
      borderRadius: 'var(--radius-xl)',
      boxShadow: 'var(--shadow-xl)',
      overflow: 'hidden',
      ...style
    }
  }, rest), /*#__PURE__*/React.createElement("div", {
    style: {
      height: 3,
      background: 'linear-gradient(90deg, var(--neon-magenta), var(--neon-cyan))'
    }
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      padding: 'var(--space-7)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'flex-start',
      justifyContent: 'space-between',
      gap: 16,
      marginBottom: 14
    }
  }, /*#__PURE__*/React.createElement("div", null, eyebrow ? /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      letterSpacing: '0.22em',
      textTransform: 'uppercase',
      color: 'var(--neon-cyan)',
      marginBottom: 6
    }
  }, eyebrow) : null, title ? /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-display)',
      fontWeight: 700,
      fontSize: 24,
      letterSpacing: '0.02em',
      textTransform: 'uppercase',
      color: 'var(--text-strong)'
    }
  }, title) : null), onClose ? /*#__PURE__*/React.createElement("button", {
    onClick: onClose,
    "aria-label": "Close",
    style: {
      background: 'none',
      border: 'none',
      color: 'var(--text-faint)',
      fontSize: 20,
      cursor: 'pointer',
      lineHeight: 1
    }
  }, "\u2715") : null), /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 15,
      lineHeight: 1.6,
      color: 'var(--text-body)'
    }
  }, children), footer ? /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      justifyContent: 'flex-end',
      gap: 10,
      marginTop: 24
    }
  }, footer) : null));
  return /*#__PURE__*/React.createElement("div", {
    onClick: onClose,
    style: {
      position: inline ? 'absolute' : 'fixed',
      inset: 0,
      zIndex: 100,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      padding: 24,
      background: 'rgba(6,6,11,0.72)',
      backdropFilter: 'blur(6px)',
      WebkitBackdropFilter: 'blur(6px)'
    }
  }, panel);
}
Object.assign(__ds_scope, { Dialog });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Dialog.jsx", error: String((e && e.message) || e) }); }

// components/feedback/ProgressBar.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Exit 66 Jukebox — ProgressBar
 * Neon fill bar for track scrub, upload, or credit progress.
 */
function ProgressBar({
  value = 0,
  max = 100,
  tone = 'magenta',
  indeterminate = false,
  height = 6,
  label,
  showValue = false,
  style = {},
  ...rest
}) {
  const tones = {
    magenta: 'var(--neon-magenta)',
    cyan: 'var(--neon-cyan)',
    amber: 'var(--neon-amber)',
    success: 'var(--status-success)'
  };
  const c = tones[tone] || tones.magenta;
  const pct = Math.max(0, Math.min(100, value / max * 100));
  return /*#__PURE__*/React.createElement("div", _extends({
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 7,
      ...style
    }
  }, rest), label || showValue ? /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      justifyContent: 'space-between',
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      letterSpacing: '0.12em',
      textTransform: 'uppercase'
    }
  }, label ? /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--text-muted)'
    }
  }, label) : /*#__PURE__*/React.createElement("span", null), showValue ? /*#__PURE__*/React.createElement("span", {
    style: {
      color: c
    }
  }, Math.round(pct), "%") : null) : null, /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'relative',
      height,
      borderRadius: 'var(--radius-pill)',
      background: 'var(--ink-700)',
      overflow: 'hidden'
    }
  }, indeterminate ? /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      top: 0,
      bottom: 0,
      width: '38%',
      borderRadius: 'var(--radius-pill)',
      background: c,
      animation: 'e66-indet 1.2s var(--ease-in-out) infinite'
    }
  }) : /*#__PURE__*/React.createElement("div", {
    style: {
      height: '100%',
      width: `${pct}%`,
      borderRadius: 'var(--radius-pill)',
      background: c,
      transition: 'width var(--dur) var(--ease-out)'
    }
  })), /*#__PURE__*/React.createElement("style", null, `@keyframes e66-indet{0%{left:-40%}100%{left:100%}}`));
}
Object.assign(__ds_scope, { ProgressBar });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/ProgressBar.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Toast.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Exit 66 Jukebox — Toast
 * Transient notification. Neon left edge keyed to tone.
 */
function Toast({
  tone = 'magenta',
  title,
  message,
  icon = null,
  onClose,
  style = {},
  ...rest
}) {
  const tones = {
    magenta: {
      c: 'var(--neon-magenta)',
      rgb: '255,46,136'
    },
    cyan: {
      c: 'var(--neon-cyan)',
      rgb: '31,224,255'
    },
    amber: {
      c: 'var(--neon-amber)',
      rgb: '255,176,46'
    },
    success: {
      c: 'var(--status-success)',
      rgb: '61,245,155'
    },
    danger: {
      c: 'var(--status-danger)',
      rgb: '255,77,94'
    }
  };
  const t = tones[tone] || tones.magenta;
  return /*#__PURE__*/React.createElement("div", _extends({
    role: "status",
    style: {
      display: 'flex',
      alignItems: 'flex-start',
      gap: 12,
      minWidth: 280,
      maxWidth: 420,
      padding: '14px 16px',
      background: 'var(--bg-surface-raised)',
      borderRadius: 'var(--radius-md)',
      border: '1px solid var(--border-default)',
      borderLeft: `3px solid ${t.c}`,
      boxShadow: 'var(--shadow-lg)',
      ...style
    }
  }, rest), icon ? /*#__PURE__*/React.createElement("span", {
    style: {
      color: t.c,
      display: 'inline-flex',
      fontSize: 18,
      marginTop: 1
    }
  }, icon) : null, /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      minWidth: 0
    }
  }, title ? /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-display)',
      fontWeight: 600,
      fontSize: 14,
      letterSpacing: '0.04em',
      textTransform: 'uppercase',
      color: 'var(--text-strong)',
      marginBottom: message ? 3 : 0
    }
  }, title) : null, message ? /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 13,
      lineHeight: 1.5,
      color: 'var(--text-muted)'
    }
  }, message) : null), onClose ? /*#__PURE__*/React.createElement("button", {
    onClick: onClose,
    "aria-label": "Dismiss",
    style: {
      background: 'none',
      border: 'none',
      color: 'var(--text-faint)',
      cursor: 'pointer',
      fontSize: 16,
      lineHeight: 1,
      padding: 2
    }
  }, "\u2715") : null);
}
Object.assign(__ds_scope, { Toast });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Toast.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Tooltip.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const {
  useState
} = React;
/**
 * Exit 66 Jukebox — Tooltip
 * Mono label bubble on hover/focus. Wrap any element.
 */
function Tooltip({
  label,
  side = 'top',
  open: openProp,
  children,
  style = {},
  ...rest
}) {
  const [hover, setHover] = useState(false);
  const open = openProp != null ? openProp : hover;
  const pos = {
    top: {
      bottom: '100%',
      left: '50%',
      transform: 'translateX(-50%)',
      marginBottom: 8
    },
    bottom: {
      top: '100%',
      left: '50%',
      transform: 'translateX(-50%)',
      marginTop: 8
    },
    left: {
      right: '100%',
      top: '50%',
      transform: 'translateY(-50%)',
      marginRight: 8
    },
    right: {
      left: '100%',
      top: '50%',
      transform: 'translateY(-50%)',
      marginLeft: 8
    }
  };
  return /*#__PURE__*/React.createElement("span", _extends({
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    style: {
      position: 'relative',
      display: 'inline-flex',
      ...style
    }
  }, rest), children, /*#__PURE__*/React.createElement("span", {
    role: "tooltip",
    style: {
      position: 'absolute',
      zIndex: 40,
      whiteSpace: 'nowrap',
      pointerEvents: 'none',
      ...pos[side],
      padding: '6px 10px',
      background: 'var(--ink-980)',
      border: '1px solid var(--neon-cyan)',
      borderRadius: 'var(--radius-sm)',
      boxShadow: 'var(--glow-soft-cyan)',
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      letterSpacing: '0.1em',
      textTransform: 'uppercase',
      color: 'var(--paper-100)',
      opacity: open ? 1 : 0,
      transform: `${pos[side].transform} translateY(${open ? 0 : side === 'top' ? 4 : -4}px)`,
      transition: 'opacity var(--dur) var(--ease-out), transform var(--dur) var(--ease-out)'
    }
  }, label));
}
Object.assign(__ds_scope, { Tooltip });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Tooltip.jsx", error: String((e && e.message) || e) }); }

// components/forms/Input.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const {
  useState
} = React;
/**
 * Exit 66 Jukebox — Input
 * Text field with mono label and neon focus ring.
 */
function Input({
  label,
  hint,
  error,
  icon = null,
  type = 'text',
  size = 'md',
  disabled = false,
  value,
  onChange,
  placeholder,
  style = {},
  id,
  ...rest
}) {
  const [focus, setFocus] = useState(false);
  const heights = {
    sm: 'var(--control-h-sm)',
    md: 'var(--control-h-md)',
    lg: 'var(--control-h-lg)'
  };
  const fieldId = id || (label ? `in-${label.replace(/\s+/g, '-').toLowerCase()}` : undefined);
  const borderColor = error ? 'var(--status-danger)' : focus ? 'var(--neon-cyan)' : 'var(--border-strong)';
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 6,
      ...style
    }
  }, label ? /*#__PURE__*/React.createElement("label", {
    htmlFor: fieldId,
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      letterSpacing: '0.16em',
      textTransform: 'uppercase',
      color: 'var(--text-muted)'
    }
  }, label) : null, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 10,
      height: heights[size] || heights.md,
      padding: '0 14px',
      background: 'var(--bg-inset)',
      border: `1.5px solid ${borderColor}`,
      borderRadius: 'var(--radius-md)',
      boxShadow: focus ? error ? '0 0 0 2px rgba(255,77,94,0.5)' : '0 0 0 2px rgba(31,224,255,0.5)' : 'none',
      transition: 'all var(--dur) var(--ease-out)',
      opacity: disabled ? 0.5 : 1
    }
  }, icon ? /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--text-faint)',
      display: 'inline-flex',
      fontSize: 16
    }
  }, icon) : null, /*#__PURE__*/React.createElement("input", _extends({
    id: fieldId,
    type: type,
    value: value,
    onChange: onChange,
    placeholder: placeholder,
    disabled: disabled,
    onFocus: () => setFocus(true),
    onBlur: () => setFocus(false),
    style: {
      flex: 1,
      minWidth: 0,
      height: '100%',
      background: 'transparent',
      border: 'none',
      outline: 'none',
      color: 'var(--text-strong)',
      fontFamily: 'var(--font-sans)',
      fontSize: 15,
      letterSpacing: '0.01em'
    }
  }, rest))), error ? /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      color: 'var(--status-danger)'
    }
  }, error) : hint ? /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      color: 'var(--text-faint)'
    }
  }, hint) : null);
}
Object.assign(__ds_scope, { Input });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Input.jsx", error: String((e && e.message) || e) }); }

// components/forms/Select.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const {
  useState,
  useRef,
  useEffect
} = React;
/**
 * Exit 66 Jukebox — Select
 * Custom dropdown with neon focus + menu. options: [{value,label}] or strings.
 */
function Select({
  options = [],
  value,
  onChange,
  placeholder = 'Select…',
  label,
  size = 'md',
  disabled = false,
  style = {},
  ...rest
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef(null);
  const norm = options.map(o => typeof o === 'string' ? {
    value: o,
    label: o
  } : o);
  const current = norm.find(o => o.value === value);
  const heights = {
    sm: 'var(--control-h-sm)',
    md: 'var(--control-h-md)',
    lg: 'var(--control-h-lg)'
  };
  useEffect(() => {
    const onDoc = e => {
      if (ref.current && !ref.current.contains(e.target)) setOpen(false);
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, []);
  return /*#__PURE__*/React.createElement("div", _extends({
    ref: ref,
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 6,
      position: 'relative',
      ...style
    }
  }, rest), label ? /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      letterSpacing: '0.16em',
      textTransform: 'uppercase',
      color: 'var(--text-muted)'
    }
  }, label) : null, /*#__PURE__*/React.createElement("button", {
    type: "button",
    disabled: disabled,
    onClick: () => !disabled && setOpen(o => !o),
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      gap: 10,
      height: heights[size] || heights.md,
      padding: '0 14px',
      background: 'var(--bg-inset)',
      color: current ? 'var(--text-strong)' : 'var(--text-faint)',
      border: `1px solid ${open ? 'var(--neon-cyan)' : 'var(--border-strong)'}`,
      borderRadius: 'var(--radius-md)',
      cursor: disabled ? 'not-allowed' : 'pointer',
      boxShadow: open ? '0 0 0 2px rgba(31,224,255,0.5)' : 'none',
      fontFamily: 'var(--font-sans)',
      fontSize: 15,
      outline: 'none',
      transition: 'all var(--dur) var(--ease-out)',
      opacity: disabled ? 0.5 : 1
    }
  }, /*#__PURE__*/React.createElement("span", null, current ? current.label : placeholder), /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--text-faint)',
      transform: open ? 'rotate(180deg)' : 'none',
      transition: 'transform var(--dur) var(--ease-out)',
      display: 'inline-flex'
    }
  }, "\u25BE")), open ? /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      top: 'calc(100% + 6px)',
      left: 0,
      right: 0,
      zIndex: 30,
      background: 'var(--bg-surface-raised)',
      border: '1px solid var(--border-strong)',
      borderRadius: 'var(--radius-md)',
      boxShadow: 'var(--shadow-lg)',
      padding: 4,
      maxHeight: 240,
      overflowY: 'auto'
    }
  }, norm.map(o => {
    const active = o.value === value;
    return /*#__PURE__*/React.createElement("div", {
      key: o.value,
      onClick: () => {
        onChange && onChange(o.value);
        setOpen(false);
      },
      style: {
        padding: '9px 12px',
        borderRadius: 'var(--radius-sm)',
        cursor: 'pointer',
        fontFamily: 'var(--font-sans)',
        fontSize: 14,
        color: active ? 'var(--neon-cyan)' : 'var(--text-body)',
        background: active ? 'rgba(31,224,255,0.10)' : 'transparent',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center'
      },
      onMouseEnter: e => {
        if (!active) e.currentTarget.style.background = 'var(--bg-surface-hover)';
      },
      onMouseLeave: e => {
        if (!active) e.currentTarget.style.background = 'transparent';
      }
    }, o.label, active ? /*#__PURE__*/React.createElement("span", null, "\u2713") : null);
  })) : null);
}
Object.assign(__ds_scope, { Select });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Select.jsx", error: String((e && e.message) || e) }); }

// components/forms/Slider.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const {
  useRef,
  useState
} = React;
/**
 * Exit 66 Jukebox — Slider
 * Neon track slider for volume / scrub / credits. Filled portion glows.
 */
function Slider({
  value = 50,
  min = 0,
  max = 100,
  step = 1,
  tone = 'magenta',
  disabled = false,
  onChange,
  label,
  showValue = false,
  style = {},
  ...rest
}) {
  const ref = useRef(null);
  const [drag, setDrag] = useState(false);
  const tones = {
    magenta: 'var(--neon-magenta)',
    cyan: 'var(--neon-cyan)',
    amber: 'var(--neon-amber)'
  };
  const c = tones[tone] || tones.magenta;
  const pct = (value - min) / (max - min) * 100;
  const setFromEvent = clientX => {
    if (!ref.current || disabled) return;
    const r = ref.current.getBoundingClientRect();
    let p = (clientX - r.left) / r.width;
    p = Math.max(0, Math.min(1, p));
    let v = min + p * (max - min);
    v = Math.round(v / step) * step;
    onChange && onChange(Math.max(min, Math.min(max, v)));
  };
  const onDown = e => {
    if (disabled) return;
    setDrag(true);
    setFromEvent(e.clientX);
    const move = ev => setFromEvent(ev.clientX);
    const up = () => {
      setDrag(false);
      window.removeEventListener('mousemove', move);
      window.removeEventListener('mouseup', up);
    };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
  };
  return /*#__PURE__*/React.createElement("div", _extends({
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 8,
      opacity: disabled ? 0.5 : 1,
      ...style
    }
  }, rest), label || showValue ? /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      justifyContent: 'space-between',
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      letterSpacing: '0.14em',
      textTransform: 'uppercase'
    }
  }, label ? /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--text-muted)'
    }
  }, label) : /*#__PURE__*/React.createElement("span", null), showValue ? /*#__PURE__*/React.createElement("span", {
    style: {
      color: c
    }
  }, Math.round(value)) : null) : null, /*#__PURE__*/React.createElement("div", {
    ref: ref,
    onMouseDown: onDown,
    style: {
      position: 'relative',
      height: 22,
      display: 'flex',
      alignItems: 'center',
      cursor: disabled ? 'not-allowed' : 'pointer'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: 0,
      right: 0,
      height: 4,
      borderRadius: 'var(--radius-pill)',
      background: 'var(--ink-700)'
    }
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: 0,
      width: `${pct}%`,
      height: 4,
      borderRadius: 'var(--radius-pill)',
      background: c
    }
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: `calc(${pct}% - 9px)`,
      width: 18,
      height: 18,
      borderRadius: '50%',
      background: 'var(--paper-100)',
      border: `2px solid ${c}`,
      boxShadow: drag ? `0 0 0 3px ${c}` : '0 1px 4px rgba(0,0,0,0.6)',
      transition: drag ? 'none' : 'box-shadow var(--dur) var(--ease-out)'
    }
  })));
}
Object.assign(__ds_scope, { Slider });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Slider.jsx", error: String((e && e.message) || e) }); }

// components/forms/Switch.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const {
  useState
} = React;
/**
 * Exit 66 Jukebox — Switch
 * Neon toggle. On = magenta track with glow.
 */
function Switch({
  checked = false,
  onChange,
  disabled = false,
  tone = 'magenta',
  label,
  style = {},
  ...rest
}) {
  const tones = {
    magenta: {
      c: 'var(--neon-magenta)',
      glow: 'var(--glow-soft-magenta)'
    },
    cyan: {
      c: 'var(--neon-cyan)',
      glow: 'var(--glow-soft-cyan)'
    },
    amber: {
      c: 'var(--neon-amber)',
      glow: 'var(--glow-amber)'
    }
  };
  const t = tones[tone] || tones.magenta;
  const toggle = /*#__PURE__*/React.createElement("span", {
    role: "switch",
    "aria-checked": checked,
    onClick: () => !disabled && onChange && onChange(!checked),
    style: {
      width: 46,
      height: 26,
      flex: 'none',
      borderRadius: 'var(--radius-pill)',
      background: checked ? t.c : 'var(--ink-700)',
      border: `1px solid ${checked ? t.c : 'var(--border-strong)'}`,
      boxShadow: checked ? t.glow : 'inset 0 1px 2px rgba(0,0,0,0.5)',
      cursor: disabled ? 'not-allowed' : 'pointer',
      position: 'relative',
      transition: 'all var(--dur) var(--ease-out)',
      opacity: disabled ? 0.5 : 1
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      position: 'absolute',
      top: 2,
      left: checked ? 22 : 2,
      width: 20,
      height: 20,
      borderRadius: '50%',
      background: checked ? 'var(--text-on-accent)' : 'var(--paper-300)',
      transition: 'left var(--dur) var(--ease-snap)',
      boxShadow: '0 1px 3px rgba(0,0,0,0.5)'
    }
  }));
  if (!label) return /*#__PURE__*/React.createElement("span", _extends({
    style: style
  }, rest), toggle);
  return /*#__PURE__*/React.createElement("label", _extends({
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      gap: 12,
      cursor: disabled ? 'not-allowed' : 'pointer',
      ...style
    }
  }, rest), toggle, /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 14,
      color: 'var(--text-body)'
    }
  }, label));
}
Object.assign(__ds_scope, { Switch });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Switch.jsx", error: String((e && e.message) || e) }); }

// components/music/CreditMeter.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Exit 66 Jukebox — CreditMeter
 * Displays the player's credit balance as a glowing pill. Optional top-up action.
 */
function CreditMeter({
  credits = 0,
  size = 'md',
  onTopUp,
  style = {},
  ...rest
}) {
  const sizes = {
    sm: {
      h: 30,
      font: 13,
      pad: '0 10px',
      icon: 13
    },
    md: {
      h: 38,
      font: 16,
      pad: '0 14px',
      icon: 16
    },
    lg: {
      h: 48,
      font: 20,
      pad: '0 18px',
      icon: 20
    }
  };
  const s = sizes[size] || sizes.md;
  return /*#__PURE__*/React.createElement("div", _extends({
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      gap: 10,
      height: s.h,
      padding: s.pad,
      background: 'rgba(255,176,46,0.08)',
      border: '1.5px solid rgba(255,176,46,0.55)',
      borderRadius: 'var(--radius-sm)',
      boxShadow: 'none',
      ...style
    }
  }, rest), /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--neon-amber)',
      fontSize: s.icon,
      lineHeight: 1
    }
  }, "\u25C8"), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontWeight: 700,
      fontSize: s.font,
      color: 'var(--neon-amber-bright)',
      letterSpacing: '0.04em'
    }
  }, credits), onTopUp ? /*#__PURE__*/React.createElement("button", {
    onClick: onTopUp,
    "aria-label": "Add credits",
    style: {
      width: s.h - 12,
      height: s.h - 12,
      marginLeft: 2,
      marginRight: -4,
      flex: 'none',
      borderRadius: 'var(--radius-sm)',
      border: '1.5px solid var(--neon-amber)',
      background: 'transparent',
      color: 'var(--neon-amber)',
      cursor: 'pointer',
      fontSize: s.icon,
      lineHeight: 1,
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center'
    }
  }, "+") : null);
}
Object.assign(__ds_scope, { CreditMeter });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/music/CreditMeter.jsx", error: String((e && e.message) || e) }); }

// components/music/NowPlayingBar.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const {
  useRef,
  useState
} = React;
function fmt(sec) {
  if (sec == null || isNaN(sec)) return '0:00';
  const m = Math.floor(sec / 60);
  const s = Math.floor(sec % 60);
  return `${m}:${s < 10 ? '0' : ''}${s}`;
}

/**
 * Exit 66 Jukebox — NowPlayingBar
 * The persistent transport bar. Slot-code art, transport, neon scrub + volume.
 */
function NowPlayingBar({
  title = 'Nothing playing',
  artist = '—',
  code = 'A6',
  art = null,
  artTone = 'magenta',
  current = 0,
  duration = 0,
  playing = false,
  volume = 70,
  onPlayPause,
  onPrev,
  onNext,
  onSeek,
  onVolume,
  style = {},
  ...rest
}) {
  const scrubRef = useRef(null);
  const [hoverPlay, setHoverPlay] = useState(false);
  const pct = duration ? Math.min(100, current / duration * 100) : 0;
  const tones = {
    magenta: 'linear-gradient(135deg, #2a0f1f, #ff2e88 280%)',
    cyan: 'linear-gradient(135deg, #08252b, #1fe0ff 280%)',
    amber: 'linear-gradient(135deg, #2a1d08, #ffb02e 280%)',
    violet: 'linear-gradient(135deg, #17112e, #8a6cff 280%)'
  };
  const seekFrom = clientX => {
    if (!scrubRef.current || !onSeek) return;
    const r = scrubRef.current.getBoundingClientRect();
    let p = (clientX - r.left) / r.width;
    onSeek(Math.max(0, Math.min(1, p)));
  };
  const tBtn = (glyph, label, onClick, primary) => /*#__PURE__*/React.createElement("button", {
    onClick: onClick,
    "aria-label": label,
    onMouseEnter: () => primary && setHoverPlay(true),
    onMouseLeave: () => primary && setHoverPlay(false),
    style: {
      width: primary ? 46 : 38,
      height: primary ? 46 : 38,
      flex: 'none',
      borderRadius: '50%',
      cursor: 'pointer',
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      fontSize: primary ? 20 : 17,
      lineHeight: 1,
      background: primary ? 'var(--neon-magenta)' : 'transparent',
      color: primary ? 'var(--text-on-accent)' : 'var(--text-body)',
      border: primary ? 'none' : '1px solid transparent',
      boxShadow: primary && hoverPlay ? 'var(--glow-magenta)' : 'none',
      transition: 'all var(--dur) var(--ease-out)'
    }
  }, glyph);
  return /*#__PURE__*/React.createElement("div", _extends({
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 20,
      height: 84,
      padding: '0 22px',
      background: 'var(--bg-surface-raised)',
      backgroundImage: 'var(--scanline)',
      borderTop: '1px solid var(--border-strong)',
      boxShadow: '0 -8px 30px rgba(0,0,0,0.5)',
      ...style
    }
  }, rest), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 14,
      width: 280,
      flex: 'none'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      width: 54,
      height: 54,
      flex: 'none',
      borderRadius: 'var(--radius-sm)',
      background: art ? `center/cover url(${art})` : tones[artTone] || tones.magenta,
      display: 'flex',
      alignItems: 'flex-end',
      padding: 6,
      boxSizing: 'border-box',
      boxShadow: playing ? 'var(--glow-soft-magenta)' : 'none'
    }
  }, !art ? /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 10,
      fontWeight: 700,
      color: 'rgba(255,255,255,0.85)'
    }
  }, code) : null), /*#__PURE__*/React.createElement("div", {
    style: {
      minWidth: 0
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontWeight: 600,
      fontSize: 15,
      color: 'var(--text-strong)',
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis'
    }
  }, title), /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 13,
      color: 'var(--text-muted)',
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis'
    }
  }, artist))), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      minWidth: 0,
      display: 'flex',
      flexDirection: 'column',
      gap: 7,
      alignItems: 'center'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 10
    }
  }, tBtn('⏮', 'Previous', onPrev, false), tBtn(playing ? '❚❚' : '▶', playing ? 'Pause' : 'Play', onPlayPause, true), tBtn('⏭', 'Next', onNext, false)), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      width: '100%',
      maxWidth: 520
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      color: 'var(--text-faint)',
      width: 38,
      textAlign: 'right'
    }
  }, fmt(current)), /*#__PURE__*/React.createElement("div", {
    ref: scrubRef,
    onMouseDown: e => seekFrom(e.clientX),
    style: {
      position: 'relative',
      flex: 1,
      height: 14,
      display: 'flex',
      alignItems: 'center',
      cursor: 'pointer'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: 0,
      right: 0,
      height: 4,
      borderRadius: 'var(--radius-pill)',
      background: 'var(--ink-700)'
    }
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: 0,
      width: `${pct}%`,
      height: 4,
      borderRadius: 'var(--radius-pill)',
      background: 'var(--neon-magenta)'
    }
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: `calc(${pct}% - 6px)`,
      width: 12,
      height: 12,
      borderRadius: '50%',
      background: 'var(--paper-100)',
      border: '2px solid var(--neon-magenta)'
    }
  })), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      color: 'var(--text-faint)',
      width: 38
    }
  }, fmt(duration)))), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 10,
      width: 160,
      flex: 'none'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--text-muted)',
      fontSize: 16
    }
  }, "\u266A"), /*#__PURE__*/React.createElement("div", {
    onMouseDown: e => {
      const r = e.currentTarget.getBoundingClientRect();
      onVolume && onVolume(Math.round(Math.max(0, Math.min(1, (e.clientX - r.left) / r.width)) * 100));
    },
    style: {
      position: 'relative',
      flex: 1,
      height: 14,
      display: 'flex',
      alignItems: 'center',
      cursor: 'pointer'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: 0,
      right: 0,
      height: 4,
      borderRadius: 'var(--radius-pill)',
      background: 'var(--ink-700)'
    }
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: 0,
      width: `${volume}%`,
      height: 4,
      borderRadius: 'var(--radius-pill)',
      background: 'var(--neon-cyan)'
    }
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      left: `calc(${volume}% - 6px)`,
      width: 12,
      height: 12,
      borderRadius: '50%',
      background: 'var(--paper-100)',
      border: '2px solid var(--neon-cyan)'
    }
  }))));
}
Object.assign(__ds_scope, { NowPlayingBar });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/music/NowPlayingBar.jsx", error: String((e && e.message) || e) }); }

// components/music/QueueItem.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const {
  useState
} = React;
/**
 * Exit 66 Jukebox — QueueItem
 * A track waiting in the lineup. Shows position, requester and boost credits.
 */
function QueueItem({
  position = 1,
  title = 'Untitled',
  artist = 'Unknown',
  code = 'A6',
  requester = '',
  credits = 0,
  artTone = 'magenta',
  draggable = true,
  onRemove,
  style = {},
  ...rest
}) {
  const [hover, setHover] = useState(false);
  const tones = {
    magenta: 'linear-gradient(135deg, #2a0f1f, #ff2e88 280%)',
    cyan: 'linear-gradient(135deg, #08252b, #1fe0ff 280%)',
    amber: 'linear-gradient(135deg, #2a1d08, #ffb02e 280%)',
    violet: 'linear-gradient(135deg, #17112e, #8a6cff 280%)'
  };
  const initials = requester.split(' ').map(w => w[0]).filter(Boolean).slice(0, 2).join('').toUpperCase();
  return /*#__PURE__*/React.createElement("div", _extends({
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      padding: '10px 12px',
      borderRadius: 'var(--radius-md)',
      background: hover ? 'var(--bg-surface-hover)' : 'var(--bg-surface)',
      border: '1px solid var(--border-default)',
      transition: 'background var(--dur) var(--ease-out)',
      ...style
    }
  }, rest), draggable ? /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--text-disabled)',
      fontSize: 14,
      cursor: 'grab',
      letterSpacing: '-2px',
      flex: 'none'
    }
  }, "\u22EE\u22EE") : null, /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 15,
      fontWeight: 700,
      color: 'var(--neon-cyan)',
      width: 22,
      textAlign: 'center',
      flex: 'none'
    }
  }, position), /*#__PURE__*/React.createElement("div", {
    style: {
      width: 38,
      height: 38,
      flex: 'none',
      borderRadius: 'var(--radius-sm)',
      background: tones[artTone] || tones.magenta,
      display: 'flex',
      alignItems: 'flex-end',
      padding: 4,
      boxSizing: 'border-box'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 8,
      fontWeight: 700,
      color: 'rgba(255,255,255,0.85)'
    }
  }, code)), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      minWidth: 0
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontWeight: 600,
      fontSize: 14,
      color: 'var(--text-strong)',
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis'
    }
  }, title), /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 12,
      color: 'var(--text-muted)',
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis'
    }
  }, artist)), credits > 0 ? /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      gap: 5,
      fontFamily: 'var(--font-mono)',
      fontSize: 12,
      fontWeight: 700,
      color: 'var(--neon-amber)',
      flex: 'none'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 13
    }
  }, "\u25C8"), credits) : null, requester ? /*#__PURE__*/React.createElement("div", {
    title: requester,
    style: {
      width: 28,
      height: 28,
      flex: 'none',
      borderRadius: '50%',
      background: 'linear-gradient(140deg, var(--ink-700), var(--ink-850))',
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      fontWeight: 700,
      color: 'var(--paper-200)'
    }
  }, initials) : null, onRemove && hover ? /*#__PURE__*/React.createElement("button", {
    onClick: onRemove,
    "aria-label": "Remove",
    style: {
      background: 'none',
      border: 'none',
      color: 'var(--text-faint)',
      cursor: 'pointer',
      fontSize: 15,
      flex: 'none'
    }
  }, "\u2715") : null);
}
Object.assign(__ds_scope, { QueueItem });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/music/QueueItem.jsx", error: String((e && e.message) || e) }); }

// components/music/TrackRow.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const {
  useState
} = React;
/**
 * Exit 66 Jukebox — TrackRow
 * A selectable track in the crate/browse list. Art tile shows the jukebox slot code.
 */
function TrackRow({
  code = 'A6',
  title = 'Untitled',
  artist = 'Unknown',
  duration = '0:00',
  art = null,
  artTone = 'magenta',
  explicit = false,
  playing = false,
  onAdd,
  onClick,
  style = {},
  ...rest
}) {
  const [hover, setHover] = useState(false);
  const tones = {
    magenta: 'linear-gradient(135deg, #2a0f1f, #ff2e88 280%)',
    cyan: 'linear-gradient(135deg, #08252b, #1fe0ff 280%)',
    amber: 'linear-gradient(135deg, #2a1d08, #ffb02e 280%)',
    violet: 'linear-gradient(135deg, #17112e, #8a6cff 280%)'
  };
  return /*#__PURE__*/React.createElement("div", _extends({
    onClick: onClick,
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 14,
      padding: '10px 12px',
      borderRadius: 'var(--radius-md)',
      background: playing ? 'rgba(255,46,136,0.08)' : hover ? 'var(--bg-surface-hover)' : 'transparent',
      border: `1px solid ${playing ? 'rgba(255,46,136,0.35)' : 'transparent'}`,
      cursor: 'pointer',
      transition: 'background var(--dur) var(--ease-out)',
      ...style
    }
  }, rest), /*#__PURE__*/React.createElement("div", {
    style: {
      width: 46,
      height: 46,
      flex: 'none',
      borderRadius: 'var(--radius-sm)',
      background: art ? `center/cover url(${art})` : tones[artTone] || tones.magenta,
      display: 'flex',
      alignItems: 'flex-end',
      justifyContent: 'flex-start',
      padding: 5,
      boxSizing: 'border-box',
      boxShadow: playing ? 'var(--glow-magenta)' : 'inset 0 0 0 1px rgba(255,255,255,0.08)',
      position: 'relative',
      overflow: 'hidden'
    }
  }, !art ? /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 9,
      fontWeight: 700,
      letterSpacing: '0.1em',
      color: 'rgba(255,255,255,0.85)'
    }
  }, code) : null, playing ? /*#__PURE__*/React.createElement("span", {
    style: {
      position: 'absolute',
      top: 5,
      right: 5,
      width: 6,
      height: 6,
      borderRadius: '50%',
      background: 'var(--neon-magenta)'
    }
  }) : null), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      minWidth: 0
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 8
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontWeight: 600,
      fontSize: 15,
      color: playing ? 'var(--neon-magenta-bright)' : 'var(--text-strong)',
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis'
    }
  }, title), explicit ? /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 9,
      fontWeight: 700,
      color: 'var(--neon-amber)',
      border: '1px solid var(--neon-amber)',
      borderRadius: 2,
      padding: '0 3px',
      lineHeight: '13px',
      flex: 'none'
    }
  }, "E") : null), /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 13,
      color: 'var(--text-muted)',
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis'
    }
  }, artist)), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 13,
      color: 'var(--text-faint)',
      flex: 'none'
    }
  }, duration), onAdd ? /*#__PURE__*/React.createElement("button", {
    onClick: e => {
      e.stopPropagation();
      onAdd();
    },
    "aria-label": "Add to queue",
    style: {
      width: 34,
      height: 34,
      flex: 'none',
      borderRadius: 'var(--radius-sm)',
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      background: hover ? 'var(--neon-magenta)' : 'var(--bg-surface-raised)',
      color: hover ? 'var(--text-on-accent)' : 'var(--text-muted)',
      border: '1px solid var(--border-default)',
      cursor: 'pointer',
      fontSize: 18,
      lineHeight: 1,
      boxShadow: hover ? 'var(--glow-soft-magenta)' : 'none',
      transition: 'all var(--dur) var(--ease-out)'
    }
  }, "+") : null);
}
Object.assign(__ds_scope, { TrackRow });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/music/TrackRow.jsx", error: String((e && e.message) || e) }); }

// ui_kits/jukebox-app/AppShell.jsx
try { (() => {
// Exit 66 Jukebox — App shell chrome (Sidebar, TopBar, EntryScreen)
(() => {
  const {
    CreditMeter,
    Avatar,
    Button,
    Badge
  } = window.Exit66JukeboxDesignSystem_cf9d10;
  function Logo({
    compact
  }) {
    return /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 12
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        width: 40,
        height: 40,
        flex: 'none',
        borderRadius: 'var(--radius-md)',
        border: '1.5px solid var(--neon-magenta)',
        boxShadow: '0 0 0 2px var(--ink-900), 0 0 0 3.5px var(--neon-magenta)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontFamily: 'var(--font-display)',
        fontWeight: 700,
        fontSize: 20,
        color: 'var(--neon-magenta-bright)',
        background: 'rgba(255,46,136,0.05)',
        textShadow: 'var(--text-glow-magenta)'
      }
    }, "66"), !compact ? /*#__PURE__*/React.createElement("div", {
      style: {
        lineHeight: 1
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        fontFamily: 'var(--font-display)',
        fontWeight: 700,
        fontSize: 18,
        letterSpacing: '0.06em',
        color: 'var(--text-strong)'
      }
    }, "EXIT\xA0", /*#__PURE__*/React.createElement("span", {
      style: {
        color: 'var(--neon-cyan)'
      }
    }, "66")), /*#__PURE__*/React.createElement("div", {
      style: {
        fontFamily: 'var(--font-mono)',
        fontSize: 9,
        letterSpacing: '0.3em',
        color: 'var(--neon-amber)',
        marginTop: 3
      }
    }, "// JUKEBOX")) : null);
  }
  function NavItem({
    icon,
    label,
    active,
    onClick
  }) {
    const [hover, setHover] = React.useState(false);
    return /*#__PURE__*/React.createElement("button", {
      onClick: onClick,
      onMouseEnter: () => setHover(true),
      onMouseLeave: () => setHover(false),
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 14,
        width: '100%',
        padding: '12px 14px',
        borderRadius: 'var(--radius-md)',
        cursor: 'pointer',
        textAlign: 'left',
        background: active ? 'rgba(255,46,136,0.10)' : hover ? 'var(--bg-surface-hover)' : 'transparent',
        border: `1px solid ${active ? 'rgba(255,46,136,0.4)' : 'transparent'}`,
        color: active ? 'var(--neon-magenta-bright)' : 'var(--text-muted)',
        boxShadow: active ? 'var(--glow-soft-magenta)' : 'none',
        transition: 'all var(--dur) var(--ease-out)'
      }
    }, /*#__PURE__*/React.createElement("i", {
      "data-lucide": icon,
      style: {
        width: 19,
        height: 19
      }
    }), /*#__PURE__*/React.createElement("span", {
      style: {
        fontFamily: 'var(--font-display)',
        fontWeight: 600,
        fontSize: 14,
        letterSpacing: '0.06em',
        textTransform: 'uppercase'
      }
    }, label));
  }
  function Sidebar({
    screen,
    setScreen,
    queueCount
  }) {
    const items = [{
      id: 'now',
      icon: 'radio',
      label: 'Now Playing'
    }, {
      id: 'browse',
      icon: 'disc-3',
      label: 'The Crate'
    }, {
      id: 'queue',
      icon: 'list-music',
      label: 'The Lineup'
    }];
    return /*#__PURE__*/React.createElement("aside", {
      style: {
        width: 240,
        flex: 'none',
        borderRight: '1px solid var(--border-default)',
        background: 'var(--ink-900)',
        padding: 22,
        display: 'flex',
        flexDirection: 'column',
        gap: 28
      }
    }, /*#__PURE__*/React.createElement(Logo, null), /*#__PURE__*/React.createElement("nav", {
      style: {
        display: 'flex',
        flexDirection: 'column',
        gap: 6
      }
    }, items.map(it => /*#__PURE__*/React.createElement("div", {
      key: it.id,
      style: {
        position: 'relative'
      }
    }, /*#__PURE__*/React.createElement(NavItem, {
      icon: it.icon,
      label: it.label,
      active: screen === it.id,
      onClick: () => setScreen(it.id)
    }), it.id === 'queue' && queueCount > 0 ? /*#__PURE__*/React.createElement("span", {
      style: {
        position: 'absolute',
        right: 14,
        top: '50%',
        transform: 'translateY(-50%)',
        fontFamily: 'var(--font-mono)',
        fontSize: 11,
        fontWeight: 700,
        color: 'var(--neon-cyan)'
      }
    }, queueCount) : null))), /*#__PURE__*/React.createElement("div", {
      style: {
        marginTop: 'auto',
        padding: 14,
        borderRadius: 'var(--radius-md)',
        border: '1px solid var(--border-default)',
        background: 'var(--bg-surface)'
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        fontFamily: 'var(--font-mono)',
        fontSize: 10,
        letterSpacing: '0.2em',
        textTransform: 'uppercase',
        color: 'var(--text-faint)',
        marginBottom: 8
      }
    }, "Your credits"), /*#__PURE__*/React.createElement(CreditMeter, {
      credits: 42,
      onTopUp: () => {}
    })));
  }
  function TopBar({
    onTopUp
  }) {
    const v = window.E66 || {};
    return /*#__PURE__*/React.createElement("header", {
      style: {
        height: 64,
        flex: 'none',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '0 28px',
        borderBottom: '1px solid var(--border-default)',
        background: 'var(--ink-950)'
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 12
      }
    }, /*#__PURE__*/React.createElement("i", {
      "data-lucide": "map-pin",
      style: {
        width: 16,
        height: 16,
        color: 'var(--neon-cyan)'
      }
    }), /*#__PURE__*/React.createElement("span", {
      style: {
        fontFamily: 'var(--font-display)',
        fontWeight: 600,
        fontSize: 15,
        letterSpacing: '0.1em',
        textTransform: 'uppercase',
        color: 'var(--text-strong)'
      }
    }, v.venue), /*#__PURE__*/React.createElement("span", {
      style: {
        fontFamily: 'var(--font-mono)',
        fontSize: 11,
        color: 'var(--text-faint)',
        letterSpacing: '0.08em'
      }
    }, "\xB7 ", v.city)), /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 16
      }
    }, /*#__PURE__*/React.createElement(Badge, {
      tone: "success",
      dot: true
    }, "Live"), /*#__PURE__*/React.createElement(Avatar, {
      name: "You",
      ring: "cyan",
      size: "sm"
    })));
  }
  function EntryScreen({
    onEnter
  }) {
    const v = window.E66 || {};
    return /*#__PURE__*/React.createElement("div", {
      style: {
        position: 'absolute',
        inset: 0,
        zIndex: 50,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 30,
        background: 'radial-gradient(circle at 50% 30%, rgba(255,46,136,0.16), transparent 55%), radial-gradient(circle at 80% 90%, rgba(31,224,255,0.14), transparent 50%), var(--ink-980)',
        backgroundImage: 'var(--scanline)'
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        textAlign: 'center'
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        fontFamily: 'var(--font-mono)',
        fontSize: 13,
        letterSpacing: '0.4em',
        color: 'var(--neon-amber)',
        marginBottom: 18
      }
    }, "// NOW SERVING"), /*#__PURE__*/React.createElement("div", {
      style: {
        fontFamily: 'var(--font-display)',
        fontWeight: 700,
        fontSize: 84,
        lineHeight: 0.92,
        letterSpacing: '0.04em',
        color: 'var(--text-strong)'
      }
    }, "EXIT\xA0", /*#__PURE__*/React.createElement("span", {
      style: {
        color: 'var(--neon-cyan)'
      }
    }, "66")), /*#__PURE__*/React.createElement("div", {
      style: {
        fontFamily: 'var(--font-display)',
        fontWeight: 600,
        fontSize: 22,
        letterSpacing: '0.5em',
        color: 'var(--text-muted)',
        marginTop: 10,
        paddingLeft: '0.5em'
      }
    }, "JUKEBOX")), /*#__PURE__*/React.createElement("div", {
      style: {
        fontFamily: 'var(--font-sans)',
        fontSize: 16,
        color: 'var(--text-muted)',
        textAlign: 'center',
        maxWidth: 360
      }
    }, "You're at ", /*#__PURE__*/React.createElement("span", {
      style: {
        color: 'var(--neon-cyan)'
      }
    }, v.venue), ". Queue a track, boost it with credits, hold the floor."), /*#__PURE__*/React.createElement(Button, {
      variant: "primary",
      size: "lg",
      icon: /*#__PURE__*/React.createElement("i", {
        "data-lucide": "log-in"
      }),
      onClick: onEnter
    }, "Tap In"));
  }
  Object.assign(window, {
    Sidebar,
    TopBar,
    EntryScreen,
    Logo
  });
})();
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/jukebox-app/AppShell.jsx", error: String((e && e.message) || e) }); }

// ui_kits/jukebox-app/BrowseScreen.jsx
try { (() => {
// Exit 66 Jukebox — Browse / Crate screen
(() => {
  const {
    Input,
    Select,
    Badge,
    TrackRow
  } = window.Exit66JukeboxDesignSystem_cf9d10;
  function BrowseScreen({
    crate,
    genre,
    setGenre,
    query,
    setQuery,
    onAdd,
    playingCode
  }) {
    const genres = window.E66 && window.E66.genres || ['All'];
    const filtered = crate.filter(t => {
      const g = genre === 'All' || t.genre === genre;
      const q = !query || (t.title + ' ' + t.artist + ' ' + t.code).toLowerCase().includes(query.toLowerCase());
      return g && q;
    });
    return /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        minHeight: 0
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        alignItems: 'flex-end',
        gap: 16,
        marginBottom: 18
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        flex: 1
      }
    }, /*#__PURE__*/React.createElement(Input, {
      label: "Search the crate",
      icon: /*#__PURE__*/React.createElement("i", {
        "data-lucide": "search"
      }),
      value: query,
      onChange: e => setQuery(e.target.value),
      placeholder: "Artist, track, or slot code\u2026"
    })), /*#__PURE__*/React.createElement("div", {
      style: {
        width: 200
      }
    }, /*#__PURE__*/React.createElement(Select, {
      label: "Sort",
      value: "boosted",
      onChange: () => {},
      options: [{
        value: 'boosted',
        label: 'Most boosted'
      }, {
        value: 'az',
        label: 'A–Z'
      }, {
        value: 'new',
        label: 'Just added'
      }]
    }))), /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        gap: 8,
        marginBottom: 18,
        flexWrap: 'wrap'
      }
    }, genres.map(g => /*#__PURE__*/React.createElement("button", {
      key: g,
      onClick: () => setGenre(g),
      style: {
        background: 'none',
        border: 'none',
        padding: 0,
        cursor: 'pointer'
      }
    }, /*#__PURE__*/React.createElement(Badge, {
      tone: genre === g ? 'cyan' : 'neutral',
      variant: genre === g ? 'soft' : 'outline'
    }, g)))), /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'baseline',
        marginBottom: 6
      }
    }, /*#__PURE__*/React.createElement("span", {
      style: {
        fontFamily: 'var(--font-mono)',
        fontSize: 11,
        letterSpacing: '0.18em',
        textTransform: 'uppercase',
        color: 'var(--text-faint)'
      }
    }, filtered.length, " tracks"), /*#__PURE__*/React.createElement("span", {
      style: {
        fontFamily: 'var(--font-mono)',
        fontSize: 11,
        letterSpacing: '0.18em',
        textTransform: 'uppercase',
        color: 'var(--text-faint)'
      }
    }, "Tap + to queue")), /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        flexDirection: 'column',
        gap: 2,
        overflowY: 'auto',
        minHeight: 0,
        marginRight: -8,
        paddingRight: 8
      }
    }, filtered.map(t => /*#__PURE__*/React.createElement(TrackRow, {
      key: t.code,
      code: t.code,
      title: t.title,
      artist: t.artist,
      duration: t.duration,
      artTone: t.tone,
      explicit: t.explicit,
      playing: t.code === playingCode,
      onAdd: () => onAdd(t)
    })), filtered.length === 0 ? /*#__PURE__*/React.createElement("div", {
      style: {
        padding: 40,
        textAlign: 'center',
        fontFamily: 'var(--font-mono)',
        color: 'var(--text-faint)',
        letterSpacing: '0.1em'
      }
    }, "NO MATCHES ON THIS SIDE") : null));
  }
  window.BrowseScreen = BrowseScreen;
})();
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/jukebox-app/BrowseScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/jukebox-app/NowPlayingScreen.jsx
try { (() => {
// Exit 66 Jukebox — Now Playing screen
(() => {
  const {
    Badge,
    CreditMeter,
    QueueItem
  } = window.Exit66JukeboxDesignSystem_cf9d10;
  const npTones = {
    magenta: 'linear-gradient(150deg, #2a0f1f, #ff2e88 320%)',
    cyan: 'linear-gradient(150deg, #08252b, #1fe0ff 320%)',
    amber: 'linear-gradient(150deg, #2a1d08, #ffb02e 320%)',
    violet: 'linear-gradient(150deg, #17112e, #8a6cff 320%)'
  };
  const npGlow = {
    magenta: 'var(--glow-magenta)',
    cyan: 'var(--glow-cyan)',
    amber: 'var(--glow-amber)',
    violet: '0 0 0 1.5px var(--neon-violet)'
  };
  function Equalizer({
    on,
    color
  }) {
    const bars = [0.5, 0.85, 0.35, 1, 0.6, 0.9, 0.45, 0.75, 0.55, 0.95, 0.4, 0.7];
    return /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        alignItems: 'flex-end',
        gap: 4,
        height: 56
      }
    }, bars.map((h, i) => /*#__PURE__*/React.createElement("div", {
      key: i,
      style: {
        width: 5,
        height: `${h * 100}%`,
        borderRadius: 0,
        background: color,
        transformOrigin: 'bottom',
        animation: on ? `e66-eq 0.9s ease-in-out ${i * 0.08}s infinite alternate` : 'none',
        opacity: on ? 1 : 0.3
      }
    })));
  }
  function NowPlayingScreen({
    np,
    queue,
    playing,
    onAddCredits
  }) {
    const tone = np.tone || 'cyan';
    const color = `var(--neon-${tone})`;
    return /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'grid',
        gridTemplateColumns: '1.5fr 1fr',
        gap: 28,
        height: '100%',
        minHeight: 0
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        position: 'relative',
        borderRadius: 'var(--radius-xl)',
        overflow: 'hidden',
        border: '1px solid var(--border-default)',
        background: 'var(--ink-900)',
        backgroundImage: 'radial-gradient(circle at 70% 0%, rgba(31,224,255,0.12), transparent 55%), var(--scanline)',
        padding: 36,
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'space-between'
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center'
      }
    }, /*#__PURE__*/React.createElement(Badge, {
      tone: "success",
      dot: true
    }, "On Air"), /*#__PURE__*/React.createElement("span", {
      style: {
        fontFamily: 'var(--font-mono)',
        fontSize: 12,
        letterSpacing: '0.2em',
        color: 'var(--text-faint)'
      }
    }, "SIDE A \xB7 ", np.code)), /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        alignItems: 'flex-end',
        gap: 28
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        width: 168,
        height: 168,
        flex: 'none',
        borderRadius: 'var(--radius-lg)',
        background: npTones[tone],
        boxShadow: npGlow[tone],
        display: 'flex',
        alignItems: 'flex-end',
        padding: 16,
        boxSizing: 'border-box'
      }
    }, /*#__PURE__*/React.createElement("span", {
      style: {
        fontFamily: 'var(--font-mono)',
        fontSize: 22,
        fontWeight: 700,
        color: 'rgba(255,255,255,0.92)'
      }
    }, np.code)), /*#__PURE__*/React.createElement("div", {
      style: {
        minWidth: 0,
        paddingBottom: 4
      }
    }, /*#__PURE__*/React.createElement(Equalizer, {
      on: playing,
      color: color
    }), /*#__PURE__*/React.createElement("h1", {
      style: {
        margin: '14px 0 6px',
        fontFamily: 'var(--font-display)',
        fontWeight: 700,
        fontSize: 46,
        lineHeight: 1,
        letterSpacing: '0.01em',
        color: 'var(--text-strong)'
      }
    }, np.title), /*#__PURE__*/React.createElement("div", {
      style: {
        fontFamily: 'var(--font-sans)',
        fontSize: 19,
        color: 'var(--text-muted)'
      }
    }, np.artist), /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        gap: 8,
        marginTop: 14
      }
    }, /*#__PURE__*/React.createElement(Badge, {
      tone: tone
    }, np.genre), np.explicit ? /*#__PURE__*/React.createElement(Badge, {
      tone: "amber",
      variant: "outline"
    }, "Explicit") : null))), /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        fontFamily: 'var(--font-sans)',
        fontSize: 13,
        color: 'var(--text-faint)'
      }
    }, /*#__PURE__*/React.createElement("span", {
      style: {
        width: 26,
        height: 26,
        borderRadius: '50%',
        background: 'linear-gradient(140deg, var(--ink-700), var(--ink-850))',
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontFamily: 'var(--font-mono)',
        fontSize: 10,
        fontWeight: 700,
        color: 'var(--paper-200)'
      }
    }, np.requester.split(' ').map(w => w[0]).join('')), "Dropped by ", /*#__PURE__*/React.createElement("span", {
      style: {
        color: 'var(--text-body)'
      }
    }, np.requester))), /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        marginBottom: 16
      }
    }, /*#__PURE__*/React.createElement("h2", {
      style: {
        margin: 0,
        fontFamily: 'var(--font-display)',
        fontWeight: 600,
        fontSize: 18,
        letterSpacing: '0.08em',
        textTransform: 'uppercase',
        color: 'var(--text-strong)'
      }
    }, "Up Next"), /*#__PURE__*/React.createElement(CreditMeter, {
      credits: 42,
      size: "sm",
      onTopUp: onAddCredits
    })), /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        flexDirection: 'column',
        gap: 10,
        overflowY: 'auto',
        minHeight: 0
      }
    }, queue.map((t, i) => /*#__PURE__*/React.createElement(QueueItem, {
      key: t.code,
      position: i + 1,
      code: t.code,
      title: t.title,
      artist: t.artist,
      requester: t.requester,
      credits: t.credits,
      artTone: t.tone,
      draggable: false
    })))));
  }
  window.NowPlayingScreen = NowPlayingScreen;
})();
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/jukebox-app/NowPlayingScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/jukebox-app/QueueScreen.jsx
try { (() => {
// Exit 66 Jukebox — Queue / Lineup screen
(() => {
  const {
    QueueItem,
    Switch,
    Badge,
    Button
  } = window.Exit66JukeboxDesignSystem_cf9d10;
  function QueueScreen({
    queue,
    onRemove,
    autoDj,
    setAutoDj
  }) {
    const totalCredits = queue.reduce((s, t) => s + (t.credits || 0), 0);
    return /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        minHeight: 0
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        marginBottom: 20
      }
    }, /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement("h1", {
      style: {
        margin: 0,
        fontFamily: 'var(--font-display)',
        fontWeight: 700,
        fontSize: 30,
        letterSpacing: '0.02em',
        textTransform: 'uppercase',
        color: 'var(--text-strong)'
      }
    }, "The Lineup"), /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        gap: 10,
        marginTop: 8
      }
    }, /*#__PURE__*/React.createElement(Badge, {
      tone: "cyan"
    }, queue.length, " in queue"), /*#__PURE__*/React.createElement(Badge, {
      tone: "amber",
      variant: "outline"
    }, "\u25C8 ", totalCredits, " boosted"))), /*#__PURE__*/React.createElement(Switch, {
      label: "Auto-DJ when idle",
      checked: autoDj,
      onChange: setAutoDj,
      tone: "cyan"
    })), /*#__PURE__*/React.createElement("div", {
      style: {
        display: 'flex',
        flexDirection: 'column',
        gap: 10,
        overflowY: 'auto',
        minHeight: 0,
        marginRight: -8,
        paddingRight: 8
      }
    }, queue.map((t, i) => /*#__PURE__*/React.createElement(QueueItem, {
      key: t.code,
      position: i + 1,
      code: t.code,
      title: t.title,
      artist: t.artist,
      requester: t.requester,
      credits: t.credits,
      artTone: t.tone,
      onRemove: () => onRemove(t)
    })), queue.length === 0 ? /*#__PURE__*/React.createElement("div", {
      style: {
        padding: 48,
        textAlign: 'center',
        border: '1px dashed var(--border-strong)',
        borderRadius: 'var(--radius-lg)'
      }
    }, /*#__PURE__*/React.createElement("div", {
      style: {
        fontFamily: 'var(--font-display)',
        fontSize: 20,
        letterSpacing: '0.06em',
        textTransform: 'uppercase',
        color: 'var(--text-muted)',
        marginBottom: 8
      }
    }, "The floor is yours"), /*#__PURE__*/React.createElement("div", {
      style: {
        fontFamily: 'var(--font-sans)',
        color: 'var(--text-faint)'
      }
    }, "Head to the crate and drop the first track.")) : null));
  }
  window.QueueScreen = QueueScreen;
})();
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/jukebox-app/QueueScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/jukebox-app/data.js
try { (() => {
// Exit 66 Jukebox — mock data for the UI kit
window.E66 = {
  venue: 'THE LAST OFFRAMP',
  city: 'Sector 66 · Open till 4AM',
  nowPlaying: {
    code: 'A6',
    title: 'Midnight Loop',
    artist: 'Neon Saito',
    genre: 'Synthwave',
    tone: 'cyan',
    explicit: false,
    current: 142,
    duration: 251,
    requester: 'DJ Halcyon'
  },
  queue: [{
    code: 'B2',
    title: 'Chrome Highway',
    artist: 'The Offramps',
    tone: 'magenta',
    requester: 'Route Runner',
    credits: 8,
    explicit: true
  }, {
    code: 'D4',
    title: 'After Hours',
    artist: 'VX',
    tone: 'violet',
    requester: 'Mika',
    credits: 5
  }, {
    code: 'C9',
    title: 'Sodium Lights',
    artist: 'Halcyon',
    tone: 'amber',
    requester: 'Jules',
    credits: 3
  }, {
    code: 'F1',
    title: 'Static Bloom',
    artist: 'Cassette Ghost',
    tone: 'cyan',
    requester: 'Andre',
    credits: 0
  }],
  crate: [{
    code: 'A6',
    title: 'Midnight Loop',
    artist: 'Neon Saito',
    duration: '4:11',
    genre: 'Synthwave',
    tone: 'cyan'
  }, {
    code: 'B2',
    title: 'Chrome Highway',
    artist: 'The Offramps',
    duration: '4:11',
    genre: 'Darkwave',
    tone: 'magenta',
    explicit: true
  }, {
    code: 'C9',
    title: 'Sodium Lights',
    artist: 'Halcyon',
    duration: '2:58',
    genre: 'City Pop',
    tone: 'amber'
  }, {
    code: 'D4',
    title: 'After Hours',
    artist: 'VX',
    duration: '3:36',
    genre: 'Synthwave',
    tone: 'violet'
  }, {
    code: 'E7',
    title: 'Neon Rust',
    artist: 'The Offramps',
    duration: '3:12',
    genre: 'Darkwave',
    tone: 'magenta'
  }, {
    code: 'F1',
    title: 'Static Bloom',
    artist: 'Cassette Ghost',
    duration: '5:02',
    genre: 'Lo-Fi Drive',
    tone: 'cyan'
  }, {
    code: 'G3',
    title: 'Tail Lights',
    artist: 'Mira Vale',
    duration: '3:48',
    genre: 'City Pop',
    tone: 'amber',
    explicit: true
  }, {
    code: 'H8',
    title: 'Overpass',
    artist: 'Null Division',
    duration: '4:27',
    genre: 'Synthwave',
    tone: 'violet'
  }],
  genres: ['All', 'Synthwave', 'Darkwave', 'City Pop', 'Lo-Fi Drive']
};
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/jukebox-app/data.js", error: String((e && e.message) || e) }); }

__ds_ns.Avatar = __ds_scope.Avatar;

__ds_ns.Badge = __ds_scope.Badge;

__ds_ns.Button = __ds_scope.Button;

__ds_ns.Card = __ds_scope.Card;

__ds_ns.IconButton = __ds_scope.IconButton;

__ds_ns.Dialog = __ds_scope.Dialog;

__ds_ns.ProgressBar = __ds_scope.ProgressBar;

__ds_ns.Toast = __ds_scope.Toast;

__ds_ns.Tooltip = __ds_scope.Tooltip;

__ds_ns.Input = __ds_scope.Input;

__ds_ns.Select = __ds_scope.Select;

__ds_ns.Slider = __ds_scope.Slider;

__ds_ns.Switch = __ds_scope.Switch;

__ds_ns.CreditMeter = __ds_scope.CreditMeter;

__ds_ns.NowPlayingBar = __ds_scope.NowPlayingBar;

__ds_ns.QueueItem = __ds_scope.QueueItem;

__ds_ns.TrackRow = __ds_scope.TrackRow;

})();
