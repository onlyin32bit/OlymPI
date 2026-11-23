import test from 'node:test';
import assert from 'node:assert/strict';
import { buildLogEntries, groupLogEntries } from '../src/lib/match-logs.ts';

function snapshot(question = 'tt.0', overrides = {}) {
	const phase = { kd_chung: 'khoi_dong', kd_rieng: 'khoi_dong', vcnv: 'vcnv', tt: 'tang_toc', vd: 've_dich' }[question.split('.')[0]] || 'tang_toc';
	return {
		match_id: 'match', status: 'playing', contestants: [{ slot: 1, name: 'An trước đây', score: 20 }],
		controller: { phase, screen: 'ques', selected_question_id: question, selected_contestant: 0,
			is_showing_question: false, is_allowing_bell: false, answer_status: [true, true, true, true],
			vcnv_state: { has_started: false, has_solved: false, row_states: ['u', 'u', 'u', 'u'], image_open_state: [false, false, false, false, false] },
			ve_dich_contestants_state: [] },
		timer: { duration_ms: 10000, remaining_ms: 5000 }, current_question: { id: question, answer: 'OFFICIAL-PRIVATE' }, ...overrides
	};
}
function event(sequence, type, state, payload = {}) {
	return { match_id: 'match', sequence, type, state, payload, created_at: '2026-01-01T00:00:00Z' };
}

test('selection, visibility and timer start are separate; raw identity and history are retained', () => {
	const state = snapshot();
	const visible = structuredClone(state); visible.controller.is_showing_question = true;
	const events = [event(1, 'Controller.Updated', state), event(2, 'Controller.Updated', visible), event(3, 'Timer.Started', visible, { duration_ms: 10000 }), event(4, 'Timer.Reset', visible)];
	const entries = buildLogEntries(events);
	assert.match(entries[0].detail, /Chọn Tăng tốc/);
	assert.doesNotMatch(entries[0].title, /Bắt đầu thời gian/);
	assert.match(entries[1].title, /Hiện câu hỏi/);
	assert.equal(entries[2].title, 'Bắt đầu thời gian trả lời');
	assert.equal(entries[3].questionID, 'tt.0');
	assert.equal(entries[3].groupID, entries[2].groupID);
	assert.equal(entries.length, events.length);
	entries.forEach((entry, i) => assert.equal(entry.event, events[i]));
});

test('answers are explicit opt-in fields; never exposed through normal or expanded detail', () => {
	const entries = buildLogEntries([
		event(1, 'Contestant.AnswerSubmitted', snapshot(), { slot: 1, answer: 'SUBMITTED-PRIVATE', time_ms: 1234 }),
		event(2, 'Contestant.AnswerSubmitted', snapshot(), { slot: 1, answer: 'SECOND-PRIVATE', time_ms: 1500 }),
		event(3, 'Contestant.Rang', snapshot(), { slot: 1, time_ms: 1700 })
	]);
	assert.equal(entries.length, 3);
	assert.equal(entries[0].answer, 'SUBMITTED-PRIVATE');
	assert.match(entries[0].title, /An trước đây/);
	assert.match(entries[0].detail, /1,234 giây/);
	for (const entry of entries) {
		assert.deepEqual(entry.slots, [1]);
		assert.doesNotMatch(`${entry.title} ${entry.detail}`, /PRIVATE|current_question|"controller"/);
	}
});

test('identical controller refresh is technical-only while explicit commands remain raw', () => {
	const state = snapshot();
	const entries = buildLogEntries([event(1, 'Controller.Updated', state), event(2, 'Controller.Updated', structuredClone(state)), event(3, 'Controller.AnswerStatusChanged', state)]);
	assert.deepEqual(entries[1].views, []);
	assert.deepEqual(entries[2].views, ['match', 'operations']);
	assert.equal(entries[2].title, 'Xác nhận trạng thái chấm câu trả lời');
	assert.deepEqual(entries[2].slots, []);
	assert.equal(entries.length, 3);
});

test('question attempts are contiguous and repeated timer starts create fresh attempts', () => {
	const entries = buildLogEntries([
		event(1, 'Controller.Updated', snapshot('tt.0')), event(2, 'Timer.Started', snapshot('tt.0')),
		event(3, 'Controller.Updated', snapshot('tt.1')), event(4, 'Controller.Updated', snapshot('tt.0')),
		event(5, 'Timer.Started', snapshot('tt.0')), event(6, 'Timer.Started', snapshot('tt.0')),
		event(7, 'Match.RecoveredPaused', snapshot('tt.0')), event(8, 'Timer.Resumed', snapshot('tt.0'))
	]);
	assert.notEqual(entries[0].groupID, entries[3].groupID);
	assert.notEqual(entries[4].groupID, entries[5].groupID);
	assert.notEqual(entries[5].groupID, entries[6].groupID);
	assert.equal(entries[6].groupID, entries[7].groupID);
	assert.equal(groupLogEntries(entries).length, 5);
});

test('single and batch scores show previous, new, delta and reason without merging', () => {
	const entries = buildLogEntries([
		event(1, 'Match.ScoreAdjusted', snapshot(), { slot: 1, delta: 10, score: 20, reason: 'Đúng' }),
		event(2, 'Match.ScoresAdjusted', snapshot(), { adjustments: [{ slot: 1, delta: -5, score: 15 }, { slot: 2, delta: 30, score: 30 }], reason: 'Tốc độ' })
	]);
	assert.match(entries[0].detail, /10 → 20 \(\+10\)/);
	assert.match(entries[0].detail, /Lý do: Đúng/);
	assert.match(entries[1].detail, /20 → 15 \(-5\)/);
	assert.match(entries[1].detail, /0 → 30 \(\+30\)/);
	assert.deepEqual(entries[1].slots, [1, 2]);
	assert.ok(entries.every(e => e.important && e.views.includes('match')));
});

test('fast Khởi động judgments are visible under the question judged, not the next one', () => {
	const first = snapshot('kd_rieng.0.0');
	const next = snapshot('kd_rieng.0.1', { contestants: [{ slot: 1, name: 'An trước đây', score: 30 }] });
	const common = snapshot('kd_chung.1');
	const end = snapshot('kd_chung.2', { contestants: [{ slot: 1, name: 'An trước đây', score: 0 }] });
	end.controller.selected_question_id = '';
	const entries = buildLogEntries([
		event(1, 'Controller.Updated', first),
		event(2, 'KhoiDong.Judged', next, { question_id: 'kd_rieng.0.0', verdict: 'correct', slot: 1, delta: 10 }),
		event(3, 'KhoiDong.Judged', common, { question_id: 'kd_chung.0', verdict: 'wrong', slot: 0, delta: 0 }),
		event(4, 'KhoiDong.Judged', end, { question_id: 'kd_chung.2', verdict: 'wrong', slot: 1, delta: -3 })
	]);
	assert.equal(entries[1].questionID, 'kd_rieng.0.0');
	assert.equal(entries[1].groupID, entries[0].groupID);
	assert.match(entries[1].title, /Chấm An trước đây: đúng/);
	assert.match(entries[1].detail, /20 → 30 \(\+10\)/);
	assert.deepEqual(entries[1].slots, [1]);
	assert.equal(entries[2].questionID, 'kd_chung.0');
	assert.match(entries[2].title, /Không có thí sinh trả lời/);
	assert.match(entries[2].detail, /Không thay đổi điểm/);
	assert.equal(entries[3].questionID, 'kd_chung.2');
	assert.match(entries[3].detail, /3 → 0 \(-3\)/);
	assert.ok(entries.slice(1).every(entry => entry.important && entry.views.includes('match') && entry.views.includes('operations')));
});

test('correctness diffs are per slot, reset defaults are not judgments', () => {
	const base = snapshot(), judged = structuredClone(base), next = snapshot('tt.1');
	judged.controller.answer_status[0] = false;
	const entries = buildLogEntries([event(1, 'Controller.Updated', base), event(2, 'Controller.AnswerStatusChanged', judged), event(3, 'Controller.Updated', next), event(4, 'Timer.Started', next)]);
	assert.match(entries[1].title, /Chấm An trước đây: sai/);
	assert.match(entries[1].detail, /đúng → sai/);
	assert.ok(entries[1].views.includes('match'));
	assert.doesNotMatch(entries[2].title, /Chấm/);
	assert.doesNotMatch(entries[3].title, /Chấm/);
});

test('compound controller edits describe fields and retain operations classification', () => {
	const before = snapshot(), after = structuredClone(before);
	after.controller.screen = 'answers_tt'; after.controller.is_allowing_bell = true;
	after.controller.vcnv_state.row_states[1] = 'c'; after.controller.vcnv_state.image_open_state[0] = true;
	after.controller.ve_dich_contestants_state = [{ selected_question_set: ['20_1', '30_2'], starred_on: 2 }];
	const entry = buildLogEntries([event(1, 'Controller.Updated', before), event(2, 'Controller.Updated', after)])[1];
	assert.match(entry.title, /Xem câu trả lời Tăng tốc/);
	assert.match(entry.detail, /Mở chuông/);
	assert.match(entry.detail, /VCNV hàng 2: đúng/);
	assert.match(entry.detail, /mảnh hình 1: mở/);
	assert.match(entry.detail, /20_1, 30_2/);
	assert.match(entry.detail, /ngôi sao câu 2/);
	assert.ok(entry.views.includes('operations'));
	assert.doesNotMatch(entry.detail, /OFFICIAL|\{|"phase"/);
});

test('lifecycle resumes use previous snapshots; missing snapshots and unknown errors are safe', () => {
	const entries = buildLogEntries([
		event(1, 'Match.Paused', snapshot('', { status: 'paused' })), event(2, 'Match.Playing', snapshot()),
		event(3, 'Unknown.Future', undefined, { answer: 'SECRET', state: { answer: 'SECRET' } }),
		event(4, 'Storage.WriteFailed', undefined, { message: 'SECRET' }), event(5, 'Match.ConfigurationChanged', undefined, { question_set_id: 'set', contestants: [{ slot: 1, name: 'Mới' }] })
	]);
	assert.equal(entries[1].title, 'Tiếp tục trận đấu');
	assert.deepEqual(entries[2].views, []);
	assert.equal(entries[2].event.type, 'Unknown.Future');
	assert.ok(entries[3].important && entries[3].views.includes('match'));
	assert.doesNotMatch(entries.map(e => `${e.title} ${e.detail}`).join('\n'), /SECRET|OFFICIAL/);
	assert.doesNotThrow(() => buildLogEntries([event(1, 'Controller.Updated', undefined, null), event(2, 'Timer.Reset', undefined, null)]));
});

test('destructive controls remain important and historical names do not use later configuration', () => {
	const old = snapshot(), renamed = snapshot('', { contestants: [{ slot: 1, name: 'Tên mới', score: 0 }] });
	const entries = buildLogEntries([event(1, 'Contestant.LockChanged', old, { slot: 1, locked: false }), event(2, 'Match.ConfigurationChanged', renamed), event(3, 'Controller.Reset', renamed), event(4, 'Match.ResultAmended', undefined, { reason: 'Phúc khảo' })]);
	assert.match(entries[0].title, /An trước đây/);
	assert.ok(entries[0].important);
	assert.deepEqual(entries[0].views, ['match', 'operations']);
	assert.ok(entries[2].important);
	assert.ok(entries[3].important);
	assert.match(entries[3].detail, /Phúc khảo/);
});

test('all timer actions retain their own rows and historical context', () => {
	const actions = ['Started', 'Paused', 'Resumed', 'Finished', 'Reset'];
	const entries = buildLogEntries(actions.map((action, i) => event(i + 1, `Timer.${action}`, snapshot('vcnv.rows.2'))));
	assert.deepEqual(entries.map(e => e.title), ['Bắt đầu thời gian trả lời', 'Tạm dừng thời gian', 'Tiếp tục thời gian', 'Hết giờ', 'Đặt lại đồng hồ']);
	assert.ok(entries.every(e => e.questionLabel === 'VCNV · Hàng 3'));
	assert.ok(entries.every(e => e.views.includes('match')));
	assert.equal(new Set(entries.map(e => e.id)).size, 5);
});

test('friendly labels use actual zero-based question paths, not question contents', () => {
	const cases = [
		['kd_chung.0', 'Khởi động chung · Câu 1'],
		['kd_rieng.2.4', 'Khởi động riêng · Thí sinh 3 · Câu 5'],
		['vcnv.center', 'VCNV · Câu trung tâm'],
		['vd.1.30.2', 'Về đích · Thí sinh 2 · Gói 30 · Câu 3']
	];
	for (const [id, label] of cases) {
		assert.equal(buildLogEntries([event(1, 'Timer.Started', snapshot(id))])[0].questionLabel, label);
	}
});

test('solve labels follow phase and compound titles contain only their main action', () => {
	for (const phase of ['tang_toc', 've_dich', 'vcnv']) {
		const before = snapshot(); before.controller.phase = phase;
		const after = structuredClone(before);
		after.controller.screen = 'solve';
		after.controller.is_allowing_bell = true;
		const entry = buildLogEntries([event(1, 'Controller.Updated', before), event(2, 'Controller.Updated', after)])[1];
		assert.equal(entry.title, `Chuyển màn hình: ${phase === 'vcnv' ? 'Giải chướng ngại vật' : 'Hiện đáp án'}`);
		assert.match(entry.detail, /Mở chuông/);
		assert.doesNotMatch(entry.title, /Mở chuông/);
	}
});

test('reset-induced VCNV and VD defaults are not reported as independent actions', () => {
	for (const kind of ['phase', 'question', 'explicit']) {
		const before = snapshot();
		before.controller.vcnv_state = { has_started: true, has_solved: true, row_states: ['c'], image_open_state: [true] };
		before.controller.ve_dich_contestants_state = [{ selected_question_set: ['30_1'], starred_on: 1 }];
		const after = structuredClone(before);
		after.controller.vcnv_state = { has_started: false, has_solved: false, row_states: ['u'], image_open_state: [false] };
		after.controller.ve_dich_contestants_state = [{ selected_question_set: [], starred_on: 0 }];
		if (kind === 'phase') after.controller.phase = 've_dich';
		if (kind === 'question') after.controller.selected_question_id = 'tt.1';
		const entry = buildLogEntries([event(1, 'Controller.Updated', before), event(2, kind === 'explicit' ? 'Controller.Reset' : 'Controller.Updated', after)])[1];
		assert.doesNotMatch(`${entry.title}\n${entry.detail}`, /VCNV hàng|mảnh hình|ngôi sao|chọn gói|Bắt đầu VCNV|Giải chướng ngại vật/);
		if (kind === 'explicit') assert.ok(entry.important);
	}
});

test('global score context ignores retained old question IDs without changing earlier history', () => {
	const question = snapshot('tt.2');
	const score = structuredClone(question);
	score.controller.phase = 'score';
	score.controller.screen = 'score';
	const entries = buildLogEntries([
		event(1, 'Timer.Started', question),
		event(2, 'Controller.Updated', score),
		event(3, 'Match.ScoreAdjusted', score, { slot: 1, delta: 10, score: 30 })
	]);
	assert.equal(entries[0].questionID, 'tt.2');
	assert.equal(entries[0].groupLabel, 'Tăng tốc · Câu 3');
	assert.equal(entries[1].questionID, '');
	assert.equal(entries[2].questionID, '');
	assert.equal(entries[2].groupLabel, 'Điểm số');
	assert.equal(entries[1].groupID, entries[2].groupID);
	assert.notEqual(entries[0].groupID, entries[2].groupID);
	assert.equal(entries[2].event.state.controller.selected_question_id, 'tt.2');
});

test('unknown legacy IDs remain raw without invented numbered labels', () => {
	const state = snapshot('legacy-question');
	const entry = buildLogEntries([event(1, 'Legacy.Event', state)])[0];
	assert.equal(entry.questionID, '');
	assert.equal(entry.event.state.controller.selected_question_id, 'legacy-question');
	assert.deepEqual(entry.views, []);
	const malformed = snapshot('tt.unknown');
	assert.equal(buildLogEntries([event(1, 'Legacy.Event', malformed)])[0].questionLabel, 'tt.unknown');
});

test('repeated explicit judgments retain precise historical who, verdict and slot filtering', () => {
	for (const correct of [true, false]) {
		const state = snapshot(); state.controller.answer_status[0] = correct;
		const payload = { ...state.controller, action: { slot: 1, correct } };
		const entries = buildLogEntries([
			event(1, 'Controller.Updated', state),
			event(2, 'Controller.AnswerStatusChanged', state, payload),
			event(3, 'Controller.AnswerStatusChanged', state, payload)
		]);
		for (const entry of entries.slice(1)) {
			assert.equal(entry.title, `Chấm An trước đây: ${correct ? 'đúng' : 'sai'}`);
			assert.deepEqual(entry.slots, [1]);
			assert.deepEqual(entry.views, ['match', 'operations']);
		}
		assert.equal(entries.filter(e => e.slots.includes(1) && e.event.type === 'Controller.AnswerStatusChanged').length, 2);
		assert.notEqual(entries[1].id, entries[2].id);
	}
});

test('repeated explicit VCNV judgments stay match-visible but row selection is operations-only', () => {
	for (const rowState of ['u', 's', 'c', 'w']) {
		const state = snapshot('vcnv.rows.1');
		state.controller.vcnv_state.row_states[1] = rowState;
		const payload = { ...state.controller, action: { row: 2, state: rowState } };
		const entries = buildLogEntries([event(1, 'Controller.Updated', state), event(2, 'Controller.VCNVRowStateChanged', state, payload), event(3, 'Controller.VCNVRowStateChanged', state, payload)]);
		for (const entry of entries.slice(1)) {
			assert.match(entry.title, /^VCNV hàng 2:/);
			assert.deepEqual(entry.views, ['c', 'w'].includes(rowState) ? ['match', 'operations'] : ['operations']);
			assert.deepEqual(entry.slots, []);
		}
	}
	const state = snapshot('vcnv.center'); state.controller.vcnv_state.has_solved = true;
	const entries = buildLogEntries([event(1, 'Controller.Updated', state), event(2, 'Controller.VCNVSolved', state, { ...state.controller, action: { solved: true } }), event(3, 'Controller.VCNVSolved', state, { ...state.controller, action: { solved: true } })]);
	for (const entry of entries.slice(1)) {
		assert.equal(entry.title, 'Giải chướng ngại vật: có');
		assert.deepEqual(entry.views, ['match', 'operations']);
	}
});

test('legacy unchanged commands stay discoverable without invented actor, row or verdict', () => {
	const state = snapshot();
	const types = ['Controller.AnswerStatusChanged', 'Controller.VCNVRowStateChanged', 'Controller.VCNVSolved', 'Controller.VCNVStarted', 'Controller.VCNVPanelChanged', 'Controller.VeDichQuestionSetSelected', 'Controller.VeDichStarChanged'];
	const entries = buildLogEntries([event(1, 'Controller.Updated', state), ...types.map((type, i) => event(i + 2, type, state, state.controller)), event(20, 'Controller.Updated', state)]);
	for (const entry of entries.slice(1, -1)) {
		assert.ok(entry.views.includes('operations'));
		assert.match(entry.title, /^Xác nhận/);
		assert.doesNotMatch(entry.title, /An trước đây|hàng [1-4]|: đúng|: sai/);
		assert.deepEqual(entry.slots, []);
	}
	assert.deepEqual(entries.at(-1).views, []);
});

test('the same question ID in a new question-set revision starts a separate historical group', () => {
	const old = snapshot('tt.0', { question_revision_id: 'revision-1' });
	const revised = snapshot('tt.0', { question_revision_id: 'revision-2' });
	const entries = buildLogEntries([
		event(1, 'Timer.Started', old, { duration_ms: 10000 }),
		event(2, 'Contestant.AnswerSubmitted', old, { slot: 1, answer: 'OLD', time_ms: 1000 }),
		event(3, 'Match.QuestionRevisionChanged', revised, { revision_id: 'revision-2' })
	]);
	assert.equal(entries[0].groupID, entries[1].groupID);
	assert.notEqual(entries[1].groupID, entries[2].groupID);
	assert.equal(entries[1].event.state.question_revision_id, 'revision-1');
});

test('VCNV row action context follows the judged row, not the currently selected row', () => {
	const before = snapshot('vcnv.rows.1'), judged = structuredClone(before);
	judged.controller.vcnv_state.row_states[0] = 'c';
	const payload = { ...judged.controller, action: { row: 1, state: 'c' } };
	const entries = buildLogEntries([
		event(1, 'Controller.Updated', before),
		event(2, 'Controller.VCNVRowStateChanged', judged, payload),
		event(3, 'Controller.VCNVRowStateChanged', judged, payload),
		event(4, 'Timer.Started', judged)
	]);
	assert.equal(entries[1].questionID, 'vcnv.rows.0');
	assert.equal(entries[1].questionLabel, 'VCNV · Hàng 1');
	assert.equal(entries[1].groupID, entries[2].groupID);
	assert.equal(entries.filter(e => e.questionID === 'vcnv.rows.0').length, 2);
	assert.equal(entries[3].questionID, 'vcnv.rows.1');
	assert.notEqual(entries[2].groupID, entries[3].groupID);
	assert.equal(entries[1].event.state.controller.selected_question_id, 'vcnv.rows.1');
});

test('legacy VCNV row context infers only an unambiguous individual row diff', () => {
	const before = snapshot('vcnv.rows.1'), single = structuredClone(before), multiple = structuredClone(before);
	single.controller.vcnv_state.row_states[0] = 'w';
	multiple.controller.vcnv_state.row_states[0] = 'c';
	multiple.controller.vcnv_state.row_states[2] = 'w';
	const entries = buildLogEntries([
		event(1, 'Controller.Updated', before),
		event(2, 'Controller.VCNVRowStateChanged', single, single.controller),
		event(3, 'Controller.VCNVRowStateChanged', single, single.controller),
		event(4, 'Controller.VCNVRowStateChanged', multiple, multiple.controller)
	]);
	assert.equal(entries[1].questionID, 'vcnv.rows.0');
	assert.equal(entries[2].questionID, 'vcnv.rows.1');
	assert.equal(entries[3].questionID, 'vcnv.rows.1');
});

test('clear answers and clear bells events appear in match and operations logs', () => {
	const state = snapshot('vcnv.rows.0');
	const entries = buildLogEntries([
		event(1, 'Contestant.AnswersCleared', state),
		event(2, 'Contestant.BellsCleared', state)
	]);
	assert.equal(entries[0].title, 'Xóa tất cả đáp án thí sinh');
	assert.equal(entries[1].title, 'Xóa tất cả chuông thí sinh');
	assert.ok(entries.every((entry) => entry.important && entry.views.includes('match') && entry.views.includes('operations')));
});
