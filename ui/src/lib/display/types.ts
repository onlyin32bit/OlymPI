import type { MatchRuntimeState, RuntimeContestant, TimerState } from '$lib/socket/opsocket';
import type { Phases, Screens, SiteSettings } from '$lib/types';

export type VCNVView = {
	obstacle?: string;
	obstacle_asset_id: string;
	row_lengths: number[];
	obstacle_length: number;
	row_answers: string[];
};

export type DisplayContext = {
	/** Public (audience) view of the runtime: the operator preview filters it with publicView. */
	match: MatchRuntimeState | null;
	site: SiteSettings;
	phase: Phases;
	screen: Screens;
	vcnv: VCNVView | null;
	/** True in the operator preview: no audio, all media muted. */
	preview: boolean;
	onVeDichVideoFinished?: (questionID: string, sessionID: number) => void | Promise<unknown>;
};
export const VE_DICH_VIDEO_FINAL_FRAME_MS = 2000;

/**
 * Router key: one scene instance per legacy {#if} branch. KĐ ques_ts1..4 share one branch,
 * VĐ pre_ques_ts1..4 share one and VĐ ques_ts1..4 share one; screens inside a key only
 * update the context. Pass the normalised phase/screen (phaseFor/screenFor).
 */
export function branchKey(phase: string, screen: string): string {
	if (phase === 'khoi_dong' && /^ques_ts[1-4]$/.test(screen)) return 'khoi_dong:ques_ts';
	if (phase === 've_dich') {
		const group = /^(pre_ques_ts|ques_ts)[1-4]$/.exec(screen)?.[1];
		if (group) return `ve_dich:${group}`;
	}
	return `${phase}:${screen}`;
}

export function assetURL(id: string): string {
	return `/api/assets/${id}`;
}

type RuntimeController = MatchRuntimeState['controller'];
type QuestionView = NonNullable<MatchRuntimeState['current_question']>;

/** Mirrors controller.AnswerVisible on the server. */
export function answerVisible(controller: RuntimeController): boolean {
	switch (controller.phase) {
		case 'tang_toc':
			return controller.screen === 'solve';
		case 'vcnv':
			return controller.vcnv_state.has_solved;
		case 'khoi_dong':
		case 've_dich':
			return controller.screen === 'end_ts' || controller.screen === 'end';
		default:
			return false;
	}
}

/** Mirrors controller.ContestantAnswersVisible on the server. */
export function contestantAnswersVisible(controller: RuntimeController): boolean {
	return (
		(controller.phase === 'tang_toc' && controller.screen === 'answers_tt') ||
		(controller.phase === 'vcnv' && controller.screen === 'answers_vcnv')
	);
}

/**
 * TS port of the server's filteredState for the public room (game/service.go): the operator
 * preview shows exactly what the TV receives. Bell normalisation stays server-side.
 */
export function publicView(runtime: MatchRuntimeState | null): MatchRuntimeState | null {
	if (!runtime) return null;
	const controller = runtime.controller;
	let question = runtime.current_question;
	if (question) {
		const ttSolve = controller.phase === 'tang_toc' && controller.screen === 'solve';
		if (!controller.is_showing_question && !ttSolve) {
			const stub: QuestionView = { id: question.id, content: '', type: question.type };
			if (question.asset_id) stub.asset_id = question.asset_id;
			if (question.time) stub.time = question.time;
			if (question.char_count) stub.char_count = question.char_count;
			question = stub;
		} else {
			const copy: QuestionView = { ...question };
			if (!answerVisible(controller)) {
				delete copy.answer;
				delete copy.keyword;
			}
			if (!ttSolve) delete copy.explanation_asset_id;
			question = copy;
		}
	}
	const showAnswers = contestantAnswersVisible(controller);
	return {
		...runtime,
		current_question: question,
		contestants: runtime.contestants.map((contestant) =>
			showAnswers ? contestant : { ...contestant, answer: '', time_ms: 0 }
		)
	};
}

/** Value imports are avoided here so Node can load this module directly in tests. */
export const emptySiteSettings: Omit<SiteSettings, 'scoring'> = {
	event_name: '',
	host_name: '',
	sound: {
		enabled: true,
		master_volume: 1,
		music_volume: 0.8,
		effects_volume: 1,
		transition_fade_ms: 120,
		bell_sound_id: ''
	}
};

/** Server timestamps anchor each running segment; duration never changes on resume. */
export function timerRemaining(timer: TimerState | undefined, serverNow: number): number {
	if (!timer) return 0;
	const segment =
		timer.is_running && !timer.is_paused ? Math.max(0, serverNow - timer.started_at) : 0;
	return Math.max(0, timer.remaining_ms - segment);
}

export function timerElapsed(timer: TimerState | undefined, serverNow: number): number {
	if (!timer) return 0;
	const segment =
		timer.is_running && !timer.is_paused ? Math.max(0, serverNow - timer.started_at) : 0;
	return Math.min(timer.duration_ms, Math.max(0, timer.elapsed_ms + segment));
}

/** Audience indicators fill with elapsed time; the operator clock counts down separately. */
export function timerProgress(timer: TimerState | undefined, serverNow: number): number {
	if (!timer || timer.duration_ms <= 0) return 0;
	return timerElapsed(timer, serverNow) / timer.duration_ms;
}

/** The endpoint redacts answers according to the current reveal state, not only the revision. */
export function vcnvRevealKey(runtime: MatchRuntimeState | null): string {
	if (!runtime?.question_revision_id) return '';
	const state = runtime.controller.vcnv_state;
	return JSON.stringify([
		runtime.match_id,
		runtime.epoch,
		runtime.question_revision_id,
		state.has_solved,
		state.row_states.map((value) => value === 'c')
	]);
}

export function reviewContestants(
	runtime: MatchRuntimeState | null,
	timed: boolean
): RuntimeContestant[] {
	const contestants: RuntimeContestant[] = Array.from(
		{ length: 4 },
		(_, index) =>
			runtime?.contestants.find((value) => value.slot === index + 1) ?? {
				slot: index + 1,
				name: '',
				class: '',
				score: 0,
				answer: '',
				time_ms: 0,
				locked: false,
				bell_locked: false,
				has_rung: false
			}
	);
	return contestants.sort(
		(a, b) =>
			Number(Boolean(b.answer)) - Number(Boolean(a.answer)) ||
			(timed && a.answer && b.answer ? a.time_ms - b.time_ms : 0) ||
			a.slot - b.slot
	);
}
