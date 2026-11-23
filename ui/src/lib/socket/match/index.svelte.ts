import type { MatchCommand, MatchRuntimeState } from '../opsocket';
import type { SocketTransport } from '../transport';
import { uuid } from '../../uuid';

type RuntimeHandler = (runtime: MatchRuntimeState | null) => void;

type Versioned = Pick<MatchRuntimeState, 'match_id' | 'epoch' | 'version'>;

/**
 * True when next is an older (epoch, version) of the match already shown.
 * A different match or epoch always replaces; equal versions are accepted
 * (public and operator snapshots of one version differ only by filtering).
 */
export function isStaleRuntime(next: Versioned | null, current: Versioned | null): boolean {
	if (!next?.match_id || !current?.match_id) return false;
	if (next.match_id !== current.match_id || next.epoch !== current.epoch) return false;
	return next.version < current.version;
}

export class MatchSocket {
	#current = $state<MatchRuntimeState | null>(null);
	#handlers: RuntimeHandler[] = [];
	// After (re)connecting the first snapshot is the new baseline, whatever its
	// version, so a restarted server can never be ignored.
	#resync = true;

	constructor(private readonly transport: SocketTransport) {
		for (const event of [
			'Match.Updated',
			'Match.PublicUpdated',
			'Match.ContestantUpdated'
		] as const) {
			transport.on(event, (runtime) => this.accept(runtime));
		}
		transport.onOpen(() => {
			this.#resync = true;
			void this.getCurrent().catch(() => undefined);
		});
	}

	get current(): MatchRuntimeState | null {
		return this.#current;
	}

	command(): MatchCommand {
		const command: MatchCommand = { command_id: uuid() };
		if (!this.#current) return command;
		return {
			...command,
			match_id: this.#current.match_id,
			expected_epoch: this.#current.epoch,
			expected_version: this.#current.version
		};
	}

	/**
	 * Applies a snapshot unless it is older than the one already applied (late
	 * events or responses); then returns the state actually current.
	 */
	accept(runtime: MatchRuntimeState): MatchRuntimeState {
		if (!this.#resync && isStaleRuntime(runtime, this.#current)) {
			return this.#current ?? runtime;
		}
		this.#resync = false;
		this.#current = runtime?.match_id ? runtime : null;
		for (const handler of this.#handlers) handler(this.#current);
		return runtime;
	}

	onUpdate(handler: RuntimeHandler): () => void {
		this.#handlers.push(handler);
		queueMicrotask(() => handler(this.#current));
		return () => (this.#handlers = this.#handlers.filter((value) => value !== handler));
	}

	async getCurrent(): Promise<MatchRuntimeState> {
		return this.accept(await this.transport.request('Match.GetCurrent', {}));
	}

	async select(matchId: string): Promise<MatchRuntimeState> {
		return this.accept(await this.transport.request('Match.Select', { match_id: matchId }));
	}

	async unload(): Promise<MatchRuntimeState> {
		return this.accept(await this.transport.request('Match.Unload', this.command()));
	}

	ready() {
		return this.transition('Match.Ready');
	}
	start() {
		return this.transition('Match.Start');
	}
	pause() {
		return this.transition('Match.Pause');
	}
	resume() {
		return this.transition('Match.Resume');
	}
	complete() {
		return this.transition('Match.Complete');
	}

	async adjustScore(slot: number, delta: number, reason: string): Promise<MatchRuntimeState> {
		return this.accept(
			await this.transport.request('Match.AdjustScore', {
				command: this.command(),
				slot,
				delta,
				reason
			})
		);
	}

	async adjustScores(
		adjustments: { slot: number; delta: number }[],
		reason: string,
		sources?: { question_id: string; version: number; kind: 'verdict' | 'correction' }[],
		command?: MatchCommand
	): Promise<MatchRuntimeState> {
		return this.accept(
			await this.transport.request('Match.AdjustScores', {
				command: command ?? this.command(),
				adjustments,
				reason,
				...(sources?.length ? { sources } : {})
			})
		);
	}

	async undoScores(undoOf: string): Promise<MatchRuntimeState> {
		return this.accept(
			await this.transport.request('Match.UndoScores', {
				command: this.command(),
				undo_of: undoOf
			})
		);
	}

	async setContestantLock(slot: number, locked: boolean): Promise<MatchRuntimeState> {
		return this.accept(
			await this.transport.request('Match.SetContestantLock', {
				command: this.command(),
				slot,
				locked
			})
		);
	}

	/** Bind the entire judgment to the exact snapshot the operator saw. */
	async judgeKhoiDong(snapshot: MatchRuntimeState, verdict: 'correct' | 'wrong') {
		const result = await this.transport.request('Match.JudgeKhoiDong', {
			command: {
				match_id: snapshot.match_id,
				expected_epoch: snapshot.epoch,
				expected_version: snapshot.version,
				expected_input_epoch: snapshot.input_epoch,
				expected_bell_epoch: snapshot.bell_epoch,
				command_id: uuid()
			},
			question_id: snapshot.controller.selected_question_id,
			verdict,
			expected_bell_slot: snapshot.contestants.find((entry) => entry.has_rung)?.slot ?? 0
		});
		this.accept(result.runtime);
		return result;
	}

	private async transition(
		operation: 'Match.Ready' | 'Match.Start' | 'Match.Pause' | 'Match.Resume' | 'Match.Complete'
	): Promise<MatchRuntimeState> {
		return this.accept(await this.transport.request(operation, this.command()));
	}
}
