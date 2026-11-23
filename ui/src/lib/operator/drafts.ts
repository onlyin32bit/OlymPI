import type { MatchRuntimeState } from '$lib/socket/opsocket';
import type { ScoringRules } from '$lib/types';
import {
	SLOTS,
	batchPoints,
	clampPoints,
	combinePending,
	emptyBatch,
	questionInfo,
	verdictAllowed,
	type ScoreBatch,
	type Slot,
	type Verdict
} from './scoring';

export const MANUAL_KEY = 'manual';
export const OBSTACLE_KEY = 'vcnv.obstacle';

export type SavedScores = {
	adjustments: { slot: Slot; delta: number }[];
	reason: string;
	commandId?: string;
	sources?: ScoreSource[];
	at: number;
};

export type DraftState = { batches: ScoreBatch[]; lastSave: SavedScores | null };

export const emptyDraftState = (): DraftState => ({ batches: [], lastSave: null });

/** Remove only one question; leave unrelated batches and saved undo untouched. */
export function discardQuestionDraft(state: DraftState, key: string): DraftState {
	return { ...state, batches: state.batches.filter((batch) => batch.key !== key) };
}

export function questionLabel(id: string): string {
	const info = questionInfo(id);
	if (!info) return id;
	switch (info.kind) {
		case 'kd_rieng':
			return `Khởi động · TS${info.owner} câu ${info.index + 1}`;
		case 'kd_chung':
			return `Khởi động chung · Câu ${info.index + 1}`;
		case 'vcnv_row':
			return `VCNV · Hàng ${info.index + 1}`;
		case 'vcnv_center':
			return 'VCNV · Ô trung tâm';
		case 'tang_toc':
			return `Tăng tốc · Câu ${info.index + 1}`;
		case 've_dich':
			return `Về đích · TS${info.owner} câu ${info.index + 1}`;
		default:
			return id;
	}
}

/** Verdict provenance distinguishes scoring from manual corrections; old saves use labels. */
export function savedQuestion(state: DraftState, key: string): boolean {
	if (!key || !questionInfo(key) || !state.lastSave) return false;
	const source = state.lastSave.sources?.find((entry) => entry.question_id === key);
	if (source) return source.kind !== 'correction';
	const label = questionLabel(key);
	return state.lastSave.reason.split(',').some((part) => part.trim() === label);
}

/** The batch that verdicts on the cards refer to right now. */
export function liveBatchKey(runtime: MatchRuntimeState | null | undefined): string {
	const id = runtime?.controller.selected_question_id ?? '';
	if (!id || runtime?.current_question?.id !== id || !questionInfo(id)) return MANUAL_KEY;
	return id;
}

/** Rows opened at this moment; 5 once the centre clue is out. */
export function obstacleRowsOpened(runtime: MatchRuntimeState | null | undefined): number {
	const controller = runtime?.controller;
	if (!controller) return 1;
	if (controller.selected_question_id === 'vcnv.center' || controller.vcnv_state.image_open_state[4])
		return 5;
	return controller.vcnv_state.row_states.filter((state) => state !== 'u').length;
}

function contestant(runtime: MatchRuntimeState | null | undefined, slot: Slot) {
	return runtime?.contestants.find((entry) => entry.slot === slot);
}

/** Copies what the suggestion depends on from the live runtime. */
export function refreshBatch(batch: ScoreBatch, runtime: MatchRuntimeState | null | undefined): ScoreBatch {
	if (!runtime) return batch;
	// Known answer times survive input resets (a restarted timer clears answers),
	// so a pending suggestion never silently loses its ranking.
	const times: ScoreBatch['times'] = { ...batch.times };
	const scores: ScoreBatch['scores'] = {};
	for (const slot of SLOTS) {
		const entry = contestant(runtime, slot);
		if (!entry) continue;
		scores[slot] = entry.score;
		if (entry.answer && entry.time_ms > 0) times[slot] = entry.time_ms;
	}
	let starred = batch.starred;
	const info = questionInfo(batch.key);
	if (info?.kind === 've_dich')
		starred = runtime.controller.ve_dich_contestants_state[info.owner - 1]?.starred_on === info.index + 1;
	return { ...batch, times, scores, starred };
}

export function createBatch(key: string, runtime: MatchRuntimeState | null | undefined): ScoreBatch {
	if (key === MANUAL_KEY) return emptyBatch(MANUAL_KEY, 'manual', 'Chỉnh tay');
	if (key === OBSTACLE_KEY) return refreshBatch(emptyBatch(OBSTACLE_KEY, 'obstacle', 'VCNV · Chướng ngại vật'), runtime);
	const info = questionInfo(key);
	if (!info) return emptyBatch(MANUAL_KEY, 'manual', 'Chỉnh tay');
	const batch = emptyBatch(key, info.kind, questionLabel(key));
	batch.owner = info.owner;
	batch.tier = info.tier;
	if (info.kind === 'kd_rieng' || info.kind === 'kd_chung') batch.sourceVersion = runtime?.version ?? 0;
	return refreshBatch(batch, runtime);
}

function isEmpty(batch: ScoreBatch): boolean {
	return Object.keys(batch.verdicts).length === 0 && Object.keys(batch.overrides).length === 0;
}

function upsert(state: DraftState, batch: ScoreBatch): DraftState {
	const others = state.batches.filter((entry) => entry.key !== batch.key);
	return { ...state, batches: isEmpty(batch) ? others : [...others, batch] };
}

function batchFor(state: DraftState, key: string, runtime: MatchRuntimeState | null | undefined): ScoreBatch {
	return state.batches.find((entry) => entry.key === key) ?? createBatch(key, runtime);
}

/** Pressing the verdict that is already set clears it; any verdict drops a manual override. */
export function toggleVerdict(
	state: DraftState,
	runtime: MatchRuntimeState | null | undefined,
	key: string,
	slot: Slot,
	verdict: Verdict
): DraftState {
	const batch = batchFor(state, key, runtime);
	if (!verdictAllowed(batch, slot)) return state;
	const verdicts = { ...batch.verdicts };
	const overrides = { ...batch.overrides };
	const obstacleRows = { ...batch.obstacleRows };
	delete overrides[slot];
	if (verdicts[slot] === verdict) {
		delete verdicts[slot];
		delete obstacleRows[slot];
	} else {
		verdicts[slot] = verdict;
		if (batch.kind === 'obstacle') obstacleRows[slot] = obstacleRowsOpened(runtime);
	}
	return upsert(state, { ...batch, verdicts, overrides, obstacleRows });
}

/** Clears every Đúng/Sai verdict in one batch; manual totals and undo history stay as they are. */
export function clearBatchVerdicts(
	state: DraftState,
	runtime: MatchRuntimeState | null | undefined,
	key: string
): DraftState {
	const batch = batchFor(state, key, runtime);
	if (Object.keys(batch.verdicts).length === 0) return state;
	return upsert(state, { ...batch, verdicts: {}, obstacleRows: {} });
}

/** Sets what a card's pending total should be, by overriding the live batch. */
export function setPendingTotal(
	state: DraftState,
	runtime: MatchRuntimeState | null | undefined,
	rules: ScoringRules,
	slot: Slot,
	total: number
): DraftState {
	const key = liveBatchKey(runtime);
	const batch = batchFor(state, key, runtime);
	const others = state.batches
		.filter((entry) => entry.key !== key)
		.reduce((sum, entry) => sum + batchPoints(entry, rules)[slot], 0);
	const overrides = { ...batch.overrides, [slot]: clampPoints(total) - others };
	return upsert(state, { ...batch, overrides });
}

export function pendingBySlot(state: DraftState, rules: ScoringRules): Record<Slot, number> {
	const totals: Record<Slot, number> = { 1: 0, 2: 0, 3: 0, 4: 0 };
	for (const batch of state.batches) {
		const points = batchPoints(batch, rules);
		for (const slot of SLOTS) totals[slot] += points[slot];
	}
	return totals;
}

/** Keeps the on-air batch in step with answers, times and the star. */
export function refreshLive(state: DraftState, runtime: MatchRuntimeState | null | undefined): DraftState {
	const key = liveBatchKey(runtime);
	const index = state.batches.findIndex((entry) => entry.key === key && entry.kind !== 'manual');
	if (index < 0) return state;
	const batches = [...state.batches];
	batches[index] = refreshBatch(batches[index], runtime);
	return { ...state, batches };
}

export type SaveRequest = { adjustments: { slot: Slot; delta: number }[]; reason: string };

export function saveRequest(state: DraftState, rules: ScoringRules): SaveRequest | null {
	const adjustments = combinePending(state.batches, rules);
	if (!adjustments.length) return null;
	const reason = state.batches
		.filter((batch) => Object.values(batchPoints(batch, rules)).some((value) => value !== 0))
		.map((batch) => batch.label)
		.join(', ');
	return { adjustments, reason: reason || 'Chỉnh điểm' };
}

export type ScoreSource = {
	question_id: string;
	version: number;
	kind: 'verdict' | 'correction';
};

/** Provenance for nonzero KD batches; duplicate sources retain the oldest version and verdict kind. */
export function scoreSources(state: DraftState, rules: ScoringRules): ScoreSource[] {
	const sources = new Map<string, ScoreSource>();
	for (const batch of state.batches) {
		if (batch.kind !== 'kd_rieng' && batch.kind !== 'kd_chung') continue;
		if (!Object.values(batchPoints(batch, rules)).some((value) => value !== 0)) continue;
		const version = Number.isSafeInteger(batch.sourceVersion) && (batch.sourceVersion ?? 0) > 0
			? batch.sourceVersion!
			: 0;
		const hasVerdict = SLOTS.some((slot) => batch.verdicts[slot] !== undefined && batch.overrides[slot] == null);
		const hasOverride = SLOTS.some((slot) => batch.overrides[slot] != null);
		const kind = hasVerdict || !hasOverride ? 'verdict' : 'correction';
		const previous = sources.get(batch.key);
		sources.set(batch.key, {
			question_id: batch.key,
			version: previous ? Math.min(previous.version, version) : version,
			kind: previous?.kind === 'verdict' || kind === 'verdict' ? 'verdict' : 'correction'
		});
	}
	return Array.from(sources.values());
}

export function undoRequest(state: DraftState): SaveRequest | null {
	if (!state.lastSave) return null;
	return {
		adjustments: state.lastSave.adjustments.map(({ slot, delta }) => ({ slot, delta: -delta })),
		reason: `Hoàn tác · ${state.lastSave.reason}`
	};
}

export function parseDraftState(raw: string | null): DraftState {
	if (!raw) return emptyDraftState();
	try {
		const value = JSON.parse(raw) as Partial<DraftState>;
		return {
			batches: Array.isArray(value.batches) ? value.batches : [],
			lastSave: value.lastSave ?? null
		};
	} catch {
		return emptyDraftState();
	}
}
