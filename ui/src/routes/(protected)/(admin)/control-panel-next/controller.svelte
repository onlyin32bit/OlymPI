<script lang="ts">
	import { panel } from '$lib/operator/panel.svelte';
	import { socket } from '$lib/socket';
	import type { Phases } from '$lib/types';
	import Bell from '@lucide/svelte/icons/bell';
	import BellOff from '@lucide/svelte/icons/bell-off';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import Eye from '@lucide/svelte/icons/eye';
	import EyeOff from '@lucide/svelte/icons/eye-off';
	import Pause from '@lucide/svelte/icons/pause';
	import Play from '@lucide/svelte/icons/play';
	import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
	import KhoiDongDeck from './decks/khoi-dong.svelte';
	import ScoreDeck from './decks/score.svelte';
	import TangTocDeck from './decks/tang-toc.svelte';
	import VcnvDeck from './decks/vcnv.svelte';
	import VeDichDeck from './decks/ve-dich.svelte';

	const staticPhases: { id: Phases; label: string }[] = [
		{ id: 'idle', label: 'Tĩnh' },
		{ id: 'main', label: 'Chính' },
		{ id: 'score', label: 'Điểm' }
	];
	const roundPhases: { id: Phases; label: string }[] = [
		{ id: 'khoi_dong', label: 'Khởi động' },
		{ id: 'vcnv', label: 'VCNV' },
		{ id: 'tang_toc', label: 'Tăng tốc' },
		{ id: 've_dich', label: 'Về đích' }
	];

	let controller = $derived(panel.controller);
	let timer = $derived(panel.runtime?.timer);
	let isSteal = $derived(Boolean(timer?.is_running && timer.duration_ms === 5000 && controller.phase === 've_dich'));
	let timerMS = $derived(
		timer?.is_running ? socket.timer.remaining : timer?.duration_ms ? timer.remaining_ms : panel.presetDuration
	);
	let timerText = $derived(
		`${Math.floor(timerMS / 60000).toString().padStart(2, '0')}:${Math.floor((timerMS % 60000) / 1000)
			.toString()
			.padStart(2, '0')}.${Math.floor((timerMS % 1000) / 100)}`
	);
	let bellRound = $derived(
		controller.phase === 'vcnv' ||
			(controller.phase === 'khoi_dong' && (controller.screen === 'ques_chung' || controller.screen === 'test_bell'))
	);
	/** Khởi động bells are cleared by hand: one at a time on Thử chuông, and on the common question. */
	let canClearBells = $derived(
		controller.phase === 'khoi_dong' && (controller.screen === 'test_bell' || controller.screen === 'ques_chung')
	);
	let canStartTimer = $derived(
		Boolean(timer?.is_paused || timer?.is_running || (panel.playing && panel.presetDuration > 0 && !panel.timerSpent))
	);
	let fast = $derived(panel.fastVerdict);
	const points = (value: number) => value > 0 ? ` +${value}` : value < 0 ? ` ${value}` : '';
</script>

<section class="controller cp-panel" aria-label="Điều khiển chương trình">
	<nav class="phases" aria-label="Vòng thi">
		{#each [staticPhases, roundPhases] as group, index (index)}
			<div class="group" class:rounds={index === 1}>
				{#each group as phase (phase.id)}
					<button
						type="button"
						class:live={controller.phase === phase.id}
						aria-current={controller.phase === phase.id ? 'step' : undefined}
						disabled={panel.busy}
						onclick={() => panel.changePhase(phase.id)}
						>{#if controller.phase === phase.id}<span class="dot" aria-hidden="true"></span>{/if}{phase.label}</button
					>
				{/each}
			</div>
		{/each}
	</nav>

	<nav class="screens" aria-label="Màn hình của vòng">
		<button
			type="button"
			class="cp-btn step"
			aria-label="Màn hình trước"
			aria-keyshortcuts="ArrowLeft"
			disabled={panel.busy || panel.screenIndex <= 0}
			onclick={() => panel.stepScreen(-1)}><ChevronLeft /></button
		>
		<button
			type="button"
			class="cp-btn step"
			aria-label="Màn hình tiếp theo"
			aria-keyshortcuts="ArrowRight"
			disabled={panel.busy || !panel.nextScreen}
			onclick={() => panel.stepScreen(1)}><ChevronRight /></button
		>
		<div class="tabs">
			{#each panel.tabs as tab (tab.id)}
				<button
					type="button"
					class:live={panel.liveTab?.id === tab.id}
					aria-current={panel.liveTab?.id === tab.id ? 'step' : undefined}
					disabled={panel.busy}
					onclick={() => panel.showTab(tab)}>{tab.label}</button
				>
			{/each}
		</div>
	</nav>

	<div class="strip" role="group" aria-label="Câu hỏi">
		<button
			type="button"
			class="cp-btn step"
			aria-label="Câu trước"
			aria-keyshortcuts="ArrowUp"
			disabled={panel.busy || panel.selectedIndex <= 0}
			onclick={() => panel.stepQuestion(-1)}><ChevronLeft /></button
		>
		<button
			type="button"
			class="cp-btn step"
			aria-label={panel.nextEndsTurn ? 'Kết thúc lượt' : 'Câu tiếp theo'}
			title={panel.nextEndsTurn ? 'Kết thúc lượt' : undefined}
			aria-keyshortcuts="ArrowDown"
			disabled={panel.busy || !panel.canStepNext}
			onclick={() => panel.stepQuestion(1)}><ChevronRight /></button
		>
		<div class="numbers">
			{#each panel.strip as option (option.id)}
				{@const selected = controller.selected_question_id === option.id}
				<button
					type="button"
					class="cp-btn"
					class:is-live={selected && controller.is_showing_question}
					class:is-selected={selected && !controller.is_showing_question}
					class:is-dim={panel.asked.includes(option.id)}
					aria-pressed={selected}
					title={option.label}
					disabled={panel.busy}
					onclick={() => panel.openQuestion(option.id)}>{option.short}</button
				>
			{:else}
				<span class="cp-label empty">{panel.runtime ? 'Không có câu hỏi cho màn này' : 'Chưa chọn trận'}</span>
			{/each}
		</div>
	</div>

	<div class="live-row">
		<button
			type="button"
			class="cp-btn show"
			class:is-live={controller.is_showing_question}
			aria-pressed={controller.is_showing_question}
			aria-keyshortcuts="H"
			disabled={panel.busy || !controller.selected_question_id}
			onclick={() => panel.toggleShowQuestion()}
			>{#if controller.is_showing_question}<EyeOff />Ẩn câu{:else}<Eye />Hiện câu{/if}</button
		>
		<output
			class="timer cp-num"
			class:running={timer?.is_running && !isSteal}
			class:paused={timer?.is_paused}
			class:steal={isSteal}
			aria-label="Thời gian còn lại">{timerText}</output
		>
		<button
			type="button"
			class="cp-btn is-primary"
			aria-label={timer?.is_running ? 'Tạm dừng đồng hồ' : timer?.is_paused ? 'Tiếp tục đồng hồ' : 'Bắt đầu đồng hồ'}
			aria-keyshortcuts="Space"
			disabled={panel.busy || !canStartTimer}
			onclick={() => panel.toggleTimer()}
			>{#if timer?.is_running}<Pause />{:else}<Play />{/if}</button
		>
		<button
			type="button"
			class="cp-btn"
			aria-label="Đặt lại đồng hồ"
			disabled={panel.busy || !panel.runtime}
			onclick={() => panel.resetTimer()}><RotateCcw /></button
		>
		<button
			type="button"
			class="cp-btn"
			disabled={panel.busy || !panel.connected || !socket.connected}
			onclick={() => void panel.run(() => socket.sound.play('builtin.main.space'))}>Dấu ...</button
		>
		{#if controller.phase === 've_dich'}
			<button
				type="button"
				class="cp-btn"
				class:is-armed={controller.is_allowing_bell || isSteal}
				aria-keyshortcuts="B"
				disabled={panel.busy || !panel.canSteal}
				onclick={() => panel.steal()}><Bell />Cướp 5s</button
			>
		{:else}
			<span class="bell" class:open={bellRound && controller.is_allowing_bell} class:none={!bellRound}>
				<Bell />{bellRound ? (controller.is_allowing_bell ? 'Chuông mở' : 'Chuông khóa') : 'Không chuông'}
			</span>
			{#if canClearBells}
				<button
					type="button"
					class="cp-btn"
					disabled={panel.busy || !panel.runtime}
					onclick={() => panel.clearBells()}><BellOff />Xoá chuông</button
				>
			{/if}
		{/if}
	</div>

	{#if fast.visible}
		<div class="quick-actions" role="group" aria-label="Chấm nhanh Khởi động">
			<span class="target">{controller.screen === 'ques_chung' ? (fast.slot ? `Chuông: ${fast.name}` : 'Chưa có chuông') : `Đang trả lời: ${fast.name}`}</span>
			<button type="button" class="cp-btn is-ok" disabled={!fast.canCorrect} onclick={(event) => { if (event.detail < 2) panel.judgeKhoiDong('correct'); }}>Đúng{points(fast.correct)} · Tiếp</button>
			<button type="button" class="cp-btn is-bad" disabled={!fast.canWrong} onclick={(event) => { if (event.detail < 2) panel.judgeKhoiDong('wrong'); }}>Sai{points(fast.wrong)} · Tiếp</button>
			{#if fast.pending}
				<span class="draft-note">Bỏ nháp câu này trước khi chấm nhanh, hoặc ghi điểm rồi dùng mũi tên câu tiếp theo.</span>
				<button type="button" class="cp-btn" disabled={panel.busy || panel.drafts.saving} onclick={() => panel.discardCurrentDraft()}>Bỏ nháp câu này</button>
			{/if}
			{#if fast.saved}
				<span class="draft-note">Điểm câu này đã ghi; dùng câu tiếp theo hoặc Hoàn tác.</span>
			{/if}
		</div>
	{:else if controller.phase === 'vcnv' || controller.phase === 'tang_toc' || controller.phase === 've_dich'}
		<div class="quick-actions" role="group" aria-label="Nhạc kết quả">
			<button type="button" class="cp-btn is-ok" disabled={!panel.connected || !socket.connected} onclick={() => panel.playVerdictSound('correct')}>Nhạc đúng</button>
			<button type="button" class="cp-btn is-bad" disabled={!panel.connected || !socket.connected} onclick={() => panel.playVerdictSound('wrong')}>Nhạc sai</button>
			<button type="button" class="cp-btn" aria-label={panel.nextEndsTurn ? 'Kết thúc lượt' : 'Quay lại câu hỏi và chuyển câu tiếp'} disabled={panel.busy || !panel.canStepNext} onclick={() => panel.stepQuestion(1)}>{panel.nextEndsTurn ? 'Kết thúc lượt' : 'Câu tiếp theo'}</button>
		</div>
	{/if}

	<div class="deck" aria-label="Điều khiển vòng">
		{#if controller.phase === 'khoi_dong' && controller.screen === 'main_kd'}
			<KhoiDongDeck />
		{:else if controller.phase === 'vcnv'}
			<VcnvDeck />
		{:else if controller.phase === 'tang_toc'}
			<TangTocDeck />
		{:else if controller.phase === 've_dich'}
			<VeDichDeck />
		{:else if controller.phase === 'score'}
			<ScoreDeck />
		{/if}
	</div>
</section>

<style>
	.controller {
		--cp-h: var(--cp-h-main);
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	.phases {
		display: flex;
		flex: none;
		gap: 10px;
		padding: 6px 6px 0;
	}
	.group {
		display: grid;
		grid-auto-columns: minmax(0, 1fr);
		grid-auto-flow: column;
		flex: 3;
		gap: 2px;
		padding: 2px;
		border-radius: 8px;
		background: var(--cp-raised);
	}
	.group.rounds {
		flex: 4;
	}
	.group button,
	.tabs button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 6px;
		height: 34px;
		padding: 0 8px;
		border-radius: 6px;
		color: var(--cp-muted);
		font: 500 14px/1 var(--cp-font);
		white-space: nowrap;
		cursor: pointer;
	}
	.group button:hover:not(:disabled),
	.tabs button:hover:not(:disabled) {
		color: var(--cp-text);
	}
	.group button.live {
		background: var(--cp-surface);
		color: var(--cp-text);
		font-weight: 600;
		box-shadow: 0 0 0 1px var(--cp-line);
	}
	.group button:disabled,
	.tabs button:disabled {
		cursor: progress;
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 999px;
		background: var(--cp-live);
	}
	.screens {
		display: flex;
		flex: none;
		align-items: center;
		gap: 4px;
		padding: 6px 6px 0;
	}
	.step {
		--cp-h: 36px;
		padding: 0;
	}
	.tabs {
		display: flex;
		flex: 1;
		gap: 2px;
		min-width: 0;
		margin-left: 6px;
		overflow-x: auto;
		scrollbar-width: none;
	}
	.tabs button {
		flex: none;
		height: 36px;
		padding: 0 10px;
		border-bottom: 2px solid transparent;
		border-radius: 0;
	}
	.tabs button.live {
		border-bottom-color: var(--cp-live);
		color: var(--cp-text);
		font-weight: 600;
	}
	.strip {
		display: flex;
		flex: none;
		align-items: center;
		gap: 4px;
		padding: 8px 6px 0;
		border-top: 1px solid var(--cp-line);
		margin-top: 6px;
	}
	.numbers {
		display: flex;
		flex: 1;
		gap: 4px;
		min-width: 0;
	}
	.numbers .cp-btn {
		flex: 1 1 0;
		min-width: 0;
		max-width: 64px;
		padding: 0;
		font-size: 16px;
		font-weight: 600;
	}
	.empty {
		align-self: center;
		padding-left: 6px;
	}
	.live-row {
		display: flex;
		flex: none;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px;
		padding: 6px 6px 8px;
		border-bottom: 1px solid var(--cp-line);
	}
	.quick-actions {
		display: flex;
		flex: none;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px;
		padding: 6px;
		border-bottom: 1px solid var(--cp-line);
	}
	.quick-actions .is-ok {
		border-color: var(--cp-ok);
		background: var(--cp-ok-bg);
		color: var(--cp-ok-ink);
		font-weight: 600;
	}
	.quick-actions .is-bad {
		border-color: var(--cp-bad);
		background: var(--cp-bad-bg);
		color: var(--cp-bad-ink);
		font-weight: 600;
	}
	.target { font-weight: 600; }
	.draft-note { color: var(--cp-muted); font-size: 13px; }
	.show {
		min-width: 128px;
	}
	/* Fixed width: a longer clock (e.g. 10:00.0) must not push the buttons beside it. */
	.timer {
		flex: none;
		width: 9.5ch;
		padding: 0 8px;
		color: var(--cp-text);
		font: 600 32px/1 var(--cp-font);
		font-variant-numeric: tabular-nums;
		text-align: center;
		white-space: nowrap;
	}
	.timer.running {
		color: var(--cp-live);
	}
	.timer.paused,
	.timer.steal {
		color: var(--cp-armed);
	}
	.bell {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: var(--cp-h);
		padding: 0 12px;
		border-radius: 6px;
		background: var(--cp-raised);
		color: var(--cp-muted);
		font-size: 14px;
		font-weight: 500;
	}
	.bell :global(svg) {
		width: 16px;
		height: 16px;
	}
	.bell.open {
		background: var(--cp-armed-bg);
		color: var(--cp-armed-ink);
		font-weight: 600;
	}
	.bell.none {
		color: var(--cp-faint);
	}
	.deck {
		--cp-h: var(--cp-h-deck);
		flex: 1;
		min-height: 0;
		padding: 8px 6px 6px;
		overflow: auto;
	}
</style>
