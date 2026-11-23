import { gameController } from '$lib/control/game-controller';
import { socket } from '$lib/socket';
import type { MatchRuntimeState } from '$lib/socket/opsocket';
import type { Phases, Screens } from '$lib/types';

type ControllerState = MatchRuntimeState['controller'];
type Controller = Pick<
	ControllerState,
	'phase' | 'screen' | 'selected_question_id' | 'is_showing_question'
>;

const KHOI_DONG_QUESTION_SCREEN = /^(?:ques_chung|ques_ts[1-4])$/;

/** Khởi động question screens (chung + per-contestant turns). */
export function isKhoiDongQuestionScreen(screen: string): boolean {
	return KHOI_DONG_QUESTION_SCREEN.test(screen);
}

/** The KĐ "KẾT THÚC" card: a KĐ question screen showing with no question selected. */
export function isKhoiDongEnd(controller: Controller | null | undefined): boolean {
	return Boolean(
		controller &&
			controller.phase === 'khoi_dong' &&
			isKhoiDongQuestionScreen(controller.screen) &&
			!controller.selected_question_id &&
			controller.is_showing_question
	);
}

/**
 * Opening a question shows only its frame; the operator reveals the text with Hiện câu hỏi.
 * Exception (legacy): within a Khởi động turn, once a question was showing, the following
 * questions on the same screen show straight away (the display blanks briefly between them).
 */
export function autoShowsOnOpen(
	controller: Controller | null | undefined,
	screen: Screens,
	questionID: string
): boolean {
	return Boolean(
		controller &&
			controller.phase === 'khoi_dong' &&
			controller.screen === screen &&
			controller.is_showing_question &&
			controller.selected_question_id !== '' &&
			controller.selected_question_id !== questionID
	);
}

/** Open a question on its actual audience scene; the operator still starts the clock. */
export async function openLiveQuestion(phase: Phases, questionID: string): Promise<void> {
	const initial = socket.match.current;
	if (!initial?.question_revision_id || initial.controller.phase !== phase)
		throw new Error('Chưa có đề thi hoặc vòng thi đã thay đổi');
	const matchID = initial.match_id;
	const epoch = initial.epoch;
	const revisionID = initial.question_revision_id;
	let inputEpoch = initial.input_epoch;

	let screen: Screens;
	let slot = 0;
	let row = -1;
	const individual = /^kd_rieng\.([0-3])\.\d+$/.exec(questionID);
	const obstacle = /^vcnv\.rows\.([0-3])$/.exec(questionID);
	const finish = /^vd\.([0-3])\.(20|30)\.([0-2])$/.exec(questionID);
	if (phase === 'khoi_dong' && /^kd_chung\.\d+$/.test(questionID)) {
		screen = 'ques_chung';
	} else if (phase === 'khoi_dong' && individual) {
		slot = Number(individual[1]) + 1;
		screen = `ques_ts${slot}` as Screens;
	} else if (phase === 'vcnv' && (obstacle || questionID === 'vcnv.center')) {
		row = obstacle ? Number(obstacle[1]) : -1;
		screen = 'ques';
	} else if (phase === 'tang_toc' && /^tt\.\d+$/.test(questionID)) {
		screen = 'ques';
	} else if (phase === 've_dich' && finish) {
		slot = Number(finish[1]) + 1;
		const choice = `${finish[2]}_${Number(finish[3]) + 1}`;
		if (
			!initial.controller.ve_dich_contestants_state[slot - 1]?.selected_question_set.includes(
				choice
			)
		)
			throw new Error('Câu hỏi không thuộc gói đã chọn');
		screen = `ques_ts${slot}` as Screens;
	} else {
		throw new Error('Câu hỏi không thuộc vòng thi hiện tại');
	}
	const autoShow = autoShowsOnOpen(initial.controller, screen, questionID);

	const verifyContext = () => {
		const latest = socket.match.current;
		if (
			latest?.match_id !== matchID ||
			latest.epoch !== epoch ||
			latest.question_revision_id !== revisionID ||
			latest.controller.phase !== phase ||
			latest.input_epoch !== inputEpoch
		)
			throw new DOMException('Trận hoặc vòng thi đã thay đổi', 'AbortError');
	};
	if (slot && socket.controller.current.selected_contestant !== slot) {
		await socket.controller.selectContestant(slot);
		verifyContext();
	}
	if (row >= 0) {
		for (let index = 0; index < 4; index++) {
			if (index !== row && socket.controller.current.vcnv_state.row_states[index] === 's') {
				await socket.controller.setVCNVRowState(index + 1, 'u');
				verifyContext();
			}
		}
		if (socket.controller.current.vcnv_state.row_states[row] !== 's') {
			await socket.controller.setVCNVRowState(row + 1, 's');
			verifyContext();
		}
	}
	await gameController.showScreen(phase, screen);
	verifyContext();
	if (socket.controller.current.selected_question_id !== questionID) {
		await socket.controller.selectQuestion(questionID);
		inputEpoch = socket.match.current?.input_epoch ?? inputEpoch;
		verifyContext();
	}
	if (
		socket.controller.current.screen !== screen ||
		socket.controller.current.selected_question_id !== questionID
	)
		throw new DOMException('Câu hỏi đang điều khiển đã thay đổi', 'AbortError');
	if (autoShow && !socket.controller.current.is_showing_question)
		await socket.controller.toggleQuestion(true);
}

/**
 * What "next question" does: open the following question of the strip, or — in Khởi động
 * after the turn's last question — show the KẾT THÚC card. Nothing follows the card.
 */
export function nextQuestionStep(
	controller: Controller | null | undefined,
	strip: readonly string[]
): { kind: 'open'; id: string } | { kind: 'end' } | null {
	if (!controller || !strip.length || isKhoiDongEnd(controller)) return null;
	const index = strip.indexOf(controller.selected_question_id);
	if (index < 0) return { kind: 'open', id: strip[0] };
	if (index < strip.length - 1) return { kind: 'open', id: strip[index + 1] };
	return controller.phase === 'khoi_dong' && isKhoiDongQuestionScreen(controller.screen)
		? { kind: 'end' }
		: null;
}

/** Khởi động: after the last question of a turn, the same screen shows the KẾT THÚC card. */
export async function showKhoiDongEnd(): Promise<void> {
	const initial = socket.controller.current;
	if (initial.phase !== 'khoi_dong' || !isKhoiDongQuestionScreen(initial.screen))
		throw new Error('Chỉ kết thúc lượt trên màn hình câu hỏi Khởi động');
	const screen = initial.screen;
	if (initial.selected_question_id) await socket.controller.selectQuestion('');
	const latest = socket.controller.current;
	if (latest.phase !== 'khoi_dong' || latest.screen !== screen || latest.selected_question_id)
		throw new DOMException('Màn hình đã thay đổi', 'AbortError');
	if (!latest.is_showing_question) await socket.controller.toggleQuestion(true);
}

type VCNVController = Pick<ControllerState, 'phase' | 'screen' | 'vcnv_state'>;

/** What the VCNV board button does next: show the empty board, then start it. */
export function vcnvBoardStep(controller: VCNVController | null | undefined): 'show' | 'start' | null {
	if (!controller || controller.phase !== 'vcnv') return null;
	if (controller.screen !== 'main_vcnv') return 'show';
	return controller.vcnv_state.has_started ? null : 'start';
}

export async function pressVCNVBoard(): Promise<void> {
	const step = vcnvBoardStep(socket.controller.current);
	if (step === 'show') await gameController.showScreen('vcnv', 'main_vcnv');
	else if (step === 'start') await socket.controller.setVCNVStarted(true);
}

/** What a VCNV row's Mở does next: choose the row on the board, then open its question. */
export function vcnvRowStep(
	controller: VCNVController | null | undefined,
	row: number
): 'select' | 'open' | null {
	if (!controller || controller.phase !== 'vcnv' || row < 0 || row > 3) return null;
	return controller.vcnv_state.row_states[row] === 's' ? 'open' : 'select';
}

export async function pressVCNVRow(row: number): Promise<void> {
	const step = vcnvRowStep(socket.controller.current, row);
	if (step === 'open') return openLiveQuestion('vcnv', `vcnv.rows.${row}`);
	if (step !== 'select') return;
	await gameController.showScreen('vcnv', 'main_vcnv');
	if (socket.controller.current.screen !== 'main_vcnv' || socket.controller.current.phase !== 'vcnv')
		throw new DOMException('Màn hình đã thay đổi', 'AbortError');
	for (let index = 0; index < 4; index++)
		if (index !== row && socket.controller.current.vcnv_state.row_states[index] === 's')
			await socket.controller.setVCNVRowState(index + 1, 'u');
	if (socket.controller.current.vcnv_state.row_states[row] !== 's')
		await socket.controller.setVCNVRowState(row + 1, 's');
}

/** The VCNV obstacle (15 s) timer runs on the board or picture while nothing else is timing. */
export function canStartObstacleTimer(runtime: MatchRuntimeState | null | undefined): boolean {
	const controller = runtime?.controller;
	return Boolean(
		runtime &&
			controller &&
			runtime.status === 'playing' &&
			controller.phase === 'vcnv' &&
			(controller.screen === 'main_vcnv' || controller.screen === 'image_vcnv') &&
			!runtime.timer.is_running &&
			!runtime.timer.is_paused
	);
}

const QUESTION_SCREEN: Record<string, RegExp> = {
	khoi_dong: /^(?:kd_chung\.\d+|kd_rieng\.[0-3]\.\d+)$/,
	vcnv: /^(?:vcnv\.rows\.[0-3]|vcnv\.center)$/,
	ve_dich: /^vd\.[0-3]\.(?:20|30)\.[0-2]$/
};

function questionOnItsScreen(controller: ControllerState, id: string): boolean {
	if (!QUESTION_SCREEN[controller.phase]?.test(id)) return false;
	switch (controller.phase) {
		case 'khoi_dong': {
			const rieng = /^kd_rieng\.([0-3])\./.exec(id);
			return controller.screen === (rieng ? `ques_ts${Number(rieng[1]) + 1}` : 'ques_chung');
		}
		case 'vcnv':
			return controller.screen === 'ques';
		case 've_dich':
			return controller.screen === `ques_ts${Number(id.split('.')[1]) + 1}`;
		default:
			return false;
	}
}

/**
 * Phát media: an audio question (any round but Tăng tốc) plays through the TV sound output;
 * a Về đích video plays as the full-screen takeover. Text/image questions and Tăng tốc
 * (inline muted video started by the timer) have nothing to play.
 */
export function questionMediaKind(
	runtime: MatchRuntimeState | null | undefined
): 'audio' | 'video' | null {
	const controller = runtime?.controller;
	const question = runtime?.current_question;
	if (!controller || !question || !question.asset_id) return null;
	if (question.id !== controller.selected_question_id) return null;
	if (!questionOnItsScreen(controller, question.id)) return null;
	if (question.type === 'audio') return 'audio';
	if (question.type === 'video' && controller.phase === 've_dich') return 'video';
	return null;
}
