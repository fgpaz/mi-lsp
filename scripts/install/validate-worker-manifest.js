#!/usr/bin/env node
'use strict';
// Validate an extracted worker bundle without requiring Python.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
function fail(message) { console.error(message); process.exit(1); }
const [manifestPath, workerRoot, expectedRID] = process.argv.slice(2);
if (!manifestPath || !workerRoot || !expectedRID) fail('usage: validate-worker-manifest.js MANIFEST WORKER_ROOT RID');
let doc;
try { doc = JSON.parse(fs.readFileSync(manifestPath, 'utf8')); }
catch (e) { fail(`Worker manifest is not valid UTF-8 JSON: ${e.message}`); }
if (!doc || typeof doc !== 'object' || Array.isArray(doc)) fail('Worker manifest root must be a JSON object.');
if (doc.schema !== 'mi-lsp-worker-manifest/v1') fail('Worker manifest schema is invalid.');
if (doc.rid !== expectedRID) fail('Worker manifest RID does not match the selected worker RID.');
if (doc.protocol !== 'mi-lsp-v1.1') fail('Worker manifest protocol is invalid.');
if (!Number.isSafeInteger(doc.file_count) || doc.file_count < 1) fail('Worker manifest file_count must be a positive integer.');
if (!Array.isArray(doc.files) || doc.file_count !== doc.files.length) fail('Worker manifest file_count does not match files.');
const root = path.resolve(workerRoot), expected = new Map();
for (const entry of doc.files) {
  if (!entry || typeof entry !== 'object' || Array.isArray(entry)) fail('Worker manifest contains a non-object file entry.');
  const {path: rel, size, sha256} = entry;
  if (typeof rel !== 'string' || !rel || rel.includes('\\') || rel.startsWith('/') || /^[A-Za-z]:/.test(rel) || rel.split('/').some(p => !p || p === '.' || p === '..') || rel === 'worker-manifest.json') fail('Worker manifest contains an unsafe file path.');
  const candidate = path.resolve(root, ...rel.split('/'));
  if (candidate !== root && !candidate.startsWith(root + path.sep)) fail('Worker manifest file escaped its worker root.');
  if (!Number.isSafeInteger(size) || size < 0) fail('Worker manifest file size is invalid.');
  if (typeof sha256 !== 'string' || !/^[0-9a-fA-F]{64}$/.test(sha256)) fail('Worker manifest file hash is invalid.');
  if (expected.has(rel)) fail('Worker manifest contains duplicate file paths.');
  let stat;
  try { stat = fs.lstatSync(candidate); } catch (_) { fail(`Worker manifest references a missing or linked file: ${rel}`); }
  if (!stat.isFile() || stat.isSymbolicLink()) fail(`Worker manifest references a missing or linked file: ${rel}`);
  if (stat.size !== size) fail(`Worker manifest file size mismatch: ${rel}`);
  const digest = crypto.createHash('sha256').update(fs.readFileSync(candidate)).digest('hex');
  if (digest.toLowerCase() !== sha256.toLowerCase()) fail(`Worker manifest file hash mismatch: ${rel}`);
  expected.set(rel, candidate);
}
const actual = new Set();
function walk(dir) {
  for (const name of fs.readdirSync(dir)) {
    if (dir === root && name === 'worker-manifest.json') continue;
    const full = path.join(dir, name), st = fs.lstatSync(full);
    if (st.isSymbolicLink()) fail(`Worker root contains a linked ${st.isDirectory() ? 'directory' : 'file'}.`);
    if (st.isDirectory()) walk(full);
    else if (st.isFile()) actual.add(path.relative(root, full).split(path.sep).join('/'));
  }
}
walk(root);
if (actual.size !== expected.size || [...actual].some(p => !expected.has(p))) fail('Worker manifest file list does not match the extracted worker bundle.');
