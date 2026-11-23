import type { MatchRuntimeState } from '$lib/socket/opsocket';
import type { Question, QuestionSetData, Screens } from '$lib/types';

export type QuestionOption = {
	id: string;
	/** Short label for the number strip ("4", "TT"). */
	short: string;
	label: string;
	question: Question;
	phase: string;
	slot?: number;
};

export function flattenQuestions(data: QuestionSetData): QuestionOption[] {
	const result: QuestionOption[] = [];
	data.kd_chung.forEach((question, index) =>
		result.push({ id: `kd_chung.${index}`, short: String(index + 1), label: `Câu ${index + 1}`, question, phase: 'khoi_dong' })
	);
	data.kd_rieng.forEach((list, slot) =>
		list.forEach((question, index) =>
			result.push({ id: `kd_rieng.${slot}.${index}`, short: String(index + 1), label: `TS${slot + 1} · Câu ${index + 1}`, question, phase: 'khoi_dong', slot: slot + 1 })
		)
	);
	data.vcnv.rows.forEach((question, index) =>
		result.push({ id: `vcnv.rows.${index}`, short: String(index + 1), label: `Hàng ${index + 1}`, question, phase: 'vcnv' })
	);
	result.push({ id: 'vcnv.center', short: 'TT', label: 'Ô trung tâm', question: data.vcnv.center, phase: 'vcnv' });
	data.tt.forEach((question, index) =>
		result.push({ id: `tt.${index}`, short: String(index + 1), label: `Câu ${index + 1}`, question, phase: 'tang_toc' })
	);
	data.vd.forEach((sets, slot) => {
		(['20', '30'] as const).forEach((tier) =>
			sets[tier].forEach((question, index) =>
				result.push({ id: `vd.${slot}.${tier}.${index}`, short: String(index + 1), label: `TS${slot + 1} · Câu ${index + 1}`, question, phase: 've_dich', slot: slot + 1 })
			)
		);
	});
	return result;
}

/** Contestant whose Về đích turn is on air or selected. */
export function veDichSlot(runtime: MatchRuntimeState | null | undefined): number {
	const controller = runtime?.controller;
	const fromScreen = controller?.screen.match(/^(?:pre_)?ques_ts([1-4])$/)?.[1];
	return fromScreen ? Number(fromScreen) : (controller?.selected_contestant ?? 0);
}

/** Questions the strip offers for the current round and turn, in play order. */
export function roundOptions(
	options: readonly QuestionOption[],
	runtime: MatchRuntimeState | null | undefined
): QuestionOption[] {
	const controller = runtime?.controller;
	if (!controller) return [];
	const inPhase = options.filter((option) => option.phase === controller.phase);
	switch (controller.phase) {
		case 'khoi_dong': {
			const common = inPhase.filter((option) => !option.slot);
			if (controller.screen === 'ques_chung') return common;
			const screenSlot = Number(controller.screen.match(/^ques_ts([1-4])$/)?.[1]);
			if (screenSlot) return inPhase.filter((option) => option.slot === screenSlot);
			if (controller.selected_question_id.startsWith('kd_chung.')) return common;
			const rieng = /^kd_rieng\.([0-3])\./.exec(controller.selected_question_id);
			if (rieng) return inPhase.filter((option) => option.slot === Number(rieng[1]) + 1);
			return [];
		}
		case 'vcnv':
		case 'tang_toc':
			return inPhase;
		case 've_dich': {
			const slot = veDichSlot(runtime);
			const choices = controller.ve_dich_contestants_state[slot - 1]?.selected_question_set ?? [];
			return choices.flatMap((choice) => {
				const [tier, question] = choice.split('_');
				const option = inPhase.find((candidate) => candidate.id === `vd.${slot - 1}.${tier}.${Number(question) - 1}`);
				return option ? [{ ...option, short: question }] : [];
			});
		}
		default:
			return [];
	}
}

/** The audience screen a question is asked on. */
export function questionScreen(id: string): Screens | null {
	if (/^kd_chung\.\d+$/.test(id)) return 'ques_chung';
	const rieng = /^kd_rieng\.([0-3])\.\d+$/.exec(id);
	if (rieng) return `ques_ts${Number(rieng[1]) + 1}` as Screens;
	if (/^vcnv\.rows\.[0-3]$/.test(id) || id === 'vcnv.center' || /^tt\.\d+$/.test(id)) return 'ques';
	const finish = /^vd\.([0-3])\.(?:20|30)\.[0-2]$/.exec(id);
	if (finish) return `ques_ts${Number(finish[1]) + 1}` as Screens;
	return null;
}
