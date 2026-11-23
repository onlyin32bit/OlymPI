<script lang="ts">
	import type { Snippet } from 'svelte';
	import { api } from '$lib/api';
	import { RichText } from '$lib/components/common';
	import { Button } from '$lib/components/ui/button';
	import { socket } from '$lib/socket';
	import {
		nextQuestionStep,
		openLiveQuestion,
		questionMediaKind,
		showKhoiDongEnd
	} from '$lib/control/question-workflow';
	import { questionDurationMS } from '$lib/control/question-timing';
	import { canStartSteal } from '$lib/control/bell-rules';
	import type { Question, QuestionSetData } from '$lib/types';
	import Bell from '@lucide/svelte/icons/bell';
	import BellOff from '@lucide/svelte/icons/bell-off';
	import Eye from '@lucide/svelte/icons/eye';
	import EyeOff from '@lucide/svelte/icons/eye-off';
	import Pause from '@lucide/svelte/icons/pause';
	import Play from '@lucide/svelte/icons/play';
	import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';

	type Option = {
		id: string;
		label: string;
		question: Question;
		phase: string;
		slot?: number;
		package?: '20' | '30';
		row?: number;
	};
	let { children }: { children?: Snippet } = $props();
	let runtime = $derived(socket.match.current);
	let remaining = $derived(socket.timer.remaining);
	let options = $state<Option[]>([]);
	let loadedRevision = $state('');
	let presetDuration = $derived(questionDurationMS(runtime));
	let loadingRevision = '';
	let busy = $state(false);
	let error = $state('');
	let availableOptions = $derived.by(() => {
		if (loadedRevision !== runtime?.question_revision_id) return [];
		const available = options.filter((option) => matchesActiveRound(option));
		if (runtime?.controller.phase !== 've_dich') return available;
		const slot = veDichSlot();
		const choices =
			runtime.controller.ve_dich_contestants_state[slot - 1]?.selected_question_set ?? [];
		return choices.flatMap((choice) => {
			const [score, question] = choice.split('_');
			const option = available.find(
				(candidate) => candidate.id === `vd.${slot - 1}.${score}.${Number(question) - 1}`
			);
			return option ? [option] : [];
		});
	});
	let selectedQuestionIndex = $derived(
		availableOptions.findIndex((option) => option.id === runtime?.controller.selected_question_id)
	);
	/** Khởi động: "next" after the last question of the turn shows the KẾT THÚC card. */
	let nextStep = $derived(
		nextQuestionStep(
			runtime?.controller,
			availableOptions.map((option) => option.id)
		)
	);
	let nextEndsTurn = $derived(nextStep?.kind === 'end');
	let mediaKind = $derived(questionMediaKind(runtime));
	let canPlayMedia = $derived(
		mediaKind === 'audio' ||
			(mediaKind === 'video' &&
				!runtime?.timer.is_running &&
				!runtime?.timer.is_paused &&
				(!runtime?.controller.ve_dich_video ||
					runtime.controller.ve_dich_video.status === 'idle' ||
					runtime.controller.ve_dich_video.status === ''))
	);
	let bellRound = $derived(
		runtime?.controller.phase === 'vcnv' ||
			(runtime?.controller.phase === 'khoi_dong' &&
				(runtime.controller.screen === 'ques_chung' || runtime.controller.screen === 'test_bell'))
	);
	let canClearBells = $derived(
		runtime?.controller.phase === 'khoi_dong' &&
			(runtime.controller.screen === 'test_bell' || runtime.controller.screen === 'ques_chung')
	);
	let emptyLabel = $derived(
		!runtime
			? 'Chưa tải trận'
			: !runtime.question_revision_id
				? 'Chưa có đề thi'
				: runtime.controller.phase === 'khoi_dong'
					? 'Chưa chọn lượt'
					: runtime.controller.phase === 'vcnv'
						? 'Chưa chọn hàng'
						: runtime.controller.phase === 've_dich'
							? 'Chưa chọn gói'
							: 'Không có câu hỏi'
	);

	$effect(() => {
		const revisionID = runtime?.question_revision_id ?? '';
		if (revisionID === loadingRevision) return;
		loadingRevision = revisionID;
		options = [];
		loadedRevision = '';
		if (!revisionID) return;
		void api.questionSets
			.revision(revisionID)
			.then((revision) => {
				if (runtime?.question_revision_id !== revisionID) return;
				options = flatten(revision.data);
				loadedRevision = revisionID;
			})
			.catch((cause) => {
				if (runtime?.question_revision_id !== revisionID) return;
				error = cause instanceof Error ? cause.message : 'Không thể tải đề thi';
			});
	});

	async function run(operation: () => Promise<unknown>) {
		if (busy) return;
		busy = true;
		error = '';
		try {
			await operation();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Thao tác thất bại';
		} finally {
			busy = false;
		}
	}

	function selectQuestionID(value: string): void {
		if (!value || value === runtime?.controller.selected_question_id) return;
		void run(() => socket.controller.selectQuestion(value));
	}
	function openQuestionID(value: string): void {
		if (!runtime || !value) return;
		const phase = runtime.controller.phase;
		void run(() => openLiveQuestion(phase, value));
	}
	function stepQuestion(direction: -1 | 1): void {
		if (direction > 0) {
			const step = nextStep;
			if (step?.kind === 'end') void run(() => showKhoiDongEnd());
			else if (step) openQuestionID(step.id);
			return;
		}
		const next = availableOptions[selectedQuestionIndex + direction];
		if (next) openQuestionID(next.id);
	}

	function timerLabel() {
		const timer = runtime?.timer;
		const milliseconds = timer?.is_running
			? remaining
			: timer?.duration_ms
				? timer.remaining_ms
				: presetDuration;
		return `${Math.floor(milliseconds / 60000)
			.toString()
			.padStart(2, '0')}:${Math.floor((milliseconds % 60000) / 1000)
			.toString()
			.padStart(2, '0')}.${Math.floor((milliseconds % 1000) / 100)}`;
	}

	function flatten(data: QuestionSetData): Option[] {
		const result: Option[] = [];
		const add = (
			id: string,
			label: string,
			question: Question,
			phase: string,
			slot?: number,
			questionPackage?: '20' | '30',
			row?: number
		) => result.push({ id, label, question, phase, slot, package: questionPackage, row });
		data.kd_chung.forEach((q, i) => add(`kd_chung.${i}`, `Câu ${i + 1}`, q, 'khoi_dong'));
		data.kd_rieng.forEach((list, slot) =>
			list.forEach((q, i) => add(`kd_rieng.${slot}.${i}`, `Câu ${i + 1}`, q, 'khoi_dong', slot + 1))
		);
		data.vcnv.rows.forEach((q, i) =>
			add(`vcnv.rows.${i}`, `Hàng ${i + 1}`, q, 'vcnv', undefined, undefined, i + 1)
		);
		add('vcnv.center', 'Ô trung tâm', data.vcnv.center, 'vcnv');
		data.tt.forEach((q, i) => add(`tt.${i}`, `Câu ${i + 1}`, q, 'tang_toc'));
		data.vd.forEach((sets, slot) => {
			(['20', '30'] as const).forEach((score) =>
				sets[score].forEach((q, i) =>
					add(
						`vd.${slot}.${score}.${i}`,
						`Câu ${i + 1} · ${score} điểm`,
						q,
						've_dich',
						slot + 1,
						score
					)
				)
			);
		});
		return result;
	}

	function matchesActiveRound(option: Option): boolean {
		const controller = runtime?.controller;
		if (!controller || option.phase !== controller.phase) return false;
		switch (controller.phase) {
			case 'khoi_dong': {
				if (controller.screen === 'ques_chung') return !option.slot;
				const screenSlot = Number(controller.screen.match(/^ques_ts([1-4])$/)?.[1]);
				if (screenSlot) return option.slot === screenSlot;
				if (controller.selected_question_id.startsWith('kd_chung.')) return !option.slot;
				return controller.selected_contestant > 0 && option.slot === controller.selected_contestant;
			}
			case 'vcnv':
				return true;
			case 'tang_toc':
				return true;
			case 've_dich': {
				const slot = veDichSlot();
				return slot > 0 && option.slot === slot;
			}
			default:
				return false;
		}
	}

	function veDichSlot(): number {
		const controller = runtime?.controller;
		const screenSlot = controller?.screen.match(/^(?:pre_)?ques_ts([1-4])$/)?.[1];
		return screenSlot ? Number(screenSlot) : (controller?.selected_contestant ?? 0);
	}
</script>

<section class="runtime-console" aria-label="Thời gian và câu hỏi">
	<section class="question-command" aria-label="Điều khiển trực tiếp">
		<label class="sr-only" for="question">Câu hỏi</label>
		<div class="question-selector">
			<Button
				size="icon"
				variant="outline"
				class="size-9 rounded-sm"
				aria-label="Mở câu trước"
				disabled={busy || selectedQuestionIndex <= 0}
				onclick={() => stepQuestion(-1)}><ChevronLeft /></Button
			>
			<select
				id="question"
				value={runtime?.controller.selected_question_id ?? ''}
				onchange={(event) => selectQuestionID(event.currentTarget.value)}
				title="Chỉ nạp câu hỏi, chưa hiện lên màn hình"
				disabled={busy || !runtime?.question_revision_id || availableOptions.length === 0}
			>
				<option value="">{availableOptions.length ? 'Chưa chọn câu' : emptyLabel}</option>
				{#each availableOptions as option (option.id)}<option value={option.id}
						>{option.label}</option
					>{/each}
			</select>
			<Button
				size="icon"
				variant="outline"
				class="size-9 rounded-sm"
				aria-label="Mở câu tiếp theo"
				title={nextEndsTurn ? 'Kết thúc lượt' : undefined}
				disabled={busy || !nextStep}
				onclick={() => stepQuestion(1)}><ChevronRight /></Button
			>
		</div>
		<Button
			size="sm"
			variant="outline"
			class={`h-10 rounded-sm px-2 text-[13px] ${runtime?.controller.is_showing_question ? 'border-blue-300 bg-blue-50 text-blue-800' : ''}`}
			aria-pressed={runtime?.controller.is_showing_question ?? false}
			disabled={busy || !runtime?.controller.selected_question_id}
			onclick={() =>
				run(() => socket.controller.toggleQuestion(!runtime?.controller.is_showing_question))}
		>
			{#if runtime?.controller.is_showing_question}<EyeOff />Ẩn câu{:else}<Eye />Hiện câu{/if}
		</Button>
		<div class="timer-controls">
			<div class="timer-readout">
				<output class:running={runtime?.timer.is_running} aria-label="Thời gian còn lại"
					>{timerLabel()}</output
				>
				<span
					>{runtime?.timer.is_paused
						? 'Tạm dừng'
						: runtime?.timer.is_running
							? 'Đang chạy'
							: 'Chờ'}</span
				>
			</div>
			{#if runtime?.timer.is_paused}
				<Button
					size="icon"
					class="size-10 rounded-sm"
					aria-label="Tiếp tục đồng hồ"
					title="Tiếp tục đồng hồ"
					disabled={busy || !runtime}
					onclick={() => run(() => socket.timer.resume())}><Play /></Button
				>
			{:else if runtime?.timer.is_running}
				<Button
					size="icon"
					class="size-10 rounded-sm"
					aria-label="Tạm dừng đồng hồ"
					title="Tạm dừng đồng hồ"
					disabled={busy}
					onclick={() => run(() => socket.timer.pause())}><Pause /></Button
				>
			{:else}
				<Button
					size="icon"
					class="size-10 rounded-sm"
					aria-label="Bắt đầu đồng hồ"
					title="Bắt đầu đồng hồ"
					disabled={busy || runtime?.status !== 'playing' || presetDuration <= 0}
					onclick={() => run(() => socket.timer.start())}><Play /></Button
				>
			{/if}
			<Button
				size="icon"
				variant="outline"
				class="size-10 rounded-sm"
				aria-label="Đặt lại đồng hồ"
				title="Đặt lại đồng hồ"
				disabled={busy || !runtime}
				onclick={() => run(() => socket.timer.reset())}><RotateCcw /></Button
			>
		</div>
		{#if runtime?.controller.phase === 've_dich'}
			<Button
				size="sm"
				variant="outline"
				class={`h-10 rounded-sm px-2 text-[13px] ${runtime.controller.is_allowing_bell ? 'border-amber-300 bg-amber-50 text-amber-900' : ''}`}
				aria-label="Mở cướp quyền 5 giây"
				aria-pressed={runtime.controller.is_allowing_bell}
				title="Thí sinh hiện tại không trả lời được: cướp quyền trong 5 giây"
				disabled={busy || !canStartSteal(runtime)}
				onclick={() => run(() => socket.timer.startSteal())}><Bell />Cướp · 5s</Button
			>
		{:else}
			<span
				class="bell-state"
				class:open={runtime?.controller.is_allowing_bell}
				role="status"
				aria-label="Trạng thái chuông"
			>
				<Bell size={16} />
				{bellRound
					? runtime?.controller.is_allowing_bell
						? 'Chuông mở'
						: 'Chuông khóa'
					: 'Không chuông'}
			</span>
			{#if canClearBells}
				<Button
					size="sm"
					variant="outline"
					class="h-10 rounded-sm px-2 text-[13px]"
					disabled={busy || !runtime}
					onclick={() => run(() => socket.controller.clearBells())}><BellOff />Xoá chuông</Button
				>
			{/if}
		{/if}
		{#if error}<p class="command-error" role="alert">{error}</p>{/if}
	</section>
	<div class="workspace">
		<section class="question-workspace" aria-label="Câu hỏi đang điều khiển">
			{#if availableOptions.length > 1}
				<div class="question-index" aria-label="Mở nhanh câu hỏi">
					{#each availableOptions as option, index (option.id)}
						<button
							class:is-selected={runtime?.controller.selected_question_id === option.id}
							aria-pressed={runtime?.controller.selected_question_id === option.id}
							aria-label={`Mở ${option.label.toLowerCase()}`}
							title={`Mở ${option.label.toLowerCase()}`}
							disabled={busy}
							onclick={() => openQuestionID(option.id)}>{index + 1}</button
						>
					{/each}
				</div>
			{/if}
			{#if runtime?.current_question}
				<div class="question-content" aria-label="Nội dung câu hỏi">
					<RichText value={runtime.current_question.content} class="text-[13px] leading-5" />
					<div class="answer-content" aria-label="Đáp án">
						<span class="block text-xs font-semibold text-slate-500">Đáp án</span>
						<RichText
							value={runtime.current_question.answer || '—'}
							class="text-[13px] leading-5 font-medium"
						/>
					</div>
					<div class="media-actions">
						<Button
							size="sm"
							title={mediaKind === 'audio'
								? 'Phát âm thanh câu hỏi trên loa'
								: mediaKind === 'video'
									? 'Phát video toàn màn hình'
									: 'Câu hỏi không có âm thanh hay video để phát'}
							disabled={busy || !canPlayMedia}
							onclick={() =>
								run(() =>
									mediaKind === 'audio'
										? socket.sound.playQuestionMedia()
										: socket.controller.veDichVideo('start')
								)}><Play />Phát media</Button
						>
					</div>
				</div>
			{/if}
		</section>
		{#if children}
			<section class="round-workspace" aria-label="Điều khiển vòng">{@render children()}</section>
		{/if}
	</div>
</section>

<style>
	.runtime-console {
		display: flex;
		flex: 1;
		flex-direction: column;
		min-height: 0;
		min-width: 0;
		overflow: hidden;
		container-type: inline-size;
	}
	.media-actions {
		display: flex;
		gap: 4px;
		margin-top: 6px;
	}
	.question-command {
		display: flex;
		flex: none;
		flex-wrap: wrap;
		align-items: center;
		gap: 4px 6px;
		padding: 4px 6px;
		border-bottom: 1px solid #cbd5e1;
		background: #f8fafc;
	}
	.question-selector {
		display: grid;
		grid-template-columns: 36px minmax(86px, 108px) 36px;
		gap: 4px;
	}
	select {
		width: 100%;
		min-width: 0;
		height: 36px;
		padding: 0 5px;
		border: 1px solid #cbd5e1;
		border-radius: 4px;
		background: white;
		font-size: 13px;
		color: #334155;
	}
	select:disabled {
		background: #f8fafc;
		color: #64748b;
	}
	select:focus-visible,
	.question-index button:focus-visible {
		outline: 2px solid #2563eb;
		outline-offset: 2px;
	}
	.timer-controls {
		display: flex;
		align-items: center;
		gap: 4px;
	}
	.timer-readout {
		display: flex;
		flex-direction: column;
		min-width: 96px;
	}
	.timer-readout output {
		font-family: ui-monospace, monospace;
		font-size: 22px;
		font-weight: 700;
		line-height: 1.1;
		font-variant-numeric: tabular-nums;
		color: #0f172a;
	}
	.timer-readout output.running {
		color: #1d4ed8;
	}
	.timer-readout span {
		color: #64748b;
		font-size: 12px;
		line-height: 14px;
	}
	.command-error {
		flex-basis: 100%;
		color: #b91c1c;
		font-size: 13px;
	}
	.bell-state {
		display: inline-flex;
		height: 40px;
		align-items: center;
		gap: 6px;
		padding: 0 8px;
		border: 1px solid #cbd5e1;
		border-radius: 3px;
		color: #64748b;
		font-size: 13px;
		white-space: nowrap;
	}
	.bell-state.open {
		border-color: #fcd34d;
		background: #fffbeb;
		color: #78350f;
	}
	.workspace {
		display: grid;
		flex: 1;
		min-height: 0;
		min-width: 0;
		grid-template-columns: 144px minmax(0, 1fr);
		gap: 8px;
		padding: 2px 8px;
		overflow: auto;
		overscroll-behavior: contain;
	}
	.question-workspace {
		display: flex;
		min-height: 0;
		min-width: 0;
		flex-direction: column;
		gap: 6px;
		overflow: hidden;
	}
	.question-index {
		display: grid;
		flex: none;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 3px;
	}
	.question-index button {
		min-width: 32px;
		height: 36px;
		border: 1px solid #cbd5e1;
		border-radius: 4px;
		color: #475569;
		font-size: 13px;
		font-variant-numeric: tabular-nums;
	}
	.question-index button:hover {
		background: #f1f5f9;
	}
	.question-index button.is-selected {
		border-color: #93c5fd;
		background: #eff6ff;
		color: #1d4ed8;
		font-weight: 600;
	}
	.question-index button:disabled {
		opacity: 0.5;
	}
	.question-content {
		flex: 1;
		min-height: 0;
		min-width: 0;
		overflow: auto;
		overflow-wrap: anywhere;
	}
	.answer-content {
		margin-top: 8px;
		padding-top: 6px;
		border-top: 1px solid #e2e8f0;
	}
	.round-workspace {
		min-width: 0;
		align-self: start;
	}
	@container (min-width: 900px) {
		.workspace {
			grid-template-columns: 220px minmax(0, 1fr);
		}
	}
	@container (max-width: 640px) {
		.workspace {
			grid-template-columns: minmax(0, 1fr);
		}
		.question-workspace {
			max-height: 140px;
		}
	}
	@media (pointer: coarse) {
		.question-selector {
			grid-template-columns: 44px minmax(86px, 108px) 44px;
		}
		select,
		.question-index button,
		.runtime-console :global(button),
		.runtime-console :global(input) {
			min-height: 44px;
			min-width: 44px;
		}
	}
</style>
