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

// Refuse linked or non-directory ancestors before creating anything. This is a
// local authoring tool: callers must also exclude concurrent filesystem writers.
function checkDirectory(directory) {
  const parent = path.dirname(directory);
  if (parent !== directory) checkDirectory(parent);
  let stat;
  try { stat = fs.lstatSync(directory); }
  catch (error) { if (error.code === 'ENOENT') return; throw error; }
  if (!stat.isDirectory()) throw new Error(`export path must be an unlinked directory: ${directory}`);
}

export function exportLibrary(libraryRoot, outDir) {
  if (!fs.lstatSync(libraryRoot).isDirectory()) throw new Error('library root must be an unlinked directory');
  libraryRoot = fs.realpathSync(libraryRoot);
  outDir = path.resolve(outDir);
  checkDirectory(outDir);
  if (inside(libraryRoot, outDir)) {
    throw new Error('refusing to write the export inside the canonical library');
  }
  const { errors, warnings, skills } = lintLibrary(libraryRoot);
  if (errors.length) {
    const failure = new Error(errors.join('\n'));
    failure.errors = errors;
    throw failure;
  }
  const published = skills.filter(skill => skill.flatName);
  // Preflight every destination before writing the first skill. Existing exact
  // copies are harmless; differing files or aliases must never be overwritten.
  for (const skill of published) {
    const destination = path.join(outDir, skill.flatName);
    checkDirectory(destination);
    const target = path.join(destination, 'SKILL.md');
    let stat;
    try { stat = fs.lstatSync(target); }
    catch (error) { if (error.code === 'ENOENT') continue; throw error; }
    if (!stat.isFile() || stat.nlink !== 1 || !fs.readFileSync(target).equals(fs.readFileSync(path.join(skill.dir, 'SKILL.md')))) {
      throw new Error(`refusing to overwrite existing export file: ${target}`);
    }
  }
  fs.mkdirSync(outDir, { recursive: true });
  const written = [];
  for (const skill of published) {
    const destination = path.join(outDir, skill.flatName);
    fs.mkdirSync(destination, { recursive: true });
    const target = path.join(destination, 'SKILL.md');
    if (!fs.existsSync(target)) fs.copyFileSync(path.join(skill.dir, 'SKILL.md'), target, fs.constants.COPYFILE_EXCL);
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
