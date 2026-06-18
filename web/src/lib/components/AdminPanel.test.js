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
    expect(source).toContain('listLibraryPaths');
    expect(source).toContain('buildEditableSettingsSnapshot');
    expect(source).toContain('hasEditableSettingsChanges');
    expect(source).toContain('beforeUnloadIfDirty');
    expect(source).toContain('loadPathBrowserLocation');
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
