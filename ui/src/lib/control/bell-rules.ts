import type { MatchRuntimeState } from '$lib/socket/opsocket';

function veDichOwner(runtime: MatchRuntimeState): number {
	const controller = runtime.controller;
	const id = controller.selected_question_id;
	const match = /^vd\.([0-3])\.(?:20|30)\.\d+$/.exec(id);
	if (!match || runtime.current_question?.id !== id) return 0;
	const owner = Number(match[1]) + 1;
	return controller.phase === 've_dich' &&
		controller.screen === `ques_ts${owner}` &&
		controller.is_showing_question
		? owner
		: 0;
}

/** Phase/window policy only. The published server gate is checked separately for input. */
export function bellWindowOpen(
	runtime: MatchRuntimeState | null | undefined,
	remainingMS: number
): boolean {
	if (!runtime || runtime.status !== 'playing') return false;
	const controller = runtime.controller;
	if (controller.phase === 'vcnv') return true;
	if (runtime.contestants.some((contestant) => contestant.has_rung)) return false;
	if (controller.phase === 'khoi_dong') {
		// Thử chuông: one ring at a time until the operator clears.
		if (controller.screen === 'test_bell') return true;
		return (
			controller.screen === 'ques_chung' &&
			controller.is_showing_question &&
			/^kd_chung\.\d+$/.test(controller.selected_question_id) &&
			runtime.current_question?.id === controller.selected_question_id
		);
	}
	return (
		veDichOwner(runtime) > 0 &&
		runtime.timer.duration_ms === 5000 &&
		runtime.timer.is_running &&
		!runtime.timer.is_paused &&
		remainingMS > 0 &&
		(!controller.ve_dich_video?.status || controller.ve_dich_video.status === 'idle')
	);
}

export function canRingBell(
	runtime: MatchRuntimeState | null | undefined,
	slot: number,
	remainingMS: number
): boolean {
	if (!runtime || !runtime.controller.is_allowing_bell || !bellWindowOpen(runtime, remainingMS))
		return false;
	const contestant = runtime.contestants.find((entry) => entry.slot === slot);
	if (!contestant || (contestant.bell_locked ?? contestant.locked) || contestant.has_rung)
		return false;
	return runtime.controller.phase !== 've_dich' || slot !== veDichOwner(runtime);
}

/** Clicking this is the operator's declaration that the owner cannot answer. */
export function canStartSteal(runtime: MatchRuntimeState | null | undefined): boolean {
	return Boolean(
		runtime &&
			runtime.status === 'playing' &&
			veDichOwner(runtime) > 0 &&
			runtime.timer.duration_ms !== 5000 &&
			!runtime.contestants.some((contestant) => contestant.has_rung) &&
			(!runtime.controller.ve_dich_video?.status ||
				runtime.controller.ve_dich_video.status === 'idle')
	);
}
