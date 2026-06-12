import { describe, it, expect } from 'vitest';
import { compareNames } from './format.js';

// sortNames is a tiny helper so each case reads as "this input order sorts to
// that output order" rather than asserting on raw comparator return signs.
const sortNames = (names) => [...names].sort(compareNames);

describe('compareNames', () => {
  it('ignores a leading "The"/"A"/"An" article', () => {
    expect(sortNames(['The Doors', 'Beatles', 'An Albatross', 'Apple']))
      .toEqual(['An Albatross', 'Apple', 'Beatles', 'The Doors']);
  });

  it('is case-insensitive (ABBA and abba sort adjacent)', () => {
    expect(sortNames(['ABBA', 'Beck', 'abba']))
      .toEqual(['ABBA', 'abba', 'Beck']);
  });

  it('folds accents (Édith sorts as Edith, interleaved with plain-E names)', () => {
    expect(sortNames(['Édith Piaf', 'Eels', 'Edge']))
      .toEqual(['Edge', 'Édith Piaf', 'Eels']);
  });

  it('strips leading punctuation so "...And Justice" files under A', () => {
    expect(sortNames(['Bjork', '...And Justice for All', 'Aphex']))
      .toEqual(['...And Justice for All', 'Aphex', 'Bjork']);
  });

  it('orders embedded numbers naturally (2 before 10)', () => {
    expect(sortNames(['Album 10', 'Album 2', 'Album 1']))
      .toEqual(['Album 1', 'Album 2', 'Album 10']);
  });

  it('treats only a whole-word article as an article, not a prefix', () => {
    // "Theatre" must not lose "The" — the article needs trailing whitespace.
    expect(sortNames(['Tom', 'Theatre', 'The Aviators']))
      .toEqual(['The Aviators', 'Theatre', 'Tom']);
  });

  it('does not throw on empty/undefined names', () => {
    expect(() => sortNames(['B', undefined, '', 'A', null])).not.toThrow();
  });
});
