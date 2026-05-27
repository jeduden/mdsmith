#!/usr/bin/env node
// pick-plan.mjs — parse PLAN.md from origin/main, surface plans by status,
// and report any git branches whose name matches each plan's number.
//
// Usage:
//   node pick-plan.mjs            # human-readable text
//   node pick-plan.mjs --json     # JSON array of {id,status,model,title,file,branches}
//   node pick-plan.mjs --available --json
//                                 # JSON array of only-🔲 plans (the ones to pick from)
//
// PR cross-referencing is intentionally NOT done here — the caller (Claude
// running this skill) queries GitHub via the MCP server and merges the
// result with this driver's output.

import { execFileSync } from 'node:child_process';

const REPO_ROOT = (() => {
  try {
    return execFileSync('git', ['rev-parse', '--show-toplevel'], {
      encoding: 'utf8',
    }).trim();
  } catch {
    console.error('pick-plan: not inside a git repository');
    process.exit(2);
  }
})();

function git(...args) {
  return execFileSync('git', args, { encoding: 'utf8', cwd: REPO_ROOT });
}

// Refresh origin/main so the parsed catalog reflects what's actually merged.
try {
  git('fetch', '--quiet', 'origin', 'main');
} catch {
  // offline — fall through and use whatever origin/main we have cached
}

let planMd;
try {
  planMd = git('show', 'origin/main:PLAN.md');
} catch (err) {
  console.error('pick-plan: cannot read PLAN.md from origin/main:', err.message);
  process.exit(2);
}

// Catalog rows look like:
// | 102 | 🔲     | opus   | [Builder interface and mdsmith build subcommand](plan/102_build-subcommand.md) |
// The `u` flag is required so the emoji alternation matches whole code
// points instead of half a surrogate pair.
const rowRE =
  /^\|\s*(\d+)\s*\|\s*(🔲|🔳|✅|⛔)\s*\|\s*([a-zA-Z]*)\s*\|\s*\[([^\]]+)\]\(([^)]+)\)\s*\|/gmu;

const plans = [];
for (const m of planMd.matchAll(rowRE)) {
  const file = m[5];
  plans.push({
    id: parseInt(m[1], 10),
    status: m[2],
    model: m[3] || '',
    title: m[4],
    file,
    slug: file.replace(/^plan\/\d+_/, '').replace(/\.md$/, ''),
  });
}

if (plans.length === 0) {
  console.error('pick-plan: parsed zero rows from PLAN.md — catalog format may have changed');
  process.exit(2);
}

const branches = git('branch', '-a', '--format=%(refname:short)')
  .split('\n')
  .map((s) => s.trim())
  .filter(Boolean)
  .filter((b) => !b.startsWith('origin/HEAD'));

function branchesForPlan(id) {
  const idStr = String(id);
  // A branch "belongs" to a plan when its name contains the plan id as a
  // token, with a non-digit boundary on at least one side so that 102 does
  // not also match 1020/1021. Common shapes:
  //   plan-102-build-subcommand
  //   plan/102-build-subcommand
  //   feature/plan-102
  //   102_build-subcommand           (matches the plan-file stem)
  const re = new RegExp(`(?:^|[^0-9])${idStr}(?![0-9])`);
  return branches.filter((b) => re.test(b));
}

for (const p of plans) {
  p.branches = branchesForPlan(p.id);
}

const args = new Set(process.argv.slice(2));
const wantJson = args.has('--json');
const onlyAvailable = args.has('--available');

let output = plans;
if (onlyAvailable) {
  output = plans.filter((p) => p.status === '🔲' && p.branches.length === 0);
}

if (wantJson) {
  process.stdout.write(JSON.stringify(output, null, 2) + '\n');
  process.exit(0);
}

const completed = plans.filter((p) => p.status === '✅');
const superseded = plans.filter((p) => p.status === '⛔');
const inProgress = plans.filter((p) => p.status === '🔳');
const notStarted = plans.filter((p) => p.status === '🔲');
const available = notStarted.filter((p) => p.branches.length === 0);
const claimed = notStarted.filter((p) => p.branches.length > 0);

console.log(`PLAN.md from origin/main: ${plans.length} plans`);
console.log(`  ${completed.length.toString().padStart(3)} completed (✅)`);
console.log(`  ${superseded.length.toString().padStart(3)} superseded (⛔)`);
console.log(`  ${inProgress.length.toString().padStart(3)} in progress (🔳)`);
console.log(`  ${notStarted.length.toString().padStart(3)} not started (🔲)  ${available.length} available, ${claimed.length} with a branch already`);
console.log();

if (inProgress.length > 0) {
  console.log('── in progress (🔳) ──');
  for (const p of inProgress) {
    const tag = p.branches.length
      ? `branches: ${p.branches.join(', ')}`
      : 'no matching branch locally';
    console.log(`  ${String(p.id).padStart(3)} [${p.model || '—'.padEnd(6)}] ${p.title}`);
    console.log(`        ${p.file} — ${tag}`);
  }
  console.log();
}

if (claimed.length > 0) {
  console.log('── not started, but a branch already exists (🔲 + branch) ──');
  for (const p of claimed) {
    console.log(`  ${String(p.id).padStart(3)} [${p.model || '—'.padEnd(6)}] ${p.title}`);
    console.log(`        ${p.file} — branches: ${p.branches.join(', ')}`);
  }
  console.log();
}

if (available.length > 0) {
  console.log('── available to start (🔲, no branch) ──');
  for (const p of available) {
    console.log(`  ${String(p.id).padStart(3)} [${p.model || '—'.padEnd(6)}] ${p.title}`);
    console.log(`        ${p.file}`);
  }
  console.log();
}

console.log('Next:  pass --json for machine-readable output, --available to drop everything but the start candidates.');
