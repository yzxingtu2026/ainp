import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { baseParse, NodeTypes } from '@vue/compiler-dom';
import { parse } from '@vue/compiler-sfc';

const DEFAULT_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const pageRootMarker = 'min-h-500px';
const forbiddenOverflowUtilities = new Set([
  'overflow-auto',
  'overflow-hidden',
  'overflow-scroll',
  'overflow-y-auto',
  'overflow-y-hidden',
  'overflow-y-scroll'
]);

async function collectVueFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(
    entries.map(entry => {
      const target = path.join(directory, entry.name);
      return entry.isDirectory() ? collectVueFiles(target) : path.extname(entry.name) === '.vue' ? [target] : [];
    })
  );
  return nested.flat();
}

function staticClassTokens(element) {
  const classAttribute = element.props.find(prop => prop.type === NodeTypes.ATTRIBUTE && prop.name === 'class');
  return new Set(classAttribute?.value?.content.split(/\s+/).filter(Boolean) || []);
}

function overflowUtility(token) {
  const utility = token.slice(token.lastIndexOf(':') + 1);
  return forbiddenOverflowUtilities.has(utility) || utility.startsWith('overflow-y-') ? token : '';
}

export async function findListPageOverflowViolations(root = DEFAULT_ROOT) {
  const viewsRoot = path.join(root, 'src/views');
  const violations = [];
  for (const file of await collectVueFiles(viewsRoot)) {
    const relativePath = path.relative(root, file);
    const source = await readFile(file, 'utf8');
    const { descriptor, errors } = parse(source, { filename: relativePath });
    if (errors.length || !descriptor.template) {
      violations.push(`${relativePath}: template parse failed`);
      continue;
    }
    const template = baseParse(descriptor.template.content);
    for (const child of template.children) {
      if (child.type !== NodeTypes.ELEMENT) continue;
      const classes = staticClassTokens(child);
      if (!classes.has(pageRootMarker)) continue;
      for (const token of classes) {
        const forbidden = overflowUtility(token);
        if (forbidden) violations.push(`${relativePath}: page root uses ${forbidden}`);
      }
    }
  }
  return violations.sort();
}

if (process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url) {
  const violations = await findListPageOverflowViolations();
  if (violations.length) {
    console.error('List page roots must leave vertical scrolling to AdminLayout.');
    for (const violation of violations) console.error(`- ${violation}`);
    process.exitCode = 1;
  } else {
    console.log('List page overflow check passed.');
  }
}
