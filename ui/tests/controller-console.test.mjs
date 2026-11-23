import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { render } from 'svelte/server';
import { createControllerTestServer, consolePath, fixturePath } from './controller-test-server.mjs';

const server = await createControllerTestServer();
after(() => server.close());
const { default: Console } = await server.ssrLoadModule(consolePath);
const { fixture } = await server.ssrLoadModule(fixturePath);
const workflow = await server.ssrLoadModule('/src/lib/control/question-workflow.ts');
const { openLiveQuestion } = workflow;
const { socket } = await server.ssrLoadModule(fixturePath);
const { TimerSocket } = await server.ssrLoadModule('/src/lib/socket/timer/index.svelte.ts');
const { panel } = await server.ssrLoadModule('/src/lib/operator/panel.svelte.ts');
const { default: ContestantCards } = await server.ssrLoadModule('/src/routes/(protected)/(admin)/control-panel-next/contestant-cards.svelte');

const controls = {
	khoi_dong: ['Lượt thi và khóa thí sinh', 'Chung', 'Minh Tuấn: Khóa', 'Xong lượt'],
	vcnv: [
		'Hàng ngang',
		'Mảnh hình',
		'Chấm đáp án',
		'Trên trái: Mở mảnh hình',
		'Trung tâm: Mở mảnh hình',
		'Mở hàng 4',
		'Đã bắt đầu',
		'Đã giải'
	],
	tang_toc: ['Mở câu tiếp theo', 'Chấm đáp án', 'Lời giải'],
	ve_dich: [
		'Gói câu hỏi và ngôi sao',
		'Câu 1: 20 điểm',
		'Câu 3: 30 điểm',
		'Đặt sao câu 1',
		'Đặt sao câu 2',
		'Đặt sao câu 3',
		'Xong lượt'
	]
};

for (const [phase, labels] of Object.entries(controls)) {
	test(`${phase} keeps manual controls accessible on start, rule, intro and end`, () => {
		for (const screen of ['start', 'rule', 'intro', 'end']) {
			fixture.reset(phase, screen);
			const { body } = render(Console);
			for (const label of labels)
				assert.ok(body.includes(label), `${phase}/${screen}: missing ${label}`);
			assert.equal((body.match(/aria-label="Màn hình của vòng"/g) ?? []).length, 1);
			assert.equal((body.match(/— Đang phát/g) ?? []).length, 1);
			assert.match(body, /aria-label="Màn hình trước"/);
			assert.match(body, /aria-label="Màn hình tiếp theo"/);
			assert.match(body, /<section[^>]*aria-label="Điều khiển trực tiếp"/);
			assert.match(body, /aria-label="Thời gian còn lại"/);
			assert.match(body, /aria-label="Bắt đầu đồng hồ"/);
			if (phase === 've_dich') assert.match(body, /aria-label="Mở cướp quyền 5 giây"/);
			else assert.match(body, /aria-label="Trạng thái chuông"/);
			assert.doesNotMatch(
				body,
				/Luật đã hiển thị|Mở đầu vòng|bảng Điều khiển thời gian|bên dưới|Sẵn sàng mở|Giới thiệu xong/
			);
		}
	});
}

test('main phase is rendered without instructional filler or a duplicate phase navigator', () => {
	fixture.reset('main', 'main');
	const { body } = render(Console);
	assert.match(body, /Đang phát:/);
	assert.equal((body.match(/aria-label="Vòng thi"/g) ?? []).length, 1);
	assert.equal((body.match(/aria-label="Màn hình của vòng"/g) ?? []).length, 1);
	assert.doesNotMatch(body, /Điều khiển thời gian và câu hỏi|Giới thiệu phần thi/);
});

test('question switching, show/hide and timer share one fixed command group before the question reader', () => {
	fixture.reset('khoi_dong', 'ques_chung');
	fixture.setQuestion('kd_chung.0');
	const { body } = render(Console);
	assert.match(body, /Nội dung câu hỏi dành cho người điều khiển chương trình/);
	assert.match(body, /Đáp án kiểm thử/);
	assert.doesNotMatch(body, /line-clamp/);
	const command = body.match(/<section class="question-command[^]*?<\/section>/)?.[0];
	assert.ok(command, 'Missing shared question command group');
	for (const control of [
		'id="question"',
		'Mở câu trước',
		'Mở câu tiếp theo',
		'Hiện câu',
		'Bắt đầu đồng hồ',
		'Đặt lại đồng hồ'
	])
		assert.ok(command.includes(control), `${control} must stay beside question selection`);
	assert.ok(body.indexOf('class="question-command') < body.indexOf('class="workspace'));
	assert.doesNotMatch(body, /<footer/);
	assert.match(body, /aria-pressed="false"[^>]*>.*Hiện câu/s);
});

for (const [phase, screen, id, time, readout] of [
	['khoi_dong', 'ques_chung', 'kd_chung.0', 90, '00:03.0'],
	['khoi_dong', 'ques_ts3', 'kd_rieng.2.0', 90, '00:03.0'],
	['vcnv', 'ques', 'vcnv.rows.1', 90, '00:15.0'],
	['vcnv', 'ques', 'vcnv.center', 23, '00:23.0'],
	['tang_toc', 'ques', 'tt.0', 13, '00:13.0'],
	['ve_dich', 'ques_ts1', 'vd.0.20.0', 90, '00:15.0'],
	['ve_dich', 'ques_ts1', 'vd.0.30.0', 90, '00:20.0']
]) {
	test(`${phase}/${id} shows its game-owned preset without a manual time input`, async () => {
		fixture.reset(phase, screen);
		await socket.controller.selectQuestion(id);
		fixture.runtime.current_question.time = time;
		const { body } = render(Console);
		assert.ok(body.includes(readout), `Missing ready timer ${readout}`);
		assert.doesNotMatch(body, /Số giây|duration-control/);
		assert.match(body, /aria-label="Bắt đầu đồng hồ"/);
	});
}

test('paused timers show their remaining time and reset restores the preset', async () => {
	fixture.reset('khoi_dong', 'ques_chung');
	await socket.controller.selectQuestion('kd_chung.0');
	fixture.runtime.timer = {
		duration_ms: 3000,
		remaining_ms: 1250,
		elapsed_ms: 1750,
		started_at: 0,
		is_running: false,
		is_paused: true
	};
	let body = render(Console).body;
	assert.ok(body.includes('00:01.2'));
	assert.match(body, /aria-label="Tiếp tục đồng hồ"/);
	await socket.timer.resume();
	assert.equal(fixture.runtime.timer.duration_ms, 3000);
	assert.equal(fixture.runtime.timer.remaining_ms, 1250);
	await socket.timer.reset();
	body = render(Console).body;
	assert.ok(body.includes('00:03.0'));
	assert.match(body, /aria-label="Bắt đầu đồng hồ"/);
});

test('timer client sends the match command and explicit steal mode, never a chosen duration', async () => {
	const calls = [];
	const command = { match_id: 'timing-fixture', expected_epoch: 1, expected_version: 2 };
	const timer = new TimerSocket(
		{
			request: async (method, payload) => {
				calls.push([method, payload]);
				return fixture.runtime;
			}
		},
		{ onUpdate() {}, command: () => command, accept: (runtime) => runtime }
	);
	await timer.start();
	await timer.startSteal();
	await timer.pause();
	await timer.resume();
	await timer.reset();
	assert.deepEqual(calls, [
		['Timer.Start', { command }],
		['Timer.Start', { command, steal: true }],
		['Timer.Pause', command],
		['Timer.Resume', command],
		['Timer.Reset', command]
	]);
});

test('the five-second steal action stays beside question/timer controls and respects its window', async () => {
	fixture.reset('ve_dich', 'ques_ts1');
	const stealButton = () => {
		const body = render(Console).body;
		const command = body.match(/<section class="question-command[^]*?<\/section>/)?.[0];
		const button = [...(command?.matchAll(/<button\b(?:[^"'>]|"[^"]*"|'[^']*')*>/g) ?? [])]
			.map(([tag]) => tag)
			.find((tag) => tag.includes('aria-label="Mở cướp quyền 5 giây"'));
		assert.ok(button, 'Steal must stay visible beside the question, show and clock buttons');
		return button;
	};
	assert.match(stealButton(), /\sdisabled(?:[=\s>])/);
	await socket.controller.selectContestant(1);
	await socket.controller.selectQuestion('vd.0.20.0');
	await socket.controller.toggleQuestion(true);
	await socket.timer.start();
	assert.doesNotMatch(stealButton(), /\sdisabled(?:[=\s>])/);
	await socket.timer.startSteal();
	assert.ok(render(Console).body.includes('00:05.0'));
	assert.match(stealButton(), /\sdisabled(?:[=\s>])/);
	await socket.timer.pause();
	assert.match(stealButton(), /\sdisabled(?:[=\s>])/);
	await socket.timer.reset();
	assert.doesNotMatch(stealButton(), /\sdisabled(?:[=\s>])/);
	assert.ok(render(Console).body.includes('00:15.0'));
	fixture.runtime.contestants[1].has_rung = true;
	fixture.syncBell();
	assert.match(stealButton(), /\sdisabled(?:[=\s>])/);
});

test('video controls are all discoverable together and respect the active question ownership', () => {
	fixture.reset('ve_dich', 'ques_ts1');
	fixture.runtime.controller.selected_contestant = 1;
	fixture.runtime.controller.ve_dich_contestants_state[0].selected_question_set = [
		'20_1',
		'20_2',
		'20_3'
	];
	fixture.setQuestion('vd.0.20.0');
	const { body } = render(Console);
	for (const label of ['Phát media', 'Tạm dừng', 'Tiếp tục', 'Dừng video', 'Âm lượng'])
		assert.ok(body.includes(label));
	assert.match(body, /id="vd-video-volume"/);
});

for (const [phase, id, screen, owner] of [
	['khoi_dong', 'kd_chung.0', 'ques_chung', 0],
	['khoi_dong', 'kd_rieng.2.0', 'ques_ts3', 3],
	['vcnv', 'vcnv.rows.2', 'ques', 0],
	['vcnv', 'vcnv.center', 'ques', 0],
	['tang_toc', 'tt.0', 'ques', 0],
	['ve_dich', 'vd.2.30.1', 'ques_ts3', 3]
]) {
	test(`${phase}/${id} opens the question frame only, without revealing it or starting the clock`, async () => {
		fixture.reset(phase, 'rule');
		fixture.runtime.controller.ve_dich_contestants_state[2].selected_question_set = [
			'20_1',
			'30_2',
			'20_3'
		];
		await openLiveQuestion(phase, id);
		assert.equal(fixture.runtime.controller.screen, screen);
		assert.equal(fixture.runtime.controller.selected_question_id, id);
		assert.equal(fixture.runtime.controller.is_showing_question, false);
		if (owner) assert.equal(fixture.runtime.controller.selected_contestant, owner);
		assert.ok(
			!fixture.calls.some(([command]) => command.startsWith('timer-') || command === 'bell')
		);
		const changes = fixture.calls.filter(([command]) =>
			['screen', 'question', 'visibility'].includes(command)
		);
		assert.deepEqual(changes, [
			['screen', screen],
			['question', id]
		]);
	});
}

test('opening another VCNV row clears only the prior pending row, not adjudicated rows', async () => {
	fixture.reset('vcnv', 'main_vcnv');
	fixture.runtime.controller.vcnv_state.row_states = ['s', 'c', 'u', 'w'];
	await openLiveQuestion('vcnv', 'vcnv.rows.2');
	assert.deepEqual(fixture.runtime.controller.vcnv_state.row_states, ['u', 'c', 's', 'w']);
	assert.equal(fixture.runtime.controller.screen, 'ques');
});

test('reopening the current question changes nothing: no reveal, answers, owner or timer reset', async () => {
	fixture.reset('tang_toc', 'ques');
	fixture.setQuestion('tt.1');
	fixture.runtime.controller.answer_status[0] = false;
	await openLiveQuestion('tang_toc', 'tt.1');
	assert.deepEqual(fixture.calls, []);
	assert.equal(fixture.runtime.controller.answer_status[0], false);
});

test('a question from another round or an unselected finish package cannot be opened', async () => {
	fixture.reset('tang_toc', 'rule');
	await assert.rejects(openLiveQuestion('tang_toc', 'kd_chung.0'));
	assert.deepEqual(fixture.calls, []);
	fixture.reset('ve_dich', 'rule');
	await assert.rejects(openLiveQuestion('ve_dich', 'vd.1.20.0'));
	assert.deepEqual(fixture.calls, []);
});

for (const changed of ['phase', 'epoch', 'revision', 'input']) {
	test(`a ${changed} change during screen navigation aborts before selecting or revealing a question`, async () => {
		fixture.reset('tang_toc', 'rule');
		const original = socket.controller.changeScreen;
		socket.controller.changeScreen = async (screen) => {
			await original(screen);
			if (changed === 'phase') fixture.runtime.controller.phase = 'main';
			if (changed === 'epoch') fixture.runtime.epoch++;
			if (changed === 'revision') fixture.runtime.question_revision_id = 'replacement';
			if (changed === 'input') fixture.runtime.input_epoch++;
		};
		try {
			await assert.rejects(openLiveQuestion('tang_toc', 'tt.0'), { name: 'AbortError' });
			assert.equal(fixture.runtime.controller.selected_question_id, '');
			assert.equal(fixture.runtime.controller.is_showing_question, false);
		} finally {
			socket.controller.changeScreen = original;
		}
	});
}

for (const [screen, previous, next] of [
	['ques_chung', 'kd_chung.0', 'kd_chung.1'],
	['ques_ts2', 'kd_rieng.1.0', 'kd_rieng.1.1']
]) {
	test(`Khởi động ${screen}: the next question in a showing turn appears straight away`, async () => {
		fixture.reset('khoi_dong', screen);
		fixture.setQuestion(previous);
		fixture.runtime.controller.selected_contestant = screen === 'ques_ts2' ? 2 : 0;
		fixture.runtime.controller.is_showing_question = true;
		await openLiveQuestion('khoi_dong', next);
		assert.deepEqual(fixture.calls, [
			['question', next],
			['visibility', true]
		]);
		assert.equal(fixture.runtime.controller.is_showing_question, true);
	});
}

test('Khởi động opens hidden when the previous question was hidden, on another screen or the end card', async () => {
	fixture.reset('khoi_dong', 'ques_chung');
	fixture.setQuestion('kd_chung.0');
	await openLiveQuestion('khoi_dong', 'kd_chung.1');
	assert.equal(fixture.runtime.controller.is_showing_question, false);

	fixture.reset('khoi_dong', 'ques_chung');
	fixture.setQuestion('kd_chung.3');
	fixture.runtime.controller.is_showing_question = true;
	await openLiveQuestion('khoi_dong', 'kd_rieng.0.0');
	assert.equal(fixture.runtime.controller.screen, 'ques_ts1');
	assert.equal(fixture.runtime.controller.is_showing_question, false);

	fixture.reset('khoi_dong', 'ques_ts1');
	fixture.runtime.controller.is_showing_question = true;
	assert.ok(workflow.isKhoiDongEnd(fixture.runtime.controller));
	await openLiveQuestion('khoi_dong', 'kd_rieng.0.0');
	assert.equal(fixture.runtime.controller.is_showing_question, false);
	assert.ok(!fixture.calls.some(([command]) => command === 'visibility'));
});

test('Khởi động next after the last question of the turn shows KẾT THÚC on the same screen', async () => {
	const strip = [0, 1, 2, 3, 4, 5].map((index) => `kd_rieng.2.${index}`);
	fixture.reset('khoi_dong', 'ques_ts3');
	assert.deepEqual(workflow.nextQuestionStep(fixture.runtime.controller, strip), { kind: 'open', id: strip[0] });
	fixture.setQuestion('kd_rieng.2.4');
	assert.deepEqual(workflow.nextQuestionStep(fixture.runtime.controller, strip), { kind: 'open', id: strip[5] });
	fixture.setQuestion('kd_rieng.2.5');
	fixture.runtime.controller.is_showing_question = true;
	assert.deepEqual(workflow.nextQuestionStep(fixture.runtime.controller, strip), { kind: 'end' });
	await workflow.showKhoiDongEnd();
	assert.deepEqual(fixture.calls, [
		['question', ''],
		['visibility', true]
	]);
	assert.equal(fixture.runtime.controller.screen, 'ques_ts3');
	assert.ok(workflow.isKhoiDongEnd(fixture.runtime.controller));
	assert.equal(workflow.nextQuestionStep(fixture.runtime.controller, strip), null);

	fixture.reset('vcnv', 'ques');
	fixture.setQuestion('vcnv.center');
	assert.equal(workflow.nextQuestionStep(fixture.runtime.controller, ['vcnv.rows.0', 'vcnv.center']), null);
	await assert.rejects(workflow.showKhoiDongEnd());
});

test('Thử chuông and the common question offer Xoá chuông, which clears every bell', async () => {
	for (const screen of ['test_bell', 'ques_chung']) {
		fixture.reset('khoi_dong', screen);
		assert.ok(render(Console).body.includes('Xoá chuông'), `${screen}: missing Xoá chuông`);
	}
	fixture.reset('khoi_dong', 'ques_ts1');
	assert.ok(!render(Console).body.includes('Xoá chuông'));
	fixture.reset('khoi_dong', 'test_bell');
	fixture.runtime.contestants[0].has_rung = true;
	await socket.controller.clearBells();
	assert.equal(fixture.runtime.contestants[0].has_rung, false);
});

test('VCNV board button shows the empty board first, then starts it', async () => {
	fixture.reset('vcnv', 'rule');
	assert.equal(workflow.vcnvBoardStep(fixture.runtime.controller), 'show');
	assert.ok(render(Console).body.includes('Mở bảng'));
	await workflow.pressVCNVBoard();
	assert.equal(fixture.runtime.controller.screen, 'main_vcnv');
	assert.equal(fixture.runtime.controller.vcnv_state.has_started, false);
	assert.equal(workflow.vcnvBoardStep(fixture.runtime.controller), 'start');
	assert.ok(render(Console).body.includes('Bắt đầu'));
	await workflow.pressVCNVBoard();
	assert.equal(fixture.runtime.controller.vcnv_state.has_started, true);
	assert.equal(workflow.vcnvBoardStep(fixture.runtime.controller), null);
	assert.deepEqual(fixture.calls, [
		['screen', 'main_vcnv'],
		['board', true]
	]);
});

test('VCNV row Mở chooses the row on the board first, the second press opens its question hidden', async () => {
	fixture.reset('vcnv', 'image_vcnv');
	fixture.runtime.controller.vcnv_state.row_states = ['s', 'c', 'u', 'w'];
	await workflow.pressVCNVRow(2);
	assert.equal(fixture.runtime.controller.screen, 'main_vcnv');
	assert.deepEqual(fixture.runtime.controller.vcnv_state.row_states, ['u', 'c', 's', 'w']);
	assert.equal(fixture.runtime.controller.selected_question_id, '');
	assert.deepEqual(fixture.calls, [
		['screen', 'main_vcnv'],
		['row', 1, 'u'],
		['row', 3, 's']
	]);
	fixture.calls.length = 0;
	await workflow.pressVCNVRow(2);
	assert.equal(fixture.runtime.controller.screen, 'ques');
	assert.equal(fixture.runtime.controller.selected_question_id, 'vcnv.rows.2');
	assert.equal(fixture.runtime.controller.is_showing_question, false);
	assert.deepEqual(fixture.calls, [
		['screen', 'ques'],
		['question', 'vcnv.rows.2']
	]);
});

test('VCNV obstacle timer is offered on the board and picture only while nothing is timing', async () => {
	for (const [screen, allowed] of [
		['main_vcnv', true],
		['image_vcnv', true],
		['ques', false],
		['rule', false]
	]) {
		fixture.reset('vcnv', screen);
		assert.equal(workflow.canStartObstacleTimer(fixture.runtime), allowed, screen);
		const button = [...render(Console).body.matchAll(/<button\b(?:[^"'>]|"[^"]*"|'[^']*')*>/g)]
			.map(([tag]) => tag)
			.find((tag) => tag.includes('title="Đồng hồ chướng ngại vật 15 giây"'));
		assert.ok(button, `${screen}: missing obstacle timer`);
		assert.equal(/\sdisabled(?:[=\s>])/.test(button), !allowed, screen);
	}
	fixture.reset('vcnv', 'main_vcnv');
	await socket.timer.startObstacle();
	assert.equal(fixture.runtime.timer.kind, 'obstacle');
	assert.equal(workflow.canStartObstacleTimer(fixture.runtime), false);
	fixture.reset('khoi_dong', 'main_kd');
	assert.equal(workflow.canStartObstacleTimer(fixture.runtime), false);
});

test('Phát media: audio in any round but Tăng tốc, video only in Về đích, nothing for text or image', () => {
	const kind = (phase, screen, id, type, extra = () => {}) => {
		fixture.reset(phase, screen);
		extra();
		fixture.setQuestion(id);
		Object.assign(fixture.runtime.current_question, { type, asset_id: 'asset' });
		return workflow.questionMediaKind(fixture.runtime);
	};
	assert.equal(kind('khoi_dong', 'ques_chung', 'kd_chung.0', 'audio'), 'audio');
	assert.equal(kind('khoi_dong', 'ques_ts1', 'kd_chung.0', 'audio'), null);
	assert.equal(kind('vcnv', 'ques', 'vcnv.rows.1', 'audio'), 'audio');
	assert.equal(kind('vcnv', 'main_vcnv', 'vcnv.rows.1', 'audio'), null);
	assert.equal(kind('vcnv', 'ques', 'vcnv.center', 'image'), null);
	assert.equal(kind('tang_toc', 'ques', 'tt.0', 'audio'), null);
	assert.equal(kind('tang_toc', 'ques', 'tt.0', 'video'), null);
	assert.equal(kind('ve_dich', 'ques_ts1', 'vd.0.20.0', 'video'), 'video');
	assert.equal(kind('ve_dich', 'ques_ts1', 'vd.0.30.1', 'audio'), 'audio');
	assert.equal(kind('ve_dich', 'ques_ts1', 'vd.0.20.0', 'text'), null);
	fixture.reset('ve_dich', 'ques_ts1');
	fixture.setQuestion('vd.0.20.0');
	fixture.runtime.current_question.asset_id = '';
	assert.equal(workflow.questionMediaKind(fixture.runtime), null);
});

test('Về đích Chốt gói stays on the package screen without opening question 1', async () => {
	fixture.reset('ve_dich', 'pre_ques_ts2');
	const { body } = render(Console);
	assert.ok(body.includes('Chốt gói'));
	assert.doesNotMatch(body, /mở câu hỏi<\/button>/i);
});

test('upper contestant card Đúng/Sai never plays sound; lower-panel sound controls do', async () => {
	for (const [phase, screen, id] of [
		['khoi_dong', 'ques_ts1', 'kd_rieng.0.0'],
		['khoi_dong', 'ques_chung', 'kd_chung.0'],
		['vcnv', 'ques', 'vcnv.rows.0'],
		['vcnv', 'answers_vcnv', 'vcnv.center'],
		['tang_toc', 'answers_tt', 'tt.0'],
		['ve_dich', 'ques_ts1', 'vd.0.20.0']
	]) {
		fixture.reset(phase, screen);
		fixture.setQuestion(id);
		fixture.calls.length = 0;
		panel.verdict(1, 'correct');
		panel.verdict(2, 'wrong');
		assert.deepEqual(
			fixture.calls.filter(([kind]) => kind === 'sound'),
			[],
			`${phase}/${id}: upper card verdict must stay silent`
		);
	}

	fixture.reset('vcnv', 'answers_vcnv');
	fixture.setQuestion('vcnv.rows.0');
	fixture.calls.length = 0;
	panel.playVerdictSound('correct');
	panel.playVerdictSound('wrong');
	panel.obstacleVerdict(1, 'correct');
	panel.obstacleVerdict(2, 'wrong');
	assert.deepEqual(
		fixture.calls.filter(([kind]) => kind === 'sound'),
		[
			['sound', 'builtin.vcnv.correctrow'],
			['sound', 'builtin.vcnv.wrongrow'],
			['sound', 'builtin.vcnv.correctobstacle'],
			['sound', 'builtin.vcnv.wrongrow']
		]
	);
});

test('contestant cards footer offers Xoá đáp án and Xoá chuông on the Hoàn tác / Ghi điểm row', async () => {
	fixture.reset('vcnv', 'answers_vcnv');
	fixture.setQuestion('vcnv.rows.0');
	fixture.runtime.contestants[0].answer = 'HÀ NỘI';
	fixture.runtime.contestants[0].time_ms = 1200;
	fixture.runtime.contestants[1].has_rung = true;
	const { body } = render(ContestantCards);
	const footer = body.match(/<footer[\s\S]*?<\/footer>/)?.[0] ?? '';
	for (const label of ['Xoá đáp án', 'Xoá chuông', 'Hoàn tác', 'Ghi điểm']) {
		assert.ok(footer.includes(label), `footer missing ${label}`);
	}
	fixture.calls.length = 0;
	await socket.controller.clearAnswers();
	assert.equal(fixture.runtime.contestants[0].answer, '');
	assert.equal(fixture.runtime.contestants[0].time_ms, 0);
	assert.equal(fixture.runtime.contestants[1].has_rung, true);
	await socket.controller.clearBells();
	assert.equal(fixture.runtime.contestants[1].has_rung, false);
});
