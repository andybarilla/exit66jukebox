import { readFileSync } from 'node:fs';
import { describe, expect, test } from 'vitest';

const source = readFileSync(new URL('./AdminPanel.svelte', import.meta.url), 'utf8');

describe('Listening groups admin wiring', () => {
  test('every group endpoint is wired to the panel', () => {
    expect(source).toMatch(
      /import\s*{[^}]*\bgetFederationGroups\b[^}]*\bcreateFederationGroup\b[^}]*\bdeleteFederationGroup\b[^}]*\baddFederationGroupMember\b[^}]*\bremoveFederationGroupMember\b[^}]*}\s*from\s*'\.\.\/auth\.js'/s,
    );
    expect(source).toContain('federationGroups = groupSettings.groups || [];');
  });

  // Andy's decision on #88: groups organise what peers see, and the UI has to
  // say so rather than implying a playback restriction it does not enforce.
  test('the wording says groups scope discovery, not playback', () => {
    const box = source.slice(source.indexOf('<h4>Listening groups</h4>'));
    expect(box).toContain('not a playback restriction');
    expect(box).toMatch(/still play it/);
    expect(box).toMatch(/no\s+groups at all, every approved peer sees every other/);
  });
});
