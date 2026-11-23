<script lang="ts">
	import { serverClock } from '$lib/display/clock.svelte';
	import { assetURL, VE_DICH_VIDEO_FINAL_FRAME_MS, type DisplayContext } from '$lib/display/types';
	import { socket } from '$lib/socket';
	import { onChosenSinkId } from '$lib/audio/output';

	type VideoQuestion = { id: string; type: string; asset_id?: string };

	/**
	 * Full-screen VĐ video takeover, synced to the server clock: position = position_ms +
	 * (serverNow − started_at) while playing; paused holds position_ms; holding shows the last
	 * frame. Never shows prompts or controls: a rejected unmuted play falls back to muted.
	 */
	let { context, question }: { context: DisplayContext; question: VideoQuestion | null | undefined } =
		$props();
	const clock = serverClock();
	const DRIFT_S = 0.3;

	let player = $state<HTMLVideoElement>();
	let forcedMute = $state(false);
	let playing = false;
	let finishTimeout: ReturnType<typeof setTimeout> | undefined;
	let reportedSession = 0;

	// Follow the speaker chosen on this TV ("Chọn loa"); the preview stays on its default.
	$effect(() => {
		const element = player as (HTMLVideoElement & { setSinkId?: (id: string) => Promise<void> }) | undefined;
		if (!element?.setSinkId || context.preview) return;
		return onChosenSinkId((id) => {
			if (id) void element.setSinkId?.(id).catch(() => undefined);
		});
	});

	let playback = $derived(context.match?.controller.ve_dich_video);
	let session = $derived(playback?.session_id ?? 0);
	let status = $derived(playback?.status ?? 'idle');
	let matchPlaying = $derived(context.match?.status === 'playing');
	let visible = $derived(
		question?.type === 'video' &&
			Boolean(question.asset_id) &&
			status !== 'idle' &&
			Boolean(status) &&
			playback?.question_id === question.id
	);
	let muted = $derived(context.preview || forcedMute);

	function targetSeconds(element: HTMLVideoElement): number {
		const duration = Number.isFinite(element.duration) ? element.duration : Infinity;
		if (status === 'holding') return Number.isFinite(duration) ? Math.max(0, duration - 0.04) : 0;
		let position = playback?.position_ms ?? 0;
		if (status === 'playing' && matchPlaying && playback?.started_at) {
			position += Math.max(0, clock.now - playback.started_at);
		}
		return Math.min(position / 1000, Number.isFinite(duration) ? duration : position / 1000);
	}

	function startPlaying(element: HTMLVideoElement): void {
		if (playing) return;
		playing = true;
		element.play().catch(() => {
			if (!element.muted) {
				forcedMute = true;
				element.muted = true;
				element.play().catch(() => (playing = false));
			} else {
				playing = false;
			}
		});
	}

	function stopPlaying(element: HTMLVideoElement): void {
		playing = false;
		if (!element.paused) element.pause();
	}

	// Follow the server clock every frame; seek only on noticeable drift.
	$effect(() => {
		void clock.now;
		const element = player;
		if (!element || !visible || element.readyState < 1) return;
		element.volume = Math.max(0, Math.min(1, playback?.volume ?? 1));
		element.muted = muted;
		const target = targetSeconds(element);
		if (Math.abs(element.currentTime - target) > DRIFT_S || status !== 'playing') {
			if (Math.abs(element.currentTime - target) > 0.01) element.currentTime = target;
		}
		if (status === 'playing' && matchPlaying && target < element.duration) startPlaying(element);
		else stopPlaying(element);
	});

	$effect(() => {
		if (session === 0) return;
		forcedMute = false;
		playing = false;
	});

	function onMetadata(): void {
		const element = player;
		if (!element || !context.preview || !question || reportedSession === session) return;
		if (!Number.isFinite(element.duration) || element.duration <= 0) return;
		reportedSession = session;
		void socket.controller
			.veDichVideo('duration', {
				session_id: session,
				question_id: question.id,
				duration_ms: Math.round(element.duration * 1000)
			})
			.catch((cause) => console.error('Unable to report the Về đích video duration', cause));
	}

	// Fallback: the server finishes the video itself; the operator preview may still nudge it.
	function onEnded(): void {
		playing = false;
		const activeSession = session;
		const questionID = question?.id;
		if (finishTimeout || !questionID || !context.onVeDichVideoFinished) return;
		finishTimeout = setTimeout(() => {
			finishTimeout = undefined;
			if (!visible || session !== activeSession || !matchPlaying) return;
			void Promise.resolve(context.onVeDichVideoFinished?.(questionID, activeSession)).catch((cause) =>
				console.error('Unable to start the Về đích timer after video playback', cause)
			);
		}, VE_DICH_VIDEO_FINAL_FRAME_MS);
	}

	$effect(() => () => {
		if (finishTimeout) clearTimeout(finishTimeout);
	});
</script>

{#if visible && question?.asset_id}
	<div class="video-takeover">
		{#key `${question.id}:${session}`}
			<video
				bind:this={player}
				class="video"
				src={assetURL(question.asset_id)}
				playsinline
				preload="auto"
				{muted}
				onloadedmetadata={onMetadata}
				onended={onEnded}
			></video>
		{/key}
	</div>
{/if}

<style>
	.video-takeover {
		position: absolute;
		inset: 0;
		z-index: 1000;
		display: flex;
		align-items: center;
		justify-content: center;
		overflow: hidden;
		background: #000;
	}
	.video {
		width: 100%;
		height: 100%;
		object-fit: contain;
		background: #000;
	}
</style>
