import { brand } from '$lib/brand';
import type { Component } from 'svelte';
import Idle from '$lib/components/display/idle/idle.svelte';
import KhoiDong from '$lib/components/display/khoi_dong/khoi_dong.svelte';
import TestBell from '$lib/components/display/khoi_dong/test-bell.svelte';
import Logo from '$lib/components/display/logo/logo.svelte';
import Main from '$lib/components/display/main/main.svelte';
import Score from '$lib/components/display/score/score.svelte';
import PhaseEnd from '$lib/components/display/shared/phase-end.svelte';
import PhaseIntro from '$lib/components/display/shared/phase-intro.svelte';
import PhaseRule from '$lib/components/display/shared/phase-rule.svelte';
import PhaseStart from '$lib/components/display/shared/phase-start.svelte';
import TangToc from '$lib/components/display/tang_toc/TangToc.svelte';
import AnswersTt from '$lib/components/display/tang_toc/AnswersTt.svelte';
import AnswersVcnv from '$lib/components/display/vcnv/AnswersVcnv.svelte';
import VCNV from '$lib/components/display/vcnv/VCNV.svelte';
import VeDich from '$lib/components/display/ve_dich/VeDich.svelte';
import KhoiDongControl from '$lib/control/khoi-dong-control.svelte';
import TangTocControl from '$lib/control/tang-toc-control.svelte';
import VcnvControl from '$lib/control/vcnv-control.svelte';
import VeDichControl from '$lib/control/ve-dich-control.svelte';
import type { Phases, Screens } from '$lib/types';
import type { DisplayContext } from './types';

type DisplayScene = Component<{ context: DisplayContext }>;
export type ScreenDefinition = {
	id: Screens;
	label: string;
	component?: DisplayScene;
};
export type PhaseControlProps = { phase: Phases; screens: readonly ScreenDefinition[] };
export type PhaseDefinition = {
	id: Phases;
	label: string;
	title: string;
	component: DisplayScene;
	control: Component<PhaseControlProps> | null;
	screens: readonly ScreenDefinition[];
};

// Screen sounds are cued by the server (sound cue director), never by the manifest.
const sharedScreens = [
	{ id: 'intro', label: 'Intro', component: PhaseIntro },
	{ id: 'rule', label: 'Luật chơi', component: PhaseRule },
	{ id: 'start', label: 'Bắt đầu', component: PhaseStart },
	{ id: 'end', label: 'Kết thúc', component: PhaseEnd }
] as const;
const [intro, rule, start, end] = sharedScreens;

export const phaseManifest: Record<Phases, PhaseDefinition> = {
	idle: { id: 'idle', label: 'Tĩnh', title: '', component: Idle, control: null, screens: [{ id: 'intro', label: 'Chờ' }] },
	logo: { id: 'logo', label: `Logo ${brand.appName}`, title: '', component: Logo, control: null, screens: [{ id: 'intro', label: `Logo ${brand.appName}` }] },
	main: { id: 'main', label: 'Màn hình chính', title: brand.mainTitle, component: Main, control: null, screens: [{ id: 'main', label: 'Danh sách thí sinh' }] },
	score: { id: 'score', label: 'Điểm', title: 'BẢNG ĐIỂM', component: Score, control: null, screens: [{ id: 'score', label: 'Bảng điểm' }] },
	khoi_dong: {
		id: 'khoi_dong', label: 'Khởi động', title: 'KHỞI ĐỘNG', component: KhoiDong, control: KhoiDongControl,
		screens: [
			start, rule, intro, { id: 'main_kd', label: 'Màn hình chính' }, { id: 'test_bell', label: 'Thử chuông', component: TestBell },
			{ id: 'ques_chung', label: 'Câu hỏi chung' }, { id: 'ques_ts1', label: 'Thí sinh 1' }, { id: 'ques_ts2', label: 'Thí sinh 2' }, { id: 'ques_ts3', label: 'Thí sinh 3' }, { id: 'ques_ts4', label: 'Thí sinh 4' },
			{ id: 'end_ts', label: 'Kết thúc lượt' }, end
		]
	},
	vcnv: {
		id: 'vcnv', label: 'Vượt chướng ngại vật', title: 'VƯỢT CHƯỚNG NGẠI VẬT', component: VCNV, control: VcnvControl,
		screens: [start, rule, intro, { id: 'main_vcnv', label: 'Chướng ngại vật' }, { id: 'image_vcnv', label: 'Hình ảnh' }, { id: 'ques', label: 'Câu hỏi' }, { id: 'answers_vcnv', label: 'Câu trả lời thí sinh', component: AnswersVcnv }, end]
	},
	tang_toc: {
		id: 'tang_toc', label: 'Tăng tốc', title: 'TĂNG TỐC', component: TangToc, control: TangTocControl,
		screens: [start, rule, intro, { id: 'ques', label: 'Câu hỏi' }, { id: 'solve', label: 'Giải thích' }, { id: 'answers_tt', label: 'Câu trả lời thí sinh', component: AnswersTt }, end]
	},
	ve_dich: {
		id: 've_dich', label: 'Về đích', title: 'VỀ ĐÍCH', component: VeDich, control: VeDichControl,
		screens: [start, rule, intro, { id: 'main_vd', label: 'Chọn thí sinh' },
			{ id: 'pre_ques_ts1', label: 'Chọn gói thí sinh 1' }, { id: 'ques_ts1', label: 'Câu hỏi thí sinh 1' }, { id: 'pre_ques_ts2', label: 'Chọn gói thí sinh 2' }, { id: 'ques_ts2', label: 'Câu hỏi thí sinh 2' }, { id: 'pre_ques_ts3', label: 'Chọn gói thí sinh 3' }, { id: 'ques_ts3', label: 'Câu hỏi thí sinh 3' }, { id: 'pre_ques_ts4', label: 'Chọn gói thí sinh 4' }, { id: 'ques_ts4', label: 'Câu hỏi thí sinh 4' }, { id: 'end_ts', label: 'Kết thúc lượt' }, end]
	}
};

export const phaseOrder: readonly Phases[] = ['idle', 'logo', 'main', 'score', 'khoi_dong', 'vcnv', 'tang_toc', 've_dich'];

export function phaseFor(value: string): PhaseDefinition { return phaseManifest[value as Phases] ?? phaseManifest.idle; }
export function screenFor(phase: string, screen: string): ScreenDefinition {
	const definition = phaseFor(phase);
	return definition.screens.find((item) => item.id === screen) ?? definition.screens[0];
}
export function defaultScreen(phase: string): ScreenDefinition { return phaseFor(phase).screens[0]; }
export function componentFor(phase: string, screen: string): DisplayScene {
	const definition = phaseFor(phase);
	return screenFor(definition.id, screen).component ?? definition.component;
}
