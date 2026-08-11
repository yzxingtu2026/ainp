import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

const sourceExtensions = new Set(['.vue', '.ts', '.tsx', '.js', '.jsx']);

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function lineAt(source, index) {
  return source.slice(0, index).split('\n').length;
}

export function findRequestErrorBoundaryViolations(source, file = 'fixture.ts') {
  const errorBindings = new Set();
  const resultBindings = new Set();
  const violations = [];

  for (const match of source.matchAll(/\b(?:const|let)\s*\{([\s\S]*?)\}\s*=\s*await\b/g)) {
    for (const binding of match[1].split(',')) {
      const errorBinding = binding.trim().match(/^error(?:\s*:\s*([A-Za-z_$][\w$]*))?$/);
      if (errorBinding) errorBindings.add(errorBinding[1] || 'error');
    }
  }

  for (const match of source.matchAll(/\b(?:const|let)\s+([A-Za-z_$][\w$]*)\s*=\s*await\b/g)) {
    resultBindings.add(match[1]);
  }

  for (const binding of errorBindings) {
    const pattern = new RegExp(`\\b${escapeRegExp(binding)}\\s*\\??\\.\\s*message\\b`, 'g');
    for (const match of source.matchAll(pattern)) {
      violations.push({ file, line: lineAt(source, match.index), binding, kind: 'destructured request error' });
    }
  }

  for (const binding of resultBindings) {
    const pattern = new RegExp(`\\b${escapeRegExp(binding)}\\s*\\.\\s*error\\s*\\??\\.\\s*message\\b`, 'g');
    for (const match of source.matchAll(pattern)) {
      violations.push({ file, line: lineAt(source, match.index), binding, kind: 'request result error' });
    }
  }

  return violations.sort((left, right) => left.line - right.line || left.binding.localeCompare(right.binding));
}

async function collectFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(
    entries.map(entry => {
      const target = path.join(directory, entry.name);
      return entry.isDirectory() ? collectFiles(target) : [target];
    })
  );
  return nested.flat();
}

async function main() {
  const sourceRoot = path.resolve('src');
  const violations = [];
  for (const file of await collectFiles(sourceRoot)) {
    if (!sourceExtensions.has(path.extname(file))) continue;
    const source = await readFile(file, 'utf8');
    violations.push(...findRequestErrorBoundaryViolations(source, path.relative(process.cwd(), file)));
  }

  if (violations.length) {
    console.error(
      'Unified request errors must not expose .message to UI. Use the default global handler or an explicit errorMode with getRequestErrorMessage().'
    );
    for (const violation of violations) {
      console.error(`- ${violation.file}:${violation.line}: ${violation.kind} (${violation.binding})`);
    }
    process.exitCode = 1;
    return;
  }
  console.log('Request error boundary check passed.');
}

if (process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url) {
  await main();
}
