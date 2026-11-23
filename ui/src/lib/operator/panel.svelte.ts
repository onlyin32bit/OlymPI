import { api } from '$lib/api';
import { canStartSteal } from '$lib/control/bell-rules';
import { gameController } from '$lib/control/game-controller';
import { questionDurationMS } from '$lib/control/question-timing';
import {
	canStartObstacleTimer,
	isKhoiDongEnd,
	nextQuestionStep,
	openLiveQuestion,
	pressVCNVBoard,
	pressVCNVRow,
	questionMediaKind,
	showKhoiDongEnd
} from '$lib/control/question-workflow';
import { phaseFor } from '$lib/display/phase-manifest';
import { socket } from '$lib/socket';
import type { MatchRuntimeState } from '$lib/socket/opsocket';
import type { Phases } from '$lib/types';
import { OBSTACLE_KEY, savedQuestion } from './drafts';
import { fastVerdictState, soundOnly } from './fast-verdict';
import { flattenQuestions, roundOptions, type QuestionOption } from './questions';
import { ScoreDrafts } from './score-drafts.svelte';
import { questionInfo, type Slot, type Verdict } from './scoring';
import { screenTabs, tabTarget, type ScreenTab } from './screens';

export type SlotView = 'reader' | 'log';

const askedKey = (matchID: string) => `4t.cp-next.asked.${matchID}`;

export class OperatorPanel {
	readonly drafts = new ScoreDrafts();
	options = $state<QuestionOption[]>([]);
	/** Chướng ngại vật keyword, operator-only. */
	obstacle = $state('');
	asked = $state<string[]>([]);
	/** Slots in the order their bells were seen; cleared when the bells reset. */
	ringOrder = $state<number[]>([]);
	view = $state<SlotView>('reader');
	busy = $state(false);
	error = $state('');
	connected = $state(true);
	#loadedRevision = $state('');
	#loadingRevision = '';
	#matchID = '';
	#errorTimer: ReturnType<typeof setTimeout> | undefined;
	#lastFastPress = -Infinity;

	runtime = $derived(socket.match.current);
	controller = $derived(socket.controller.current);
	phase = $derived(phaseFor(this.controller.phase));
	tabs = $derived(screenTabs(this.phase.screens));
	liveTab = $derived(this.tabs.find((tab) => tab.screens.includes(this.controller.screen)));
	strip = $derived(
		this.#loadedRevision && this.#loadedRevision === this.runtime?.question_revision_id
			? roundOptions(this.options, this.runtime)
			: []
	);
	selectedIndex = $derived(this.strip.findIndex((option) => option.id === this.controller.selected_question_id));
	selectedOption = $derived(this.options.find((option) => option.id === this.controller.selected_question_id));
	/** Khởi động KẾT THÚC card is on air: the turn is over, nothing comes next in it. */
	turnEnded = $derived(isKhoiDongEnd(this.controller));
	nextStep = $derived(nextQuestionStep(this.controller, this.strip.map((option) => option.id)));
	nextOption = $derived.by(() => {
		const step = this.nextStep;
		return step?.kind === 'open' ? this.strip.find((option) => option.id === step.id) : undefined;
	});
	/** Khởi động: "next" after the last question of the turn shows the KẾT THÚC card. */
	nextEndsTurn = $derived(this.nextStep?.kind === 'end');
	canStepNext = $derived(this.nextStep !== null);
	mediaKind = $derived(questionMediaKind(this.runtime));
	canStartObstacle = $derived(canStartObstacleTimer(this.runtime));
	screenIndex = $derived(this.phase.screens.findIndex((screen) => screen.id === this.controller.screen));
	nextScreen = $derived(this.phase.screens[this.screenIndex + 1]);
	presetDuration = $derived(questionDurationMS(this.runtime));
	/** The clock already ran out for this question; restarting would wipe the answers. */
	timerSpent = $derived(
		Boolean(this.runtime && !this.runtime.timer.is_running && !this.runtime.timer.is_paused && this.runtime.timer.duration_ms > 0)
	);
	canSteal = $derived(canStartSteal(this.runtime));
	playing = $derived(this.runtime?.status === 'playing');
	fastVerdict = $derived(fastVerdictState(
		this.runtime,
		api.settings.current?.scoring,
		this.connected && socket.connected,
		this.busy || this.drafts.saving,
		Boolean(this.runtime?.controller.selected_question_id && this.drafts.batch(this.runtime.controller.selected_question_id)),
		savedQuestion(this.drafts.state, this.runtime?.controller.selected_question_id ?? '')
	));

	judgeKhoiDong(verdict: Verdict): void {
		const snapshot = this.runtime;
		const state = this.fastVerdict;
		if (!snapshot || !(verdict === 'correct' ? state.canCorrect : state.canWrong)) return;
		const now = performance.now();
		if (now - this.#lastFastPress < 500) return;
		this.#lastFastPress = now;
		let warning = '';
		void this.run(async () => {
			const result = await socket.match.judgeKhoiDong(snapshot, verdict);
			warning = result.sound_warning ?? '';
		}).then(() => {
			if (warning) this.fail(new Error(warning));
		});
	}

	discardCurrentDraft(): void {
		const id = this.runtime?.controller.selected_question_id;
		if (id && !this.busy && !this.drafts.saving) this.drafts.discardQuestion(id);
	}

	playVerdictSound(verdict: Verdict): void {
		const id = soundOnly(this.controller.phase, this.controller.screen, verdict);
		if (id) void socket.sound.play(id).catch((cause) => this.fail(cause));
	}

	/** Subscribes to the live match; returns the cleanup. */
	attach(): () => void {
		const unsubscribe = socket.match.onUpdate((runtime) => this.#sync(runtime));
		const poll = window.setInterval(() => (this.connected = socket.connected), 1000);
		const refreshSound = () => void socket.sound.getStatus().catch(() => undefined);
		refreshSound();
		const soundPoll = window.setInterval(refreshSound, 5000);
		return () => {
			unsubscribe();
			window.clearInterval(poll);
			window.clearInterval(soundPoll);
			clearTimeout(this.#errorTimer);
		};
	}

	#sync(runtime: MatchRuntimeState | null): void {
		this.drafts.sync(runtime);
		const matchID = runtime?.match_id ?? '';
		if (matchID !== this.#matchID) {
			this.#matchID = matchID;
			this.ringOrder = [];
			this.asked = matchID ? JSON.parse(sessionStorage.getItem(askedKey(matchID)) ?? '[]') : [];
		}
		this.#trackRings(runtime);
		this.#trackAsked(runtime);
		this.#loadQuestions(runtime?.question_revision_id ?? '');
	}

	#trackRings(runtime: MatchRuntimeState | null): void {
		const rung = (runtime?.contestants ?? [])
			.filter((entry) => entry.has_rung)
			.sort((a, b) => a.time_ms - b.time_ms)
			.map((entry) => entry.slot);
		const kept = this.ringOrder.filter((slot) => rung.includes(slot));
		const added = rung.filter((slot) => !kept.includes(slot));
		if (added.length || kept.length !== this.ringOrder.length) this.ringOrder = [...kept, ...added];
	}

	#trackAsked(runtime: MatchRuntimeState | null): void {
		const id = runtime?.controller.selected_question_id;
		if (!runtime || !id || !runtime.controller.is_showing_question || this.asked.includes(id)) return;
		this.asked = [...this.asked, id];
		sessionStorage.setItem(askedKey(runtime.match_id), JSON.stringify(this.asked));
	}

	#loadQuestions(revisionID: string): void {
		if (revisionID === this.#loadingRevision) return;
		this.#loadingRevision = revisionID;
		this.options = [];
		this.obstacle = '';
		this.#loadedRevision = '';
		if (!revisionID) return;
		void api.questionSets
			.revision(revisionID)
			.then((revision) => {
				if (this.#loadingRevision !== revisionID) return;
				this.options = flattenQuestions(revision.data);
				this.obstacle = revision.data.vcnv?.obstacle ?? '';
				this.#loadedRevision = revisionID;
			})
			.catch((cause) => this.fail(cause, 'Không thể tải đề thi'));
	}

	fail(cause: unknown, fallback = 'Thao tác thất bại'): void {
		if (cause instanceof DOMException && cause.name === 'AbortError') return;
		this.error = cause instanceof Error && cause.message ? cause.message : fallback;
		clearTimeout(this.#errorTimer);
		this.#errorTimer = setTimeout(() => (this.error = ''), 8000);
	}

	/** Runs one show action at a time and surfaces its error in the on-air bar. */
	async run(action: () => Promise<unknown>): Promise<void> {
		if (this.busy) return;
		this.busy = true;
		try {
			await action();
			this.error = '';
		} catch (cause) {
			this.fail(cause);
		} finally {
			this.busy = false;
		}
	}

	changePhase(phase: Phases): void {
		if (phase !== this.controller.phase) void this.run(() => gameController.changePhase(phase));
	}

	showTab(tab: ScreenTab): void {
		const target = tabTarget(tab, this.runtime);
		if (target && target !== this.controller.screen)
			void this.run(() => gameController.showScreen(this.phase.id, target));
	}

	stepScreen(direction: -1 | 1): void {
		const target = this.phase.screens[this.screenIndex + direction];
		if (target) void this.run(() => gameController.showScreen(this.phase.id, target.id));
	}

	openQuestion(id: string): void {
		const phase = this.controller.phase;
		void this.run(() => openLiveQuestion(phase, id));
	}

	stepQuestion(direction: -1 | 1): void {
		if (direction > 0) {
			const step = this.nextStep;
			if (step?.kind === 'end') void this.run(() => showKhoiDongEnd());
			else if (step) this.openQuestion(step.id);
			return;
		}
		const target = this.selectedIndex > 0 ? this.strip[this.selectedIndex - 1] : undefined;
		if (target) this.openQuestion(target.id);
	}

	toggleShowQuestion(): void {
		if (!this.controller.selected_question_id) return;
		const next = !this.controller.is_showing_question;
		void this.run(() => socket.controller.toggleQuestion(next));
	}

	toggleTimer(): void {
		const timer = this.runtime?.timer;
		if (!timer) return;
		if (timer.is_paused) void this.run(() => socket.timer.resume());
		else if (timer.is_running) void this.run(() => socket.timer.pause());
		else if (this.playing && this.presetDuration > 0 && !this.timerSpent) void this.run(() => socket.timer.start());
	}

	resetTimer(): void {
		if (this.runtime) void this.run(() => socket.timer.reset());
	}

	clearBells(): void {
		void this.run(() => socket.controller.clearBells());
	}

	clearAnswers(): void {
		void this.run(() => socket.controller.clearAnswers());
	}

	/** VCNV board button: shows the empty board, a second press starts it. */
	pressBoard(): void {
		void this.run(() => pressVCNVBoard());
	}

	/** VCNV row Mở: chooses the row on the board, a second press opens its question. */
	pressRow(row: number): void {
		void this.run(() => pressVCNVRow(row));
	}

	startObstacleTimer(): void {
		if (this.canStartObstacle) void this.run(() => socket.timer.startObstacle());
	}

	/** Phát media: audio through the TV sound output, Về đích video as the takeover. */
	playMedia(): void {
		if (this.mediaKind === 'audio') void this.run(() => socket.sound.playQuestionMedia());
		else if (this.mediaKind === 'video') void this.run(() => socket.controller.veDichVideo('start'));
	}

	steal(): void {
		if (this.canSteal) void this.run(() => socket.timer.startSteal());
	}

	stopSound(): void {
		const fade = api.settings.current?.sound.transition_fade_ms ?? 120;
		void this.run(() => socket.sound.stopAll(fade));
	}

	toggleView(): void {
		this.view = this.view === 'reader' ? 'log' : 'reader';
	}

	/** Đúng/Sai on a card: fills the pending points and tells the display which answers were right. */
	verdict(slot: Slot, verdict: Verdict): void {
		if (this.busy) return;
		const key = this.drafts.liveKey;
		const info = questionInfo(key);
		if (!info || !this.runtime) return;
		const result = this.drafts.toggle(key, slot, verdict);
		if (!result) return;
		// Upper contestant cards show on the display at once (answer status), while the points stay
		// staged until Ghi điểm. Sounds are still triggered from the lower panel only.
		void socket.controller.setAnswerStatus(slot, result === 'correct').catch((cause) => this.fail(cause));
	}

	/** Chướng ngại vật keyword: a wrong answer locks the contestant out of the round. */
	obstacleVerdict(slot: Slot, verdict: Verdict): void {
		const before = this.drafts.verdict(OBSTACLE_KEY, slot);
		const result = this.drafts.toggle(OBSTACLE_KEY, slot, verdict);
		if (result) {
			const id = result === 'correct' ? 'builtin.vcnv.correctobstacle' : 'builtin.vcnv.wrongrow';
			void socket.sound.play(id).catch((cause) => this.fail(cause));
		}
		const locked = this.runtime?.contestants.find((entry) => entry.slot === slot)?.locked ?? false;
		const shouldLock = result === 'wrong';
		if (shouldLock !== locked && (shouldLock || before === 'wrong'))
			void socket.match.setContestantLock(slot, shouldLock).catch((cause) => this.fail(cause));
	}

	/** Clears the staged Đúng/Sai of the live question and puts every card back to unmarked on the display. */
	clearVerdicts(): void {
		if (this.busy) return;
		const key = this.drafts.liveKey;
		this.drafts.clearVerdicts(key);
		void this.run(() =>
			Promise.all([1, 2, 3, 4].map((slot) => socket.controller.setAnswerStatus(slot as Slot, true)))
		);
	}

	setLock(slot: Slot, locked: boolean): void {
		void this.run(() => socket.match.setContestantLock(slot, locked));
	}

	save(): void {
		void this.run(() => this.drafts.save());
	}

	undo(): void {
		void this.run(() => this.drafts.undo());
	}
}

export const panel = new OperatorPanel();
