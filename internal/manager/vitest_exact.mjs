import { createRequire } from 'node:module';
import { realpathSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const [file, project, name, report] = process.argv.slice(1);
if (!file || !name || !report) throw new Error('Vitest selection is missing its file, name or report path');
const require = createRequire(resolve(process.cwd(), 'package.json'));
const { version } = require('vitest/package.json');
if (!/^4\./.test(version)) throw new Error(`Exact Vitest selection currently requires Vitest 4; found ${version}`);
const { createVitest } = await import(pathToFileURL(require.resolve('vitest/node')).href);
const ctx = await createVitest('test', { watch: false, run: true, reporters: ['default', 'junit'], outputFile: { junit: report } });
try {
  await ctx.standalone();
  const canonical = path => {
    const value = realpathSync(path);
    return process.platform === 'win32' ? value.toLowerCase() : value;
  };
  const targetFile = canonical(file);
  const specifications = (await ctx.globTestSpecifications()).filter(spec =>
    (spec.project.name || '') === project && canonical(spec.moduleId) === targetFile
  );
  if (specifications.length !== 1) throw new Error('Vitest file and project identity is missing or ambiguous; rediscover tests');
  if (!('testIds' in specifications[0]) || !('testNamePattern' in specifications[0]) || typeof ctx.collectTests !== 'function' || typeof ctx.runTestSpecifications !== 'function') {
    throw new Error('This Vitest version does not provide the required exact-selection API');
  }
  const collected = await ctx.collectTests(specifications);
  if (collected.unhandledErrors.length || collected.testModules.length !== 1) throw new Error('Vitest collection did not complete successfully');
  const module = collected.testModules[0];
  if (module.errors().length || [...module.children.allSuites()].some(suite => suite.errors().length)) throw new Error('Vitest collection reported a suite error');
  const matches = [...module.children.allTests()].filter(test => test.fullName === name);
  if (matches.length !== 1) throw new Error('Vitest test identity is missing or ambiguous; rediscover tests');
  const segments = [matches[0].name];
  let parent = matches[0].parent;
  while (parent.type === 'suite') {
    segments.unshift(parent.name);
    parent = parent.parent;
  }
  const runtimeName = segments.join(' ');
  specifications[0].testIds = [matches[0].id];
  specifications[0].testNamePattern = new RegExp(`^${runtimeName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`);
  const result = await ctx.runTestSpecifications(specifications);
  if (result.unhandledErrors.length) process.exitCode = 1;
} finally {
  await ctx.close();
}
