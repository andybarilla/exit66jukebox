import { readFileSync } from 'node:fs';
import { describe, expect, test } from 'vitest';

const source = readFileSync(new URL('./AdminPanel.svelte', import.meta.url), 'utf8');

function functionBody(name) {
  const match = source.match(new RegExp(`function\\s+${name}\\s*\\([^)]*\\)\\s*{([\\s\\S]*?)\\n  }`));

  if (!match) throw new Error(`${name} function was not found`);

  return match[1];
}

describe('AdminPanel Svelte wiring', () => {
  test('imports and uses settings path browser helpers', () => {
    expect(source).toMatch(/import\s*{[^}]*\blistLibraryPaths\b[^}]*}\s*from\s*'\.\.\/auth\.js'/s);
    expect(source).toMatch(/import\s*{[^}]*\bbeforeUnloadIfDirty\b[^}]*\bbuildEditableSettingsSnapshot\b[^}]*\bhasEditableSettingsChanges\b[^}]*\bloadPathBrowserLocation\b[^}]*}\s*from\s*'\.\.\/settingsPanelState\.js'/s);
    expect(functionBody('handleBeforeUnload')).toMatch(/\bbeforeUnloadIfDirty\s*\(/);
    expect(functionBody('refreshCleanSettingsSnapshot')).toMatch(/\bbuildEditableSettingsSnapshot\s*\(/);
    expect(functionBody('updateUnsavedState')).toMatch(/\bhasEditableSettingsChanges\s*\(/);
    expect(functionBody('loadLibraryPath')).toMatch(/\bloadPathBrowserLocation\s*\(\s*listLibraryPaths\b/);
  });

  test('successful saves update only their clean settings subset', () => {
    expect(source).toContain('updateCleanSettingsSnapshot');
    expect(functionBody('saveLibraries')).toMatch(/updateCleanSettingsSnapshot\s*\(\s*{\s*libraries\s*,\s*federation\s*}\s*\)/);
    expect(functionBody('onToggleSignup')).toMatch(/updateCleanSettingsSnapshot\s*\(\s*{\s*signupEnabled\s*}\s*\)/);
    expect(functionBody('onToggleSignup')).not.toMatch(/refreshCleanSettingsSnapshot\s*\(/);
    expect(functionBody('onToggleGuest')).toMatch(/updateCleanSettingsSnapshot\s*\(\s*{\s*guestAccess\s*}\s*\)/);
    expect(functionBody('onToggleGuest')).not.toMatch(/refreshCleanSettingsSnapshot\s*\(/);
  });

  test('contains path browser state and folder selection wiring', () => {
    expect(source).toContain('openPathBrowser');
    expect(source).toContain('loadLibraryPath');
    expect(source).toContain('choosePathBrowserFolder');
    expect(source).toContain('pathBrowser.parent');
    expect(source).toContain('pathBrowser.directories');
    expect(source).toContain("setLibraryField(pathBrowser.row, 'path', pathBrowser.path)");
  });

  test('contains path browser controls', () => {
    expect(source).toContain('Browse');
    expect(source).toContain('Use this folder');
    expect(source).toContain('Cancel');
  });

  test('keeps path browser actions visible while folder list scrolls', () => {
    expect(source).toMatch(/\.path-browser-modal\s*{[^}]*display:\s*flex[^}]*flex-direction:\s*column/s);
    expect(source).toMatch(/\.path-browser-list\s*{[^}]*min-height:\s*0/s);
    expect(source).toMatch(/\.path-browser-list\s*{[^}]*flex:\s*1 1 auto/s);
    expect(source).toMatch(/\.path-browser-actions\s*{[^}]*flex:\s*none/s);
  });

  test('contains dirty close guard and beforeunload wiring', () => {
    expect(source).toContain('requestCloseSettings');
    expect(source).toContain("Discard unsaved settings changes?");
    expect(source).toContain('beforeunload');
    expect(source).toContain('cleanSettingsSnapshot');
    expect(source).toContain('hasUnsavedChanges');
    expect(source).toContain('refreshCleanSettingsSnapshot');
  });

  test('choosing a browser folder does not save libraries immediately', () => {
    expect(functionBody('choosePathBrowserFolder')).not.toMatch(/saveLibraries\s*\(/);
  });
});
