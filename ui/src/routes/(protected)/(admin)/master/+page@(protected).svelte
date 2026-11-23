<script lang="ts">
	import { brand } from '$lib/brand';
	import { onMount, tick } from 'svelte';
	import { api } from '$lib/api';
	import { Logo } from '$lib/components/common';
	import DisplayFrame from '$lib/components/display/display-frame.svelte';
	import { socket } from '$lib/socket';
	import type { MatchEvent } from '$lib/types';

	let runtime = $derived(socket.match.current);
	let season = $derived(runtime?.season);
	let contestants = $derived([1, 2, 3, 4].map((slot) => ({
		slot,
		contestant: runtime?.contestants?.find((contestant) => contestant.slot === slot)
	})));
	let events = $state<MatchEvent[]>([]);
	let entries = $derived(events.map((event, index) => ({
		event,
		source: eventSource(event),
		details: eventDetails(event, events[index - 1])
	})));
	let logView = $state<HTMLDivElement>();
	let loading = $state(true);
	let loadError = $state('');
	let fullscreenError = $state('');
	let error = $derived([loadError, fullscreenError].filter(Boolean).join(' · '));
	let mounted = false;
	let refreshing = false;
	let reloadQueued = false;
	let loadedMatchID = '';
	let matchGeneration = 0;

	onMount(() => {
		mounted = true;
		const unsubscribe = socket.match.onUpdate(() => void loadEvents());
		return () => {
			mounted = false;
			matchGeneration += 1;
			unsubscribe();
		};
	});

	async function loadEvents(): Promise<void> {
		if (!mounted) return;
		const matchID = socket.match.current?.match_id ?? '';
		if (matchID !== loadedMatchID) {
			loadedMatchID = matchID;
			matchGeneration += 1;
			events = [];
			loadError = '';
			loading = Boolean(matchID);
		}
		if (!matchID) {
			loading = false;
			return;
		}
		if (refreshing) {
			reloadQueued = true;
			return;
		}
		refreshing = true;
		do {
			reloadQueued = false;
			const requestedMatchID = loadedMatchID;
			const generation = matchGeneration;
			if (!requestedMatchID) break;
			const isCurrent = () => mounted && generation === matchGeneration
				&& socket.match.current?.match_id === requestedMatchID;
			try {
				const nextEvents = await api.matches.events(requestedMatchID);
				if (!isCurrent()) continue;
				nextEvents.sort((left, right) => left.sequence - right.sequence);
				const hasNewEntries = events.at(-1)?.sequence !== nextEvents.at(-1)?.sequence;
				events = nextEvents;
				loadError = '';
				loading = false;
				if (hasNewEntries) {
					await tick();
					if (isCurrent() && logView) logView.scrollTop = logView.scrollHeight;
				}
			} catch (cause) {
				if (isCurrent()) {
					loadError = cause instanceof Error ? cause.message : 'Không thể tải nhật ký trận đấu';
					loading = false;
				}
			}
		} while (mounted && reloadQueued);
		refreshing = false;
	}

	async function toggleFullscreen(): Promise<void> {
		try {
			if (document.fullscreenElement) await document.exitFullscreen();
			else await document.documentElement.requestFullscreen();
			if (mounted) fullscreenError = '';
		} catch {
			if (mounted) fullscreenError = 'Không thể đổi chế độ toàn màn hình. Nhấn F11 để bật hoặc thoát.';
		}
	}

	function record(value: unknown): Record<string, unknown> {
		return value !== null && typeof value === 'object' && !Array.isArray(value)
			? value as Record<string, unknown> : {};
	}

	function number(value: unknown): number | undefined {
		return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
	}

	function text(value: unknown): string {
		return typeof value === 'string' ? value : '';
	}

	function formatTime(value: unknown, includeUnit = true): string {
		const milliseconds = number(value);
		return milliseconds !== undefined && milliseconds >= 0 ? `${(milliseconds / 1000).toFixed(2)}${includeUnit ? 's' : ''}` : '—';
	}

	function contestantName(event: MatchEvent, slot: unknown): string {
		const players = event.state?.contestants;
		const name = Array.isArray(players) ? players.find((contestant) => contestant.slot === slot)?.name : undefined;
		return name?.trim() ? `TS ${slot} (${name})` : `Thí sinh ${slot ?? '—'}`;
	}

	function eventSource(event: MatchEvent): string {
		if (event.type === 'Contestant.AnswerSubmitted' || event.type === 'Contestant.Rang') {
			return contestantName(event, record(event.payload).slot);
		}
		return event.actor_id ? 'BTC' : 'Hệ thống';
	}

	const phaseNames: Record<string, string> = {
		idle: 'Chờ', main: 'Màn hình chính', score: 'Điểm', khoi_dong: 'Khởi động',
		vcnv: 'Vượt chướng ngại vật', tang_toc: 'Tăng tốc', ve_dich: 'Về đích'
	};
	const screenNames: Record<string, string> = {
		intro: 'Giới thiệu', main: 'Màn hình chính', score: 'Điểm', start: 'Bắt đầu', rule: 'Luật chơi',
		main_kd: 'Khởi động', ques_chung: 'Câu hỏi chung', end_ts: 'Kết thúc lượt thí sinh', end: 'Kết thúc',
		main_vcnv: 'Vượt chướng ngại vật', image_vcnv: 'Hình ảnh chướng ngại vật', ques: 'Câu hỏi',
		solve: 'Đáp án', main_vd: 'Về đích',
		ques_ts1: 'Câu hỏi thí sinh 1', ques_ts2: 'Câu hỏi thí sinh 2',
		ques_ts3: 'Câu hỏi thí sinh 3', ques_ts4: 'Câu hỏi thí sinh 4',
		pre_ques_ts1: 'Chuẩn bị thí sinh 1', pre_ques_ts2: 'Chuẩn bị thí sinh 2',
		pre_ques_ts3: 'Chuẩn bị thí sinh 3', pre_ques_ts4: 'Chuẩn bị thí sinh 4'
	};
	const rowStateNames: Record<string, string> = {
		u: 'Chưa chọn', s: 'Đang chọn', c: 'Đúng', w: 'Sai'
	};

	function controllerDetails(event: MatchEvent, previous?: MatchEvent): string {
		const current = record(event.payload);
		const before = record(previous?.state?.controller);
		const details: string[] = [];
		const changed = (value: unknown, oldValue: unknown) => JSON.stringify(value) !== JSON.stringify(oldValue);
		const field = (key: string, label: string, format: (value: unknown) => string) => {
			if (key in current && changed(current[key], before[key])) details.push(`${label}: ${format(current[key])}`);
		};
		field('phase', 'Phần thi', (value) => phaseNames[text(value)] ?? text(value));
		field('screen', 'Màn hình', (value) => screenNames[text(value)] ?? text(value));
		field('selected_question_id', 'Câu hỏi', (value) => text(value) || 'Chưa chọn');
		field('selected_contestant', 'Thí sinh được chọn', (value) => number(value) ? contestantName(event, value) : 'Chưa chọn');
		field('is_allowing_bell', 'Chuông', (value) => value ? 'Cho phép rung' : 'Đã đóng');
		field('is_showing_question', 'Câu hỏi trên màn hình', (value) => value ? 'Hiện' : 'Ẩn');
		field('answer_status', 'Đánh dấu đáp án', (value) => Array.isArray(value) && value.length
			? value.map((marked, index) => `${index + 1}: ${marked ? 'bật' : 'tắt'}`).join(', ') : 'Đã xóa');

		const obstacle = record(current.vcnv_state);
		const oldObstacle = record(before.vcnv_state);
		if ('has_started' in obstacle && changed(obstacle.has_started, oldObstacle.has_started)) {
			details.push(`Chướng ngại vật: ${obstacle.has_started ? 'Đã bắt đầu' : 'Chưa bắt đầu'}`);
		}
		if ('has_solved' in obstacle && changed(obstacle.has_solved, oldObstacle.has_solved)) {
			details.push(`Chướng ngại vật: ${obstacle.has_solved ? 'Đã giải' : 'Chưa giải'}`);
		}
		if (Array.isArray(obstacle.row_states)) {
			const oldRows = Array.isArray(oldObstacle.row_states) ? oldObstacle.row_states : [];
			obstacle.row_states.forEach((state, index) => {
				if (changed(state, oldRows[index])) details.push(`Hàng ngang ${index + 1}: ${rowStateNames[text(state)] ?? text(state)}`);
			});
		}
		if (Array.isArray(obstacle.image_open_state)) {
			const oldPanels = Array.isArray(oldObstacle.image_open_state) ? oldObstacle.image_open_state : [];
			obstacle.image_open_state.forEach((open, index) => {
				if (changed(open, oldPanels[index])) details.push(`Mảnh ảnh ${index === 4 ? 'trung tâm' : index + 1}: ${open ? 'Mở' : 'Đóng'}`);
			});
		}
		if (Array.isArray(current.ve_dich_contestants_state)) {
			const oldContestants = Array.isArray(before.ve_dich_contestants_state) ? before.ve_dich_contestants_state : [];
			current.ve_dich_contestants_state.forEach((value, index) => {
				const contestant = record(value);
				const oldContestant = record(oldContestants[index]);
				if (changed(contestant.selected_question_set, oldContestant.selected_question_set)) {
					const questions = Array.isArray(contestant.selected_question_set) ? contestant.selected_question_set : [];
					details.push(`Gói câu hỏi TS ${index + 1}: ${questions.length ? questions.map((question) => text(question).replace('_', ' điểm, câu ')).join('; ') : 'Chưa chọn'}`);
				}
				if (changed(contestant.starred_on, oldContestant.starred_on)) {
					details.push(`Ngôi sao hy vọng TS ${index + 1}: ${number(contestant.starred_on) ? `Câu ${contestant.starred_on}` : 'Không dùng'}`);
				}
			});
		}
		return details.length ? details.join('\n') : 'Đã cập nhật điều khiển (không thay đổi trạng thái).';
	}

	function scoreDetails(event: MatchEvent, value: unknown): string {
		const adjustment = record(value);
		const delta = number(adjustment.delta);
		return `${contestantName(event, adjustment.slot)}: ${delta !== undefined ? `${delta > 0 ? '+' : ''}${delta}` : '—'} điểm; tổng ${number(adjustment.score) ?? '—'} điểm`;
	}

	function eventDetails(event: MatchEvent, previous?: MatchEvent): string {
		const payload = record(event.payload);
		const reason = text(payload.reason).trim();
		const withReason = (details: string) => reason ? `${details}\nLý do: ${reason}` : details;
		switch (event.type) {
			case 'Match.Loaded': return `Đã tải trận đấu${event.state?.match_name ? `: ${event.state.match_name}` : ''}.`;
			case 'Match.Unloaded': return text(payload.next_match_id) ? 'Đã rời trận đấu để chuyển sang trận khác.' : 'Đã dỡ trận đấu.';
			case 'Match.Ready': return 'Trận đấu đã sẵn sàng.';
			case 'Match.Playing': return previous?.state?.status === 'paused' ? 'Đã tiếp tục trận đấu.' : 'Đã bắt đầu trận đấu.';
			case 'Match.Paused': return 'Đã tạm dừng trận đấu; đầu vào thí sinh bị khóa.';
			case 'Match.Completed': return 'Đã kết thúc trận đấu.';
			case 'Match.RecoveredPaused': return 'Đã khôi phục và tạm dừng trận đấu sau khi máy chủ khởi động lại.';
			case 'Match.ConfigurationChanged': {
				const players = Array.isArray(payload.contestants) ? payload.contestants.map((value) => {
					const player = record(value);
					return `TS ${player.slot}: ${text(player.name) || 'Chưa có tên'}${text(player.class) ? ` (${player.class})` : ''}`;
				}) : [];
				return `Đã cập nhật cấu hình trận đấu.\nBộ câu hỏi: ${text(payload.question_set_id) || 'Chưa chọn'}${players.length ? `\n${players.join('\n')}` : ''}`;
			}
			case 'Match.QuestionRevisionChanged': return `Đã cập nhật bộ câu hỏi: ${text(payload.question_set_id) || '—'}.`;
			case 'Contestant.AnswerSubmitted': return `${contestantName(event, payload.slot)} gửi câu trả lời: ${text(payload.answer)}\nThời gian: ${formatTime(payload.time_ms)}.`;
			case 'Contestant.Rang': return `${contestantName(event, payload.slot)} đã rung chuông. Thời gian: ${formatTime(payload.time_ms)}.`;
			case 'Contestant.LockChanged': return `${contestantName(event, payload.slot)}: ${payload.locked ? 'Đã khóa' : 'Đã mở khóa'} đầu vào.`;
			case 'Match.ScoreAdjusted': return withReason(scoreDetails(event, payload));
			case 'Match.ScoresAdjusted': return withReason(Array.isArray(payload.adjustments)
				? payload.adjustments.map((value) => scoreDetails(event, value)).join('\n') : 'Đã điều chỉnh điểm.');
			case 'Timer.Started': return `Bắt đầu tính giờ: ${formatTime(payload.duration_ms)}.`;
			case 'Timer.Paused': return `Đã tạm dừng tính giờ. Còn lại: ${formatTime(payload.remaining_ms)}.`;
			case 'Timer.Resumed': return `Tiếp tục tính giờ. Còn lại: ${formatTime(payload.remaining_ms)}.`;
			case 'Timer.Reset': return 'Đã đặt lại đồng hồ.';
			case 'Timer.Finished': return 'Hết thời gian.';
		}
		if (event.type.startsWith('Controller.')) return controllerDetails(event, previous);
		const values = Object.entries(payload).filter(([, value]) => ['string', 'number', 'boolean'].includes(typeof value));
		return `Sự kiện ${event.type}${values.length ? `: ${values.map(([key, value]) => `${key}: ${value}`).join('; ')}` : ''}`;
	}
</script>

<svelte:head><title>Trang MC | BTC {brand.appName}</title></svelte:head>

<DisplayFrame fullScreen interactive>
	<section class="mc-display" aria-label="Trang MC">
		<header class="mc-header">
			<button type="button" class="fullscreen-logo" aria-label="Bật hoặc thoát toàn màn hình" title="Toàn màn hình" onclick={toggleFullscreen}>
				<Logo color="blue" class="h-10 w-auto" />
			</button>
			<h1>TRANG MC - {brand.eventName.toUpperCase()}{season !== undefined ? ` MÙA ${season}` : ''}</h1>
			{#if runtime?.status === 'paused'}<p class="paused" role="status">Trận đấu đang tạm dừng</p>{/if}
			{#if error}<p class="load-error" role="alert">{error}</p>{/if}
		</header>

		<div class="log-panel">
			<!-- Keyboard users must be able to scroll the log region. -->
			<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
			<div class="logs" bind:this={logView} tabindex="0" role="region" aria-label="Nhật ký trận đấu" aria-busy={loading}>
				<table class="log-table">
					<colgroup><col class="source-column" /><col /></colgroup>
					<thead><tr><th scope="col">Đến từ</th><th scope="col">Nội dung</th></tr></thead>
					<tbody>
						{#each entries as entry, index (`${entry.event.match_id}-${entry.event.sequence}`)}
							<tr class:system={!entry.event.actor_id} class:latest={index === entries.length - 1} title={`${entry.event.created_at} · Sự kiện ${entry.event.sequence}`}>
								<td class="source">{entry.source}</td><td class="details">{entry.details}</td>
							</tr>
						{:else}
							<tr><td colspan="2" class="empty-log">{!runtime ? 'Chưa chọn trận đấu.' : loading ? 'Đang tải nhật ký trận đấu…' : loadError ? 'Nhật ký chưa tải được. Dữ liệu sẽ được tải lại khi trận đấu cập nhật.' : 'Chưa có sự kiện.'}</td></tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>

		<div class="contestant-panel">
			<table class="contestant-table" aria-label="Câu trả lời và điểm của bốn thí sinh">
				<colgroup><col class="name-column" /><col /><col class="time-column" /><col class="score-column" /></colgroup>
				<thead><tr><th scope="col">TÊN THÍ SINH</th><th scope="col">CÂU TRẢ LỜI</th><th scope="col" title="Thời gian trả lời (giây)">THỜI GIAN</th><th scope="col">ĐIỂM</th></tr></thead>
				<tbody>
					{#each contestants as row (row.slot)}
						<tr>
							<th scope="row" title={row.contestant ? `${row.contestant.name || `Thí sinh ${row.slot}`} - Lớp ${row.contestant.class || '—'}` : `Thí sinh ${row.slot}`}>
								<div class="name-content">
									<span>{row.contestant?.name || `Thí sinh ${row.slot}`}</span>
									{#if row.contestant?.has_rung}
										<span class="bell" title="Đã rung chuông">
											<svg aria-hidden="true" viewBox="0 0 448 512"><path d="M224 0c-17.7 0-32 14.3-32 32l0 19.2C119 66 64 130.6 64 208l0 18.8c0 47-17.3 92.4-48.5 127.6l-7.4 8.3c-8.4 9.4-10.4 22.9-5.3 34.4S19.4 416 32 416l384 0c12.6 0 24-7.4 29.2-18.9s3.1-25-5.3-34.4l-7.4-8.3C401.3 319.2 384 273.9 384 226.8l0-18.8c0-77.4-55-142-128-156.8L256 32c0-17.7-14.3-32-32-32zm45.3 493.3c12-12 18.7-28.3 18.7-45.3l-64 0-64 0c0 17 6.7 33.3 18.7 45.3s28.3 18.7 45.3 18.7s33.3-6.7 45.3-18.7z" /></svg>
											<span class="sr-only">Đã rung chuông</span>
										</span>
									{/if}
									{#if row.contestant?.locked}<span class="locked">Đã khóa</span>{/if}
									{#if !row.contestant}<span class="missing-player">{runtime ? 'Chưa có thí sinh' : 'Chưa chọn trận đấu'}</span>{/if}
								</div>
							</th>
							<td>
								<!-- Keyboard users must be able to scroll long answers. -->
								<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
								<div class="answer" role="region" aria-label={`Câu trả lời thí sinh ${row.slot}`} tabindex={row.contestant?.answer ? 0 : undefined}>{row.contestant ? row.contestant.answer?.trim() ? row.contestant.answer : '_' : '—'}</div>
							</td>
							<td class="time">{formatTime(row.contestant?.time_ms, false)}</td>
							<td class="score">{row.contestant?.score ?? '—'}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
</DisplayFrame>

<style>
	.mc-display {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 2fr);
		grid-template-rows: 50px minmax(0, 1fr);
		width: 1920px;
		height: 1080px;
		box-sizing: border-box;
		overflow: hidden;
		border: 3px solid #9ca3af;
		background: #fff;
		color: #111827;
	}
	.mc-header {
		grid-column: 1 / -1;
		display: flex;
		align-items: center;
		gap: 32px;
		min-width: 0;
		border: 3px solid #9ca3af;
		padding: 0 8px;
		font-size: 20px;
		font-weight: 600;
	}
	.mc-header h1 { flex-shrink: 0; }
	.fullscreen-logo { display: flex; flex-shrink: 0; align-items: center; cursor: pointer; }
	.fullscreen-logo:active { opacity: 0.7; }
	.fullscreen-logo:focus-visible, .logs:focus-visible, .answer:focus-visible { outline: 3px solid #2563eb; outline-offset: -3px; }
	.paused { color: #9a3412; font-size: 16px; }
	.load-error { margin-left: auto; min-width: 0; color: #b91c1c; font-size: 14px; line-height: 1.2; overflow-wrap: anywhere; }
	.log-panel, .contestant-panel { min-width: 0; min-height: 0; overflow: hidden; border: 3px solid #9ca3af; }
	.logs { width: 100%; height: 100%; overflow: auto; overscroll-behavior: contain; }
	.log-table { width: 100%; border-collapse: collapse; table-layout: fixed; font-size: 18px; line-height: 1.5; }
	.source-column { width: 144px; }
	.log-table th { position: sticky; top: 0; z-index: 1; border-bottom: 2px solid #d1d5db; background: #fff; padding: 12px; text-align: left; font-weight: 700; }
	.log-table td { border-bottom: 1px solid #e5e7eb; padding: 12px; vertical-align: top; overflow-wrap: anywhere; }
	.log-table .system { background: #fef2f2; }
	.log-table .system .source { color: #b91c1c; }
	.log-table .latest { background: #f0fdf4; }
	.source { font-weight: 600; }
	.details { white-space: pre-wrap; }
	.empty-log { color: #4b5563; }
	.contestant-table { width: 100%; height: 100%; border-collapse: collapse; table-layout: fixed; font-size: 40px; }
	.name-column { width: calc((100% - 320px) / 3); }
	.time-column, .score-column { width: 160px; }
	.contestant-table thead { height: 40px; }
	.contestant-table thead th { border-bottom: 2px solid #d1d5db; padding: 4px 12px; color: #4b5563; font-size: 18px; font-weight: 700; text-align: left; }
	.contestant-table thead th:nth-child(n + 3) { text-align: center; }
	.contestant-table tbody tr { height: 244px; }
	.contestant-table tbody th, .contestant-table tbody td { border-bottom: 2px solid #d1d5db; padding: 12px; vertical-align: middle; }
	.name-content { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; max-height: 210px; overflow: auto; overflow-wrap: anywhere; text-align: left; font-size: 36px; font-weight: 600; }
	.bell { display: inline-flex; flex-shrink: 0; }
	.bell svg { width: 36px; height: 40px; fill: #dc2626; }
	.locked, .missing-player { font-size: 18px; font-weight: 500; color: #4b5563; }
	.missing-player { width: 100%; }
	.answer { max-height: 210px; overflow: auto; overscroll-behavior: contain; overflow-wrap: anywhere; white-space: pre-wrap; line-height: 1.3; }
	.time, .score { text-align: center; font-family: ui-monospace, monospace; font-variant-numeric: tabular-nums; }
	.contestant-table .time { padding: 0 4px; font-size: 40px; }
	.contestant-table .score { font-size: 60px; font-weight: 700; }
</style>
