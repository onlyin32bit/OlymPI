<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import AnswerReviewControl from '$lib/control/answer-review-control.svelte';
	import { socket } from '$lib/socket';
	import {
		canStartObstacleTimer,
		openLiveQuestion,
		pressVCNVBoard,
		pressVCNVRow,
		vcnvBoardStep
	} from '$lib/control/question-workflow';
	import type { PhaseControlProps } from '$lib/display/phase-manifest';

	let { phase }: PhaseControlProps = $props();
	let controller = $derived(socket.controller.current);
	let panels = $derived(controller.vcnv_state.image_open_state);
	let rows = $derived(controller.vcnv_state.row_states);
	/** First press shows the empty board, the second starts it. */
	let boardStep = $derived(vcnvBoardStep(controller));
	let busy = $state(false);
	let error = $state('');
	const rowLabels: Record<string, string> = { u: 'Chưa chọn', s: 'Đang chọn', c: 'Đúng', w: 'Sai' };
	const pieces = [
		{ index: 0, label: 'Trên trái', position: 'col-start-1 row-start-1' },
		{ index: 1, label: 'Trên phải', position: 'col-start-3 row-start-1' },
		{ index: 2, label: 'Dưới trái', position: 'col-start-1 row-start-3' },
		{ index: 3, label: 'Dưới phải', position: 'col-start-3 row-start-3' },
		{ index: 4, label: 'Trung tâm', position: 'col-start-2 row-start-2' }
	];

	async function run(action: () => Promise<unknown>): Promise<void> {
		if (busy) return;
		busy = true;
		error = '';
		try {
			await action();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Thao tác thất bại';
		} finally {
			busy = false;
		}
	}


	function setRow(index: number, state: 'u' | 's' | 'c' | 'w'): void {
		void run(() => socket.controller.setVCNVRowState(index + 1, state));
	}
</script>

<div class="obstacle-deck">
	<div class="workbench">
		<div class="rows" role="group" aria-label="Hàng ngang">
			{#each [0, 1, 2, 3] as index (index)}
				{@const row = rows[index] ?? 'u'}
				<div
					class="row"
					role="group"
					aria-label={`Hàng ${index + 1}: ${rowLabels[row] ?? 'Chưa chọn'}`}
				>
					<span>Hàng {index + 1}</span>
					<Button
						size="sm"
						variant={row === 'u' ? 'default' : 'outline'}
						aria-label={`Đặt lại hàng ${index + 1}`}
						aria-pressed={row === 'u'}
						disabled={busy}
						onclick={() => setRow(index, 'u')}>Đặt lại</Button
					>
					<Button
						size="sm"
						variant={row === 's' ? 'default' : 'outline'}
						aria-label={`Mở hàng ${index + 1}`}
						title={row === 's' ? 'Mở câu hỏi' : 'Chọn hàng trên bảng'}
						aria-pressed={row === 's'}
						disabled={busy || !socket.match.current?.question_revision_id}
						onclick={() => void run(() => pressVCNVRow(index))}>Mở</Button
					>
					<Button
						size="sm"
						variant={row === 'c' ? 'default' : 'outline'}
						aria-label={`Hàng ${index + 1}: Đúng`}
						aria-pressed={row === 'c'}
						disabled={busy}
						onclick={() => setRow(index, 'c')}>Đúng</Button
					>
					<Button
						size="sm"
						variant={row === 'w' ? 'destructive' : 'outline'}
						aria-label={`Hàng ${index + 1}: Sai`}
						aria-pressed={row === 'w'}
						disabled={busy}
						onclick={() => setRow(index, 'w')}>Sai</Button
					>
				</div>
			{/each}
		</div>
		<div class="image-tools">
			<div class="pieces" role="group" aria-label="Mảnh hình">
				{#each pieces as piece (piece.index)}
					<Button
						size="sm"
						class={piece.position}
						variant={panels[piece.index] ? 'default' : 'outline'}
						aria-pressed={panels[piece.index] ?? false}
						aria-label={`${piece.label}: ${panels[piece.index] ? 'Đóng' : 'Mở'} mảnh hình`}
						title={`${panels[piece.index] ? 'Đóng' : 'Mở'} mảnh hình ${piece.label}`}
						disabled={busy}
						onclick={() =>
							void run(() => socket.controller.setVCNVPanelOpen(piece.index, !panels[piece.index]))}
					>
						{piece.index === 4 ? 'Giữa' : piece.index + 1}
					</Button>
				{/each}
			</div>
			<div class="image-actions">
				<Button size="sm" disabled={busy || !boardStep} onclick={() => void run(() => pressVCNVBoard())}
					>{boardStep === 'start' ? 'Bắt đầu' : 'Mở bảng'}</Button
				>
				<Button
					size="sm"
					variant="outline"
					title="Đồng hồ chướng ngại vật 15 giây"
					disabled={busy || !canStartObstacleTimer(socket.match.current)}
					onclick={() => void run(() => socket.timer.startObstacle())}>15 giây CNV</Button
				>
				<Button
					size="sm"
					variant="outline"
					disabled={busy || !socket.match.current?.question_revision_id}
					onclick={() => void run(() => openLiveQuestion(phase, 'vcnv.center'))}>Trung tâm</Button
				>
				<div class="toggles">
					<label
						><input
							type="checkbox"
							checked={controller.vcnv_state.has_started}
							disabled={busy}
							onchange={(event) => {
								const checked = event.currentTarget.checked;
								void run(() => socket.controller.setVCNVStarted(checked));
							}}
						/>Đã bắt đầu</label
					>
					<label
						><input
							type="checkbox"
							checked={controller.vcnv_state.has_solved}
							disabled={busy}
							onchange={(event) => {
								const checked = event.currentTarget.checked;
								void run(() => socket.controller.setVCNVSolved(checked));
							}}
						/>Đã giải</label
					>
				</div>
			</div>
		</div>
	</div>
	<AnswerReviewControl
		busy={busy || !controller.selected_question_id}
		onchange={(slot, correct) => void run(() => socket.controller.setAnswerStatus(slot, correct))}
	/>
	{#if error}<p role="alert">{error}</p>{/if}
</div>

<style>
	.obstacle-deck {
		display: grid;
		gap: 5px;
		min-width: 0;
	}
	.workbench {
		display: grid;
		grid-template-columns: minmax(260px, 1fr) minmax(230px, 0.88fr);
		gap: 6px;
	}
	.rows {
		display: grid;
		align-content: start;
		gap: 2px;
	}
	.row {
		display: grid;
		grid-template-columns: 52px minmax(52px, 1fr) repeat(3, minmax(44px, 1fr));
		gap: 2px;
		align-items: center;
	}
	.row > span {
		font-size: 13px;
		font-weight: 500;
	}
	.obstacle-deck :global(button) {
		height: 36px;
		min-height: 36px;
		padding: 0 5px;
		border-radius: 3px;
		font-size: 13px;
	}
	.image-tools {
		display: grid;
		grid-template-columns: 112px minmax(0, 1fr);
		align-items: start;
		gap: 6px;
	}
	.pieces {
		display: grid;
		grid-template-columns: repeat(3, 36px);
		grid-template-rows: repeat(3, 36px);
		gap: 2px;
	}
	.image-actions {
		display: grid;
		gap: 2px;
	}
	.toggles {
		display: grid;
		gap: 0;
		font-size: 13px;
	}
	label {
		display: flex;
		align-items: center;
		gap: 4px;
		min-height: 36px;
	}
	input {
		accent-color: var(--primary);
	}
	p {
		margin: 0;
		color: var(--destructive);
		font-size: 12px;
	}
</style>
