export * from './controller/type';

type ContestantID = 1 | 2 | 3 | 4;

export type Contestant = {
	id: ContestantID;
	name: string;
	class: string;
	answer: string;
	time: number;
	score: number;
	is_locked_down: boolean;
};

export type User = {
	id: number;
	username: string;
	role: UserRole;
	fixed_slot?: number;
	password_change_required?: boolean;
	created_at: string;
	disabled: boolean;
};

export type UserRole = 'admin' | 'contestant';

export type CompetitionStage = 'quarterfinal' | 'semifinal' | 'final';

export type SiteSettings = {
	event_name: string;
	host_name: string;
	sound: SoundSettings;
	scoring: ScoringRules;
};

/** Every point value the operator panel suggests. Saved scores never change with these. */
export type ScoringRules = {
	khoi_dong: {
		individual_correct: number;
		individual_wrong: number;
		common_correct: number;
		common_wrong: number;
		common_floor_zero: boolean;
	};
	vcnv: {
		row_correct: number;
		row_wrong: number;
		center_correct: number;
		center_wrong: number;
		obstacle_by_rows: [number, number, number, number];
		obstacle_after_center: number;
		obstacle_wrong: number;
	};
	tang_toc: { ranks: [number, number, number, number]; wrong: number };
	/** Percentages are applied to the question's tier value. */
	ve_dich: {
		tiers: [number, number];
		correct_percent: number;
		wrong_percent: number;
		star_correct_percent: number;
		star_wrong_percent: number;
		steal_correct_percent: number;
		steal_wrong_percent: number;
		stolen_percent: number;
		stolen_star_percent: number;
	};
	adjust: { down: number; up: number };
};

export type SystemSettings = {
	bind: string;
	port: number;
	secure_cookies: boolean;
	setup_complete: boolean;
};

export type SoundSettings = {
	enabled: boolean;
	master_volume: number;
	music_volume: number;
	effects_volume: number;
	transition_fade_ms: number;
	bell_sound_id: string;
};

export type SoundBus = 'music' | 'effects' | 'timer';
export type Sound = {
	id: string;
	key: string;
	name: string;
	category: string;
	sort_order: number;
	bus: SoundBus;
	volume: number;
	loop: boolean;
	fade_in_ms: number;
	fade_out_ms: number;
	asset_id?: string;
	builtin: boolean;
	enabled: boolean;
	url: string;
	created_at: string;
	updated_at: string;
};

export type SoundUpdate = Pick<
	Sound,
	| 'name'
	| 'category'
	| 'sort_order'
	| 'bus'
	| 'volume'
	| 'loop'
	| 'fade_in_ms'
	| 'fade_out_ms'
	| 'enabled'
>;

export type MatchStatus = 'draft' | 'ready' | 'playing' | 'paused' | 'completed' | 'archived';
export type MatchContestant = { slot: number; user_id: number; name: string; class: string };
export type Match = {
	id: string;
	name: string;
	season: number;
	stage: string;
	status: MatchStatus;
	question_set_id?: string;
	question_set_revision_id?: string;
	version: number;
	result: Record<string, unknown>;
	contestants: MatchContestant[];
	created_at: string;
	updated_at: string;
	selected: boolean;
	archived_at?: string;
};
export type MatchInput = {
	name: string;
	season: number;
	stage: string;
	question_set_id?: string;
	contestants: MatchContestant[];
	version?: number;
};
export type QuestionSet = {
	id: string;
	name: string;
	category: string;
	current_revision_id: string;
	revision: number;
	version: number;
	linked_matches: number;
	created_at: string;
	updated_at: string;
	archived_at?: string;
};
export type QuestionSetRevision = {
	id: string;
	question_set_id: string;
	revision: number;
	data: QuestionSetData;
	created_at: string;
};
export type QuestionType = 'text' | 'image' | 'audio' | 'video';
export type Question = {
	content: string;
	answer?: string;
	type: QuestionType;
	asset_id?: string;
	explanation_asset_id?: string;
	time?: number;
	keyword?: string;
	char_count?: number;
};
export type QuestionSetData = {
	kd_chung: Question[];
	kd_rieng: Question[][];
	vcnv: { obstacle: string; obstacle_asset_id?: string; rows: Question[]; center: Question };
	tt: Question[];
	vd: Array<Record<'20' | '30', Question[]>>;
};
export type MatchEvent = {
	match_id: string;
	sequence: number;
	type: string;
	payload: unknown;
	actor_id?: number;
	client_id?: string;
	created_at: string;
	state?: import('$lib/socket/opsocket').MatchRuntimeState;
};
export type AuditEntry = {
	id: number;
	actor_id?: number;
	action: string;
	entity_type: string;
	entity_id: string;
	match_id?: string;
	before?: unknown;
	after?: unknown;
	reason?: string;
	created_at: string;
};
export type ContestantAccount = User & { fixed_slot: number; password_change_required: boolean };
export type AuthSession = {
	id: string;
	user_id: number;
	username: string;
	role: UserRole;
	fixed_slot?: number;
	ip_address: string;
	user_agent: string;
	created_at: string;
	last_seen_at: string;
	expires_at: string;
	online: boolean;
	current: boolean;
};
export type Asset = {
	id: string;
	sha256: string;
	original_name: string;
	media_type: string;
	size: number;
	created_at: string;
};

// export type Contestant = {
//     id: number;
//     name: string;
//     score: number;
//     is_active: boolean;
// }
