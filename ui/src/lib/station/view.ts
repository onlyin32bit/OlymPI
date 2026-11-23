import type { MatchRuntimeState, RuntimeContestant } from '$lib/socket/opsocket';
import { canRingBell } from '$lib/control/bell-rules';

export type BellState = 'closed' | 'open' | 'rung' | 'eliminated';
export type BellView = { state: BellState; label: string };

export type MainView =
	| { kind: 'status'; title: string; detail?: string }
	| { kind: 'standings'; title: string }
	| { kind: 'round'; title: string; rule: boolean; ended: boolean }
	| { kind: 'question'; round: string; turn: string; ownTurn: boolean; label: string }
	| { kind: 'vcnv' }
	| { kind: 'review'; timed: boolean }
	| { kind: 've_dich'; ownTurn: boolean; choosing: boolean };

export type QuestionView = {
	content: string;
	/** Image asset to show; stations never play audio or video. */
	imageID?: string;
};

const ROUND_TITLES: Record<string, string> = {
	khoi_dong: 'Khởi động',
	vcnv: 'Vượt chướng ngại vật',
	tang_toc: 'Tăng tốc',
	ve_dich: 'Về đích'
};

export function me(runtime: MatchRuntimeState | null | undefined, slot: number): RuntimeContestant | undefined {
	return runtime?.contestants.find((entry) => entry.slot === slot);
}

/** The bell keeps one fixed place; only its state and label change. */
export function bellView(runtime: MatchRuntimeState | null | undefined, slot: number, remainingMS: number): BellView {
	const self = me(runtime, slot);
	if (runtime?.controller.phase === 'vcnv' && self?.bell_locked) return { state: 'eliminated', label: 'Đã bị loại' };
	if (self?.has_rung) return { state: 'rung', label: 'Đã bấm chuông' };
	if (canRingBell(runtime, slot, remainingMS)) return { state: 'open', label: 'Nhấn chuông' };
	return { state: 'closed', label: 'Chuông khóa' };
}

/** Typing is allowed only while a VCNV/Tăng tốc question is on screen and its clock runs. */
export function inputOpen(runtime: MatchRuntimeState | null | undefined, slot: number, remainingMS: number): boolean {
	const controller = runtime?.controller;
	const self = me(runtime, slot);
	return Boolean(
		runtime &&
			controller &&
			self &&
			!self.locked &&
			runtime.status === 'playing' &&
			(controller.phase === 'vcnv' || controller.phase === 'tang_toc') &&
			controller.screen === 'ques' &&
			controller.is_showing_question &&
			runtime.current_question &&
			runtime.timer.is_running &&
			!runtime.timer.is_paused &&
			remainingMS > 0
	);
}

/** Owner of the individual Khởi động set or the Về đích turn on screen; 0 when shared. */
export function turnOwner(runtime: MatchRuntimeState | null | undefined): number {
	const controller = runtime?.controller;
	if (!controller) return 0;
	const screenSlot = Number(/^(?:pre_)?ques_ts([1-4])$/.exec(controller.screen)?.[1] ?? 0);
	if (controller.phase === 'khoi_dong') {
		const rieng = /^kd_rieng\.([0-3])\./.exec(controller.selected_question_id);
		return screenSlot || (rieng ? Number(rieng[1]) + 1 : 0);
	}
	if (controller.phase === 've_dich') {
		const owner = /^vd\.([0-3])\./.exec(controller.selected_question_id);
		return screenSlot || (owner ? Number(owner[1]) + 1 : 0) || controller.selected_contestant;
	}
	return 0;
}

/** The 5-second Về đích steal window. */
export function stealWindow(runtime: MatchRuntimeState | null | undefined): boolean {
	return Boolean(
		runtime?.controller.phase === 've_dich' &&
			runtime.timer.is_running &&
			!runtime.timer.is_paused &&
			runtime.timer.duration_ms === 5000
	);
}

/**
 * The question as the station may show it. Tăng tốc video questions stay on
 * the TV only; elsewhere video and audio questions show their text, never media.
 */
export function questionView(runtime: MatchRuntimeState | null | undefined): QuestionView | null {
	const question = runtime?.current_question;
	const controller = runtime?.controller;
	if (!question || !controller?.is_showing_question || question.id !== controller.selected_question_id) return null;
	if (question.type === 'video' && controller.phase === 'tang_toc') return null;
	return {
		content: question.content,
		imageID: question.type === 'image' && question.asset_id ? question.asset_id : undefined
	};
}

function questionLabel(id: string): string {
	let match = /^kd_(?:chung|rieng\.[0-3])\.(\d+)$/.exec(id);
	if (match) return `Câu ${Number(match[1]) + 1}`;
	match = /^vcnv\.rows\.([0-3])$/.exec(id);
	if (match) return `Hàng ${Number(match[1]) + 1}`;
	if (id === 'vcnv.center') return 'Ô trung tâm';
	match = /^tt\.(\d+)$/.exec(id);
	if (match) return `Câu ${Number(match[1]) + 1}`;
	match = /^vd\.[0-3]\.(?:20|30)\.([0-2])$/.exec(id);
	if (match) return `Câu ${Number(match[1]) + 1}`;
	return '';
}

/** Decides what the main area shows for this station right now. */
export function mainView(runtime: MatchRuntimeState | null | undefined, slot: number): MainView {
	if (!runtime) return { kind: 'status', title: 'Chưa có trận đấu', detail: 'Chờ ban tổ chức chọn trận.' };
	if (!me(runtime, slot)) return { kind: 'status', title: 'Chưa có thí sinh ở vị trí này' };
	if (runtime.status === 'paused') return { kind: 'status', title: 'Trận đấu đang tạm dừng' };
	if (runtime.status !== 'playing') return { kind: 'status', title: 'Chờ bắt đầu trận đấu' };
	const { phase, screen, selected_question_id: selected } = runtime.controller;
	if (phase === 'idle' || phase === 'logo') return { kind: 'status', title: 'Chờ chương trình' };
	if (phase === 'main') return { kind: 'standings', title: 'Thí sinh' };
	if (phase === 'score') return { kind: 'standings', title: 'Bảng điểm' };
	const round = ROUND_TITLES[phase] ?? '';
	if (screen === 'rule' || screen === 'intro' || screen === 'start' || screen === 'end')
		return { kind: 'round', title: round, rule: screen === 'rule', ended: screen === 'end' };
	if (screen === 'answers_tt' || screen === 'answers_vcnv') return { kind: 'review', timed: screen === 'answers_tt' };
	if (phase === 'vcnv') return { kind: 'vcnv' };
	if (phase === 've_dich') {
		if (screen === 'main_vd' || screen === 'end_ts') return { kind: 'round', title: round, rule: false, ended: screen === 'end_ts' };
		return { kind: 've_dich', ownTurn: turnOwner(runtime) === slot, choosing: screen.startsWith('pre_ques') };
	}
	if (phase === 'khoi_dong') {
		if (screen === 'main_kd' || screen === 'end_ts' || screen === 'test_bell')
			return { kind: 'round', title: round, rule: false, ended: screen === 'end_ts' };
		const owner = turnOwner(runtime);
		const shared = screen === 'ques_chung' || selected.startsWith('kd_chung.');
		return { kind: 'question', round, turn: shared ? 'Lượt chung' : 'Lượt riêng', ownTurn: !shared && owner === slot, label: questionLabel(selected) };
	}
	return { kind: 'question', round, turn: '', ownTurn: false, label: questionLabel(selected) };
}

/** Countdown readout: whole seconds while above ten, tenths in the last ten. */
export function formatCountdown(ms: number): string {
	const safe = Math.max(0, ms);
	if (safe >= 10_000) return String(Math.ceil(safe / 1000));
	return (Math.ceil(safe / 100) / 10).toFixed(1);
}
