import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const server = await createServer({
	configFile: false,
	resolve: { alias: { $lib: fileURLToPath(new URL('../src/lib', import.meta.url)) } },
	server: { middlewareMode: true, hmr: false, watch: null }
});
after(() => server.close());
const scoring = await server.ssrLoadModule('/src/lib/operator/scoring.ts');
const drafts = await server.ssrLoadModule('/src/lib/operator/drafts.ts');
const keyboard = await server.ssrLoadModule('/src/lib/operator/keyboard.ts');
const screens = await server.ssrLoadModule('/src/lib/operator/screens.ts');
const questions = await server.ssrLoadModule('/src/lib/operator/questions.ts');
const fast = await server.ssrLoadModule('/src/lib/operator/fast-verdict.ts');
const rules = scoring.DEFAULT_SCORING;

function batch(key, verdicts, extra = {}) {
	const info = scoring.questionInfo(key) ?? { kind: extra.kind, owner: 0, tier: 0 };
	return { ...scoring.emptyBatch(key, info.kind, key), owner: info.owner, tier: info.tier, verdicts, ...extra };
}

function runtime({ selected = '', phase = 'khoi_dong', screen = 'intro', contestants = [], rows = ['u', 'u', 'u', 'u'], images = [false, false, false, false, false], vd = [], showing = false, version = 1 } = {}) {
	const people = [1, 2, 3, 4].map((slot) => ({ slot, name: `TS${slot}`, class: '', score: 0, time_ms: 0, locked: false, bell_locked: false, has_rung: false, ...contestants.find((entry) => entry.slot === slot) }));
	return {
		match_id: 'm1',
		question_revision_id: 'r1',
		version,
		status: 'playing',
		controller: {
			phase,
			screen,
			selected_question_id: selected,
			selected_contestant: 0,
			is_showing_question: showing,
			vcnv_state: { has_started: true, has_solved: false, row_states: rows, image_open_state: images },
			ve_dich_contestants_state: [0, 1, 2, 3].map((index) => vd[index] ?? { selected_question_set: [], starred_on: 0 })
		},
		current_question: selected ? { id: selected, content: 'Q', type: 'text' } : undefined,
		contestants: people,
		timer: { is_running: false, is_paused: false, duration_ms: 0, remaining_ms: 0, elapsed_ms: 0, started_at: 0 }
	};
}

test('fast Khởi động controls bind the target, floor and active question state', () => {
	const own = runtime({ selected: 'kd_rieng.1.0', screen: 'ques_ts2', showing: true });
	let state = fast.fastVerdictState(own, rules, true, false, false);
	assert.equal(state.name, 'TS2');
	assert.equal(state.correct, 10);
	assert.equal(state.wrong, 0);
	assert.equal(state.canCorrect, true);
	assert.equal(fast.fastVerdictState(own, rules, true, true, false).canWrong, false);
	assert.equal(fast.fastVerdictState(own, rules, false, false, false).canWrong, false);
	assert.equal(fast.fastVerdictState(own, undefined, true, false, false).canCorrect, false);
	assert.equal(fast.fastVerdictState(own, { ...rules, khoi_dong: { ...rules.khoi_dong, individual_correct: 25 } }, true, false, false).correct, 25);
	assert.equal(fast.fastVerdictState({ ...own, timer: { ...own.timer, is_paused: true } }, rules, true, false, false).canWrong, true);
	assert.equal(fast.fastVerdictState(own, rules, true, false, true).canWrong, false);
	assert.equal(fast.fastVerdictState({ ...own, status: 'paused' }, rules, true, false, false).canWrong, false);
	assert.equal(fast.fastVerdictState({ ...own, controller: { ...own.controller, is_showing_question: false } }, rules, true, false, false).canWrong, false);
	assert.equal(fast.fastVerdictState({ ...own, current_question: null }, rules, true, false, false).canWrong, false);
	assert.equal(fast.fastVerdictState({ ...own, controller: { ...own.controller, screen: 'ques_ts1' } }, rules, true, false, false).canWrong, false);
	assert.equal(fast.fastVerdictState(runtime({ screen: 'ques_chung', showing: true }), rules, true, false, false).visible, false);
	const common = runtime({ selected: 'kd_chung.0', screen: 'ques_chung', showing: true });
	state = fast.fastVerdictState(common, rules, true, false, false);
	assert.equal(state.canCorrect, false);
	assert.equal(state.canWrong, true);
	assert.equal(state.wrong, 0);
	common.contestants[2].has_rung = true;
	common.contestants[2].score = 3;
	state = fast.fastVerdictState(common, rules, true, false, false);
	assert.equal(state.slot, 3);
	assert.equal(state.name, 'TS3');
	assert.equal(state.wrong, -3);
	assert.equal(fast.fastVerdictState(common, { ...rules, khoi_dong: { ...rules.khoi_dong, common_floor_zero: false } }, true, false, false).wrong, -5);
});

test('sound-only mapping repeats without recording verdicts', () => {
	for (const [phase, screen, correct, wrong] of [
		['vcnv', 'main_vcnv', 'builtin.vcnv.correctobstacle', 'builtin.vcnv.wrongrow'],
		['vcnv', 'image_vcnv', 'builtin.vcnv.correctobstacle', 'builtin.vcnv.wrongrow'],
		['vcnv', 'ques', 'builtin.vcnv.correctrow', 'builtin.vcnv.wrongrow'],
		['tang_toc', 'ques', 'builtin.tt.correct', 'builtin.vcnv.wrongrow'],
		['ve_dich', 'ques_ts1', 'builtin.vd.correctfinish', 'builtin.vd.wrongfinish']
	]) {
		assert.equal(fast.soundOnly(phase, screen, 'correct'), correct);
		assert.equal(fast.soundOnly(phase, screen, 'correct'), correct);
		assert.equal(fast.soundOnly(phase, screen, 'wrong'), wrong);
	}
	assert.equal(fast.soundOnly('khoi_dong', 'ques_chung', 'correct'), undefined);
});

test('discarding the current question draft preserves unrelated batches and saved undo', () => {
	const live = runtime({ selected: 'kd_chung.0', screen: 'ques_chung', showing: true });
	let state = drafts.toggleVerdict(drafts.emptyDraftState(), live, 'kd_chung.0', 1, 'wrong');
	state = drafts.toggleVerdict(state, live, 'tt.0', 2, 'correct');
	state = { ...state, lastSave: { adjustments: [{ slot: 4, delta: 10 }], reason: 'previous', at: 1 } };
	const after = drafts.discardQuestionDraft(state, 'kd_chung.0');
	assert.deepEqual(after.batches.map((entry) => entry.key), ['tt.0']);
	assert.deepEqual(after.lastSave, state.lastSave);
	assert.deepEqual(drafts.undoRequest(after).adjustments, [{ slot: 4, delta: -10 }]);
});

test('a draft saved for this question blocks quick scoring until undo', () => {
	const key = 'kd_chung.0';
	const live = runtime({ selected: key, screen: 'ques_chung', showing: true, contestants: [{ slot: 1, has_rung: true, score: 20 }] });
	let state = drafts.toggleVerdict(drafts.emptyDraftState(), live, key, 1, 'correct');
	state = drafts.toggleVerdict(state, live, 'tt.0', 2, 'correct');
	const view = () => fast.fastVerdictState(live, rules, true, false, state.batches.some((batch) => batch.key === key), drafts.savedQuestion(state, key));
	assert.equal(view().canCorrect, false);
	const request = drafts.saveRequest(state, rules);
	state = { batches: [], lastSave: { ...request, at: 1 } };
	assert.equal(view().saved, true);
	assert.equal(view().canWrong, false);
	assert.equal(drafts.savedQuestion(state, 'kd_chung.1'), false);
	assert.equal(drafts.savedQuestion({ ...state, lastSave: { ...state.lastSave, reason: `${drafts.questionLabel(key)} supplémentaire` } }, key), false);
	assert.ok(drafts.undoRequest(state));
	state = { ...state, lastSave: null };
	assert.equal(view().canCorrect, true);
});

test('saved KD corrections allow quick scoring while verdicts and legacy saves block it', () => {
	const key = 'kd_chung.0';
	const live = runtime({ selected: key, screen: 'ques_chung', showing: true, contestants: [{ slot: 1, has_rung: true, score: 20 }] });
	const saved = (state) => ({
		batches: [],
		lastSave: { ...drafts.saveRequest(state, rules), at: 1, sources: drafts.scoreSources(state, rules) }
	});
	const view = (state) => fast.fastVerdictState(live, rules, true, false, false, drafts.savedQuestion(state, key));
	const correction = saved(drafts.setPendingTotal(drafts.emptyDraftState(), live, rules, 1, 5));
	assert.equal(correction.lastSave.sources[0].kind, 'correction');
	assert.equal(view(correction).canCorrect, true);
	assert.equal(view(correction).canWrong, true);
	const verdict = saved(drafts.toggleVerdict(drafts.emptyDraftState(), live, key, 1, 'correct'));
	assert.equal(verdict.lastSave.sources[0].kind, 'verdict');
	assert.equal(view(verdict).canCorrect, false);
	assert.equal(view(verdict).canWrong, false);
	const legacySave = { ...drafts.saveRequest(drafts.toggleVerdict(drafts.emptyDraftState(), live, key, 1, 'correct'), rules), at: 1 };
	const legacy = drafts.parseDraftState(JSON.stringify({ batches: [], lastSave: legacySave }));
	assert.equal(view(legacy).canCorrect, false);
	assert.equal(view(legacy).canWrong, false);
});

test('KD draft captures the creation version across edits, live refresh and question advance', () => {
	const key = 'kd_chung.0';
	const live = runtime({ selected: key, version: 10, contestants: [{ slot: 1, score: 20 }] });
	let state = drafts.toggleVerdict(drafts.emptyDraftState(), live, key, 1, 'correct');
	assert.equal(state.batches[0].sourceVersion, 10);

	const updated = runtime({ selected: key, version: 11, contestants: [{ slot: 1, score: 30 }] });
	state = drafts.refreshLive(state, updated);
	assert.equal(state.batches[0].sourceVersion, 10);
	assert.equal(state.batches[0].scores[1], 30);
	state = drafts.toggleVerdict(state, updated, key, 1, 'wrong');
	state = drafts.setPendingTotal(state, updated, rules, 1, 7);
	assert.equal(state.batches[0].sourceVersion, 10);

	const next = runtime({ selected: 'kd_rieng.0.1', version: 12 });
	state = drafts.refreshLive(state, next);
	state = drafts.toggleVerdict(state, next, 'kd_rieng.0.1', 1, 'correct');
	assert.deepEqual(drafts.scoreSources(state, rules), [
		{ question_id: key, version: 10, kind: 'correction' },
		{ question_id: 'kd_rieng.0.1', version: 12, kind: 'verdict' }
	]);
	const correction = drafts.toggleVerdict(drafts.emptyDraftState(), updated, key, 1, 'correct');
	assert.equal(correction.batches[0].sourceVersion, 11);
	assert.equal(drafts.createBatch(key, null).sourceVersion, 0);
	assert.equal(drafts.createBatch('tt.0', live).sourceVersion, undefined);
});

test('score sources are nonzero KD batches only, conservative for old or duplicate drafts, and pure', () => {
	const state = {
		batches: [
			batch('kd_chung.0', { 1: 'correct' }, { sourceVersion: 9 }),
			batch('kd_chung.0', { 2: 'correct' }, { sourceVersion: 7 }),
			batch('kd_rieng.1.0', { 2: 'correct' }),
			batch('kd_chung.2', { 3: 'wrong' }, { scores: { 3: 0 }, sourceVersion: 8 }),
			batch('kd_rieng.0.3', { 1: 'correct' }, { overrides: { 1: 0 }, sourceVersion: 8 }),
			batch('tt.0', { 1: 'correct' }, { sourceVersion: 8 }),
			batch(drafts.MANUAL_KEY, {}, { kind: 'manual', overrides: { 4: 5 } })
		],
		lastSave: { adjustments: [{ slot: 4, delta: 10 }], reason: 'previous', at: 1 }
	};
	const restored = drafts.parseDraftState(JSON.stringify(state));
	const before = JSON.stringify(restored);
	assert.deepEqual(drafts.scoreSources(restored, rules), [
		{ question_id: 'kd_chung.0', version: 7, kind: 'verdict' },
		{ question_id: 'kd_rieng.1.0', version: 0, kind: 'verdict' }
	]);
	assert.equal(JSON.stringify(restored), before);
	const refreshed = drafts.refreshLive(restored, runtime({ selected: 'kd_rieng.1.0', version: 100 }));
	assert.equal(refreshed.batches[2].sourceVersion, undefined);
	assert.equal(drafts.scoreSources(refreshed, rules)[1].version, 0);
	restored.batches.push(batch('kd_chung.0', { 3: 'correct' }));
	assert.equal(drafts.scoreSources(restored, rules)[0].version, 0);
	assert.equal(drafts.scoreSources({ batches: [restored.batches[3], restored.batches[5]], lastSave: null }, rules).length, 0);
	const overridden = drafts.setPendingTotal(drafts.emptyDraftState(), runtime({ selected: 'kd_rieng.1.0', version: 14 }), rules, 2, 5);
	assert.deepEqual(drafts.scoreSources(overridden, rules), [{ question_id: 'kd_rieng.1.0', version: 14, kind: 'correction' }]);
});

test('KD sources distinguish verdicts from effective manual corrections', () => {
	const key = 'kd_chung.0';
	const live = runtime({ selected: key, version: 20, contestants: [{ slot: 1, score: 20 }, { slot: 2, score: 20 }] });
	const verdict = drafts.toggleVerdict(drafts.emptyDraftState(), live, key, 1, 'correct');
	const source = (state) => drafts.scoreSources(state, rules);
	assert.deepEqual(source(verdict), [{ question_id: key, version: 20, kind: 'verdict' }]);
	for (const delta of [5, -5]) {
		const manual = drafts.setPendingTotal(drafts.emptyDraftState(), live, rules, 1, delta);
		assert.deepEqual(drafts.saveRequest(manual, rules).adjustments, [{ slot: 1, delta }]);
		assert.deepEqual(source(manual), [{ question_id: key, version: 20, kind: 'correction' }]);
		const overridden = drafts.setPendingTotal(verdict, live, rules, 1, delta);
		assert.equal(overridden.batches[0].verdicts[1], 'correct');
		assert.deepEqual(source(overridden), [{ question_id: key, version: 20, kind: 'correction' }]);
	}
	const mixed = drafts.setPendingTotal(verdict, live, rules, 2, -5);
	assert.deepEqual(source(mixed), [{ question_id: key, version: 20, kind: 'verdict' }]);
	const zeroOverride = drafts.setPendingTotal(mixed, live, rules, 1, 0);
	assert.deepEqual(source(zeroOverride), [{ question_id: key, version: 20, kind: 'correction' }]);
});

test('duplicate KD sources retain the oldest version and verdict regardless of ordering', () => {
	const key = 'kd_chung.0';
	const correction = batch(key, {}, { overrides: { 1: -5 }, sourceVersion: 10 });
	const verdict = batch(key, { 2: 'correct' }, { sourceVersion: 20 });
	for (const batches of [[correction, verdict], [verdict, correction]]) {
		assert.deepEqual(drafts.scoreSources({ batches, lastSave: null }, rules), [
			{ question_id: key, version: 10, kind: 'verdict' }
		]);
	}
	const legacy = batch(key, {}, { overrides: { 3: 5 } });
	assert.deepEqual(drafts.scoreSources({ batches: [correction, legacy], lastSave: null }, rules), [
		{ question_id: key, version: 0, kind: 'correction' }
	]);
});

test('Khởi động riêng only scores the owner; chung penalty never takes a score below zero', () => {
	const own = scoring.suggestedPoints(batch('kd_rieng.1.3', { 1: 'correct', 2: 'correct' }), rules);
	assert.deepEqual(own, { 1: 0, 2: 10, 3: 0, 4: 0 });
	const common = scoring.suggestedPoints(batch('kd_chung.0', { 1: 'wrong', 2: 'wrong', 3: 'correct' }, { scores: { 1: 0, 2: 30 } }), rules);
	assert.deepEqual(common, { 1: 0, 2: -5, 3: 10, 4: 0 });
	const floorOff = { ...rules, khoi_dong: { ...rules.khoi_dong, common_floor_zero: false } };
	assert.equal(scoring.suggestedPoints(batch('kd_chung.0', { 1: 'wrong' }, { scores: { 1: 0 } }), floorOff)[1], -5);
	const partial = scoring.suggestedPoints(batch('kd_chung.0', { 1: 'wrong' }, { scores: { 1: 3 } }), rules);
	assert.equal(partial[1], -3);
});

test('VCNV rows, centre and obstacle values follow the rows opened', () => {
	assert.deepEqual(scoring.suggestedPoints(batch('vcnv.rows.2', { 1: 'correct', 4: 'wrong' }), rules), { 1: 10, 2: 0, 3: 0, 4: 0 });
	assert.equal(scoring.suggestedPoints(batch('vcnv.center', { 2: 'correct' }), rules)[2], 10);
	assert.deepEqual([0, 1, 2, 3, 4, 5].map((rows) => scoring.obstacleValue(rows, rules)), [60, 60, 50, 40, 30, 20]);
	const obstacle = batch(drafts.OBSTACLE_KEY, { 1: 'correct', 3: 'wrong' }, { kind: 'obstacle', obstacleRows: { 1: 2, 3: 2 } });
	assert.deepEqual(scoring.suggestedPoints(obstacle, rules), { 1: 50, 2: 0, 3: 0, 4: 0 });
});

test('Tăng tốc ranks correct answers by time and equal times share the higher value', () => {
	const result = scoring.suggestedPoints(batch('tt.1', { 1: 'correct', 2: 'correct', 3: 'correct', 4: 'wrong' }, { times: { 1: 3210, 2: 2100, 3: 2100, 4: 1000 } }), rules);
	assert.deepEqual(result, { 1: 20, 2: 40, 3: 40, 4: 0 });
	const noAnswer = scoring.suggestedPoints(batch('tt.1', { 1: 'correct', 2: 'correct' }, { times: { 2: 5000 } }), rules);
	assert.deepEqual(noAnswer, { 1: 30, 2: 40, 3: 0, 4: 0 });
});

test('Về đích: star doubles, star wrong subtracts, steals move or cost points', () => {
	assert.equal(scoring.suggestedPoints(batch('vd.0.30.1', { 1: 'correct' }), rules)[1], 30);
	assert.equal(scoring.suggestedPoints(batch('vd.0.30.1', { 1: 'correct' }, { starred: true }), rules)[1], 60);
	assert.equal(scoring.suggestedPoints(batch('vd.0.20.1', { 1: 'wrong' }, { starred: true }), rules)[1], -20);
	assert.deepEqual(scoring.suggestedPoints(batch('vd.0.30.1', { 1: 'wrong', 3: 'correct' }), rules), { 1: -30, 2: 0, 3: 30, 4: 0 });
	assert.deepEqual(scoring.suggestedPoints(batch('vd.0.30.1', { 1: 'wrong', 3: 'correct' }, { starred: true }), rules), { 1: -30, 2: 0, 3: 30, 4: 0 });
	assert.deepEqual(scoring.suggestedPoints(batch('vd.1.30.0', { 2: 'wrong', 4: 'wrong' }), rules), { 1: 0, 2: 0, 3: 0, 4: -15 });
	const custom = { ...rules, ve_dich: { ...rules.ve_dich, tiers: [20, 40] } };
	assert.equal(scoring.suggestedPoints(batch('vd.0.30.0', { 1: 'correct' }), custom)[1], 40);
});

test('toggling verdicts fills, switches and clears pending points; overrides win until the next verdict', () => {
	const live = runtime({ selected: 'kd_chung.0', contestants: [{ slot: 2, score: 20 }] });
	let state = drafts.emptyDraftState();
	state = drafts.toggleVerdict(state, live, 'kd_chung.0', 2, 'correct');
	assert.equal(drafts.pendingBySlot(state, rules)[2], 10);
	state = drafts.toggleVerdict(state, live, 'kd_chung.0', 2, 'wrong');
	assert.equal(drafts.pendingBySlot(state, rules)[2], -5);
	state = drafts.setPendingTotal(state, live, rules, 2, 7);
	assert.equal(drafts.pendingBySlot(state, rules)[2], 7);
	state = drafts.toggleVerdict(state, live, 'kd_chung.0', 2, 'correct');
	assert.equal(drafts.pendingBySlot(state, rules)[2], 10);
	state = drafts.toggleVerdict(state, live, 'kd_chung.0', 2, 'correct');
	assert.equal(state.batches.length, 0);
});

test('unsaved points for an earlier question stay frozen and are saved together with one reason', () => {
	let live = runtime({ phase: 'tang_toc', selected: 'tt.0', contestants: [{ slot: 1, answer: 'A', time_ms: 3000 }, { slot: 2, answer: 'B', time_ms: 2000 }] });
	let state = drafts.toggleVerdict(drafts.emptyDraftState(), live, 'tt.0', 1, 'correct');
	state = drafts.toggleVerdict(state, live, 'tt.0', 2, 'correct');
	assert.deepEqual(drafts.pendingBySlot(state, rules), { 1: 30, 2: 40, 3: 0, 4: 0 });
	live = runtime({ phase: 'tang_toc', selected: 'tt.1' });
	state = drafts.refreshLive(state, live);
	assert.deepEqual(drafts.pendingBySlot(state, rules), { 1: 30, 2: 40, 3: 0, 4: 0 }, 'answers reset for the next question must not change frozen points');
	state = drafts.toggleVerdict(state, live, 'tt.1', 1, 'correct');
	const request = drafts.saveRequest(state, rules);
	assert.deepEqual(request.adjustments, [{ slot: 1, delta: 70 }, { slot: 2, delta: 40 }]);
	assert.equal(request.reason, 'Tăng tốc · Câu 1, Tăng tốc · Câu 2');
	const undo = drafts.undoRequest({ batches: [], lastSave: { ...request, at: 1 } });
	assert.deepEqual(undo.adjustments, [{ slot: 1, delta: -70 }, { slot: 2, delta: -40 }]);
	assert.match(undo.reason, /^Hoàn tác · /);
});

test('the on-air batch follows answer times and the hope star', () => {
	let state = drafts.toggleVerdict(drafts.emptyDraftState(), runtime({ phase: 've_dich', selected: 'vd.0.30.1' }), 'vd.0.30.1', 1, 'correct');
	assert.equal(drafts.pendingBySlot(state, rules)[1], 30);
	state = drafts.refreshLive(state, runtime({ phase: 've_dich', selected: 'vd.0.30.1', vd: [{ selected_question_set: ['20_1', '30_2', '20_3'], starred_on: 2 }] }));
	assert.equal(drafts.pendingBySlot(state, rules)[1], 60);
});

test('obstacle verdicts freeze the rows opened when they were given', () => {
	let state = drafts.toggleVerdict(drafts.emptyDraftState(), runtime({ phase: 'vcnv', rows: ['c', 'u', 'u', 'u'] }), drafts.OBSTACLE_KEY, 3, 'correct');
	state = drafts.refreshLive(state, runtime({ phase: 'vcnv', rows: ['c', 'w', 's', 'u'] }));
	assert.equal(drafts.pendingBySlot(state, rules)[3], 60);
	state = drafts.toggleVerdict(state, runtime({ phase: 'vcnv', selected: 'vcnv.center', rows: ['c', 'w', 'c', 'c'] }), drafts.OBSTACLE_KEY, 1, 'correct');
	assert.equal(drafts.pendingBySlot(state, rules)[1], 20);
});

test('manual totals without a question go into a manual batch', () => {
	const state = drafts.setPendingTotal(drafts.emptyDraftState(), runtime(), rules, 4, -10);
	assert.equal(state.batches[0].key, drafts.MANUAL_KEY);
	assert.deepEqual(drafts.saveRequest(state, rules), { adjustments: [{ slot: 4, delta: -10 }], reason: 'Chỉnh tay' });
	assert.deepEqual(drafts.parseDraftState('not json'), drafts.emptyDraftState());
});

const key = (code, extra = {}) => ({ key: '', code, shiftKey: false, ctrlKey: false, metaKey: false, altKey: false, repeat: false, ...extra });

test('shortcuts map to show actions and ignore chords, repeats and fields', () => {
	assert.deepEqual(keyboard.resolveShortcut(key('Digit3')), { type: 'verdict', slot: 3, verdict: 'correct' });
	assert.deepEqual(keyboard.resolveShortcut(key('Digit3', { shiftKey: true })), { type: 'verdict', slot: 3, verdict: 'wrong' });
	assert.deepEqual(keyboard.resolveShortcut(key('PageDown')), { type: 'question', direction: 1 });
	assert.deepEqual(keyboard.resolveShortcut(key('ArrowLeft')), { type: 'screen', direction: -1 });
	assert.deepEqual(keyboard.resolveShortcut(key('Space')), { type: 'timer' });
	assert.deepEqual(keyboard.resolveShortcut(key('Enter')), { type: 'save' });
	assert.deepEqual(keyboard.resolveShortcut(key('Escape')), { type: 'stop-sound' });
	assert.equal(keyboard.resolveShortcut(key('KeyH', { ctrlKey: true })), null);
	assert.equal(keyboard.resolveShortcut(key('Space', { repeat: true })), null);
	assert.equal(keyboard.resolveShortcut(key('KeyB', { shiftKey: true })), null);
	assert.equal(keyboard.resolveShortcut(key('KeyQ')), null);
	const field = { closest: (selector) => (selector.includes('input') ? {} : null), isContentEditable: false };
	assert.equal(keyboard.ignoresShortcuts(field), true);
	assert.equal(keyboard.ignoresShortcuts({ closest: () => null, isContentEditable: false }), false);
	const guard = new keyboard.DoublePressGuard(300);
	assert.equal(guard.accept({ type: 'timer' }, 1000), true);
	assert.equal(guard.accept({ type: 'timer' }, 1200), false);
	assert.equal(guard.accept({ type: 'save' }, 1250), true);
	assert.equal(guard.accept({ type: 'timer' }, 1600), true);
});

test('question screens collapse into one tab and the tab targets the selected question', () => {
	const kd = [
		{ id: 'start', label: 'Bắt đầu' }, { id: 'rule', label: 'Luật chơi' }, { id: 'main_kd', label: 'Màn hình chính' },
		{ id: 'ques_chung', label: 'Câu hỏi chung' }, { id: 'ques_ts1', label: 'TS1' }, { id: 'ques_ts2', label: 'TS2' },
		{ id: 'end_ts', label: 'Kết thúc lượt' }
	];
	const tabs = screens.screenTabs(kd);
	assert.deepEqual(tabs.map((tab) => tab.label), ['Bắt đầu', 'Luật chơi', 'Chính', 'Câu hỏi', 'Xong lượt']);
	const question = tabs.find((tab) => tab.id === 'question');
	assert.equal(screens.tabTarget(question, runtime({ selected: 'kd_rieng.1.0', current: true })), 'ques_ts2');
	assert.equal(screens.tabTarget(question, runtime()), null);
	const vd = screens.screenTabs([
		{ id: 'main_vd', label: 'Chọn thí sinh' }, { id: 'pre_ques_ts1', label: 'Gói 1' }, { id: 'ques_ts1', label: 'Câu 1' },
		{ id: 'pre_ques_ts2', label: 'Gói 2' }, { id: 'ques_ts2', label: 'Câu 2' }
	]);
	assert.deepEqual(vd.map((tab) => tab.label), ['Thí sinh', 'Gói', 'Câu hỏi']);
	const live = runtime({ phase: 've_dich', screen: 'main_vd' });
	live.controller.selected_contestant = 2;
	assert.equal(screens.tabTarget(vd[1], live), 'pre_ques_ts2');
	assert.equal(screens.tabTarget(vd[2], live), 'ques_ts2');
});

test('the strip offers the round questions and the Về đích package in order', () => {
	const q = (content) => ({ content, type: 'text' });
	const options = questions.flattenQuestions({
		kd_chung: [q('a'), q('b')],
		kd_rieng: [[q('r1')], [q('r2a'), q('r2b')], [], []],
		vcnv: { rows: [q('1'), q('2'), q('3'), q('4')], center: q('c') },
		tt: [q('t')],
		vd: [{ 20: [q('v20a'), q('v20b'), q('v20c')], 30: [q('v30a'), q('v30b'), q('v30c')] }, { 20: [], 30: [] }, { 20: [], 30: [] }, { 20: [], 30: [] }]
	});
	assert.deepEqual(questions.roundOptions(options, runtime({ screen: 'ques_ts2' })).map((option) => option.id), ['kd_rieng.1.0', 'kd_rieng.1.1']);
	assert.deepEqual(questions.roundOptions(options, runtime({ phase: 'vcnv' })).map((option) => option.short), ['1', '2', '3', '4', 'TT']);
	const live = runtime({ phase: 've_dich', screen: 'ques_ts1', vd: [{ selected_question_set: ['30_1', '20_2', '30_3'], starred_on: 0 }] });
	assert.deepEqual(questions.roundOptions(options, live).map((option) => option.id), ['vd.0.30.0', 'vd.0.20.1', 'vd.0.30.2']);
	assert.equal(questions.questionScreen('vd.2.20.1'), 'ques_ts3');
	assert.equal(questions.questionScreen('vcnv.center'), 'ques');
});
