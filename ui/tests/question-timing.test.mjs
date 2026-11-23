import assert from 'node:assert/strict';
import test from 'node:test';
import { questionDurationMS } from '../src/lib/control/question-timing.ts';

function runtime(phase, id, time) {
	return { controller: { phase, selected_question_id: id }, current_question: { id, time } };
}

for (const [phase, id, time, duration] of [
	['khoi_dong', 'kd_chung.0', 90, 3000],
	['khoi_dong', 'kd_rieng.2.0', 90, 3000],
	['vcnv', 'vcnv.rows.2', 90, 15000],
	['vcnv', 'vcnv.center', 23, 23000],
	['vcnv', 'vcnv.center', 0, 15000],
	['tang_toc', 'tt.0', 12, 12000],
	['tang_toc', 'tt.0', 0, 30000],
	['tang_toc', 'tt.0', 600, 600000],
	['ve_dich', 'vd.0.20.0', 90, 15000],
	['ve_dich', 'vd.2.30.1', 90, 20000]
]) {
	test(`${phase}/${id} with question time ${time} previews ${duration}ms`, () => {
		assert.equal(questionDurationMS(runtime(phase, id, time)), duration);
	});
}

test('no question or stale question snapshot cannot arm a timer', () => {
	assert.equal(questionDurationMS(null), 0);
	assert.equal(questionDurationMS(undefined), 0);
	const state = runtime('tang_toc', 'tt.0', 10);
	state.current_question = null;
	assert.equal(questionDurationMS(state), 0);
	state.current_question = { id: 'tt.1', time: 10 };
	assert.equal(questionDurationMS(state), 0);
});

test('a question from another phase or an unsupported package cannot arm a timer', () => {
	for (const [phase, id] of [
		['idle', 'tt.0'],
		['main', 'tt.0'],
		['khoi_dong', 'tt.0'],
		['khoi_dong', 'kd_rieng.0'],
		['vcnv', 'kd_chung.0'],
		['vcnv', 'vcnv.rows'],
		['tang_toc', 'vcnv.center'],
		['tang_toc', 'tt.0.extra'],
		['ve_dich', 'vd.0.40.0']
	])
		assert.equal(questionDurationMS(runtime(phase, id, 10)), 0);
});

test('invalid configured durations do not become timer commands', () => {
	for (const time of [601, 12.5, Infinity]) {
		assert.equal(questionDurationMS(runtime('tang_toc', 'tt.0', time)), 0);
		assert.equal(questionDurationMS(runtime('vcnv', 'vcnv.center', time)), 0);
	}
});
