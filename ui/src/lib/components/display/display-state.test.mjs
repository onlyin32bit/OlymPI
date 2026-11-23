import assert from 'node:assert/strict';
import test from 'node:test';

// Native Node type stripping keeps this suite independent of browser/test frameworks.
const {
	timerRemaining,
	timerElapsed,
	timerProgress,
	vcnvRevealKey,
	reviewContestants,
	VE_DICH_VIDEO_FINAL_FRAME_MS,
	assetURL,
	branchKey,
	publicView
} = await import(new URL('../../display/types.ts', import.meta.url).href);

function runtime() {
	return {
		match_id: 'match-1',
		epoch: 1,
		input_epoch: 1,
		version: 1,
		question_revision_id: 'revision-1',
		controller: {
			answer_status: [true, false, true, true],
			vcnv_state: {
				has_started: true,
				has_solved: false,
				row_states: ['u', 'u', 'u', 'u'],
				image_open_state: [false, false, false, false, false]
			}
		},
		contestants: [
			{ slot: 1, name: 'One', answer: 'A', time_ms: 3000 },
			{ slot: 2, name: 'Two', answer: 'B', time_ms: 1500 },
			{ slot: 3, name: 'Three', time_ms: 0 },
			{ slot: 4, name: 'Four', answer: 'D', time_ms: 1500 }
		]
	};
}

test('Về đích video holds the final frame for two seconds before the timer handoff', () => {
	assert.equal(VE_DICH_VIDEO_FINAL_FRAME_MS, 2000);
});

test('late mount and unrelated state updates retain the server timer anchor', () => {
	const timer = {
		is_running: true,
		is_paused: false,
		duration_ms: 30000,
		remaining_ms: 30000,
		elapsed_ms: 0,
		started_at: 100000
	};
	assert.equal(timerRemaining(timer, 106000), 24000);
	assert.equal(timerElapsed(timer, 106000), 6000);
	assert.equal(timerRemaining({ ...timer }, 107000), 23000);
	assert.equal(timerElapsed({ ...timer }, 107000), 7000);
});

test('pause/resume freezes elapsed time and preserves the original duration', () => {
	const paused = {
		is_running: false,
		is_paused: true,
		duration_ms: 30000,
		remaining_ms: 24000,
		elapsed_ms: 6000,
		started_at: 100000
	};
	assert.equal(timerRemaining(paused, 150000), 24000);
	assert.equal(timerElapsed(paused, 150000), 6000);
	const resumed = { ...paused, is_running: true, is_paused: false, started_at: 150000 };
	assert.equal(timerRemaining(resumed, 153000), 21000);
	assert.equal(timerElapsed(resumed, 153000), 9000);
	assert.equal(resumed.duration_ms, 30000);
	assert.equal(timerRemaining(resumed, 200000), 0);
	assert.equal(timerElapsed(resumed, 200000), 30000);
	assert.equal(timerRemaining(resumed, 149000), 24000);
});

test('audience progress fills from zero while the controller time counts down', () => {
	for (const duration of [3000, 15000, 13000, 20000]) {
		const timer = {
			duration_ms: duration,
			remaining_ms: duration,
			elapsed_ms: 0,
			started_at: 100000,
			is_running: true,
			is_paused: false
		};
		for (const fraction of [0, 0.25, 0.5, 1, 2]) {
			const now = timer.started_at + duration * fraction;
			assert.equal(timerProgress(timer, now), Math.min(fraction, 1));
			assert.equal(timerRemaining(timer, now), Math.max(duration * (1 - fraction), 0));
		}
	}
});

test('audience progress freezes on pause and resumes from the accrued elapsed time', () => {
	const paused = {
		duration_ms: 15000,
		remaining_ms: 9000,
		elapsed_ms: 6000,
		started_at: 0,
		is_running: false,
		is_paused: true
	};
	assert.equal(timerProgress(paused, 100000), 0.4);
	assert.equal(timerProgress(paused, 200000), 0.4);
	const resumed = { ...paused, is_running: true, is_paused: false, started_at: 200000 };
	assert.equal(timerProgress(resumed, 201500), 0.5);
	assert.equal(timerProgress(resumed, 199000), 0.4);
});

test('finished display indicators stay full until reset or question selection clears the timer', () => {
	const finished = {
		duration_ms: 15000,
		remaining_ms: 0,
		elapsed_ms: 15000,
		started_at: 0,
		is_running: false,
		is_paused: false
	};
	assert.equal(timerProgress(finished, 100000), 1);
	assert.equal(timerRemaining(finished, 100000), 0);
	assert.equal(timerProgress({ ...finished, duration_ms: 0, elapsed_ms: 0 }, 100000), 0);
});

test('missing, zero and invalid durations never produce a visible timer fill', () => {
	assert.equal(timerProgress(undefined, 100000), 0);
	for (const duration of [0, -1000]) {
		assert.equal(timerProgress({ duration_ms: duration }, 100000), 0);
	}
});

test('VCNV reveal cache ignores unrelated versions but changes on row/obstacle reveals and reset', () => {
	const state = runtime();
	const original = vcnvRevealKey(state);
	state.version += 1;
	assert.equal(vcnvRevealKey(state), original);
	state.controller.vcnv_state.row_states[0] = 'c';
	const solvedRow = vcnvRevealKey(state);
	assert.notEqual(solvedRow, original);
	state.controller.vcnv_state.has_solved = true;
	assert.notEqual(vcnvRevealKey(state), solvedRow);
	state.controller.vcnv_state.has_solved = false;
	state.controller.vcnv_state.row_states[0] = 'u';
	state.epoch += 1;
	assert.notEqual(vcnvRevealKey(state), original);
	assert.equal(vcnvRevealKey(null), '');
});

test('TT reviews rank four responses by response time without changing correctness slots', () => {
	const state = runtime();
	const sorted = reviewContestants(state, true);
	assert.deepEqual(
		sorted.map((/** @type {{slot:number}} */ value) => value.slot),
		[2, 4, 1, 3]
	);
	assert.equal(state.controller.answer_status[sorted[0].slot - 1], false);
	assert.equal(state.controller.answer_status[sorted[1].slot - 1], true);
	assert.deepEqual(
		state.contestants.map((value) => value.slot),
		[1, 2, 3, 4]
	);
});

test('VCNV reviews preserve legacy answered-first slot ordering and keep four positions', () => {
	assert.deepEqual(
		reviewContestants(runtime(), false).map((/** @type {{slot:number}} */ value) => value.slot),
		[1, 2, 4, 3]
	);
	assert.deepEqual(
		reviewContestants(null, false).map((/** @type {{slot:number}} */ value) => value.slot),
		[1, 2, 3, 4]
	);
});

test('asset URLs point at the asset endpoint', () => {
	assert.equal(assetURL('abc'), '/api/assets/abc');
});

test('router branch keys follow the legacy {#if} branches', () => {
	for (const slot of [1, 2, 3, 4]) {
		assert.equal(branchKey('khoi_dong', `ques_ts${slot}`), 'khoi_dong:ques_ts');
		assert.equal(branchKey('ve_dich', `pre_ques_ts${slot}`), 've_dich:pre_ques_ts');
		assert.equal(branchKey('ve_dich', `ques_ts${slot}`), 've_dich:ques_ts');
	}
	assert.equal(branchKey('khoi_dong', 'ques_chung'), 'khoi_dong:ques_chung');
	assert.notEqual(branchKey('ve_dich', 'pre_ques_ts1'), branchKey('ve_dich', 'ques_ts1'));
	assert.equal(branchKey('khoi_dong', 'test_bell'), 'khoi_dong:test_bell');
	assert.equal(branchKey('vcnv', 'main_vcnv'), 'vcnv:main_vcnv');
	assert.notEqual(branchKey('vcnv', 'main_vcnv'), branchKey('vcnv', 'image_vcnv'));
	assert.notEqual(branchKey('khoi_dong', 'end'), branchKey('vcnv', 'end'));
	assert.notEqual(branchKey('idle', 'intro'), branchKey('logo', 'intro'));
});

function adminRuntime(controller = {}) {
	return {
		...runtime(),
		controller: {
			phase: 'khoi_dong',
			screen: 'ques_ts1',
			is_showing_question: false,
			answer_status: [true, true, true, true],
			vcnv_state: {
				has_started: false,
				has_solved: false,
				row_states: ['u', 'u', 'u', 'u'],
				image_open_state: [false, false, false, false, false]
			},
			...controller
		},
		current_question: {
			id: 'q1',
			content: 'Question?',
			answer: 'Answer',
			keyword: 'KEY',
			type: 'image',
			asset_id: 'asset-1',
			explanation_asset_id: 'explain-1',
			time: 15,
			char_count: 3
		}
	};
}

test('public view stubs a hidden question to its frame data', () => {
	const admin = adminRuntime();
	const view = publicView(admin);
	assert.deepEqual(view.current_question, {
		id: 'q1',
		content: '',
		type: 'image',
		asset_id: 'asset-1',
		time: 15,
		char_count: 3
	});
	assert.equal(admin.current_question.content, 'Question?', 'admin state is not mutated');
	assert.equal(publicView(null), null);
	assert.equal(publicView({ ...admin, current_question: null }).current_question, null);
});

test('public view hides answers and keyword until the round allows them', () => {
	const shown = publicView(adminRuntime({ is_showing_question: true }));
	assert.equal(shown.current_question.content, 'Question?');
	assert.equal(shown.current_question.answer, undefined);
	assert.equal(shown.current_question.keyword, undefined);
	assert.equal(shown.current_question.explanation_asset_id, undefined);
	const end = publicView(adminRuntime({ is_showing_question: true, screen: 'end_ts' }));
	assert.equal(end.current_question.answer, 'Answer');
	const vcnv = adminRuntime({ phase: 'vcnv', screen: 'ques', is_showing_question: true });
	assert.equal(publicView(vcnv).current_question.keyword, undefined);
	vcnv.controller.vcnv_state.has_solved = true;
	assert.equal(publicView(vcnv).current_question.keyword, 'KEY');
});

test('public view gives Tăng tốc solve the text and explanation even while hidden', () => {
	const view = publicView(adminRuntime({ phase: 'tang_toc', screen: 'solve' }));
	assert.equal(view.current_question.content, 'Question?');
	assert.equal(view.current_question.answer, 'Answer');
	assert.equal(view.current_question.explanation_asset_id, 'explain-1');
});

test('public view hides contestant answers except on the answers screens', () => {
	const hidden = publicView(adminRuntime({ phase: 'tang_toc', screen: 'ques' }));
	assert.ok(hidden.contestants.every((value) => value.answer === '' && value.time_ms === 0));
	for (const [phase, screen] of [
		['tang_toc', 'answers_tt'],
		['vcnv', 'answers_vcnv']
	]) {
		const shown = publicView(adminRuntime({ phase, screen }));
		assert.deepEqual(
			shown.contestants.map((value) => value.answer),
			['A', 'B', undefined, 'D']
		);
		assert.equal(shown.contestants[1].time_ms, 1500);
	}
});
