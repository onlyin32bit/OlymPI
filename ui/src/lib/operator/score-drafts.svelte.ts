import { api } from '$lib/api';
import { socket } from '$lib/socket';
import type { MatchRuntimeState } from '$lib/socket/opsocket';
import {
	OBSTACLE_KEY,
	discardQuestionDraft,
	emptyDraftState,
	liveBatchKey,
	parseDraftState,
	pendingBySlot,
	refreshLive,
	saveRequest,
	scoreSources,
	setPendingTotal,
	clearBatchVerdicts,
	toggleVerdict,
	undoRequest,
	type DraftState
} from './drafts';
import { DEFAULT_SCORING, batchPoints, type ScoreBatch, type Slot, type Verdict } from './scoring';

const storageKey = (matchID: string) => `4t.cp-next.drafts.${matchID}`;

/**
 * Pending points for the operator panel. Verdicts fill suggestions; nothing
 * reaches the score until save(), which sends one audited adjustment.
 */
export class ScoreDrafts {
	state = $state<DraftState>(emptyDraftState());
	saving = $state(false);
	#matchID = '';

	rules = $derived(api.settings.current?.scoring ?? DEFAULT_SCORING);
	liveKey = $derived(liveBatchKey(socket.match.current));
	pending = $derived(pendingBySlot(this.state, this.rules));
	hasPending = $derived(Object.values(this.pending).some((value) => value !== 0));
	/** Batches with unsaved points for a question that is no longer on air. */
	stale = $derived(
		this.state.batches.filter(
			(batch) =>
				batch.key !== this.liveKey &&
				batch.key !== OBSTACLE_KEY &&
				batch.kind !== 'manual' &&
				Object.values(batchPoints(batch, this.rules)).some((value) => value !== 0)
		)
	);

	batch(key: string): ScoreBatch | undefined {
		return this.state.batches.find((entry) => entry.key === key);
	}

	verdict(key: string, slot: Slot): Verdict | undefined {
		return this.batch(key)?.verdicts[slot];
	}

	/** Follows the selected match and keeps the on-air suggestion current. */
	sync(runtime: MatchRuntimeState | null): void {
		const matchID = runtime?.match_id ?? '';
		if (matchID !== this.#matchID) {
			this.#matchID = matchID;
			this.state = matchID ? parseDraftState(sessionStorage.getItem(storageKey(matchID))) : emptyDraftState();
		}
		this.#commit(refreshLive(this.state, runtime));
	}

	/** Returns the verdict now set for the slot, or undefined when the press cleared it. */
	toggle(key: string, slot: Slot, verdict: Verdict): Verdict | undefined {
		this.#commit(toggleVerdict(this.state, socket.match.current, key, slot, verdict));
		return this.verdict(key, slot);
	}

	/** Clears the Đúng/Sai verdicts of one question's batch. */
	clearVerdicts(key: string): void {
		this.#commit(clearBatchVerdicts(this.state, socket.match.current, key));
	}

	setTotal(slot: Slot, total: number): void {
		this.#commit(setPendingTotal(this.state, socket.match.current, this.rules, slot, total));
	}

	nudge(slot: Slot, direction: -1 | 1): void {
		const step = direction < 0 ? -this.rules.adjust.down : this.rules.adjust.up;
		this.setTotal(slot, this.pending[slot] + step);
	}

	discard(): void {
		this.#commit({ ...this.state, batches: [] });
	}

	/** Discard only this question's draft, preserving other questions and undo history. */
	discardQuestion(key: string): void {
		this.#commit(discardQuestionDraft(this.state, key));
	}

	async save(): Promise<boolean> {
		const request = saveRequest(this.state, this.rules);
		if (!request || this.saving) return false;
		this.saving = true;
		try {
			const command = socket.match.command();
			const sources = scoreSources(this.state, this.rules);
			await socket.match.adjustScores(request.adjustments, request.reason, sources, command);
			this.#commit({ batches: [], lastSave: { ...request, at: Date.now(), commandId: command.command_id, sources } });
			return true;
		} finally {
			this.saving = false;
		}
	}

	async undo(): Promise<boolean> {
		const request = undoRequest(this.state);
		if (!request || this.saving) return false;
		this.saving = true;
		try {
			if (this.state.lastSave?.commandId) {
				await socket.match.undoScores(this.state.lastSave.commandId);
			} else {
				await socket.match.adjustScores(request.adjustments, request.reason);
			}
			this.#commit({ ...this.state, lastSave: null });
			return true;
		} finally {
			this.saving = false;
		}
	}

	#commit(next: DraftState): void {
		this.state = next;
		if (this.#matchID && typeof sessionStorage !== 'undefined')
			sessionStorage.setItem(storageKey(this.#matchID), JSON.stringify(next));
	}
}
