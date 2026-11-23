import type { MatchEvent } from './types';

export type LogView = 'match' | 'operations' | 'technical';
export type LogEntry = {
	id: string; sequence: number; event: MatchEvent; title: string; detail?: string; answer?: string;
	views: Array<'match' | 'operations'>; phase: string; phaseLabel: string;
	questionID: string; questionLabel: string; groupID: string; groupLabel: string;
	slots: number[]; important: boolean;
};
export type LogGroup = { id: string; label: string; entries: LogEntry[] };
type RecordValue = Record<string, unknown>;
const record = (value: unknown): RecordValue => value !== null && typeof value === 'object' && !Array.isArray(value) ? value as RecordValue : {};
const array = (value: unknown): unknown[] => Array.isArray(value) ? value : [];
const text = (value: unknown): string => typeof value === 'string' ? value : '';
const number = (value: unknown): number => typeof value === 'number' && Number.isFinite(value) ? value : 0;
const phases: Record<string, string> = { idle: 'Chờ', main: 'Tổng quan', score: 'Điểm số', khoi_dong: 'Khởi động', vcnv: 'Vượt chướng ngại vật', tang_toc: 'Tăng tốc', ve_dich: 'Về đích' };
const screens: Record<string, string> = { intro: 'Giới thiệu', main: 'Tổng quan', score: 'Bảng điểm', start: 'Mở đầu', rule: 'Luật chơi', main_kd: 'Khởi động', ques_chung: 'Câu hỏi chung', end_ts: 'Kết thúc lượt', end: 'Kết thúc', main_vcnv: 'Vượt chướng ngại vật', image_vcnv: 'Hình ảnh chướng ngại vật', ques: 'Câu hỏi', solve: 'Giải chướng ngại vật', answers_tt: 'Xem câu trả lời Tăng tốc', answers_vcnv: 'Xem câu trả lời VCNV', main_vd: 'Về đích' };
const screenLabel = (value: unknown, phase: unknown): string => {
	const id = text(value);
	if (id === 'solve') return phase === 'vcnv' ? 'Giải chướng ngại vật' : 'Hiện đáp án';
	const slot = /^(pre_)?ques_ts([1-4])$/.exec(id);
	return slot ? `${slot[1] ? 'Chuẩn bị' : 'Câu hỏi'} thí sinh ${slot[2]}` : screens[id] || id || '—';
};
function questionLabel(id: string): string {
	const p = id.split('.');
	if (!/^(kd_chung\.\d+|kd_rieng\.\d+\.\d+|tt\.\d+|vcnv\.(center|rows\.\d+)|vd\.\d+\.(20|30)\.\d+)$/.test(id)) return id;
	const index = (s: string | undefined) => String(number(Number(s)) + 1);
	switch (p[0]) {
		case 'kd_chung': return `Khởi động chung · Câu ${index(p[1])}`;
		case 'kd_rieng': return `Khởi động riêng · Thí sinh ${index(p[1])} · Câu ${index(p[2])}`;
		case 'tt': return `Tăng tốc · Câu ${index(p[1])}`;
		case 'vcnv': return p[1] === 'center' ? 'VCNV · Câu trung tâm' : `VCNV · Hàng ${index(p[2])}`;
		case 'vd': return `Về đích · Thí sinh ${index(p[1])} · Gói ${p[2] || '—'} · Câu ${index(p[3])}`;
		default: return id;
	}
}
const seconds = (value: unknown) => `${(number(value) / 1000).toLocaleString('vi-VN', { maximumFractionDigits: 3 })} giây`;
const equal = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

export function buildLogEntries(events: readonly MatchEvent[]): LogEntry[] {
	let previous: RecordValue = {};
	let previousController: RecordValue = {};
	let context = '';
	let groupID = '';
	let attemptStarted = false;
	return events.map((event, position) => {
		const payload = record(event.payload);
		const hasState = event.state != null;
		const state = hasState ? record(event.state) : previous;
		const controller = hasState ? record(state.controller) : event.type.startsWith('Controller.') && 'phase' in payload ? payload : previousController;
		const phase = text(controller.phase);
		const selectedQuestionID = text(controller.selected_question_id);
		const questionPhases: Record<string, string> = { kd_chung: 'khoi_dong', kd_rieng: 'khoi_dong', vcnv: 'vcnv', tt: 'tang_toc', vd: 've_dich' };
		let questionID = questionPhases[selectedQuestionID.split('.')[0]] === phase ? selectedQuestionID : '';
		// A fast judgment snapshot already shows the next question (or end card).
		// Group the score with the question actually judged, from its event.
		if (phase === 'khoi_dong' && event.type === 'KhoiDong.Judged' &&
			/^kd_(?:chung\.\d+|rieng\.[0-3]\.\d+)$/.test(text(payload.question_id)))
			questionID = text(payload.question_id);
		if (phase === 'vcnv' && event.type === 'Controller.VCNVRowStateChanged') {
			const action = record(payload.action);
			if (Number.isInteger(action.row) && number(action.row) >= 1 && number(action.row) <= 4) {
				questionID = `vcnv.rows.${number(action.row) - 1}`;
			} else {
				const rows = array(record(controller.vcnv_state).row_states);
				const oldRows = array(record(previousController.vcnv_state).row_states);
				const changedRows = rows.flatMap((value, i) => i < 4 && oldRows[i] !== undefined && value !== oldRows[i] ? [i] : []);
				if (changedRows.length === 1) questionID = `vcnv.rows.${changedRows[0]}`;
			}
		}
		const phaseLabel = phases[phase] || phase || 'Trận đấu';
		const qLabel = questionID ? questionLabel(questionID) : '';
		const nextContext = `${event.match_id}|${text(state.question_revision_id)}|${phase}|${questionID}`;
		const boundary = /^(Match\.(Loaded|Unloaded|RecoveredPaused|Reset)|Controller\.Reset)/.test(event.type);
		if (context !== nextContext || boundary || event.type === 'Timer.Started' && attemptStarted) {
			groupID = `${event.match_id}:${event.sequence}:${position}`;
			attemptStarted = false;
		}
		context = nextContext;
		if (event.type === 'Timer.Started') attemptStarted = true;
		const entry: LogEntry = { id: `${event.match_id}:${event.sequence}:${position}`, sequence: event.sequence, event, title: event.type, views: [], phase, phaseLabel, questionID, questionLabel: qLabel, groupID, groupLabel: qLabel || phaseLabel, slots: [], important: false };
		const contestants = array(state.contestants).map(record);
		const who = (slot: unknown) => {
			const n = number(slot);
			if (n > 0 && !entry.slots.includes(n)) entry.slots.push(n);
			return text(contestants.find(c => c.slot === n)?.name) || `Thí sinh ${n || '—'}`;
		};
		const both = () => { entry.views = ['match', 'operations']; };
		const type = event.type;
		if (type.startsWith('Controller.')) {
			const changes: string[] = [];
			const titles: string[] = [];
			let meaningful = false;
			const changed = (key: string) => key in controller && !equal(previousController[key], controller[key]);
			const add = (title: string, detail = title, match = false) => { titles.push(title); changes.push(detail); meaningful ||= match; };
			const reset = changed('phase') || changed('selected_question_id') || /Reset/.test(type);
			if (changed('phase')) add(`Chuyển phần thi: ${phaseLabel}`, `Phần thi: ${phases[text(previousController.phase)] || text(previousController.phase) || '—'} → ${phaseLabel}`, true);
			if (changed('screen')) add(`Chuyển màn hình: ${screenLabel(controller.screen, phase)}`, `Màn hình: ${screenLabel(previousController.screen, previousController.phase)} → ${screenLabel(controller.screen, phase)}`, text(controller.screen).startsWith('answers') || controller.screen === 'solve');
			if (changed('selected_question_id')) add(questionID ? `Chọn ${qLabel}` : 'Bỏ chọn câu hỏi');
			if (changed('selected_contestant')) add(`Chọn lượt: ${who(controller.selected_contestant)}`);
			if (changed('is_showing_question')) add(controller.is_showing_question ? 'Hiện câu hỏi' : 'Ẩn câu hỏi');
			if (changed('is_allowing_bell')) add(controller.is_allowing_bell ? 'Mở chuông' : 'Đóng chuông');
			if (!reset) array(controller.answer_status).forEach((correct, i) => {
				if (correct !== array(previousController.answer_status)[i]) add(`Chấm ${who(i + 1)}: ${correct ? 'đúng' : 'sai'}`, `${who(i + 1)}: ${array(previousController.answer_status)[i] === undefined ? 'chưa ghi nhận' : array(previousController.answer_status)[i] ? 'đúng' : 'sai'} → ${correct ? 'đúng' : 'sai'}`, true);
			});
			const vcnv = record(controller.vcnv_state), oldVCNV = record(previousController.vcnv_state);
			for (const [key, label] of [['has_started', 'Bắt đầu VCNV'], ['has_solved', 'Giải chướng ngại vật']]) {
				if (key in vcnv && !equal(vcnv[key], oldVCNV[key]) && !(reset && vcnv[key] === false)) add(`${label}: ${vcnv[key] ? 'có' : 'không'}`, undefined, true);
			}
			array(vcnv.row_states).forEach((value, i) => {
				const rowLabels: Record<string, string> = { u: 'chưa chọn', s: 'đã chọn', c: 'đúng', w: 'sai' };
				if (value !== array(oldVCNV.row_states)[i] && !(reset && value === 'u')) add(`VCNV hàng ${i + 1}: ${rowLabels[text(value)] || text(value)}`, undefined, value === 'c' || value === 'w');
			});
			array(vcnv.image_open_state).forEach((value, i) => {
				if (value !== array(oldVCNV.image_open_state)[i] && !(reset && value === false)) add(`VCNV mảnh hình ${i + 1}: ${value ? 'mở' : 'đóng'}`);
			});
			array(controller.ve_dich_contestants_state).forEach((value, i) => {
				const current = record(value), old = record(array(previousController.ve_dich_contestants_state)[i]);
				if ('selected_question_set' in current && !equal(current.selected_question_set, old.selected_question_set) && !(reset && array(current.selected_question_set).length === 0)) add(`${who(i + 1)} chọn gói Về đích`, `Gói ${who(i + 1)}: ${array(old.selected_question_set).map(text).join(', ') || '—'} → ${array(current.selected_question_set).map(text).join(', ') || '—'}`);
				if ('starred_on' in current && current.starred_on !== old.starred_on && !(reset && number(current.starred_on) === 0)) add(`${who(i + 1)}: ${number(current.starred_on) ? `ngôi sao câu ${number(current.starred_on)}` : 'bỏ ngôi sao'}`, undefined, true);
			});
			for (const key of ['countdown', 'countdown_ms', 'is_counting_down']) if (changed(key)) add('Đổi đếm ngược', `${key}: ${typeof controller[key] === 'boolean' ? controller[key] ? 'bật' : 'tắt' : number(controller[key])}`);
			entry.title = titles[0] || 'Đồng bộ điều khiển (không thay đổi)';
			entry.detail = changes.join('\n') || undefined;
			entry.views = changes.length ? meaningful ? ['match', 'operations'] : ['operations'] : [];
			const action = record(payload.action);
			if (type === 'Controller.AnswerStatusChanged' && Number.isInteger(action.slot) && number(action.slot) >= 1 && number(action.slot) <= 4 && typeof action.correct === 'boolean') {
				entry.title = `Chấm ${who(action.slot)}: ${action.correct ? 'đúng' : 'sai'}`;
				if (!changes.length) entry.detail = 'Xác nhận lại kết quả chấm; trạng thái không thay đổi.';
				both();
			} else if (type === 'Controller.VCNVRowStateChanged' && Number.isInteger(action.row) && number(action.row) >= 1 && number(action.row) <= 4 && ['u', 's', 'c', 'w'].includes(text(action.state))) {
				const rowLabels: Record<string, string> = { u: 'chưa chọn', s: 'đã chọn', c: 'đúng', w: 'sai' };
				entry.title = `VCNV hàng ${number(action.row)}: ${rowLabels[text(action.state)]}`;
				if (!changes.length) entry.detail = 'Xác nhận lại trạng thái hàng; trạng thái không thay đổi.';
				entry.views = action.state === 'c' || action.state === 'w' ? ['match', 'operations'] : ['operations'];
			} else if (type === 'Controller.VCNVSolved' && typeof action.solved === 'boolean') {
				entry.title = `Giải chướng ngại vật: ${action.solved ? 'có' : 'không'}`;
				if (!changes.length) entry.detail = 'Xác nhận lại trạng thái giải chướng ngại vật; trạng thái không thay đổi.';
				both();
			} else if (!changes.length) {
				const confirmations: Record<string, string> = {
					'Controller.AnswerStatusChanged': 'Xác nhận trạng thái chấm câu trả lời',
					'Controller.VCNVRowStateChanged': 'Xác nhận trạng thái hàng VCNV',
					'Controller.VCNVSolved': 'Xác nhận trạng thái giải chướng ngại vật',
					'Controller.VCNVStarted': 'Xác nhận trạng thái bắt đầu VCNV',
					'Controller.VCNVPanelChanged': 'Xác nhận trạng thái mảnh hình VCNV',
					'Controller.VeDichQuestionSetSelected': 'Xác nhận gói câu hỏi Về đích',
					'Controller.VeDichStarChanged': 'Xác nhận ngôi sao Về đích'
				};
				if (confirmations[type]) {
					entry.title = confirmations[type];
					entry.detail = type === 'Controller.AnswerStatusChanged' ? 'Trạng thái chấm không thay đổi.' : 'Trạng thái không thay đổi.';
					entry.views = ['Controller.AnswerStatusChanged', 'Controller.VCNVRowStateChanged', 'Controller.VCNVSolved'].includes(type) ? ['match', 'operations'] : ['operations'];
				}
			}
			if (/Reset/.test(type)) { entry.title = 'Đặt lại điều khiển'; entry.important = true; both(); }
		} else if (type === 'Contestant.AnswerSubmitted' || type === 'Contestant.Rang') {
			entry.title = `${who(payload.slot)} ${type.endsWith('Rang') ? 'rung chuông' : 'gửi câu trả lời'}`;
			entry.detail = `Thời gian: ${seconds(payload.time_ms)}`;
			if (type.endsWith('AnswerSubmitted') && typeof payload.answer === 'string') entry.answer = payload.answer;
			entry.views = ['match'];
		} else if (type === 'KhoiDong.Judged') {
			const slot = number(payload.slot);
			const delta = number(payload.delta);
			const verdict = text(payload.verdict) === 'correct' ? 'đúng' : 'sai';
			if (slot >= 1 && slot <= 4) {
				const next = number(contestants.find(c => c.slot === slot)?.score);
				entry.title = `Chấm ${who(slot)}: ${verdict}`;
				entry.detail = `${qLabel}\nĐiểm: ${next - delta} → ${next} (${delta >= 0 ? '+' : ''}${delta})`;
			} else {
				entry.title = 'Không có thí sinh trả lời';
				entry.detail = `${qLabel}\nKhông thay đổi điểm.`;
			}
			both(); entry.important = true;
		} else if (type === 'Match.ScoreAdjusted' || type === 'Match.ScoresAdjusted') {
			const adjustments = type === 'Match.ScoresAdjusted' ? array(payload.adjustments).map(record) : [payload];
			entry.title = 'Điều chỉnh điểm';
			entry.detail = adjustments.map(a => {
				const next = typeof a.score === 'number' ? a.score : number(contestants.find(c => c.slot === a.slot)?.score);
				const delta = number(a.delta);
				return `${who(a.slot)}: ${next - delta} → ${next} (${delta >= 0 ? '+' : ''}${delta})`;
			}).join('\n') + (text(payload.reason) ? `\nLý do: ${text(payload.reason)}` : '');
			both(); entry.important = true;
		} else if (type === 'Contestant.LockChanged') {
			entry.title = `${payload.locked ? 'Khóa' : 'Mở khóa'} đầu vào: ${who(payload.slot)}`;
			entry.views = ['operations'];
			if (!payload.locked) { entry.detail = 'Xóa câu trả lời, lượt chuông và thời gian đã ghi nhận của thí sinh.'; entry.important = true; both(); }
		} else if (type === 'Contestant.AnswersCleared') {
			entry.title = 'Xóa tất cả đáp án thí sinh';
			entry.detail = 'Xóa câu trả lời và thời gian trả lời đã ghi nhận của cả 4 thí sinh.';
			both(); entry.important = true;
		} else if (type === 'Contestant.BellsCleared') {
			entry.title = 'Xóa tất cả chuông thí sinh';
			entry.detail = 'Xóa lượt bấm chuông và mở lại chuông cho cả 4 thí sinh.';
			both(); entry.important = true;
		} else if (type.startsWith('Timer.')) {
			const labels: Record<string, string> = { Started: 'Bắt đầu thời gian trả lời', Paused: 'Tạm dừng thời gian', Resumed: 'Tiếp tục thời gian', Finished: 'Hết giờ', Reset: 'Đặt lại đồng hồ' };
			const action = type.slice(6);
			entry.title = labels[action] || type;
			if (labels[action]) { both(); entry.important = action === 'Reset'; }
			const timer = Object.keys(payload).length ? payload : record(state.timer);
			if (action === 'Started') entry.detail = `Thời lượng: ${seconds(timer.duration_ms)}`;
			if (action === 'Paused' || action === 'Resumed') entry.detail = `Còn lại: ${seconds(timer.remaining_ms)}`;
		} else {
			const labels: Record<string, string> = {
				'Match.Playing': previous.status === 'paused' ? 'Tiếp tục trận đấu' : 'Bắt đầu trận đấu',
				'Match.Paused': 'Tạm dừng trận đấu', 'Match.Ready': 'Trận đấu sẵn sàng', 'Match.Draft': 'Chuyển về bản nháp', 'Match.Completed': 'Hoàn tất trận đấu', 'Match.Archived': 'Lưu trữ trận đấu',
				'Match.Loaded': 'Nạp trận đấu', 'Match.Unloaded': 'Dỡ trận đấu', 'Match.RecoveredPaused': 'Khôi phục trận đấu ở trạng thái tạm dừng',
				'Match.ConfigurationChanged': 'Cập nhật cấu hình trận đấu', 'Match.QuestionRevisionChanged': 'Cập nhật phiên bản bộ câu hỏi',
				'Match.Reset': 'Đặt lại trận đấu', 'Match.ResultAmended': 'Đính chính kết quả', 'Match.ResultFinalized': 'Chốt kết quả'
			};
			if (labels[type]) {
				entry.title = labels[type]; both();
				entry.important = /Reset|Recovered|Amended|Completed|Unloaded|RevisionChanged/.test(type);
				if (text(payload.reason)) entry.detail = `Lý do: ${text(payload.reason)}`;
				if (type === 'Match.QuestionRevisionChanged') entry.detail = `Phiên bản: ${text(payload.revision_id) || text(payload.question_revision_id) || '—'}`;
				if (type === 'Match.ConfigurationChanged') entry.detail = `Bộ câu hỏi: ${text(payload.question_set_id) || '—'}\nThí sinh: ${array(payload.contestants).map(record).map(c => `${number(c.slot)}: ${text(c.name)}`).join(', ') || '—'}`;
			}
		}
		if (/error|fail|reject|abort/i.test(type)) { entry.important = true; both(); entry.title = `Sự cố: ${type}`; }
		if (hasState) previous = state;
		previousController = controller;
		return entry;
	});
}

export function groupLogEntries(entries: readonly LogEntry[]): LogGroup[] {
	const groups: LogGroup[] = [];
	for (const entry of entries) {
		let group = groups[groups.length - 1];
		if (!group || group.id !== entry.groupID) {
			group = { id: entry.groupID, label: entry.groupLabel, entries: [] };
			groups.push(group);
		}
		group.entries.push(entry);
	}
	return groups;
}
