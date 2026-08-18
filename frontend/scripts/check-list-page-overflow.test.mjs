import assert from 'node:assert/strict';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { findListPageOverflowViolations } from './check-list-page-overflow.mjs';

async function writeView(root, name, template) {
  const directory = path.join(root, 'src/views', name);
  await mkdir(directory, { recursive: true });
  await writeFile(path.join(directory, 'index.vue'), `<template>${template}</template>`);
}

test('rejects vertical overflow utilities on list page roots', async t => {
  const root = await mkdtemp(path.join(os.tmpdir(), 'list-page-overflow-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  await writeView(
    root,
    'invalid',
    '<div class="min-h-500px flex-col-stretch overflow-hidden lt-sm:overflow-auto xl:overflow-y-clip"><NDataTable /></div>'
  );
  await writeView(
    root,
    'valid',
    '<div class="min-h-500px flex-col-stretch"><NDataTable /><div class="max-h-60vh overflow-y-auto" /></div>'
  );

  assert.deepEqual(await findListPageOverflowViolations(root), [
    'src/views/invalid/index.vue: page root uses lt-sm:overflow-auto',
    'src/views/invalid/index.vue: page root uses overflow-hidden',
    'src/views/invalid/index.vue: page root uses xl:overflow-y-clip'
  ]);
});
