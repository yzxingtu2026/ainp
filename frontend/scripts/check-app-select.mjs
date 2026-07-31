import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';

const sourceRoot = path.resolve('src');
const allowedFiles = new Set([
  path.join(sourceRoot, 'components/common/app-select.vue'),
  path.join(sourceRoot, 'components/common/app-select.test.ts'),
  path.join(sourceRoot, 'typings/components.d.ts')
]);
const sourceExtensions = new Set(['.vue', '.ts', '.tsx', '.js', '.jsx']);
const forbiddenPatterns = [
  { label: 'template usage', pattern: /<\/?NSelect\b/ },
  { label: 'Naive UI import', pattern: /import[\s\S]*?\bNSelect\b[\s\S]*?from\s+['"]naive-ui['"]/ },
  { label: 'dynamic rendering', pattern: /\bh\(\s*NSelect\b/ }
];

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

const violations = [];
for (const file of await collectFiles(sourceRoot)) {
  if (allowedFiles.has(file) || !sourceExtensions.has(path.extname(file))) continue;
  const content = await readFile(file, 'utf8');
  for (const { label, pattern } of forbiddenPatterns) {
    if (pattern.test(content)) violations.push(`${path.relative(process.cwd(), file)}: ${label}`);
  }
}

if (violations.length > 0) {
  console.error('Direct NSelect usage is forbidden. Use AppSelect and follow docs/development/frontend-stack.md.');
  for (const violation of violations) console.error(`- ${violation}`);
  process.exitCode = 1;
} else {
  console.log('AppSelect architecture check passed.');
}
