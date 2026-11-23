<script lang="ts">
	import { vcnvBoardStep } from '$lib/control/question-workflow';
	import { OBSTACLE_KEY, obstacleRowsOpened } from '$lib/operator/drafts';
	import { panel } from '$lib/operator/panel.svelte';
	import { SLOTS, obstacleValue, type Slot } from '$lib/operator/scoring';
	import { socket } from '$lib/socket';
	import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
	import Timer from '@lucide/svelte/icons/timer';

	type RowState = 'u' | 's' | 'c' | 'w';
	const rowStateLabels: Record<RowState, string> = { u: 'Chưa mở', s: 'Đang mở', c: 'Đúng', w: 'Sai' };
	/** Picture tiles laid out as on the board: corners around the centre. */
	const tiles = [
		{ index: 0, label: '1', area: 'a' },
		{ index: 1, label: '2', area: 'b' },
		{ index: 4, label: 'TT', area: 'c' },
		{ index: 2, label: '3', area: 'd' },
		{ index: 3, label: '4', area: 'e' }
	];

	let vcnv = $derived(panel.controller.vcnv_state);
	let rows = $derived(vcnv.row_states as RowState[]);
	let hasSet = $derived(Boolean(panel.runtime?.question_revision_id));
	let keywordValue = $derived(obstacleValue(obstacleRowsOpened(panel.runtime), panel.drafts.rules));
	/** Contestants who rang first, then the rest in stage order. */
	let keywordOrder = $derived<Slot[]>([
		...(panel.ringOrder.filter((slot) => SLOTS.includes(slot as Slot)) as Slot[]),
		...SLOTS.filter((slot) => !panel.ringOrder.includes(slot))
	]);

	function setRow(index: number, state: RowState): void {
		void panel.run(() => socket.controller.setVCNVRowState(index + 1, state));
	}

	/** First press shows the empty board, the second starts it. */
	let boardStep = $derived(vcnvBoardStep(panel.controller));
</script>

<div class="obstacle">
	<div class="rows" role="group" aria-label="Hàng ngang">
		{#each [0, 1, 2, 3] as index (index)}
			{@const state = rows[index] ?? 'u'}
			<div class="row">
				<span class={`tile state-${state}`} title={rowStateLabels[state]}>Hàng {index + 1}</span>
				<button
					type="button"
					class="cp-btn"
					class:is-live={state === 's' && panel.controller.selected_question_id === `vcnv.rows.${index}`}
					class:is-selected={state === 's' && panel.controller.selected_question_id !== `vcnv.rows.${index}`}
					aria-label={state === 's' ? `Mở câu hỏi hàng ${index + 1}` : `Chọn hàng ${index + 1}`}
					title={state === 's' ? 'Mở câu hỏi' : 'Chọn hàng trên bảng'}
					disabled={panel.busy || !hasSet}
					onclick={() => panel.pressRow(index)}>Mở</button
				>
				<button type="button" class="cp-btn is-ok" aria-pressed={state === 'c'} disabled={panel.busy} onclick={() => setRow(index, 'c')}>Đúng</button>
				<button type="button" class="cp-btn is-bad" aria-pressed={state === 'w'} disabled={panel.busy} onclick={() => setRow(index, 'w')}>Sai</button>
				<button
					type="button"
					class="cp-btn is-ghost reset"
					aria-label={`Đặt lại hàng ${index + 1}`}
					title="Đặt lại"
					disabled={panel.busy || state === 'u'}
					onclick={() => setRow(index, 'u')}><RotateCcw /></button
				>
			</div>
		{/each}
		<div class="row">
			<span class="tile" class:state-s={panel.controller.selected_question_id === 'vcnv.center'}>Trung tâm</span>
			<button
				type="button"
				class="cp-btn"
				class:is-live={panel.controller.selected_question_id === 'vcnv.center' && panel.controller.is_showing_question}
				disabled={panel.busy || !hasSet}
				onclick={() => panel.openQuestion('vcnv.center')}>Mở</button
			>
		</div>
	</div>

	<div class="picture">
		<div class="tiles" role="group" aria-label="Mảnh hình">
			{#each tiles as tile (tile.index)}
				{@const open = vcnv.image_open_state[tile.index] ?? false}
				<button
					type="button"
					class="cp-btn"
					style:grid-area={tile.area}
					aria-pressed={open}
					aria-label={`${open ? 'Đóng' : 'Mở'} mảnh hình ${tile.label}`}
					disabled={panel.busy}
					onclick={() => void panel.run(() => socket.controller.setVCNVPanelOpen(tile.index, !open))}>{tile.label}</button
				>
			{/each}
		</div>
		<div class="board-actions">
			<button
				type="button"
				class="cp-btn"
				class:is-primary={boardStep === 'start'}
				aria-pressed={vcnv.has_started}
				disabled={panel.busy || !boardStep}
				onclick={() => panel.pressBoard()}>{boardStep === 'start' ? 'Bắt đầu' : 'Mở bảng'}</button
			>
			<button
				type="button"
				class="cp-btn"
				aria-pressed={vcnv.has_solved}
				disabled={panel.busy}
				onclick={() => void panel.run(() => socket.controller.setVCNVSolved(!vcnv.has_solved))}>Giải CNV</button
			>
			<button
				type="button"
				class="cp-btn obstacle-timer"
				class:is-armed={panel.runtime?.timer.kind === 'obstacle' && panel.runtime.timer.is_running}
				title="Đồng hồ chướng ngại vật 15 giây"
				disabled={panel.busy || !panel.canStartObstacle}
				onclick={() => panel.startObstacleTimer()}><Timer />15 giây CNV</button
			>
		</div>
	</div>

	<div class="keyword" role="group" aria-label="Chướng ngại vật">
		<p class="answer"><span class="cp-label">CNV</span> <strong>{panel.obstacle || '—'}</strong></p>
		{#each keywordOrder as slot (slot)}
			{@const entry = panel.runtime?.contestants.find((value) => value.slot === slot)}
			{@const verdict = panel.drafts.verdict(OBSTACLE_KEY, slot)}
			{@const ring = panel.ringOrder.indexOf(slot)}
			<div class="contestant" class:rung={ring >= 0} class:out={entry?.locked && verdict !== 'wrong'}>
				<span class="name" title={entry?.name}>{#if ring >= 0}<b class="cp-num">{ring + 1}</b>{/if}{entry?.name || `TS${slot}`}</span>
				<button
					type="button"
					class="cp-btn is-ok"
					aria-pressed={verdict === 'correct'}
					disabled={!entry || (entry.locked && verdict !== 'wrong')}
					onclick={() => panel.obstacleVerdict(slot, 'correct')}>Đúng <span class="cp-num points">+{keywordValue}</span></button
				>
				<button
					type="button"
					class="cp-btn is-bad"
					aria-pressed={verdict === 'wrong'}
					disabled={!entry || (entry.locked && verdict !== 'wrong')}
					onclick={() => panel.obstacleVerdict(slot, 'wrong')}>Sai</button
				>
			</div>
		{/each}
	</div>
</div>

<style>
	.obstacle {
		display: grid;
		grid-template-columns: minmax(0, 1.25fr) minmax(0, 0.7fr) minmax(0, 1.1fr);
		gap: 12px;
	}
	.rows,
	.keyword {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.row {
		display: grid;
		grid-template-columns: minmax(0, 1.2fr) repeat(3, minmax(0, 1fr)) var(--cp-h);
		gap: 4px;
	}
	.tile {
		display: flex;
		align-items: center;
		padding: 0 8px;
		border-radius: 6px;
		background: var(--cp-raised);
		color: var(--cp-muted);
		font-size: 14px;
		font-weight: 600;
		white-space: nowrap;
	}
	.tile.state-s {
		background: var(--cp-live-bg);
		color: var(--cp-live-ink);
	}
	.tile.state-c {
		background: var(--cp-ok-bg);
		color: var(--cp-ok-ink);
	}
	.tile.state-w {
		background: var(--cp-bad-bg);
		color: var(--cp-bad-ink);
	}
	.reset {
		padding: 0;
	}
	.picture {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.tiles {
		display: grid;
		grid-template-areas: 'a . b' '. c .' 'd . e';
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 4px;
	}
	.tiles .cp-btn {
		padding: 0;
	}
	.board-actions {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 4px;
	}
	.board-actions .cp-btn {
		padding: 0 6px;
	}
	.board-actions .obstacle-timer {
		grid-column: 1 / -1;
	}
	.answer {
		display: flex;
		align-items: baseline;
		gap: 8px;
		min-height: 24px;
		overflow: hidden;
		font-size: 16px;
		white-space: nowrap;
	}
	.answer strong {
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.contestant {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) minmax(0, 0.6fr);
		gap: 4px;
	}
	.contestant .name {
		display: flex;
		align-items: center;
		gap: 6px;
		overflow: hidden;
		font-size: 14px;
		font-weight: 500;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.contestant .name b {
		display: grid;
		flex: none;
		place-items: center;
		width: 20px;
		height: 20px;
		border-radius: 999px;
		background: var(--cp-armed-bg);
		color: var(--cp-armed-ink);
		font-size: 12px;
	}
	.contestant.out .name {
		color: var(--cp-faint);
		text-decoration: line-through;
	}
	.points {
		opacity: 0.75;
	}
</style>
