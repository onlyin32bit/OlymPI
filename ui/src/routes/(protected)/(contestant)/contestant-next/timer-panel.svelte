<script lang="ts">
	import type { MatchRuntimeState } from '$lib/socket/opsocket';
	import { formatCountdown, stealWindow } from '$lib/station/view';

	let { runtime, remaining }: { runtime: MatchRuntimeState | null; remaining: number } = $props();

	let timer = $derived(runtime?.timer);
	let running = $derived(Boolean(timer?.is_running && !timer.is_paused && remaining > 0));
	let urgent = $derived(running && remaining <= 5000);
	let total = $derived(timer?.duration_ms ?? 0);
	/** Remaining share of the clock; empty once time is up. */
	let fraction = $derived(
		total <= 0 ? 0 : Math.min(1, Math.max(0, (running ? remaining : timer?.is_paused ? timer.remaining_ms : 0) / total))
	);
	let message = $derived(
		runtime?.status === 'paused' || timer?.is_paused
			? 'Tạm dừng'
			: running
				? ''
				: total > 0
					? 'Hết giờ'
					: '—'
	);
</script>

<div class="timer" class:urgent class:steal={stealWindow(runtime)} role="timer" aria-label="Thời gian còn lại">
	<span class="label">{stealWindow(runtime) ? 'Cướp quyền' : 'Thời gian'}</span>
	{#if message}
		<strong class="message">{message}</strong>
	{:else}
		<strong class="count">{formatCountdown(remaining)}</strong>
		<span class="total">/ {Math.round(total / 1000)} giây</span>
	{/if}
	<div class="bar" aria-hidden="true"><span style:width={`${fraction * 100}%`}></span></div>
</div>

<style>
	.timer {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.2em;
		min-height: 0;
		padding: 0.8em;
	}
	.label {
		color: var(--muted);
		font-size: 0.8em;
		font-weight: 600;
		text-transform: uppercase;
	}
	.count {
		font-size: 3.6em;
		font-weight: 800;
		line-height: 1;
	}
	.message {
		font-size: 1.6em;
		font-weight: 700;
		text-align: center;
	}
	.total {
		color: var(--faint);
		font-size: 0.8em;
	}
	.bar {
		width: 100%;
		height: 0.45em;
		margin-top: 0.5em;
		overflow: hidden;
		border-radius: 999px;
		background: var(--raised);
	}
	.bar span {
		display: block;
		height: 100%;
		background: var(--text);
	}
	.urgent .count,
	.steal .count {
		color: #dc2626;
	}
	.urgent .bar span,
	.steal .bar span {
		background: #dc2626;
	}
	.steal .label {
		color: #dc2626;
	}
</style>
