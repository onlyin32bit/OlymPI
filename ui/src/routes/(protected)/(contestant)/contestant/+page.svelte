<script lang="ts">
	import { brand } from '$lib/brand';
	import { tick } from 'svelte';
	import { api } from '$lib/api';
	import { Logo } from '$lib/components/common';
	import { Display } from '$lib/components/display';
	import DisplayFrame from '$lib/components/display/display-frame.svelte';
	import { socket } from '$lib/socket';
	import { canRingBell } from '$lib/control/bell-rules';
	import BellRing from '@lucide/svelte/icons/bell-ring';
	import BellOff from '@lucide/svelte/icons/bell-off';

	let answer = $state('');
	let busy = $state(false);
	let bellBusy = $state(false);
	let bellError = $state('');
	let error = $state('');
	let inputError = $derived(error || bellError);
	let answerInput = $state<HTMLInputElement>();
	let draftKey = $state('');
	let pendingRequest = 0;
	let pendingBellRequest = 0;
	let activeBellKey = $state('');
	let runtime = $derived(socket.match.current);
	let me = $derived(runtime?.contestants.find((value) => value.slot === api.auth.user?.fixed_slot));
	let contestants = $derived(Array.from({ length: 4 }, (_, index) =>
		runtime?.contestants.find((value) => value.slot === index + 1)
	));
	let remaining = $derived(socket.timer.remaining);
	let elapsed = $derived(socket.timer.elapsed);
	let questionKey = $derived(runtime
		? `${runtime.match_id}:${runtime.epoch}:${runtime.input_epoch}`
		: '');
	let bellKey = $derived(runtime ? `${runtime.match_id}:${runtime.epoch}:${runtime.bell_epoch}` : '');
	let questionScreen = $derived(runtime?.controller.screen === 'ques');
	let answerPhase = $derived(runtime?.controller.phase === 'vcnv' || runtime?.controller.phase === 'tang_toc');
	let canAnswer = $derived(Boolean(me) && runtime?.status === 'playing' && answerPhase && questionScreen
		&& runtime.controller.is_showing_question && Boolean(runtime.current_question)
		&& runtime.timer.is_running && !runtime.timer.is_paused && remaining > 0 && !me?.locked);
	let canRing = $derived(canRingBell(runtime, me?.slot ?? 0, remaining));
	let showAnswers = $derived((runtime?.controller.phase === 'tang_toc' && runtime.controller.screen === 'answers_tt')
		|| (runtime?.controller.phase === 'vcnv' && runtime.controller.screen === 'answers_vcnv'));
	let season = $derived(runtime?.season);
	let status = $derived(!runtime ? 'Chưa chọn trận đấu'
		: !me ? 'Chưa có thí sinh ở vị trí này'
		: runtime.status === 'paused' ? 'Trận đấu đang tạm dừng'
		: runtime.status !== 'playing' ? 'Chờ bắt đầu trận đấu'
		: me.locked ? (me.answer ? 'Đã gửi câu trả lời' : 'Đã khóa đầu vào') : '');

	$effect(() => {
		if (draftKey === questionKey) return;
		draftKey = questionKey;
		pendingRequest += 1;
		busy = false;
		answer = '';
		error = '';
	});
	$effect(() => {
		if (activeBellKey === bellKey) return;
		activeBellKey = bellKey;
		pendingBellRequest += 1;
		bellBusy = false;
		bellError = '';
	});
	$effect(() => {
		if (canAnswer && !busy) {
			void tick().then(() => {
				if (canAnswer && !busy) answerInput?.focus({ preventScroll: true });
			});
		}
	});

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (busy || !canAnswer) return;
		const value = answer.trim();
		if (!value) {
			error = 'Chưa nhập câu trả lời';
			return;
		}
		const key = questionKey;
		const request = ++pendingRequest;
		busy = true;
		error = '';
		try {
			await socket.contestant.submitAnswer(value.toUpperCase());
			if (questionKey === key && pendingRequest === request) answer = '';
		} catch (cause) {
			if (questionKey === key && pendingRequest === request) error = cause instanceof Error ? cause.message : 'Không thể gửi câu trả lời';
		} finally {
			if (questionKey === key && pendingRequest === request) busy = false;
		}
	}

	async function ring(): Promise<void> {
		if (bellBusy || !canRing) return;
		const key = bellKey;
		const request = ++pendingBellRequest;
		bellBusy = true;
		bellError = '';
		try {
			await socket.contestant.ring();
		} catch (cause) {
			if (bellKey === key && pendingBellRequest === request)
				bellError = cause instanceof Error ? cause.message : 'Không thể bấm chuông';
		} finally {
			if (bellKey === key && pendingBellRequest === request) bellBusy = false;
		}
	}

	async function toggleFullscreen(): Promise<void> {
		try {
			if (document.fullscreenElement) await document.exitFullscreen();
			else await document.documentElement.requestFullscreen();
		} catch {
			error = 'Không thể bật toàn màn hình. Nhấn F11 để sử dụng toàn màn hình của trình duyệt.';
		}
	}

	function inputPlaceholder(): string {
		if (!runtime) return 'Chờ chọn trận đấu';
		if (runtime.status === 'paused') return 'Trận đấu đang tạm dừng';
		if (me?.locked) return me.answer ? 'Đã gửi câu trả lời' : 'Đầu vào đã khóa';
		if (!canAnswer) return 'Chờ mở câu hỏi và bắt đầu thời gian';
		return 'Nhập câu trả lời của bạn, ENTER để gửi';
	}
</script>

<svelte:head><title>Màn hình thí sinh - {brand.appName}</title></svelte:head>

<DisplayFrame fullScreen interactive>
	<main class="contestant-station" aria-label="Màn hình thí sinh">
		<header class="station-header">
			<button class="fullscreen-logo" type="button" onclick={toggleFullscreen} aria-label="Bật hoặc thoát toàn màn hình" title="Toàn màn hình (hoặc F11)">
				<Logo color="blue" class="h-12" />
			</button>
			<h1>TRANG THÍ SINH - {brand.eventName.toUpperCase()}{season ? ` MÙA ${season}` : ''}</h1>
			<div class="identity">Thí sinh hiện tại: {me?.name ?? '—'}{me?.class ? ` - ${me.class}` : ''}</div>
			{#if canRing}<BellRing size={28} aria-label="Chuông đang mở" />{:else}<BellOff size={28} aria-label="Chuông đang khóa" />{/if}
		</header>
		<div class="contestant-strip" aria-label="Điểm và trạng thái chuông">
			{#each contestants as contestant, index (index)}
				<div class:has-rung={contestant?.has_rung}>
					<span class="contestant-name">{contestant?.name || `Thí sinh ${index + 1}`}</span>
					{#if contestant?.has_rung}<BellRing size={28} aria-label="Đã bấm chuông" />{/if}
					<span class="strip-value">{showAnswers ? contestant?.answer || '—' : contestant?.score ?? 0}</span>
				</div>
			{/each}
		</div>
		<div class="station-body">
			<div class="display-and-answer">
				<div class="live-display" aria-label="Màn hình chương trình"><Display /></div>
				<div class="sent-answer" aria-live="polite">
					<span class="answer-label">Câu trả lời đã gửi:</span>
					<strong class="answer-value">{me?.answer || '—'}</strong>
					<span class="answer-time">{((me?.time_ms ?? 0) / 1000).toFixed(3)}s</span>
				</div>
			</div>
			<aside class="timer-and-bell">
				<div class="station-timer" aria-label="Thời gian">
					<span>Thời gian:</span>
					{#if runtime?.status === 'paused' || runtime?.timer.is_paused}
						<strong class="timer-message">Tạm dừng</strong>
					{:else if runtime?.timer.is_running && remaining > 0}
						<strong>{(elapsed / 1000).toFixed(2)}s</strong>
					{:else}
						<strong class="timer-message">{runtime?.timer.duration_ms ? 'Đã hết thời gian' : 'Chưa bắt đầu'}</strong>
					{/if}
				</div>
				<button class="ring-button" type="button" onclick={ring} disabled={bellBusy || !canRing} aria-label="Nhấn chuông">
					<BellRing size={64} />
					<span>{me?.has_rung ? 'ĐÃ BẤM CHUÔNG' : 'NHẤN CHUÔNG'}</span>
				</button>
			</aside>
		</div>
		<form class="answer-form" onsubmit={submit} aria-label="Gửi câu trả lời">
			<input bind:this={answerInput} bind:value={answer} type="text" maxlength="1000" autocomplete="off" aria-label="Câu trả lời của bạn" placeholder={inputPlaceholder()} disabled={busy || !canAnswer} />
			<p class:error-message={Boolean(inputError)} role={inputError ? 'alert' : 'status'}>{inputError || (busy || bellBusy ? 'Đang gửi…' : status)}</p>
		</form>
	</main>
</DisplayFrame>

<style>
	.contestant-station { display: grid; width: 1920px; height: 1080px; grid-template-rows: 60px 60px minmax(0, 1fr) 140px; border: 3px solid #9ca3af; background: white; color: black; font-family: 'Montserrat Variable', 'Montserrat', sans-serif; }
	.station-header { display: flex; align-items: center; gap: 24px; min-width: 0; padding: 0 16px; border: 3px solid #9ca3af; }
	.station-header h1 { font-size: 28px; font-weight: 600; white-space: nowrap; }
	.fullscreen-logo { flex-shrink: 0; cursor: pointer; }
	.identity { margin-left: auto; min-width: 0; overflow: hidden; font-size: 20px; text-overflow: ellipsis; white-space: nowrap; }
	.contestant-strip { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); }
	.contestant-strip > div { display: flex; align-items: center; gap: 12px; padding: 0 16px; min-width: 0; border: 3px solid #9ca3af; font-size: 26px; font-weight: 600; }
	.contestant-strip .has-rung { background: #ffff77; }
	.contestant-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-transform: uppercase; }
	.strip-value { margin-left: auto; max-width: 60%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.station-body { display: grid; min-height: 0; grid-template-columns: minmax(0, 4fr) minmax(0, 1fr); }
	.display-and-answer { display: grid; min-height: 0; min-width: 0; grid-template-rows: minmax(0, 1fr) 120px; }
	.live-display { overflow: hidden; min-height: 0; border: 3px solid #9ca3af; background: black; }
	.sent-answer { display: grid; min-width: 0; grid-template-columns: minmax(0, 1fr) 220px; grid-template-rows: 36px minmax(0, 1fr); border: 3px solid #9ca3af; }
	.answer-label { padding: 0 16px; font-size: 28px; }
	.answer-value { grid-row: 2; min-width: 0; overflow: auto; padding: 0 16px; font-size: 48px; font-weight: 600; white-space: nowrap; }
	.answer-time { grid-column: 2; grid-row: 1 / 3; align-self: center; text-align: center; font-family: monospace; font-size: 48px; font-weight: bold; }
	.timer-and-bell { display: grid; min-height: 0; grid-template-rows: 1fr 1fr; }
	.station-timer { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 24px; border: 3px solid #9ca3af; font-family: monospace; }
	.station-timer > span { font-size: 28px; }
	.station-timer strong { font-size: 72px; font-variant-numeric: tabular-nums; }
	.station-timer .timer-message { font-size: 48px; font-weight: 500; text-align: center; }
	.ring-button { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 24px; border: 3px solid #9ca3af; cursor: pointer; font-family: monospace; font-size: 54px; line-height: 1.25; }
	.ring-button:enabled:hover { background: #fee2e2; }
	.ring-button:disabled { color: #6b7280; cursor: not-allowed; }
	.answer-form { display: grid; min-width: 0; grid-template-rows: minmax(0, 1fr) 34px; border: 3px solid #9ca3af; }
	.answer-form input { min-width: 0; width: 100%; padding: 0 16px; font-size: 54px; text-transform: uppercase; outline-offset: -4px; }
	.answer-form input:disabled { color: #6b7280; }
	.answer-form p { padding: 0 16px; font-size: 24px; }
	.error-message { color: #b91c1c; }
	button:focus-visible, input:focus-visible { outline: 4px solid #2563eb; outline-offset: -4px; }
</style>
