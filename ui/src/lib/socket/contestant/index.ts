import type { MatchCommand, MatchRuntimeState } from '../opsocket';
import type { MatchSocket } from '../match/index.svelte';
import type { SocketTransport } from '../transport';
import { uuid } from '../../uuid';

export class ContestantSocket {
	constructor(
		private readonly transport: SocketTransport,
		private readonly match: MatchSocket
	) {}

	async submitAnswer(answer: string): Promise<MatchRuntimeState> {
		const command = this.inputCommand();
		return this.acceptInput(
			await this.transport.request('Contestant.SubmitAnswer', { command, answer }),
			command,
			'answer'
		);
	}

	async ring(): Promise<MatchRuntimeState> {
		const command = this.inputCommand();
		return this.acceptInput(
			await this.transport.request('Contestant.Ring', command),
			command,
			'bell'
		);
	}

	private inputCommand(): MatchCommand {
		const runtime = this.match.current;
		if (!runtime) throw new Error('Chưa có trận đấu đang diễn ra');
		return {
			command_id: uuid(),
			match_id: runtime.match_id,
			expected_epoch: runtime.epoch,
			expected_input_epoch: runtime.input_epoch,
			expected_bell_epoch: runtime.bell_epoch
		};
	}

	private acceptInput(
		runtime: MatchRuntimeState,
		command: MatchCommand,
		kind: 'answer' | 'bell'
	): MatchRuntimeState {
		const current = this.match.current;
		if (
			!current ||
			current.match_id !== command.match_id ||
			current.epoch !== command.expected_epoch ||
			(kind === 'bell'
				? current.bell_epoch !== command.expected_bell_epoch
				: current.input_epoch !== command.expected_input_epoch)
		) {
			throw new DOMException('Lượt trả lời đã thay đổi', 'AbortError');
		}
		// A broadcast can arrive before the response to this independent contestant input.
		if (current.version > runtime.version) return current;
		return this.match.accept(runtime);
	}
}
