import type { MatchRuntimeState } from '$lib/socket/opsocket';

/** Preview only: Timer.Start resolves the authoritative duration on the server. */
export function questionDurationMS(runtime: MatchRuntimeState | null | undefined): number {
	const question = runtime?.current_question;
	const id = runtime?.controller.selected_question_id;
	if (!question || !id || question.id !== id) return 0;
	const parts = id.split('.');
	switch (runtime.controller.phase) {
		case 'khoi_dong':
			return (parts.length === 2 && parts[0] === 'kd_chung') ||
				(parts.length === 3 && parts[0] === 'kd_rieng')
				? 3000
				: 0;
		case 'vcnv':
			if (parts.length === 3 && parts[0] === 'vcnv' && parts[1] === 'rows') return 15000;
			if (id !== 'vcnv.center') return 0;
			break;
		case 'tang_toc':
			if (parts.length !== 2 || parts[0] !== 'tt') return 0;
			break;
		case 've_dich': {
			if (parts.length !== 4 || parts[0] !== 'vd') return 0;
			return parts[2] === '20' ? 15000 : parts[2] === '30' ? 20000 : 0;
		}
		default:
			return 0;
	}
	const fallback = id === 'vcnv.center' ? 15 : 30;
	const seconds = typeof question.time === 'number' && question.time > 0 ? question.time : fallback;
	return Number.isInteger(seconds) && seconds <= 600 ? seconds * 1000 : 0;
}
