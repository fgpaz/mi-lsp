// src/demo/ambiguous-b.mjs
// Exports resolveConflict alongside ambiguous-a.mjs for ambiguity testing.

/**
 * @description Ambiguous symbol: both ambiguous-a and ambiguous-b export this name.
 * @returns {string}
 */
export function resolveConflict() {
  return 'from-ambiguous-b';
}