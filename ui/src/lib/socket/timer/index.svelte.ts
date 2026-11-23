import type { MatchRuntimeState, TimerState } from '../opsocket';
import type { MatchSocket } from '../match/index.svelte';
import type { SocketTransport } from '../transport';

export class TimerSocket {
	#remaining = $state(0);
	#elapsed = $state(0);
	#frame: number | null = null;
	#timer: TimerState | null = null;

	constructor(
		private readonly transport: SocketTransport,
		private readonly match: MatchSocket
	) {
		match.onUpdate((runtime) => {
			if (runtime) this.sync(runtime.timer);
			else this.clear();
		});
	}

	get remaining(): number {
		return this.#remaining;
	}

	get elapsed(): number {
		return this.#elapsed;
	}

	start() {
		return this.apply(
			this.transport.request('Timer.Start', {
				command: this.match.command()
			})
		);
	}

	startSteal() {
		return this.apply(
			this.transport.request('Timer.Start', {
				command: this.match.command(),
				steal: true
			})
		);
	}

	/** VCNV 15 s obstacle timer on the board/image screen; inputs are untouched. */
	startObstacle() {
		return this.apply(
			this.transport.request('Timer.Start', {
				command: this.match.command(),
				obstacle: true
			})
		);
	}

	pause() {
		return this.apply(this.transport.request('Timer.Pause', this.match.command()));
	}

	resume() {
		return this.apply(this.transport.request('Timer.Resume', this.match.command()));
	}

	reset() {
		return this.apply(this.transport.request('Timer.Reset', this.match.command()));
	}

	private async apply(request: Promise<MatchRuntimeState>): Promise<MatchRuntimeState> {
		return this.match.accept(await request);
	}

	private sync(timer: TimerState): void {
		this.#timer = timer;
		if (typeof window !== 'undefined' && this.#frame !== null) cancelAnimationFrame(this.#frame);
		this.#frame = null;
		this.#elapsed = timer.elapsed_ms;
		if (timer.is_paused) {
			this.#remaining = timer.remaining_ms;
			return;
		}
		if (!timer.is_running) {
			this.#remaining = 0;
			return;
		}
		this.tick();
	}

	private clear(): void {
		this.#timer = null;
		this.#remaining = 0;
		this.#elapsed = 0;
		if (typeof window !== 'undefined' && this.#frame !== null) cancelAnimationFrame(this.#frame);
		this.#frame = null;
	}

	private tick = (): void => {
		if (!this.#timer?.is_running || this.#timer.is_paused) return;
		const serverNow = Date.now() + this.transport.serverClockOffset;
		const segmentElapsed = Math.max(serverNow - this.#timer.started_at, 0);
		this.#remaining = Math.max(this.#timer.remaining_ms - segmentElapsed, 0);
		this.#elapsed = Math.min(this.#timer.duration_ms, this.#timer.elapsed_ms + segmentElapsed);
		if (this.#remaining > 0 && typeof window !== 'undefined')
			this.#frame = requestAnimationFrame(this.tick);
		else this.#frame = null;
	};
}
