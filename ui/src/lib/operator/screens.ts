import type { ScreenDefinition } from '$lib/display/phase-manifest';
import type { MatchRuntimeState } from '$lib/socket/opsocket';
import type { Screens } from '$lib/types';
import { questionScreen, veDichSlot } from './questions';

export type ScreenTab = {
	id: string;
	label: string;
	/** Audience screens this tab stands for; the tab is on air while one of them is. */
	screens: Screens[];
};

const SHORT_LABELS: Partial<Record<Screens, string>> = {
	main_kd: 'Chính',
	main_vcnv: 'Bảng',
	image_vcnv: 'Hình',
	ques: 'Câu hỏi',
	answers_vcnv: 'Đáp án TS',
	answers_tt: 'Đáp án TS',
	solve: 'Lời giải',
	main_vd: 'Thí sinh',
	end_ts: 'Xong lượt',
	end: 'Kết thúc'
};

const QUESTION_GROUP = /^(?:ques_chung|ques_ts[1-4])$/;
const PACKAGE_GROUP = /^pre_ques_ts[1-4]$/;

/** Collapses per-contestant question screens into one tab so something is always marked on air. */
export function screenTabs(screens: readonly ScreenDefinition[]): ScreenTab[] {
	const tabs: ScreenTab[] = [];
	for (const screen of screens) {
		const group = QUESTION_GROUP.test(screen.id) ? 'question' : PACKAGE_GROUP.test(screen.id) ? 'package' : '';
		if (!group) {
			tabs.push({ id: screen.id, label: SHORT_LABELS[screen.id] ?? screen.label, screens: [screen.id] });
			continue;
		}
		const existing = tabs.find((tab) => tab.id === group);
		if (existing) existing.screens.push(screen.id);
		else tabs.push({ id: group, label: group === 'question' ? 'Câu hỏi' : 'Gói', screens: [screen.id] });
	}
	return tabs;
}

/** Where pressing a tab goes; null when the tab has no sensible target yet. */
export function tabTarget(tab: ScreenTab, runtime: MatchRuntimeState | null | undefined): Screens | null {
	if (tab.screens.length === 1) return tab.screens[0];
	const controller = runtime?.controller;
	if (!controller) return null;
	if (tab.screens.includes(controller.screen)) return controller.screen;
	if (tab.id === 'package') {
		const slot = veDichSlot(runtime);
		return slot ? (`pre_ques_ts${slot}` as Screens) : null;
	}
	const fromQuestion = questionScreen(controller.selected_question_id);
	if (fromQuestion && tab.screens.includes(fromQuestion)) return fromQuestion;
	if (controller.phase === 've_dich') {
		const slot = veDichSlot(runtime);
		return slot ? (`ques_ts${slot}` as Screens) : null;
	}
	return null;
}
