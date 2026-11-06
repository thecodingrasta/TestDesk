import { appendFileSync } from 'node:fs';
import { test } from 'vitest';

test('same [a]+', context => {
  appendFileSync(process.env.MARKER, `${context.task.file.projectName}:target\n`);
});
test('another case', () => {});
test('third case', () => {});
