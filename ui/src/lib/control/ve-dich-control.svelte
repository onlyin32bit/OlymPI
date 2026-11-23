<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { gameController } from '$lib/control/game-controller';
	import { socket } from '$lib/socket';
	import type { PhaseControlProps } from '$lib/display/phase-manifest';
	import type { Screens, VDQuestionChoices } from '$lib/types';
	import Pause from '@lucide/svelte/icons/pause';
	import Play from '@lucide/svelte/icons/play';
	import Square from '@lucide/svelte/icons/square';
	import Volume2 from '@lucide/svelte/icons/volume-2';

	type Slot = 1 | 2 | 3 | 4;

	const slots: readonly Slot[] = [1, 2, 3, 4];
	const preQuestionScreen: Record<Slot, Screens> = {
		1: 'pre_ques_ts1',
		2: 'pre_ques_ts2',
		3: 'pre_ques_ts3',
		4: 'pre_ques_ts4'
	};
	const questionScreen: Record<Slot, Screens> = {
		1: 'ques_ts1',
		2: 'ques_ts2',
		3: 'ques_ts3',
		4: 'ques_ts4'
	};

	let { phase }: PhaseControlProps = $props();
	let busy = $state(false);
	let error = $state('');
	let packageChoices = $state<(20 | 30)[]>([20, 20, 20]);
	let packageKey = '';
	let controller = $derived(socket.controller.current);
	let screen = $derived(controller.screen);
	let slot = $derived(slotFor(screen, controller.selected_contestant));
	let contestants = $derived(socket.match.current?.contestants ?? []);
	let hasQuestion = $derived(
		slot !== 0 && controller.selected_question_id.startsWith(`vd.${slot - 1}.`)
	);
	let packageSelected = $derived(
		slot !== 0 &&
			(controller.ve_dich_contestants_state[slot - 1]?.selected_question_set.length ?? 0) > 0
	);
	let videoState = $derived(controller.ve_dich_video);
	let selectedQuestion = $derived(socket.match.current?.current_question);
	let isVideoQuestion = $derived(
		Boolean(
			hasQuestion &&
				selectedQuestion?.id === controller.selected_question_id &&
				selectedQuestion?.type === 'video' &&
				selectedQuestion.asset_id
		)
	);
	let canVideoCommand = $derived(
		isVideoQuestion && controller.phase === phase && slot !== 0 && screen === questionScreen[slot]
	);
	let ownsVideo = $derived(videoState?.question_id === controller.selected_question_id);
	let videoVolume = $derived(videoState?.volume ?? 1);
	$effect(() => {
		const selected = controller.ve_dich_contestants_state[slot - 1]?.selected_question_set ?? [];
		const key = `${socket.match.current?.match_id ?? ''}:${socket.match.current?.epoch ?? ''}:${slot}:${selected.join(',')}`;
		if (key === packageKey) return;
		packageKey = key;
		if (slot === 0) return;
		packageChoices =
			selected.length === 3
				? selected.map((choice) => (choice.startsWith('30_') ? 30 : 20))
				: [20, 20, 20];
	});

	function slotFor(activeScreen: Screens, selected: number): Slot | 0 {
		if (activeScreen === 'pre_ques_ts1' || activeScreen === 'ques_ts1') return 1;
		if (activeScreen === 'pre_ques_ts2' || activeScreen === 'ques_ts2') return 2;
		if (activeScreen === 'pre_ques_ts3' || activeScreen === 'ques_ts3') return 3;
		if (activeScreen === 'pre_ques_ts4' || activeScreen === 'ques_ts4') return 4;
		return slots.includes(selected as Slot) ? (selected as Slot) : 0;
	}

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
	function videoAction(
		action: 'start' | 'pause' | 'resume' | 'stop' | 'volume',
		volume?: number
	): void {
		void run(() => socket.controller.veDichVideo(action, volume === undefined ? {} : { volume }));
	}

	function changeVideoVolume(event: Event): void {
		const volume = Number((event.currentTarget as HTMLInputElement).value);
		videoVolume = volume;
		videoAction('volume', volume);
	}

	function selectContestant(nextSlot: Slot): void {
		void run(async () => {
			await socket.controller.selectContestant(nextSlot);
			await gameController.showScreen(phase, preQuestionScreen[nextSlot]);
		});
	}

	function choosePackage(): void {
		const selectedSlot = slot;
		if (selectedSlot === 0) return;
		const choices = [1, 2, 3].map(
			(question) => `${packageChoices[question - 1]}_${question}` as VDQuestionChoices
		);
		// Stays on the package screen; question 1 is opened with the question controls.
		void run(() => socket.controller.selectVeDichQuestionSet(selectedSlot, choices));
	}
</script>

<div class="finish-deck">
	<div class="contestants" role="group" aria-label="Thí sinh">
		{#each slots as contestantSlot (contestantSlot)}
			{@const contestant = contestants.find((entry) => entry.slot === contestantSlot)}
			<Button
				size="sm"
				variant={slot === contestantSlot ? 'default' : 'outline'}
				aria-pressed={slot === contestantSlot}
				disabled={busy || !contestant}
				onclick={() => selectContestant(contestantSlot)}
				>{contestant?.name || `Thí sinh ${contestantSlot}`}</Button
			>
		{/each}
	</div>
	<div class="package" role="group" aria-label="Gói câu hỏi và ngôi sao">
		{#each [1, 2, 3] as question (question)}
			{@const starred = controller.ve_dich_contestants_state[slot - 1]?.starred_on === question}
			<div class="choice" role="group" aria-label={`Điểm và sao câu ${question}`}>
				<span>Câu {question}</span>
				<Button
					size="sm"
					variant={packageChoices[question - 1] === 20 ? 'default' : 'outline'}
					aria-label={`Câu ${question}: 20 điểm`}
					aria-pressed={packageChoices[question - 1] === 20}
					disabled={busy || slot === 0}
					onclick={() => (packageChoices[question - 1] = 20)}>20</Button
				>
				<Button
					size="sm"
					variant={packageChoices[question - 1] === 30 ? 'default' : 'outline'}
					aria-label={`Câu ${question}: 30 điểm`}
					aria-pressed={packageChoices[question - 1] === 30}
					disabled={busy || slot === 0}
					onclick={() => (packageChoices[question - 1] = 30)}>30</Button
				>
				<Button
					size="sm"
					variant={starred ? 'default' : 'outline'}
					aria-pressed={starred}
					aria-label={`${starred ? 'Sao' : 'Đặt sao'} câu ${question}`}
					disabled={busy || !packageSelected}
					onclick={() => void run(() => socket.controller.setVeDichStar(slot, question))}
					>Sao</Button
				>
			</div>
		{/each}
	</div>
	<div class="package-actions">
		<Button
			size="sm"
			disabled={busy || slot === 0 || !socket.match.current?.question_revision_id}
			onclick={choosePackage}>Chốt gói</Button
		>
		<Button
			size="sm"
			variant="outline"
			disabled={busy || slot === 0}
			onclick={() => {
				const selectedSlot = slot;
				if (selectedSlot !== 0)
					void run(() => gameController.showScreen(phase, preQuestionScreen[selectedSlot]));
			}}>Gói</Button
		>
		<Button
			size="sm"
			variant="outline"
			disabled={busy || !packageSelected}
			onclick={() => {
				const selectedSlot = slot;
				if (selectedSlot !== 0)
					void run(() => gameController.showScreen(phase, questionScreen[selectedSlot]));
			}}>Câu hỏi</Button
		>
	</div>
	{#if isVideoQuestion}
		<div class="video" role="group" aria-label="Video">
			<!-- Phát media (start) sits with the question; these steer the running video. -->
			<Button
				size="sm"
				variant="outline"
				disabled={busy || !canVideoCommand || !ownsVideo || videoState?.status !== 'playing'}
				onclick={() => videoAction('pause')}><Pause />Tạm dừng</Button
			>
			<Button
				size="sm"
				variant="outline"
				disabled={busy || !canVideoCommand || !ownsVideo || videoState?.status !== 'paused'}
				onclick={() => videoAction('resume')}><Play />Tiếp tục</Button
			>
			<Button
				size="sm"
				aria-label="Dừng video"
				variant="destructive"
				disabled={busy ||
					!canVideoCommand ||
					!ownsVideo ||
					(videoState?.status !== 'playing' && videoState?.status !== 'paused')}
				onclick={() => videoAction('stop')}><Square />Dừng</Button
			>
			<label for="vd-video-volume"
				><Volume2 /><span class="sr-only">Âm lượng</span>
				<input
					id="vd-video-volume"
					type="range"
					min="0"
					max="1"
					step="0.05"
					value={videoVolume}
					disabled={busy ||
						!canVideoCommand ||
						!ownsVideo ||
						!videoState ||
						videoState.status === 'idle' ||
						videoState.status === 'holding'}
					onchange={changeVideoVolume}
				/>
				<span>{Math.round(videoVolume * 100)}%</span>
			</label>
			{#if videoState?.status === 'holding'}
				<span class="video-status" role="status">Giữ hình cuối · 2 giây</span>
			{/if}
		</div>
	{/if}
	{#if error}<p role="alert">{error}</p>{/if}
</div>

<style>
	.finish-deck {
		display: grid;
		gap: 5px;
		min-width: 0;
	}
	.finish-deck :global(button) {
		height: 36px;
		min-height: 36px;
		padding: 0 6px;
		gap: 4px;
		border-radius: 3px;
		font-size: 13px;
	}
	.contestants {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 4px;
	}
	.contestants :global(button) {
		height: auto;
		white-space: normal;
		line-height: 16px;
	}
	.package {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 4px;
		border-block: 1px solid var(--border);
		padding-block: 4px;
	}
	.choice {
		display: grid;
		grid-template-columns: auto repeat(3, minmax(34px, 1fr));
		gap: 2px;
		align-items: center;
		min-width: 0;
	}
	.choice > span {
		font-size: 13px;
		font-weight: 500;
	}
	.package-actions,
	.video {
		display: flex;
		align-items: center;
		gap: 4px;
		flex-wrap: wrap;
	}
	.video {
		border-top: 1px solid var(--border);
		padding-top: 4px;
	}
	.video :global(svg) {
		width: 14px;
		height: 14px;
	}
	.video label {
		display: flex;
		align-items: center;
		gap: 4px;
		font-size: 13px;
	}
	.video label > span:not(.sr-only) {
		min-width: 32px;
		text-align: right;
	}
	.video input {
		width: 64px;
		min-width: 0;
		accent-color: var(--primary);
	}
	.video-status {
		font-size: 12px;
		color: var(--muted-foreground);
	}
	p {
		margin: 0;
		color: var(--destructive);
		font-size: 12px;
	}
</style>
