import { createHash } from 'node:crypto';
import { cp, mkdir, readFile, readdir, stat, writeFile } from 'node:fs/promises';
import { resolve, sep } from 'node:path';
import process from 'node:process';

const root = resolve(import.meta.dirname, '..');
const manifestPath = resolve(root, 'admin-baseline.json');
const baselineFiles = [
  'src/composition/application.ts',
  'src/composition/contract.ts',
  'src/composition/define-app.ts',
  'src/composition/index.ts',
  'src/composition/page.ts',
  'src/composition/core-pages.ts',
  'src/composition/system-pages.ts',
  'src/composition/operations-pages.ts',
  'src/composition/ai-pages.ts',
  'src/composition/demo-pages.ts',
  'src/composition/platform-layouts.ts',
  'src/composition/runtime.ts'
];

async function sha256(path) {
  return createHash('sha256').update(await readFile(path)).digest('hex');
}

async function buildManifest() {
  const pkg = JSON.parse(await readFile(resolve(root, 'package.json'), 'utf8'));
  const files = {};
  for (const file of baselineFiles) files[file] = await sha256(resolve(root, file));
  return {
    schemaVersion: 1,
    version: process.env.VITE_AINP_BASELINE_VERSION || pkg.version,
    commit: process.env.VITE_BUILD_COMMIT || 'development',
    node: pkg.engines.node,
    pnpm: pkg.engines.pnpm,
    generatedFilesVersion: 1,
    modules: (process.env.AINP_ADMIN_MODULES || 'core,system,operations,ai,demo').split(',').sort(),
    files
  };
}

async function generate() {
  await writeFile(manifestPath, `${JSON.stringify(await buildManifest(), null, 2)}\n`);
}

async function verify() {
  const expected = JSON.parse(await readFile(manifestPath, 'utf8'));
  const actual = await buildManifest();
  const drift = baselineFiles.filter(file => expected.files[file] !== actual.files[file]);
  if (drift.length) throw new Error(`AINP baseline drift: ${drift.join(', ')}`);
}

async function initialize(targetArg) {
  if (!targetArg) throw new Error('init requires a target directory');
  const target = resolve(process.cwd(), targetArg);
  if (target.startsWith(`${root}${sep}`)) throw new Error('target must be outside the AINP frontend directory');
  try {
    await stat(target);
    throw new Error(`target already exists: ${target}`);
  } catch (error) {
    if (error.code !== 'ENOENT') throw error;
  }
  await mkdir(target, { recursive: true });
  const excluded = new Set(['node_modules', 'dist']);
  for (const entry of await readdir(root)) {
    if (excluded.has(entry)) continue;
    await cp(resolve(root, entry), resolve(target, entry), { recursive: true, errorOnExist: true });
  }
}

const [command, argument] = process.argv.slice(2);
if (command === 'generate') await generate();
else if (command === 'verify' || command === 'preflight') await verify();
else if (command === 'init') await initialize(argument);
else throw new Error('usage: admin-baseline.mjs <generate|verify|preflight|init> [target]');
