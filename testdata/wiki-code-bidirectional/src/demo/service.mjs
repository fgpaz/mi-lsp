// src/demo/service.mjs
// Implementation for RF-DEMO-001 demo
import { compute } from './helper.mjs';

/**
 * @description Core demo function for bidirectional wiki-code navigation.
 * @param {Object} input
 * @returns {Object}
 */
export function runDemo(input) {
  return { result: compute(input.value), symbol: 'runDemo' };
}