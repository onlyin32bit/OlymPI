import { screenFor } from '$lib/display/phase-manifest';
import { socket } from '$lib/socket';
import type { Phases } from '$lib/types';

// Screen sounds are cued by the server's sound cue director; the client only changes state.
export const gameController = {
	async changePhase(phase: Phases): Promise<void> {
		if (socket.controller.current.phase === phase) return;
		await socket.controller.changePhase(phase);
	},

	async showScreen(phase: Phases, screenID: string): Promise<void> {
		const screen = screenFor(phase, screenID);
		if (socket.controller.current.phase !== phase) return;
		const slot =
			phase === 've_dich' ? Number(screen.id.match(/^(?:pre_)?ques_ts([1-4])$/)?.[1] ?? 0) : 0;
		if (slot && socket.controller.current.selected_contestant !== slot)
			await socket.controller.selectContestant(slot);
		if (socket.controller.current.screen === screen.id) return;
		await socket.controller.changeScreen(screen.id);
	}
};
