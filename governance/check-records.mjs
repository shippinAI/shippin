import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { validatePlan, validateJournal } from './validate.mjs';

const root = path.dirname(fileURLToPath(import.meta.url));
try {
  const plans = fs.readdirSync(path.join(root, 'plans')).filter(name => name.endsWith('.json'));
  for (const name of plans) {
    const plan = JSON.parse(fs.readFileSync(path.join(root, 'plans', name), 'utf8'));
    validatePlan(plan);
    const journal = path.join(root, 'journal', name.replace(/\.json$/, '.jsonl'));
    const entries = fs.readFileSync(journal, 'utf8').split(/\r?\n/).filter(line => line.trim()).map(line => JSON.parse(line));
    validateJournal(plan, entries);
  }
  console.log(`Validated ${plans.length} plans and journals`);
} catch (err) { console.error(err.message); process.exitCode = 1; }
