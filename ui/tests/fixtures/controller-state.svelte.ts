import type { MatchRuntimeState } from '../../src/lib/socket/opsocket';
import type { Question, QuestionSetData, Phases, Screens } from '../../src/lib/types';
import { questionDurationMS } from '../../src/lib/control/question-timing';
import { bellWindowOpen, canRingBell, canStartSteal } from '../../src/lib/control/bell-rules';

const question = (content: string, video = false): Question => ({
	content,
	answer: 'Đáp án kiểm thử',
	time: 30,
	type: video ? 'video' : 'text',
	asset_id: video ? 'fixture-video' : ''
});
export const questionData: QuestionSetData = {
	kd_chung: Array.from({ length: 12 }, (_, i) =>
		question(`Câu hỏi chung ${i + 1}: Nội dung câu hỏi dành cho người điều khiển chương trình.`)
	),
	kd_rieng: Array.from({ length: 4 }, (_, slot) =>
		Array.from({ length: 6 }, (_, i) => question(`Câu ${i + 1}, thí sinh ${slot + 1}`))
	),
	vcnv: {
		obstacle: 'CHƯỚNG NGẠI VẬT',
		obstacle_asset_id: '',
		rows: Array.from({ length: 4 }, (_, i) => question(`Hàng ngang ${i + 1}`)),
		center: question('Câu hỏi trung tâm')
	},
	tt: Array.from({ length: 4 }, (_, i) => question(`Câu Tăng tốc ${i + 1}`)),
	vd: Array.from({ length: 4 }, (_, slot) => ({
		'20': Array.from({ length: 3 }, (_, i) =>
			question(`Hãy trả lời câu hỏi sau bằng tiếng Anh. Thí sinh ${slot + 1}, câu ${i + 1}.`, true)
		),
		'30': Array.from({ length: 3 }, (_, i) =>
			question(`Về đích 30 điểm, thí sinh ${slot + 1}, câu ${i + 1}`)
		)
	}))
};

function initial(phase: Phases, screen: Screens): MatchRuntimeState {
	return {
		match_id: 'controller-fixture',
		match_name: 'Kiểm thử điều khiển',
		stage: 'Bán kết 2',
		season: 9,
		epoch: 1,
		version: 1,
		input_epoch: 1,
		bell_epoch: 1,
		status: 'playing',
		question_revision_id: 'fixture-revision',
		question_set_id: 'fixture-set',
		updated_at: '2026-01-01T00:00:00Z',
		controller: {
			phase,
			screen,
			selected_question_id: '',
			selected_contestant: 0,
			is_showing_question: false,
			is_allowing_bell: phase === 'vcnv',
			answer_status: [true, true, true, true],
			vcnv_state: {
				has_started: false,
				has_solved: false,
				row_states: ['u', 'u', 'u', 'u'],
				image_open_state: [false, false, false, false, false]
			},
			ve_dich_contestants_state: Array.from({ length: 4 }, () => ({
				selected_question_set: [],
				starred_on: 0
			})),
			ve_dich_video: {
				status: 'idle',
				volume: 1,
				session_id: 0,
				started_at: 0,
				position_ms: 0,
				duration_ms: 0
			}
		},
		contestants: ['Minh Tuấn', 'Khả Vinh', 'Hoàng Đức', 'Gia Hân'].map((name, index) => ({
			slot: index + 1,
			name,
			class: '12A1',
			score: index * 20,
			answer: '',
			time_ms: 0,
			locked: false,
			bell_locked: false,
			has_rung: false
		})),
		timer: {
			duration_ms: 30000,
			remaining_ms: 30000,
			elapsed_ms: 0,
			started_at: 0,
			is_running: false,
			is_paused: false
		},
		current_question: null
	};
}
let current = $state(initial('khoi_dong', 'rule'));
const fixtureUser = $state({ fixed_slot: 1 });
let calls: unknown[][] = [];
function record(command: string, ...values: unknown[]) {
	calls.push([command, ...values]);
}
function syncBell() {
	const open = bellWindowOpen(current, current.timer.remaining_ms);
	if (open && !current.controller.is_allowing_bell) current.bell_epoch++;
	current.controller.is_allowing_bell = open;
}
function questionFor(id: string): Question {
	const parts = id.split('.');
	if (parts[0] === 'kd_chung') return questionData.kd_chung[Number(parts[1])];
	if (parts[0] === 'kd_rieng') return questionData.kd_rieng[Number(parts[1])][Number(parts[2])];
	if (parts[0] === 'tt') return questionData.tt[Number(parts[1])];
	if (parts[0] === 'vcnv')
		return parts[1] === 'center'
			? questionData.vcnv.center
			: questionData.vcnv.rows[Number(parts[2])];
	return questionData.vd[Number(parts[1])][parts[2] as '20' | '30'][Number(parts[3])];
}
export const fixture = {
	reset(phase: Phases = 'khoi_dong', screen: Screens = 'rule') {
		current = initial(phase, screen);
		calls = [];
	},
	get calls() {
		return calls;
	},
	get runtime() {
		return current;
	},
	syncBell,
	setScreen(screen: Screens) {
		current.controller.screen = screen;
		syncBell();
	},
	setQuestion(id: string) {
		current.controller.selected_question_id = id;
		current.current_question = id
			? ({ ...questionFor(id), id } as MatchRuntimeState['current_question'])
			: null;
		syncBell();
	},
	setTimer(remaining: number) {
		current.timer.remaining_ms = remaining;
		syncBell();
	}
};
export const socket = {
	presence: { isOnline: () => false },
	get serverNow() {
		return Date.now();
	},
	match: {
		get current() {
			return current;
		},
		async setContestantLock(slot: number, locked: boolean) {
			record('lock', slot, locked);
			current.contestants[slot - 1].locked = locked;
			current.contestants[slot - 1].bell_locked = locked;
			if (!locked && current.controller.phase === 'vcnv')
				current.contestants[slot - 1].has_rung = false;
			syncBell();
		}
	},
	controller: {
		get current() {
			return current.controller;
		},
		async changePhase(phase: Phases) {
			record('phase', phase);
			current.input_epoch++;
			current.bell_epoch++;
			current.current_question = null;
			current.controller.selected_question_id = '';
			current.controller.is_showing_question = false;
			current.timer = {
				duration_ms: 0,
				remaining_ms: 0,
				elapsed_ms: 0,
				started_at: 0,
				is_running: false,
				is_paused: false
			};
			for (const contestant of current.contestants) {
				contestant.answer = '';
				contestant.time_ms = 0;
				contestant.locked = false;
				contestant.bell_locked = false;
				contestant.has_rung = false;
			}
			current.controller.phase = phase;
			current.controller.screen =
				phase === 'idle'
					? 'intro'
					: phase === 'main'
						? 'main'
						: phase === 'score'
							? 'score'
							: 'start';
			syncBell();
		},
		async changeScreen(screen: Screens) {
			record('screen', screen);
			current.controller.screen = screen;
			syncBell();
		},
		async selectContestant(slot: number) {
			record('contestant', slot);
			current.controller.selected_contestant = slot;
			syncBell();
		},
		async selectQuestion(id: string) {
			record('question', id);
			if (id !== current.controller.selected_question_id) {
				current.input_epoch++;
				current.controller.is_showing_question = false;
				if (current.controller.phase !== 'vcnv') current.bell_epoch++;
				for (const contestant of current.contestants) {
					contestant.answer = '';
					contestant.time_ms = 0;
					contestant.locked = current.controller.phase === 'vcnv' && contestant.bell_locked;
					if (current.controller.phase !== 'vcnv') {
						contestant.has_rung = false;
						contestant.bell_locked = false;
					}
				}
				current.controller.answer_status = [true, true, true, true];
				current.timer = {
					duration_ms: 0,
					remaining_ms: 0,
					elapsed_ms: 0,
					started_at: 0,
					is_running: false,
					is_paused: false
				};
			}
			fixture.setQuestion(id);
		},
		async toggleQuestion(show: boolean) {
			record('visibility', show);
			current.controller.is_showing_question = show;
			syncBell();
		},
		async clearBells() {
			record('clear-bells');
			for (const contestant of current.contestants) {
				contestant.has_rung = false;
				contestant.bell_locked = false;
			}
			current.bell_epoch++;
			syncBell();
		},
		async clearAnswers() {
			record('clear-answers');
			for (const contestant of current.contestants) {
				if (!contestant.has_rung || contestant.answer) contestant.time_ms = 0;
				contestant.answer = '';
			}
			current.controller.answer_status = [true, true, true, true];
			current.input_epoch++;
		},
		async toggleBell(open: boolean) {
			record('bell', open);
			syncBell();
		},
		async setAnswerStatus(slot: number, correct: boolean) {
			record('correctness', slot, correct);
			current.controller.answer_status[slot - 1] = correct;
		},
		async setVCNVRowState(row: number, state: string) {
			record('row', row, state);
			current.controller.vcnv_state.row_states[row - 1] = state;
		},
		async setVCNVPanelOpen(panel: number, open: boolean) {
			record('panel', panel, open);
			current.controller.vcnv_state.image_open_state[panel] = open;
		},
		async setVCNVStarted(started: boolean) {
			record('board', started);
			current.controller.vcnv_state.has_started = started;
		},
		async setVCNVSolved(solved: boolean) {
			record('solved', solved);
			current.controller.vcnv_state.has_solved = solved;
		},
		async selectVeDichQuestionSet(slot: number, choices: string[]) {
			record('package', slot, choices);
			current.controller.ve_dich_contestants_state[slot - 1].selected_question_set = choices;
		},
		async setVeDichStar(slot: number, question: number) {
			record('star', slot, question);
			current.controller.ve_dich_contestants_state[slot - 1].starred_on = question;
		},
		async veDichVideo(action: string, options: { volume?: number } = {}) {
			record('video', action, options);
			const video = current.controller.ve_dich_video;
			video.question_id = current.controller.selected_question_id;
			if (action === 'volume') video.volume = options.volume ?? 1;
			else video.status = action === 'pause' ? 'paused' : action === 'stop' ? 'idle' : 'playing';
		}
	},
	contestant: {
		async submitAnswer(answer: string) {
			const contestant = current.contestants[fixtureUser.fixed_slot - 1];
			record('answer', answer);
			contestant.answer = answer;
			contestant.locked = true;
			contestant.time_ms = current.timer.elapsed_ms;
			return current;
		},
		async ring() {
			const slot = fixtureUser.fixed_slot;
			if (!canRingBell(current, slot, current.timer.remaining_ms)) throw new Error('Bell closed');
			record('ring', slot);
			const contestant = current.contestants[slot - 1];
			contestant.has_rung = true;
			if (!contestant.answer) contestant.time_ms = current.timer.elapsed_ms;
			syncBell();
			return current;
		}
	},
	timer: {
		get remaining() {
			return current.timer.remaining_ms;
		},
		get elapsed() {
			return current.timer.elapsed_ms;
		},
		async start() {
			const duration = questionDurationMS(current);
			if (!duration) throw new Error('Select a timed question first');
			record('timer-start');
			current.input_epoch++;
			current.timer = {
				duration_ms: duration,
				remaining_ms: duration,
				elapsed_ms: 0,
				started_at: Date.now(),
				is_running: true,
				is_paused: false
			};
			syncBell();
		},
		async startObstacle() {
			const screen = current.controller.screen;
			if (current.controller.phase !== 'vcnv' || (screen !== 'main_vcnv' && screen !== 'image_vcnv'))
				throw new Error('Obstacle timer runs on the board');
			record('timer-obstacle');
			current.timer = {
				duration_ms: 15000,
				remaining_ms: 15000,
				elapsed_ms: 0,
				started_at: Date.now(),
				is_running: true,
				is_paused: false,
				kind: 'obstacle'
			};
		},
		async startSteal() {
			if (!canStartSteal(current)) throw new Error('Steal unavailable');
			record('timer-steal');
			current.input_epoch++;
			current.bell_epoch++;
			current.timer = {
				duration_ms: 5000,
				remaining_ms: 5000,
				elapsed_ms: 0,
				started_at: Date.now(),
				is_running: true,
				is_paused: false
			};
			syncBell();
		},
		async pause() {
			record('timer-pause');
			current.timer.is_running = false;
			current.timer.is_paused = true;
			syncBell();
		},
		async resume() {
			record('timer-resume');
			current.timer.is_running = true;
			current.timer.is_paused = false;
			syncBell();
		},
		async reset() {
			record('timer-reset');
			current.input_epoch++;
			current.timer = {
				duration_ms: 0,
				remaining_ms: 0,
				elapsed_ms: 0,
				started_at: 0,
				is_running: false,
				is_paused: false
			};
			syncBell();
		}
	},
	sound: {
		async play(id: string) {
			record('sound', id);
		},
		async playQuestionMedia() {
			record('question-media');
		}
	}
};
export const api = {
	auth: { user: fixtureUser },
	settings: { current: null },
	public: {
		async vcnv() {
			return {
				obstacle_asset_id: '',
				row_lengths: [1, 1, 1, 1],
				obstacle_length: questionData.vcnv.obstacle.length,
				row_answers: ['', '', '', '']
			};
		}
	},
	questionSets: {
		async revision() {
			return { data: questionData };
		}
	}
};
