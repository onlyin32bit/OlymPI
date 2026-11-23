import assert from 'node:assert/strict';
import test from 'node:test';
import { bellWindowOpen, canRingBell, canStartSteal } from '../src/lib/control/bell-rules.ts';

function runtime(phase, screen, id, owner = 0) {
	return {
		status: 'playing',
		controller: {
			phase,
			screen,
			selected_question_id: id,
			selected_contestant: owner,
			is_showing_question: true,
			is_allowing_bell: true,
			ve_dich_video: { status: 'idle' }
		},
		current_question: id ? { id } : null,
		contestants: [1, 2, 3, 4].map((slot) => ({
			slot,
			locked: false,
			bell_locked: false,
			has_rung: false
		})),
		timer: {
			duration_ms: 0,
			remaining_ms: 0,
			elapsed_ms: 0,
			started_at: 0,
			is_running: false,
			is_paused: false
		}
	};
}

function steal() {
	const state = runtime('ve_dich', 'ques_ts3', 'vd.2.30.0', 3);
	state.timer = { ...state.timer, duration_ms: 5000, remaining_ms: 5000, is_running: true };
	return state;
}

test('all four Khởi động chung stations can claim an unclaimed revealed question', () => {
	const state = runtime('khoi_dong', 'ques_chung', 'kd_chung.0');
	for (const slot of [1, 2, 3, 4]) assert.equal(canRingBell(state, slot, 0), true);
	state.contestants[1].has_rung = true;
	for (const slot of [1, 2, 3, 4]) assert.equal(canRingBell(state, slot, 0), false);
});

test('common question bells require the matching visible common question scene', () => {
	for (const change of ['hidden', 'missing', 'stale', 'screen', 'individual']) {
		const state = runtime('khoi_dong', 'ques_chung', 'kd_chung.0');
		if (change === 'hidden') state.controller.is_showing_question = false;
		if (change === 'missing') state.current_question = null;
		if (change === 'stale') state.current_question.id = 'kd_chung.1';
		if (change === 'screen') state.controller.screen = 'main_kd';
		if (change === 'individual') state.controller.selected_question_id = 'kd_rieng.0.0';
		assert.equal(canRingBell(state, 1, 0), false, change);
	}
});

for (const [phase, screen, id] of [
	['khoi_dong', 'ques_ts1', 'kd_rieng.0.0'],
	['tang_toc', 'ques', 'tt.0'],
	['idle', 'intro', ''],
	['main', 'main', ''],
	['score', 'score', '']
]) {
	test(`${phase}/${screen} cannot enable bells with a forged global flag`, () => {
		const state = runtime(phase, screen, id);
		state.timer = { ...state.timer, duration_ms: 5000, remaining_ms: 5000, is_running: true };
		for (const slot of [1, 2, 3, 4]) assert.equal(canRingBell(state, slot, 5000), false);
	});
}

for (const screen of [
	'start',
	'rule',
	'intro',
	'main_vcnv',
	'image_vcnv',
	'ques',
	'answers_vcnv',
	'end'
]) {
	test(`VCNV bells remain available on ${screen}, without a question or timer`, () => {
		const state = runtime('vcnv', screen, '');
		for (const slot of [1, 2, 3, 4]) assert.equal(canRingBell(state, slot, 0), true);
		state.contestants[0].has_rung = true;
		assert.equal(canRingBell(state, 1, 0), false);
		for (const slot of [2, 3, 4]) assert.equal(canRingBell(state, slot, 0), true);
	});
}

test('VCNV answer submission does not lock the independent bell, but an operator lock does', () => {
	const state = runtime('vcnv', 'ques', 'vcnv.rows.0');
	state.contestants[0].locked = true;
	state.contestants[0].answer = 'PRIVATE ANSWER';
	assert.equal(canRingBell(state, 1, 0), true);
	state.contestants[0].bell_locked = true;
	assert.equal(canRingBell(state, 1, 0), false);
});

test('Về đích grants only the three other stations access during the steal timer', () => {
	const state = steal();
	for (const slot of [1, 2, 4]) assert.equal(canRingBell(state, slot, 5000), true);
	assert.equal(canRingBell(state, 3, 5000), false);
	state.contestants[1].has_rung = true;
	for (const slot of [1, 2, 3, 4]) assert.equal(canRingBell(state, slot, 5000), false);
});

test('normal question timers, expired steals and paused steals cannot open Về đích bells', () => {
	for (const duration of [0, 15000, 20000]) {
		const state = steal();
		state.timer.duration_ms = duration;
		assert.equal(canRingBell(state, 1, 5000), false);
	}
	for (const change of ['expired', 'stopped', 'paused', 'hidden', 'screen', 'snapshot', 'video']) {
		const state = steal();
		if (change === 'stopped') state.timer.is_running = false;
		if (change === 'paused') state.timer.is_paused = true;
		if (change === 'hidden') state.controller.is_showing_question = false;
		if (change === 'screen') state.controller.screen = 'ques_ts1';
		if (change === 'snapshot') state.current_question.id = 'vd.1.30.0';
		if (change === 'video') state.controller.ve_dich_video.status = 'holding';
		assert.equal(canRingBell(state, 1, change === 'expired' ? 0 : 5000), false, change);
	}
});

test('Về đích ownership follows the question slot, not a stale selected-contestant value', () => {
	const state = steal();
	state.controller.selected_contestant = 1;
	assert.equal(canRingBell(state, 1, 5000), true);
	assert.equal(canRingBell(state, 3, 5000), false);
	state.timer.duration_ms = 20000;
	assert.equal(canStartSteal(state), true);
});

test('the five-second action can replace the owner timer but cannot duplicate a steal or claimed question', () => {
	for (const duration of [0, 15000, 20000]) {
		const state = runtime('ve_dich', 'ques_ts3', 'vd.2.30.0', 3);
		state.timer.duration_ms = duration;
		state.timer.is_running = duration > 0;
		assert.equal(canStartSteal(state), true);
	}
	for (const change of [
		'running',
		'paused',
		'ended',
		'claimed',
		'video',
		'hidden',
		'paused match'
	]) {
		const state = steal();
		if (change === 'paused') {
			state.timer.is_running = false;
			state.timer.is_paused = true;
		}
		if (change === 'ended') {
			state.timer.is_running = false;
			state.timer.remaining_ms = 0;
		}
		if (change === 'claimed') {
			state.timer.duration_ms = 15000;
			state.contestants[1].has_rung = true;
		}
		if (change === 'video') {
			state.timer.duration_ms = 0;
			state.controller.ve_dich_video.status = 'playing';
		}
		if (change === 'hidden') {
			state.timer.duration_ms = 0;
			state.controller.is_showing_question = false;
		}
		if (change === 'paused match') {
			state.timer.duration_ms = 0;
			state.status = 'paused';
		}
		assert.equal(canStartSteal(state), false, change);
	}
});

test('paused matches, unknown slots and server-closed gates remain closed on every station', () => {
	const state = runtime('vcnv', 'start', '');
	assert.equal(canRingBell(state, 0, 0), false);
	state.status = 'paused';
	assert.equal(bellWindowOpen(state, 0), false);
	state.status = 'playing';
	state.controller.is_allowing_bell = false;
	assert.equal(canRingBell(state, 1, 0), false);
	assert.equal(canRingBell(null, 1, 0), false);
	assert.equal(canStartSteal(null), false);
});
