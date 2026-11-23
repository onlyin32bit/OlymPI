import type { ScoringRules } from '$lib/types';

/** Mirrors settings.DefaultScoring() in Go; used until the server settings load. */
export const DEFAULT_SCORING: ScoringRules = {
	khoi_dong: {
		individual_correct: 10,
		individual_wrong: 0,
		common_correct: 10,
		common_wrong: -5,
		common_floor_zero: true
	},
	vcnv: {
		row_correct: 10,
		row_wrong: 0,
		center_correct: 10,
		center_wrong: 0,
		obstacle_by_rows: [60, 50, 40, 30],
		obstacle_after_center: 20,
		obstacle_wrong: 0
	},
	tang_toc: { ranks: [40, 30, 20, 10], wrong: 0 },
	ve_dich: {
		tiers: [20, 30],
		correct_percent: 100,
		wrong_percent: 0,
		star_correct_percent: 200,
		star_wrong_percent: -100,
		steal_correct_percent: 100,
		steal_wrong_percent: -50,
		stolen_percent: -100,
		stolen_star_percent: 0
	},
	adjust: { down: 5, up: 10 }
};

export type Verdict = 'correct' | 'wrong';
export type Slot = 1 | 2 | 3 | 4;
export const SLOTS: readonly Slot[] = [1, 2, 3, 4];

export type BatchKind =
	| 'kd_rieng'
	| 'kd_chung'
	| 'vcnv_row'
	| 'vcnv_center'
	| 'obstacle'
	| 'tang_toc'
	| 've_dich'
	| 'manual';

/**
 * Everything needed to suggest points for one question. A batch is refreshed
 * from the live runtime while its question is on air and frozen afterwards, so
 * moving on cannot change a suggestion that has not been saved yet.
 */
export type ScoreBatch = {
	key: string;
	kind: BatchKind;
	label: string;
	/** Runtime version when this KD draft was first created; absent in legacy drafts. */
	sourceVersion?: number;
	/** Question owner for individual rounds; 0 when everyone may answer. */
	owner: number;
	/** Về đích tier index into rules.ve_dich.tiers. */
	tier: 0 | 1;
	starred: boolean;
	/** Answer time in ms per slot; missing or 0 means no answer. */
	times: Partial<Record<Slot, number>>;
	/** Saved score per slot when the suggestion was made, for the no-negative floor. */
	scores: Partial<Record<Slot, number>>;
	/** Obstacle only: rows opened when each verdict was given; 5 means after the centre clue. */
	obstacleRows: Partial<Record<Slot, number>>;
	verdicts: Partial<Record<Slot, Verdict>>;
	/** Manual value that replaces the suggestion for one slot. */
	overrides: Partial<Record<Slot, number>>;
};

export function emptyBatch(key: string, kind: BatchKind, label: string): ScoreBatch {
	return {
		key,
		kind,
		label,
		owner: 0,
		tier: 0,
		starred: false,
		times: {},
		scores: {},
		obstacleRows: {},
		verdicts: {},
		overrides: {}
	};
}

export type QuestionInfo = { kind: BatchKind; owner: number; tier: 0 | 1; index: number };

/** Classifies a question id; null when it cannot be scored. */
export function questionInfo(id: string): QuestionInfo | null {
	let match = /^kd_rieng\.([0-3])\.(\d+)$/.exec(id);
	if (match) return { kind: 'kd_rieng', owner: Number(match[1]) + 1, tier: 0, index: Number(match[2]) };
	match = /^kd_chung\.(\d+)$/.exec(id);
	if (match) return { kind: 'kd_chung', owner: 0, tier: 0, index: Number(match[1]) };
	match = /^vcnv\.rows\.([0-3])$/.exec(id);
	if (match) return { kind: 'vcnv_row', owner: 0, tier: 0, index: Number(match[1]) };
	if (id === 'vcnv.center') return { kind: 'vcnv_center', owner: 0, tier: 0, index: 4 };
	match = /^tt\.(\d+)$/.exec(id);
	if (match) return { kind: 'tang_toc', owner: 0, tier: 0, index: Number(match[1]) };
	match = /^vd\.([0-3])\.(20|30)\.([0-2])$/.exec(id);
	if (match)
		return {
			kind: 've_dich',
			owner: Number(match[1]) + 1,
			tier: match[2] === '20' ? 0 : 1,
			index: Number(match[3])
		};
	return null;
}

/** Which slots may receive a verdict for this kind of question. */
export function verdictAllowed(batch: Pick<ScoreBatch, 'kind' | 'owner'>, slot: Slot): boolean {
	if (batch.kind === 'manual') return false;
	if (batch.kind === 'kd_rieng') return slot === batch.owner;
	return true;
}

/** Rounded percentage of a value, half away from zero. */
export function percentOf(value: number, percent: number): number {
	const exact = (value * percent) / 100;
	return exact < 0 ? -Math.round(-exact) : Math.round(exact);
}

export function obstacleValue(rowsOpened: number, rules: ScoringRules): number {
	if (rowsOpened >= 5) return rules.vcnv.obstacle_after_center;
	const index = Math.min(Math.max(rowsOpened, 1), 4) - 1;
	return rules.vcnv.obstacle_by_rows[index];
}

function answerTime(batch: ScoreBatch, slot: Slot): number {
	const time = batch.times[slot];
	return typeof time === 'number' && time > 0 ? time : Number.POSITIVE_INFINITY;
}

/** Suggested points per slot from the verdicts alone, before manual overrides. */
export function suggestedPoints(batch: ScoreBatch, rules: ScoringRules): Record<Slot, number> {
	const points: Record<Slot, number> = { 1: 0, 2: 0, 3: 0, 4: 0 };
	const verdict = (slot: Slot) => (verdictAllowed(batch, slot) ? batch.verdicts[slot] : undefined);
	switch (batch.kind) {
		case 'kd_rieng':
			for (const slot of SLOTS) {
				const value = verdict(slot);
				if (value) points[slot] = value === 'correct' ? rules.khoi_dong.individual_correct : rules.khoi_dong.individual_wrong;
			}
			break;
		case 'kd_chung':
			for (const slot of SLOTS) {
				const value = verdict(slot);
				if (value === 'correct') points[slot] = rules.khoi_dong.common_correct;
				else if (value === 'wrong') {
					const penalty = rules.khoi_dong.common_wrong;
					const score = batch.scores[slot] ?? 0;
					points[slot] = rules.khoi_dong.common_floor_zero && penalty < 0 ? (score > 0 ? Math.max(penalty, -score) : 0) : penalty;
				}
			}
			break;
		case 'vcnv_row':
		case 'vcnv_center': {
			const row = batch.kind === 'vcnv_row';
			for (const slot of SLOTS) {
				const value = verdict(slot);
				if (value === 'correct') points[slot] = row ? rules.vcnv.row_correct : rules.vcnv.center_correct;
				else if (value === 'wrong') points[slot] = row ? rules.vcnv.row_wrong : rules.vcnv.center_wrong;
			}
			break;
		}
		case 'obstacle':
			for (const slot of SLOTS) {
				const value = verdict(slot);
				if (value === 'correct') points[slot] = obstacleValue(batch.obstacleRows[slot] ?? 1, rules);
				else if (value === 'wrong') points[slot] = rules.vcnv.obstacle_wrong;
			}
			break;
		case 'tang_toc': {
			const correct = SLOTS.filter((slot) => verdict(slot) === 'correct');
			for (const slot of SLOTS) {
				const value = verdict(slot);
				if (value === 'wrong') points[slot] = rules.tang_toc.wrong;
				if (value !== 'correct') continue;
				// Equal times share the higher rank: rank = number of strictly faster correct answers.
				const rank = correct.filter((other) => answerTime(batch, other) < answerTime(batch, slot)).length;
				points[slot] = rank < rules.tang_toc.ranks.length ? rules.tang_toc.ranks[rank] : 0;
			}
			break;
		}
		case 've_dich': {
			const tierValue = rules.ve_dich.tiers[batch.tier];
			const owner = batch.owner as Slot;
			const ownerVerdict = batch.verdicts[owner];
			if (ownerVerdict === 'correct')
				points[owner] = percentOf(tierValue, batch.starred ? rules.ve_dich.star_correct_percent : rules.ve_dich.correct_percent);
			else if (ownerVerdict === 'wrong')
				points[owner] = percentOf(tierValue, batch.starred ? rules.ve_dich.star_wrong_percent : rules.ve_dich.wrong_percent);
			for (const slot of SLOTS) {
				if (slot === owner) continue;
				const value = batch.verdicts[slot];
				if (value === 'correct') {
					points[slot] = percentOf(tierValue, rules.ve_dich.steal_correct_percent);
					points[owner] += percentOf(tierValue, batch.starred ? rules.ve_dich.stolen_star_percent : rules.ve_dich.stolen_percent);
				} else if (value === 'wrong') {
					points[slot] = percentOf(tierValue, rules.ve_dich.steal_wrong_percent);
				}
			}
			break;
		}
		case 'manual':
			break;
	}
	return points;
}

/** Points that will be saved per slot: overrides win over suggestions. */
export function batchPoints(batch: ScoreBatch, rules: ScoringRules): Record<Slot, number> {
	const suggested = suggestedPoints(batch, rules);
	const result: Record<Slot, number> = { 1: 0, 2: 0, 3: 0, 4: 0 };
	for (const slot of SLOTS) result[slot] = batch.overrides[slot] ?? suggested[slot];
	return result;
}

export function hasPending(batch: ScoreBatch, rules: ScoringRules): boolean {
	const points = batchPoints(batch, rules);
	return SLOTS.some((slot) => points[slot] !== 0) || Object.keys(batch.verdicts).length > 0;
}

/** Sums every pending batch into one adjustment per slot, dropping zeros. */
export function combinePending(
	batches: readonly ScoreBatch[],
	rules: ScoringRules
): { slot: Slot; delta: number }[] {
	const totals: Record<Slot, number> = { 1: 0, 2: 0, 3: 0, 4: 0 };
	for (const batch of batches) {
		const points = batchPoints(batch, rules);
		for (const slot of SLOTS) totals[slot] += points[slot];
	}
	return SLOTS.map((slot) => ({ slot, delta: totals[slot] })).filter((entry) => entry.delta !== 0);
}

export function clampPoints(value: number): number {
	if (!Number.isFinite(value)) return 0;
	return Math.max(-1000, Math.min(1000, Math.trunc(value)));
}
