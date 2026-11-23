import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';
import ts from 'typescript';

const source = readFileSync(new URL('../src/lib/api/match/index.ts', import.meta.url), 'utf8');
const compiled = ts.transpileModule(source, {
	compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS }
}).outputText;
const exports = {};
vm.runInNewContext(compiled, { exports });
const { MatchesApi } = exports;

test('incremental match history uses an exclusive cursor, encoded match ID and the caller abort signal', async () => {
	const calls = [];
	const event = { sequence: 24 };
	const api = new MatchesApi({ request: async (...args) => { calls.push(args); return { events: [event] }; } });
	const controller = new AbortController();
	const result = await api.events('match/one', { afterSequence: 23, signal: controller.signal });
	assert.equal(calls[0][0], '/matches/match%2Fone/events?after_sequence=23');
	assert.equal(calls[0][1].signal, controller.signal);
	assert.equal(result[0], event);
});

test('existing replay/full-history requests retain their URL and null histories normalize to an empty array', async () => {
	const calls = [];
	const api = new MatchesApi({ request: async (path) => { calls.push(path); return { events: null }; } });
	assert.equal((await api.events('match')).length, 0);
	assert.equal(calls[0], '/matches/match/events');
	await api.events('match', { afterSequence: 0 });
	assert.equal(calls[1], '/matches/match/events?after_sequence=0');
});
