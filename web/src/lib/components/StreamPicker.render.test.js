// @vitest-environment jsdom
import { afterEach, beforeEach, expect, test } from 'vitest';
import { mount, unmount } from 'svelte';
import StreamPicker from './StreamPicker.svelte';

// Criterion 11: the selector spans the shared streams, and the personal stream
// is a pinned control rather than an entry in that list. These mount the real
// component because the distinction is about where the control lives in the
// rendered tree, which source text cannot see.

let host;
let app;

beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
});

afterEach(() => {
  if (app) unmount(app);
  app = null;
  host.remove();
});

const STREAMS = [
  { id: 'house', name: 'House', house: true, listeners: 3 },
  { id: 'a1b2', name: 'Kitchen', house: false, listeners: 0 },
  { id: 'c3d4', name: 'Patio', house: false, listeners: 1 },
];

function render(props = {}) {
  app = mount(StreamPicker, {
    target: host,
    props: { streams: STREAMS, current: 'house', personalId: 'me', ...props },
  });
}

function optionLabels() {
  return [...host.querySelectorAll('[role="option"]')].map((el) =>
    el.textContent.replace(/\s+/g, ' ').trim()
  );
}

test('every shared stream is an option in the list', () => {
  render();
  const labels = optionLabels();
  expect(labels).toHaveLength(3);
  expect(labels.some((l) => l.startsWith('House'))).toBe(true);
  expect(labels.some((l) => l.startsWith('Kitchen'))).toBe(true);
  expect(labels.some((l) => l.startsWith('Patio'))).toBe(true);
});

test('the personal stream is pinned outside the shared list', () => {
  render();
  const list = host.querySelector('[role="listbox"]');
  const personal = [...host.querySelectorAll('button')].find((b) => b.textContent.trim() === 'Personal');
  expect(personal).toBeTruthy();
  expect(list.contains(personal)).toBe(false);
  expect(optionLabels().some((l) => l.includes('Personal'))).toBe(false);
});

test('selecting a stream reports its id', () => {
  const picked = [];
  render({ onSelect: (id) => picked.push(id) });
  const kitchen = [...host.querySelectorAll('[role="option"]')].find((b) =>
    b.textContent.includes('Kitchen'));
  kitchen.click();
  expect(picked).toEqual(['a1b2']);
});

test('selecting personal reports the personal id', () => {
  const picked = [];
  render({ onSelect: (id) => picked.push(id) });
  [...host.querySelectorAll('button')].find((b) => b.textContent.trim() === 'Personal').click();
  expect(picked).toEqual(['me']);
});

test('the tuned-in stream is the selected option', () => {
  render({ current: 'c3d4' });
  const selected = [...host.querySelectorAll('[role="option"]')].filter(
    (el) => el.getAttribute('aria-selected') === 'true');
  expect(selected).toHaveLength(1);
  expect(selected[0].textContent).toContain('Patio');
});

test('management controls are hidden without permission', () => {
  render({ canManage: false });
  expect(host.textContent).not.toContain('New stream');
  expect(host.querySelector('[aria-label="Delete Kitchen"]')).toBeNull();
  expect(host.querySelector('[aria-label="Rename Kitchen"]')).toBeNull();
});

test('house can be renamed but never deleted', () => {
  render({ canManage: true });
  expect(host.querySelector('[aria-label="Rename House"]')).toBeTruthy();
  expect(host.querySelector('[aria-label="Delete House"]')).toBeNull();
  expect(host.querySelector('[aria-label="Delete Kitchen"]')).toBeTruthy();
});

test('at the cap the create affordance is replaced by the limit notice', () => {
  render({ canManage: true, atCap: true });
  expect(host.textContent).not.toContain('New stream');
  expect(host.textContent).toContain('Stream limit reached');
});

test('creating a stream submits the trimmed name', async () => {
  const created = [];
  render({ canManage: true, onCreate: (name) => created.push(name) });
  [...host.querySelectorAll('button')].find((b) => b.textContent.includes('New stream')).click();
  await Promise.resolve();
  const input = host.querySelector('[aria-label="New stream name"]');
  input.value = '  Garage  ';
  input.dispatchEvent(new Event('input', { bubbles: true }));
  input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
  await Promise.resolve();
  expect(created).toEqual(['Garage']);
});

// #128: in the open security modes there is no personal stream, so the app
// passes personalId={null} and the pinned control must not be rendered at all
// — offering a control whose route 404s is worse than not offering it.
test('the pinned Personal control is absent when there is no personal stream', () => {
  render({ personalId: null });
  const labels = [...host.querySelectorAll('button')].map((el) => el.textContent.trim());
  expect(labels).not.toContain('Personal');
});

test('the pinned Personal control is present when there is one', () => {
  render({ personalId: 'me' });
  const labels = [...host.querySelectorAll('button')].map((el) => el.textContent.trim());
  expect(labels).toContain('Personal');
});
