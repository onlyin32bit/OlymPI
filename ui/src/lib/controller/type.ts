export type VDQuestionScores = 20 | 30;
export type VDQuestionIndex = 1 | 2 | 3;
export type VDQuestionChoices = `${VDQuestionScores}_${VDQuestionIndex}`;

export type Phases = 'idle' | 'logo' | 'main' | 'score' | 'khoi_dong' | 'vcnv' | 'tang_toc' | 've_dich';
export type Screens =
	| 'intro'
	| 'main'
	| 'score'
	| 'start'
	| 'rule'
	| 'main_kd'
	| 'test_bell'
	| 'ques_chung'
	| 'ques_ts1'
	| 'ques_ts2'
	| 'ques_ts3'
	| 'ques_ts4'
	| 'end_ts'
	| 'end'
	| 'main_vcnv'
	| 'image_vcnv'
	| 'ques'
	| 'solve'
	| 'answers_tt'
	| 'answers_vcnv'
	| 'main_vd'
	| 'pre_ques_ts1'
	| 'pre_ques_ts2'
	| 'pre_ques_ts3'
	| 'pre_ques_ts4';

export enum VCNVRowState {
	Unselected = 'u',
	Selected = 's',
	Correct = 'c',
	Wrong = 'w'
}

type VCNVRowStateType = `${VCNVRowState}`;

export type Controller = {
	phase: Phases;
	screen: Screens;
	selected_question_id: string;
	selected_contestant: number;
	is_allowing_bell: boolean;
	is_showing_question: boolean;
	answer_status: Array<boolean>;
	vcnv_state: {
		has_started: boolean;
		has_solved: boolean;
		row_states: Array<VCNVRowStateType>;
		image_open_state: Array<boolean>;
	};
	ve_dich_contestants_state: Array<{
		selected_question_set: Array<VDQuestionChoices>;
		starred_on: number;
	}>;
	ve_dich_video: {
		session_id: number;
		question_id?: string;
		status: 'idle' | 'playing' | 'paused' | 'holding' | '';
		volume: number;
		/** Server ms when the current play segment started; 0 unless playing. */
		started_at: number;
		/** Media position at segment start, or where it was paused/stopped. */
		position_ms: number;
		/** Media length once reported; 0 while unknown. */
		duration_ms: number;
	};
};
