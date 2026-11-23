import type { Controller } from '$lib/types';
import type { MatchRuntimeState } from '../opsocket';
import type { MatchSocket } from '../match/index.svelte';
import type { SocketTransport } from '../transport';

const emptyController: Controller = {
	phase: 'idle',
	screen: 'intro',
	selected_question_id: '',
	selected_contestant: 0,
	is_allowing_bell: false,
	is_showing_question: false,
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
		session_id: 0,
		status: 'idle',
		volume: 1,
		started_at: 0,
		position_ms: 0,
		duration_ms: 0
	}
};

export class ControllerSocket {
	constructor(
		private readonly transport: SocketTransport,
		private readonly match: MatchSocket
	) {}

	get current(): Controller {
		return (this.match.current?.controller as Controller | undefined) ?? emptyController;
	}

	update(state: Controller) {
		return this.apply(
			this.transport.request('Controller.Update', { command: this.match.command(), state })
		);
	}

	changePhase(value: Controller['phase']) {
		return this.apply(
			this.transport.request('Controller.ChangePhase', { command: this.match.command(), value })
		);
	}

	changeScreen(value: Controller['screen']) {
		return this.apply(
			this.transport.request('Controller.ChangeScreen', { command: this.match.command(), value })
		);
	}

	selectQuestion(value: string) {
		return this.apply(
			this.transport.request('Controller.SelectQuestion', { command: this.match.command(), value })
		);
	}

	selectContestant(value: number) {
		return this.apply(
			this.transport.request('Controller.SelectContestant', {
				command: this.match.command(),
				value
			})
		);
	}

	toggleBell(value: boolean) {
		return this.apply(
			this.transport.request('Controller.ToggleBell', { command: this.match.command(), value })
		);
	}

	toggleQuestion(value: boolean) {
		return this.apply(
			this.transport.request('Controller.ToggleQuestion', { command: this.match.command(), value })
		);
	}

	setAnswerStatus(slot: number, correct: boolean) {
		return this.apply(
			this.transport.request('Controller.SetAnswerStatus', {
				command: this.match.command(),
				slot,
				correct
			})
		);
	}

	setVCNVStarted(value: boolean) {
		return this.apply(
			this.transport.request('Controller.SetVCNVStarted', { command: this.match.command(), value })
		);
	}

	setVCNVRowState(row: number, state: 'u' | 's' | 'c' | 'w') {
		return this.apply(
			this.transport.request('Controller.SetVCNVRowState', {
				command: this.match.command(),
				row,
				state
			})
		);
	}

	setVCNVPanelOpen(index: number, open: boolean) {
		return this.apply(
			this.transport.request('Controller.SetVCNVPanelOpen', {
				command: this.match.command(),
				index,
				open
			})
		);
	}

	setVCNVSolved(value: boolean) {
		return this.apply(
			this.transport.request('Controller.SetVCNVSolved', { command: this.match.command(), value })
		);
	}

	selectVeDichQuestionSet(
		slot: number,
		questionIDs: Controller['ve_dich_contestants_state'][number]['selected_question_set']
	) {
		return this.apply(
			this.transport.request('Controller.SelectVeDichQuestionSet', {
				command: this.match.command(),
				slot,
				question_ids: questionIDs
			})
		);
	}

	setVeDichStar(slot: number, question: number) {
		return this.apply(
			this.transport.request('Controller.SetVeDichStar', {
				command: this.match.command(),
				slot,
				question
			})
		);
	}

	/** `duration` reports the media length (ms) so the server can end the video on its own clock. */
	veDichVideo(
		action: 'start' | 'pause' | 'resume' | 'stop' | 'volume' | 'duration' | 'finish',
		options: {
			question_id?: string;
			session_id?: number;
			volume?: number;
			duration_ms?: number;
		} = {}
	) {
		return this.apply(
			this.transport.request('Controller.VeDichVideo', {
				command: this.match.command(),
				action,
				...options
			})
		);
	}

	/** Lets every contestant ring again (Thử chuông and any phase). */
	clearBells() {
		return this.apply(this.transport.request('Controller.ClearBells', this.match.command()));
	}

	/** Clears every contestant's typed answer and answer time without touching bells. */
	clearAnswers() {
		return this.apply(this.transport.request('Controller.ClearAnswers', this.match.command()));
	}

	reset() {
		return this.apply(this.transport.request('Controller.Reset', this.match.command()));
	}

	private async apply(request: Promise<MatchRuntimeState>): Promise<MatchRuntimeState> {
		return this.match.accept(await request);
	}
}
