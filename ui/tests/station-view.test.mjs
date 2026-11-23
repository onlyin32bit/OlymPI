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
const view = await server.ssrLoadModule('/src/lib/station/view.ts');
const answer = await server.ssrLoadModule('/src/lib/station/answer.ts');

function runtime({ phase = 'khoi_dong', screen = 'intro', selected = '', showing = false, type = 'text', asset = '', status = 'playing', timer = {}, contestants = [], bell = false, selectedContestant = 0 } = {}) {
	return {
		match_id: 'm', status, epoch: 1, input_epoch: 1, bell_epoch: 1,
		controller: {
			phase, screen, selected_question_id: selected, selected_contestant: selectedContestant,
			is_allowing_bell: bell, is_showing_question: showing,
			vcnv_state: { row_states: ['u', 'u', 'u', 'u'], image_open_state: [false, false, false, false, false] },
			ve_dich_contestants_state: [0, 1, 2, 3].map(() => ({ selected_question_set: [], starred_on: 0 })),
			ve_dich_video: { session_id: 0, status: 'idle', volume: 1 }
		},
		current_question: selected ? { id: selected, content: 'Câu hỏi', type, asset_id: asset } : undefined,
		contestants: [1, 2, 3, 4].map((slot) => ({ slot, name: `TS${slot}`, class: '', score: 0, time_ms: 0, locked: false, bell_locked: false, has_rung: false, ...contestants.find((entry) => entry.slot === slot) })),
		timer: { is_running: false, is_paused: false, duration_ms: 0, remaining_ms: 0, elapsed_ms: 0, started_at: 0, ...timer }
	};
}

test('the bell label follows its own state, never another contestant', () => {
	const open = runtime({ screen: 'ques_chung', selected: 'kd_chung.0', showing: true, bell: true });
	assert.deepEqual(view.bellView(open, 2, 0), { state: 'open', label: 'Nhấn chuông' });
	const won = runtime({ screen: 'ques_chung', selected: 'kd_chung.0', showing: true, bell: true, contestants: [{ slot: 3, has_rung: true }] });
	assert.equal(view.bellView(won, 2, 0).state, 'closed', 'someone else rang: closed, no name');
	assert.equal(view.bellView(won, 3, 0).label, 'Đã bấm chuông');
	const out = runtime({ phase: 'vcnv', screen: 'ques', bell: true, contestants: [{ slot: 1, bell_locked: true, locked: true }] });
	assert.equal(view.bellView(out, 1, 0).label, 'Đã bị loại');
	assert.equal(view.bellView(runtime({ phase: 'tang_toc', screen: 'ques' }), 1, 0).state, 'closed');
});

test('typing opens only for running VCNV and Tăng tốc questions', () => {
	const running = { is_running: true, duration_ms: 15000, remaining_ms: 15000, started_at: Date.now() };
	assert.equal(view.inputOpen(runtime({ phase: 'vcnv', screen: 'ques', selected: 'vcnv.rows.0', showing: true, timer: running }), 1, 9000), true);
	assert.equal(view.inputOpen(runtime({ phase: 'vcnv', screen: 'ques', selected: 'vcnv.rows.0', showing: true, timer: running, contestants: [{ slot: 1, locked: true }] }), 1, 9000), false);
	assert.equal(view.inputOpen(runtime({ phase: 'vcnv', screen: 'ques', selected: 'vcnv.rows.0', showing: true }), 1, 0), false);
	assert.equal(view.inputOpen(runtime({ phase: 'khoi_dong', screen: 'ques_chung', selected: 'kd_chung.0', showing: true, timer: running }), 1, 9000), false);
	assert.equal(view.inputOpen(runtime({ phase: 'tang_toc', screen: 'ques', selected: 'tt.0', showing: true, timer: running, status: 'paused' }), 1, 9000), false);
});

test('media rules: images show, Tăng tốc video is blank, other video shows only its text', () => {
	assert.deepEqual(view.questionView(runtime({ phase: 'vcnv', screen: 'ques', selected: 'vcnv.rows.1', showing: true, type: 'image', asset: 'img' })), { content: 'Câu hỏi', imageID: 'img' });
	assert.equal(view.questionView(runtime({ phase: 'tang_toc', screen: 'ques', selected: 'tt.3', showing: true, type: 'video', asset: 'vid' })), null);
	assert.deepEqual(view.questionView(runtime({ phase: 've_dich', screen: 'ques_ts1', selected: 'vd.0.30.0', showing: true, type: 'video', asset: 'vid' })), { content: 'Câu hỏi', imageID: undefined });
	assert.deepEqual(view.questionView(runtime({ phase: 'tang_toc', screen: 'ques', selected: 'tt.0', showing: true, type: 'audio', asset: 'a' })), { content: 'Câu hỏi', imageID: undefined });
	assert.equal(view.questionView(runtime({ phase: 'khoi_dong', screen: 'ques_chung', selected: 'kd_chung.0', showing: false })), null);
});

test('only the owner station is told it is their turn; other stations get no name', () => {
	const rieng = runtime({ screen: 'ques_ts2', selected: 'kd_rieng.1.3', showing: true });
	assert.deepEqual(view.mainView(rieng, 2), { kind: 'question', round: 'Khởi động', turn: 'Lượt riêng', ownTurn: true, label: 'Câu 4' });
	assert.equal(view.mainView(rieng, 1).ownTurn, false);
	assert.equal(JSON.stringify(view.mainView(rieng, 1)).includes('TS2'), false);
	assert.equal(view.mainView(runtime({ screen: 'ques_chung', selected: 'kd_chung.0' }), 3).turn, 'Lượt chung');
	const vd = runtime({ phase: 've_dich', screen: 'ques_ts4', selected: 'vd.3.20.1' });
	assert.deepEqual(view.mainView(vd, 4), { kind: 've_dich', ownTurn: true, choosing: false });
	assert.equal(view.mainView(vd, 1).ownTurn, false);
	assert.equal(view.mainView(runtime({ phase: 've_dich', screen: 'pre_ques_ts1', selectedContestant: 1 }), 1).choosing, true);
});

test('main area modes for status, standings, round slides, VCNV and review', () => {
	assert.equal(view.mainView(null, 1).kind, 'status');
	assert.equal(view.mainView(runtime({ status: 'paused' }), 1).title, 'Trận đấu đang tạm dừng');
	assert.equal(view.mainView(runtime({ status: 'ready' }), 1).title, 'Chờ bắt đầu trận đấu');
	assert.equal(view.mainView(runtime({ phase: 'score', screen: 'score' }), 1).kind, 'standings');
	assert.deepEqual(view.mainView(runtime({ phase: 'tang_toc', screen: 'rule' }), 1), { kind: 'round', title: 'Tăng tốc', rule: true, ended: false });
	assert.equal(view.mainView(runtime({ phase: 'vcnv', screen: 'main_vcnv' }), 1).kind, 'vcnv');
	assert.deepEqual(view.mainView(runtime({ phase: 'tang_toc', screen: 'answers_tt' }), 1), { kind: 'review', timed: true });
});

test('the steal window is the 5-second Về đích timer', () => {
	const running = { is_running: true, duration_ms: 5000, remaining_ms: 5000, started_at: Date.now() };
	assert.equal(view.stealWindow(runtime({ phase: 've_dich', screen: 'ques_ts1', timer: running })), true);
	assert.equal(view.stealWindow(runtime({ phase: 've_dich', screen: 'ques_ts1', timer: { ...running, duration_ms: 15000 } })), false);
	assert.equal(view.formatCountdown(14_200), '15');
	assert.equal(view.formatCountdown(4_210), '4.3');
	assert.equal(view.formatCountdown(0), '0.0');
});

test('answers are sent in NFC capitals and raw Telex is flagged without flagging English', () => {
	const decomposed = 'tro\u0302\u0301ng';
	assert.equal(answer.normalizeAnswer(`  ${decomposed}   dong  `), 'TRỐNG DONG');
	assert.equal(answer.normalizeAnswer(decomposed).length, 'TRỐNG'.length);
	for (const raw of ['troongs', 'Hoaf', 'dduwowngf', 'vieetj nam', 'nguwowif'])
		assert.equal(answer.looksLikeRawTelex(raw), true, raw);
	for (const ok of ['TRỐNG', 'European Union', 'books', 'tree', 'bao nhan dan', 'tu mien', 'of', 'Hình quạt tròn'])
		assert.equal(answer.looksLikeRawTelex(ok), false, ok);
});
