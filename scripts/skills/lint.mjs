// Lints the canonical assistant skill library. Node built-ins only.
// Usage: node scripts/skills/lint.mjs [library-root]
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export const LIMITS = {
  descriptionMax: 300,
  bodyMaxLines: 120,
  bodyMaxBytes: 6000,
  nameMax: 64,
  factMaxAgeDays: 90,
};

const SLUG = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const DAY = 86400000;

// Matched per line. Patterns are written so the README can name them without
// embedding a sample that this scan would flag.
export const SECRET_PATTERNS = [
  { name: 'aws-access-key', re: /AKIA[0-9A-Z]{16}/ },
  { name: 'private-key', re: /-----BEGIN (?:[A-Z0-9]+ )?PRIVATE KEY-----/ },
  { name: 'github-token', re: /ghp_[A-Za-z0-9]{20,}/ },
  { name: 'github-pat', re: /github_pat_[A-Za-z0-9_]{20,}/ },
  { name: 'slack-token', re: /xox[baprs]-[A-Za-z0-9-]{10,}/ },
  { name: 'openai-style-key', re: /\bsk-[A-Za-z0-9]{20,}/ },
  { name: 'assigned-secret', re: /(?:api[_-]?key|client[_-]?secret|secret|token|password)\s*[:=]\s*['"][^'"\s]{8,}['"]/i },
];

const here = path.dirname(fileURLToPath(import.meta.url));
export const defaultLibraryRoot = path.resolve(here, '../../.agents/library');

export function parseFrontmatter(text) {
  const normalized = text.replace(/\r\n/g, '\n');
  if (!normalized.startsWith('---\n')) return { error: 'SKILL.md must start with --- frontmatter' };
  const end = normalized.indexOf('\n---\n', 4);
  if (end < 0) return { error: 'SKILL.md frontmatter is not closed with ---' };
  const fields = {};
  for (const line of normalized.slice(4, end).split('\n')) {
    if (line === '') continue;
    const match = /^([A-Za-z][A-Za-z0-9_-]*):\s*(.*)$/.exec(line);
    if (!match) return { error: `frontmatter line is not a simple key: ${line}` };
    if (Object.hasOwn(fields, match[1])) return { error: `duplicate frontmatter key ${match[1]}` };
    let value = match[2].trim();
    if (value.length >= 2 && value.startsWith('"') && value.endsWith('"')) value = value.slice(1, -1);
    fields[match[1]] = value;
  }
  return { fields, body: normalized.slice(end + '\n---\n'.length) };
}

export function parseVerifiedDate(value) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const date = new Date(Date.UTC(year, month - 1, day));
  if (date.getUTCFullYear() !== year || date.getUTCMonth() !== month - 1 || date.getUTCDate() !== day) return null;
  return date;
}

function display(root, file) {
  return path.relative(root, file).split(path.sep).join('/');
}

function characters(value) {
  return Array.from(value).length;
}

function bodyLineCount(body) {
  if (body === '') return 0;
  const parts = body.split('\n');
  if (body.endsWith('\n')) parts.pop();
  return parts.length;
}

function sections(text) {
  return text.replace(/\r\n/g, '\n').split(/^## /m).slice(1).map(part => {
    const breakAt = part.indexOf('\n');
    return {
      title: (breakAt < 0 ? part : part.slice(0, breakAt)).trim(),
      body: breakAt < 0 ? '' : part.slice(breakAt + 1),
    };
  });
}

function scanSecrets(file, text, errors) {
  text.split(/\r?\n/).forEach((line, index) => {
    for (const pattern of SECRET_PATTERNS) {
      if (pattern.re.test(line)) errors.push(`${file}:${index + 1}: matches secret pattern ${pattern.name}`);
    }
  });
}

function lintFacts(file, text, now, errors, warnings) {
  const found = sections(text);
  if (found.length === 0) {
    errors.push(`${file}: facts.md needs at least one ## fact`);
    return;
  }
  const today = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate());
  for (const section of found) {
    const dates = [];
    for (const line of section.body.split('\n')) {
      const match = /(?:^|\s)verified:\s*(\S+)/.exec(line);
      if (match) dates.push(match[1]);
    }
    const label = `${file}: fact "${section.title}"`;
    if (dates.length === 0) errors.push(`${label} has no verified: date`);
    if (dates.length > 1) errors.push(`${label} has more than one verified: date`);
    for (const raw of dates) {
      const parsed = parseVerifiedDate(raw);
      if (!parsed) {
        errors.push(`${label} verified date ${raw} is not a real YYYY-MM-DD`);
        continue;
      }
      const age = Math.round((today - parsed.getTime()) / DAY);
      if (age < 0) errors.push(`${label} verified date ${raw} is in the future`);
      else if (age > LIMITS.factMaxAgeDays) warnings.push(`${label} verified ${raw} is ${age} days old; re-check before use`);
    }
  }
}

function lintTests(file, text, errors) {
  const found = new Map(sections(text).map(section => [section.title, section.body]));
  for (const title of ['Should trigger', 'Should not trigger']) {
    const body = found.get(title);
    if (body === undefined) {
      errors.push(`${file}: tests.md is missing "## ${title}"`);
      continue;
    }
    if (!body.split('\n').some(line => /^\s*-\s+\S/.test(line))) {
      errors.push(`${file}: "## ${title}" needs at least one example prompt`);
    }
  }
}

function lintSkill(root, skill, now, errors, warnings) {
  for (const name of ['SKILL.md', 'facts.md', 'tests.md']) {
    if (!fs.existsSync(path.join(skill.dir, name))) errors.push(`${skill.label}: missing ${name}`);
  }
  const entries = fs.readdirSync(skill.dir, { withFileTypes: true });
  for (const entry of entries) {
    const full = path.join(skill.dir, entry.name);
    if (entry.isDirectory()) {
      errors.push(`${skill.label}: ${entry.name} is a subdirectory; link extra material by repo path instead`);
      continue;
    }
    if (!entry.isFile()) continue;
    scanSecrets(display(root, full), fs.readFileSync(full, 'utf8'), errors);
  }
  const skillFile = path.join(skill.dir, 'SKILL.md');
  if (!fs.existsSync(skillFile)) return;
  const parsed = parseFrontmatter(fs.readFileSync(skillFile, 'utf8'));
  const shown = display(root, skillFile);
  if (parsed.error) {
    errors.push(`${shown}: ${parsed.error}`);
    return;
  }
  for (const key of Object.keys(parsed.fields)) {
    if (key !== 'name' && key !== 'description') errors.push(`${shown}: unexpected frontmatter key ${key}`);
  }
  const name = parsed.fields.name ?? '';
  if (!name) errors.push(`${shown}: name is empty`);
  else {
    if (characters(name) > LIMITS.nameMax) errors.push(`${shown}: name is ${characters(name)} characters; maximum is ${LIMITS.nameMax}`);
    if (!SLUG.test(name)) errors.push(`${shown}: name must be lowercase letters, numbers, and single hyphens`);
    if (skill.flatName && name !== skill.flatName) errors.push(`${shown}: name is ${name}; flat export name is ${skill.flatName}`);
  }
  if (!Object.hasOwn(parsed.fields, 'description') || parsed.fields.description.length === 0) {
    errors.push(`${shown}: description is empty`);
  } else {
    const description = parsed.fields.description;
    if (description.includes('\n')) errors.push(`${shown}: description must be one line`);
    if (characters(description) > LIMITS.descriptionMax) {
      errors.push(`${shown}: description is ${characters(description)} characters; maximum is ${LIMITS.descriptionMax}`);
    }
    if (!description.startsWith('Use when ')) errors.push(`${shown}: description must start with "Use when "`);
  }
  const lines = bodyLineCount(parsed.body);
  const bytes = Buffer.byteLength(parsed.body, 'utf8');
  if (parsed.body.trim() === '') errors.push(`${shown}: body is empty`);
  if (lines > LIMITS.bodyMaxLines) errors.push(`${shown}: body is ${lines} lines; maximum is ${LIMITS.bodyMaxLines}`);
  if (bytes > LIMITS.bodyMaxBytes) errors.push(`${shown}: body is ${bytes} bytes; maximum is ${LIMITS.bodyMaxBytes}`);
  const factsFile = path.join(skill.dir, 'facts.md');
  if (fs.existsSync(factsFile)) lintFacts(display(root, factsFile), fs.readFileSync(factsFile, 'utf8'), now, errors, warnings);
  const testsFile = path.join(skill.dir, 'tests.md');
  if (fs.existsSync(testsFile)) lintTests(display(root, testsFile), fs.readFileSync(testsFile, 'utf8'), errors);
}

export function lintLibrary(libraryRoot, { now = new Date() } = {}) {
  const errors = [];
  const warnings = [];
  const skills = [];
  if (!fs.existsSync(libraryRoot) || !fs.statSync(libraryRoot).isDirectory()) {
    return { errors: [`${libraryRoot}: library root is missing`], warnings, skills };
  }
  const readme = path.join(libraryRoot, 'README.md');
  if (!fs.existsSync(readme)) errors.push('README.md: the library README is required');
  else scanSecrets('README.md', fs.readFileSync(readme, 'utf8'), errors);

  for (const entry of fs.readdirSync(libraryRoot, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
    if (entry.name === 'README.md') continue;
    const full = path.join(libraryRoot, entry.name);
    if (!entry.isDirectory()) {
      errors.push(`${entry.name}: unexpected file in the library root`);
      if (entry.isFile()) scanSecrets(entry.name, fs.readFileSync(full, 'utf8'), errors);
      continue;
    }
    if (entry.name === '_template') {
      skills.push({ dir: full, label: '_template', flatName: null, template: true });
      continue;
    }
    if (entry.name.startsWith('_')) {
      errors.push(`${entry.name}: only _template may use a leading underscore`);
      continue;
    }
    if (!SLUG.test(entry.name)) {
      errors.push(`${entry.name}: area name must be lowercase letters, numbers, and single hyphens`);
      continue;
    }
    if (fs.existsSync(path.join(full, 'SKILL.md'))) errors.push(`${entry.name}: an area directory cannot contain SKILL.md`);
    const children = fs.readdirSync(full, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name));
    if (children.length === 0) errors.push(`${entry.name}: area has no skills`);
    for (const child of children) {
      if (!child.isDirectory()) {
        errors.push(`${entry.name}/${child.name}: unexpected file in an area`);
        continue;
      }
      if (!SLUG.test(child.name)) {
        errors.push(`${entry.name}/${child.name}: skill name must be lowercase letters, numbers, and single hyphens`);
        continue;
      }
      const flatName = `shippin-${entry.name}-${child.name}`;
      if (characters(flatName) > LIMITS.nameMax) errors.push(`${entry.name}/${child.name}: flat name ${flatName} exceeds ${LIMITS.nameMax} characters`);
      skills.push({ dir: path.join(full, child.name), label: `${entry.name}/${child.name}`, flatName, template: false });
    }
  }
  for (const skill of skills) lintSkill(libraryRoot, skill, now, errors, warnings);
  return { errors, warnings, skills };
}

function main(args) {
  const root = path.resolve(args[0] ?? defaultLibraryRoot);
  const { errors, warnings, skills } = lintLibrary(root);
  for (const warning of warnings) console.error(`warning: ${warning}`);
  for (const error of errors) console.error(error);
  const published = skills.filter(skill => skill.flatName).length;
  console.log(`${published} skills, ${errors.length} errors, ${warnings.length} warnings`);
  if (errors.length) process.exitCode = 1;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main(process.argv.slice(2));
