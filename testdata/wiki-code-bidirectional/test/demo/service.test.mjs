// test/demo/service.test.mjs
// Tests for RF-DEMO-001

import { runDemo } from '../../src/demo/service.mjs';

/**
 * @description Validates that runDemo returns the expected shape and computed value.
 */
export function testRunDemo() {
  const input = { value: 5 };
  const output = runDemo(input);
  if (output.result !== 10 || output.symbol !== 'runDemo') {
    throw new Error('testRunDemo failed: expected result=10 and symbol=runDemo');
  }
  return 'testRunDemo passed';
}

// Minimal runner (no external dependencies)
if (typeof process !== 'undefined' && process.argv) {
  console.log(testRunDemo());
}