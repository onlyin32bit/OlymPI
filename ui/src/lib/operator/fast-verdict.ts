import type { MatchRuntimeState } from '$lib/socket/opsocket';
import type { ScoringRules } from '$lib/types';
import { questionInfo, type Verdict } from './scoring';

/** The server accepts only one claim for a Khởi động common question. */
export function bellSlot(runtime: MatchRuntimeState): number {
	return runtime.contestants.find((entry) => entry.has_rung)?.slot ?? 0;
}

export function fastVerdictState(runtime: MatchRuntimeState | null, rules: ScoringRules | undefined, connected: boolean, busy: boolean, pending: boolean, saved = false) {
	const controller = runtime?.controller;
	const screen = controller?.screen ?? '';
	const id = controller?.selected_question_id ?? '';
	const visible = Boolean(id && controller?.phase === 'khoi_dong' && (screen === 'ques_chung' || /^ques_ts[1-4]$/.test(screen)));
	const info = questionInfo(id);
	const common = screen === 'ques_chung';
	const valid = common ? info?.kind === 'kd_chung' : info?.kind === 'kd_rieng' && screen === `ques_ts${info.owner}`;
	const slot = runtime && valid ? (common ? bellSlot(runtime) : info!.owner) : 0;
	const person = runtime?.contestants.find((entry) => entry.slot === slot);
	const correct = rules ? (common ? rules.khoi_dong.common_correct : rules.khoi_dong.individual_correct) : 0;
	let wrong = rules ? (common ? rules.khoi_dong.common_wrong : rules.khoi_dong.individual_wrong) : 0;
	if (common && !slot) wrong = 0;
	else if (common && rules?.khoi_dong.common_floor_zero && wrong < 0) wrong = Math.max(wrong, -Math.max(0, person?.score ?? 0));
	const enabled = Boolean(visible && runtime?.match_id && runtime.status === 'playing' && connected && !busy && !pending && !saved && rules && controller?.is_showing_question && valid && runtime.current_question?.id === id && (!slot || person));
	return { visible, slot, name: person?.name || (slot ? `Thí sinh ${slot}` : 'Chưa có chuông'), correct, wrong, canCorrect: enabled && (!common || slot !== 0), canWrong: enabled, pending, saved };
}

/** Explicit music buttons never mutate a verdict or the question. */
export function soundOnly(phase: string, screen: string, verdict: Verdict): string | undefined {
	if (phase === 'vcnv') return verdict === 'wrong' ? 'builtin.vcnv.wrongrow' : (screen === 'main_vcnv' || screen === 'image_vcnv' ? 'builtin.vcnv.correctobstacle' : 'builtin.vcnv.correctrow');
	if (phase === 'tang_toc') return verdict === 'correct' ? 'builtin.tt.correct' : 'builtin.vcnv.wrongrow';
	if (phase === 've_dich') return verdict === 'correct' ? 'builtin.vd.correctfinish' : 'builtin.vd.wrongfinish';
	return undefined;
}
