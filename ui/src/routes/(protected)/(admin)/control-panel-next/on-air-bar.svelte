<script lang="ts">
	import { panel } from '$lib/operator/panel.svelte';
	import type { PanelThemeState } from '$lib/operator/theme.svelte';
	import { socket } from '$lib/socket';
	import Moon from '@lucide/svelte/icons/moon';
	import Sun from '@lucide/svelte/icons/sun';
	import House from '@lucide/svelte/icons/house';

	let { theme }: { theme: PanelThemeState } = $props();

	const statusLabels: Record<string, string> = {
		draft: 'Nháp',
		ready: 'Chưa bắt đầu',
		playing: 'Đang thi',
		paused: 'Tạm dừng',
		completed: 'Đã kết thúc',
		archived: 'Lưu trữ'
	};

	let runtime = $derived(panel.runtime);
	let screenLabel = $derived(
		panel.phase.screens.find((screen) => screen.id === panel.controller.screen)?.label ?? panel.controller.screen
	);
	let questionText = $derived(
		panel.selectedOption && panel.selectedIndex >= 0
			? `${panel.selectedOption.label} · ${panel.selectedIndex + 1}/${panel.strip.length}`
			: ''
	);
	let nextText = $derived(
		panel.nextEndsTurn
			? 'Kết thúc lượt'
			: panel.strip.length && panel.nextOption
			? panel.nextOption.label
			: panel.nextScreen
				? panel.nextScreen.label
				: ''
	);
	let warnings = $derived.by(() => {
		const list: { tone: 'bad' | 'armed'; text: string }[] = [];
		if (!panel.connected) list.push({ tone: 'bad', text: 'Mất kết nối máy chủ' });
		if (!runtime) list.push({ tone: 'armed', text: 'Chưa chọn trận' });
		else if (runtime.status !== 'playing')
			list.push({
				tone: runtime.status === 'paused' ? 'bad' : 'armed',
				text: runtime.status === 'paused' ? 'Trận đang tạm dừng · thao tác bị khóa' : (statusLabels[runtime.status] ?? runtime.status)
			});
		if (runtime?.status === 'playing' && socket.sound.status.output_count === 0)
			list.push({ tone: 'armed', text: 'Không có máy phát âm thanh' });
		if (panel.drafts.stale.length)
			list.push({ tone: 'armed', text: `Điểm chưa ghi: ${panel.drafts.stale.map((batch) => batch.label).join(', ')}` });
		return list;
	});
</script>

<header class="bar cp-panel">
	<a class="home" href="#/" aria-label="Về trang chủ" title="Về trang chủ"><House /></a>
	<p class="on-air" aria-live="polite">
		<span class="dot" class:idle={!runtime} aria-hidden="true"></span>
		<span class="sr-only">Đang phát:</span>
		<strong>{panel.phase.label}</strong>
		<span class="sep" aria-hidden="true">›</span>
		<span>{screenLabel}</span>
		{#if questionText}<span class="sep" aria-hidden="true">·</span><span class="cp-num">{questionText}</span>{/if}
	</p>
	{#if nextText}<p class="next"><span>Tiếp</span> {nextText}</p>{/if}
	<div class="alerts" role="status">
		{#if panel.error}<span class="chip bad" title={panel.error}>{panel.error}</span>{/if}
		{#each warnings as warning (warning.text)}
			<span class={`chip ${warning.tone}`} title={warning.text}>{warning.text}</span>
		{/each}
	</div>
	{#if runtime}<p class="match" title={runtime.match_name}>{runtime.match_name}</p>{/if}
	<button
		type="button"
		class="cp-btn is-ghost theme"
		aria-label={theme.mode === 'dark' ? 'Chuyển sang giao diện sáng' : 'Chuyển sang giao diện tối'}
		title={theme.mode === 'dark' ? 'Giao diện sáng' : 'Giao diện tối'}
		onclick={() => theme.toggle()}
	>
		{#if theme.mode === 'dark'}<Sun />{:else}<Moon />{/if}
	</button>
</header>

<style>
	.bar {
		display: flex;
		grid-column: 1 / -1;
		align-items: center;
		gap: 14px;
		padding: 0 6px 0 4px;
		overflow: hidden;
		font-size: 15px;
	}
	.home {
		display: grid;
		flex: none;
		place-items: center;
		width: 34px;
		height: 34px;
		border-radius: 6px;
		color: var(--cp-muted);
	}
	.home:hover {
		background: var(--cp-hover);
	}
	.home :global(svg) {
		width: 18px;
		height: 18px;
	}
	.on-air {
		display: flex;
		flex: none;
		align-items: center;
		gap: 8px;
		min-width: 0;
		white-space: nowrap;
	}
	.on-air strong {
		font-weight: 600;
	}
	.dot {
		width: 10px;
		height: 10px;
		border-radius: 999px;
		background: var(--cp-live);
	}
	.dot.idle {
		background: var(--cp-faint);
	}
	.sep {
		color: var(--cp-faint);
	}
	.next {
		flex: none;
		color: var(--cp-muted);
		white-space: nowrap;
	}
	.next span {
		margin-right: 4px;
		color: var(--cp-faint);
		font-size: 13px;
	}
	.alerts {
		display: flex;
		flex: 1;
		justify-content: flex-end;
		gap: 6px;
		min-width: 0;
		overflow: hidden;
	}
	.chip {
		overflow: hidden;
		max-width: 360px;
		padding: 4px 10px;
		border-radius: 999px;
		font-size: 13px;
		font-weight: 600;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.chip.armed {
		background: var(--cp-armed-bg);
		color: var(--cp-armed-ink);
	}
	.chip.bad {
		background: var(--cp-bad-bg);
		color: var(--cp-bad-ink);
	}
	.match {
		flex: none;
		overflow: hidden;
		max-width: 220px;
		color: var(--cp-muted);
		font-size: 13px;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.theme {
		--cp-h: 34px;
		flex: none;
		padding: 0;
	}
</style>
