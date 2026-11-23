<script lang="ts">
	import { panel } from '$lib/operator/panel.svelte';
	import { socket } from '$lib/socket';
	import Pause from '@lucide/svelte/icons/pause';
	import Play from '@lucide/svelte/icons/play';
	import Square from '@lucide/svelte/icons/square';
	import Volume2 from '@lucide/svelte/icons/volume-2';

	let controller = $derived(panel.controller);
	let video = $derived(controller.ve_dich_video);
	let owner = $derived(Number(/^vd\.([0-3])\./.exec(controller.selected_question_id)?.[1] ?? -1) + 1);
	let question = $derived(panel.runtime?.current_question);
	/** Commands are valid only while the video question is on its own audience screen. */
	let canCommand = $derived(
		owner > 0 &&
			controller.phase === 've_dich' &&
			controller.screen === `ques_ts${owner}` &&
			question?.id === controller.selected_question_id &&
			question?.type === 'video' &&
			Boolean(question.asset_id)
	);
	let owns = $derived(video?.question_id === controller.selected_question_id);
	let timerActive = $derived(Boolean(panel.runtime?.timer.is_running || panel.runtime?.timer.is_paused));
	let volume = $derived(video?.volume ?? 1);
	/** Only a Về đích video has transport controls; audio just (re)plays through the TV. */
	let isVideo = $derived(
		controller.phase === 've_dich' && question?.id === controller.selected_question_id && question?.type === 'video'
	);
	let canPlay = $derived(
		panel.mediaKind === 'audio' ||
			(panel.mediaKind === 'video' && canCommand && !timerActive && (!video || video.status === 'idle' || video.status === ''))
	);

	function command(action: 'start' | 'pause' | 'resume' | 'stop' | 'volume', value?: number): void {
		void panel.run(() => socket.controller.veDichVideo(action, value === undefined ? {} : { volume: value }));
	}
</script>

<div class="video" role="group" aria-label="Media câu hỏi">
	<button
		type="button"
		class="cp-btn is-primary"
		title={panel.mediaKind === 'audio'
			? 'Phát âm thanh câu hỏi trên loa'
			: panel.mediaKind === 'video'
				? 'Phát video toàn màn hình'
				: 'Câu hỏi không có âm thanh hay video để phát'}
		disabled={panel.busy || !canPlay}
		onclick={() => panel.playMedia()}><Play />Phát media</button
	>
	{#if isVideo}
		{#if owns && video?.status === 'paused'}
			<button type="button" class="cp-btn" disabled={panel.busy || !canCommand} onclick={() => command('resume')}
				><Play />Tiếp tục</button
			>
		{:else}
			<button
				type="button"
				class="cp-btn"
				disabled={panel.busy || !canCommand || !owns || video?.status !== 'playing'}
				onclick={() => command('pause')}><Pause />Tạm dừng</button
			>
		{/if}
		<button
			type="button"
			class="cp-btn"
			disabled={panel.busy || !canCommand || !owns || (video?.status !== 'playing' && video?.status !== 'paused')}
			onclick={() => command('stop')}><Square />Dừng</button
		>
		<label class="volume">
			<Volume2 /><span class="sr-only">Âm lượng video</span>
			<input
				type="range"
				min="0"
				max="1"
				step="0.05"
				value={volume}
				disabled={panel.busy || !canCommand || !owns || !video || video.status === 'idle' || video.status === 'holding'}
				onchange={(event) => command('volume', Number(event.currentTarget.value))}
			/>
			<span class="cp-num">{Math.round(volume * 100)}%</span>
		</label>
		{#if video?.status === 'holding'}<span class="cp-label">Giữ hình cuối</span>{/if}
	{/if}
</div>

<style>
	.video {
		--cp-h: 38px;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px;
	}
	.volume {
		display: flex;
		align-items: center;
		gap: 6px;
		color: var(--cp-muted);
		font-size: 13px;
	}
	.volume :global(svg) {
		width: 18px;
		height: 18px;
	}
	.volume input {
		width: 110px;
		accent-color: var(--cp-text);
	}
</style>
