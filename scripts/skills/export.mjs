// Copies each canonical SKILL.md into <output>/<flat-name>/SKILL.md.
// Does not contact an assistant, delete stale folders, or copy facts.md or tests.md.
// Usage: node scripts/skills/export.mjs <output-directory>
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { defaultLibraryRoot, lintLibrary } from './lint.mjs';

function inside(parent, child) {
  const relative = path.relative(path.resolve(parent), path.resolve(child));
  return relative === '' || (!relative.startsWith('..') && !path.isAbsolute(relative));
}

export function exportLibrary(libraryRoot, outDir) {
  if (inside(libraryRoot, outDir)) {
    throw new Error('refusing to write the export inside the canonical library');
  }
  const { errors, warnings, skills } = lintLibrary(libraryRoot);
  if (errors.length) {
    const failure = new Error(errors.join('\n'));
    failure.errors = errors;
    throw failure;
  }
  fs.mkdirSync(outDir, { recursive: true });
  const written = [];
  for (const skill of skills) {
    if (!skill.flatName) continue;
    const destination = path.join(outDir, skill.flatName);
    fs.mkdirSync(destination, { recursive: true });
    fs.copyFileSync(path.join(skill.dir, 'SKILL.md'), path.join(destination, 'SKILL.md'));
    written.push(skill.flatName);
  }
  return { written, warnings };
}

function main(args) {
  const outDir = args[0];
  if (!outDir) {
    console.error('usage: node scripts/skills/export.mjs <output-directory>');
    process.exitCode = 2;
    return;
  }
  try {
    const { written, warnings } = exportLibrary(defaultLibraryRoot, path.resolve(outDir));
    for (const warning of warnings) console.error(`warning: ${warning}`);
    for (const name of written) console.log(`${name}/SKILL.md`);
    console.log(`${written.length} skills exported`);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main(process.argv.slice(2));
