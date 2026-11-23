<script lang="ts">
	import { brand } from '$lib/brand';
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { Logo } from '$lib/components/common';
	import { socket } from '$lib/socket';
	import { bellView, inputOpen, mainView, me as findMe } from '$lib/station/view';
	import BellRing from '@lucide/svelte/icons/bell-ring';
	import Maximize from '@lucide/svelte/icons/maximize';
	import Minimize from '@lucide/svelte/icons/minimize';
	import AnswerDock from './answer-dock.svelte';
	import BellButton from './bell-button.svelte';
	import MainArea from './main-area.svelte';
	import TimerPanel from './timer-panel.svelte';

	let runtime = $derived(socket.match.current);
	let slot = $derived(api.auth.user?.fixed_slot ?? 0);
	let self = $derived(findMe(runtime, slot));
	let remaining = $derived(socket.timer.remaining);
	let bell = $derived(bellView(runtime, slot, remaining));
	let canType = $derived(inputOpen(runtime, slot, remaining));
	let main = $derived(mainView(runtime, slot));
	let typingRound = $derived(runtime?.controller.phase === 'vcnv' || runtime?.controller.phase === 'tang_toc');
	let connected = $state(true);
	let fullscreen = $state(false);

	onMount(() => {
		const poll = window.setInterval(() => (connected = socket.connected), 1000);
		const onFullscreen = () => (fullscreen = Boolean(document.fullscreenElement));
		document.addEventListener('fullscreenchange', onFullscreen);

		// Keep the station awake for the whole show; browsers drop the lock when hidden.
		let wakeLock: WakeLockSentinel | null = null;
		const keepAwake = async () => {
			if (document.visibilityState !== 'visible' || !('wakeLock' in navigator)) return;
			try {
				wakeLock = await navigator.wakeLock.request('screen');
			} catch {
				wakeLock = null;
			}
		};
		void keepAwake();
		document.addEventListener('visibilitychange', keepAwake);

		// A refresh mid-question would drop a typed answer; ask first while playing.
		const guard = (event: BeforeUnloadEvent) => {
			if (socket.match.current?.status === 'playing') event.preventDefault();
		};
		window.addEventListener('beforeunload', guard);
		return () => {
			window.clearInterval(poll);
			document.removeEventListener('fullscreenchange', onFullscreen);
			document.removeEventListener('visibilitychange', keepAwake);
			window.removeEventListener('beforeunload', guard);
			void wakeLock?.release().catch(() => undefined);
		};
	});

	async function toggleFullscreen(): Promise<void> {
		try {
			if (document.fullscreenElement) await document.exitFullscreen();
			else await document.documentElement.requestFullscreen();
		} catch {
			// Browsers may refuse without a user gesture; F11 still works.
		}
	}
</script>

<svelte:head><title>Thí sinh {slot || ''} - {api.settings.current?.event_name ?? brand.appName}</title></svelte:head>

<div class="st">
	<header class="head">
		<Logo color="blue" class="logo" />
		<p class="event">{api.settings.current?.event_name ?? ''}</p>
		<p class="identity">
			<span class="slot">TS{slot || '—'}</span>
			<strong>{self?.name ?? 'Chưa có thí sinh'}</strong>
			{#if self?.class}<span class="class">{self.class}</span>{/if}
		</p>
		<span class="connection" class:lost={!connected} role="status">
			<span class="dot" aria-hidden="true"></span>{connected ? 'Đã kết nối' : 'Mất kết nối'}
		</span>
		<button type="button" class="icon" onclick={toggleFullscreen} aria-label={fullscreen ? 'Thoát toàn màn hình' : 'Toàn màn hình'}>
			{#if fullscreen}<Minimize />{:else}<Maximize />{/if}
		</button>
	</header>

	<ol class="strip" aria-label="Điểm thí sinh">
		{#each [1, 2, 3, 4] as value (value)}
			{@const entry = runtime?.contestants.find((item) => item.slot === value)}
			<li class:rung={entry?.has_rung} class:self={value === slot}>
				<span class="name">{entry?.name || `Thí sinh ${value}`}</span>
				{#if entry?.has_rung}<BellRing class="bell-mark" aria-label="Đã bấm chuông" />{/if}
				<span class="score">{entry?.score ?? 0}</span>
			</li>
		{/each}
	</ol>

	<div class="body">
		<section class="left">
			<div class="main"><MainArea {runtime} {slot} view={main} /></div>
			<div class="echo" aria-live="polite">
				<span class="label">Câu trả lời đã ghi</span>
				<strong class="recorded">{typingRound && self?.answer ? self.answer : '—'}</strong>
				<span class="time">{typingRound && self?.answer ? `${((self.time_ms ?? 0) / 1000).toFixed(2)}s` : ''}</span>
			</div>
		</section>
		<aside class="right">
			<TimerPanel {runtime} {remaining} />
			<BellButton {bell} />
		</aside>
	</div>

	<AnswerDock open={canType} {runtime} {self} />
</div>

<style>
	.st {
		--line: #e4e4e7;
		--text: #18181b;
		--muted: #52525b;
		--faint: #71717a;
		--raised: #f4f4f5;
		display: grid;
		grid-template-rows: auto auto minmax(0, 1fr) auto;
		width: 100vw;
		height: 100dvh;
		overflow: hidden;
		background: #fff;
		color: var(--text);
		font-family: 'Inter Variable', 'Inter', system-ui, sans-serif;
		font-size: clamp(16px, 1.1vw + 0.6vh, 26px);
		font-variant-numeric: tabular-nums;
	}
	.head {
		display: flex;
		align-items: center;
		gap: 0.9em;
		padding: 0.45em 0.9em;
		border-bottom: 1px solid var(--line);
	}
	.head :global(.logo) {
		height: 1.9em;
		width: auto;
	}
	.event {
		color: var(--muted);
		font-weight: 600;
		white-space: nowrap;
	}
	.identity {
		display: flex;
		align-items: baseline;
		gap: 0.5em;
		min-width: 0;
		margin-left: auto;
		white-space: nowrap;
	}
	.identity .slot,
	.identity .class {
		color: var(--muted);
		font-size: 0.85em;
	}
	.identity strong {
		overflow: hidden;
		font-size: 1.15em;
		text-overflow: ellipsis;
		text-transform: uppercase;
	}
	.connection {
		display: inline-flex;
		align-items: center;
		gap: 0.4em;
		color: var(--muted);
		font-size: 0.75em;
		white-space: nowrap;
	}
	.connection .dot {
		width: 0.6em;
		height: 0.6em;
		border-radius: 999px;
		background: #16a34a;
	}
	.connection.lost {
		color: #b91c1c;
		font-weight: 700;
	}
	.connection.lost .dot {
		background: #dc2626;
	}
	.icon {
		display: grid;
		place-items: center;
		width: 2em;
		height: 2em;
		border-radius: 6px;
		color: var(--muted);
		cursor: pointer;
	}
	.icon:hover {
		background: var(--raised);
	}
	.icon :global(svg) {
		width: 1em;
		height: 1em;
	}
	.strip {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		border-bottom: 1px solid var(--line);
	}
	.strip li {
		display: flex;
		align-items: center;
		gap: 0.5em;
		min-width: 0;
		padding: 0.5em 0.9em;
		font-weight: 600;
	}
	.strip li + li {
		border-left: 1px solid var(--line);
	}
	.strip li.self {
		box-shadow: inset 0 -3px 0 var(--text);
	}
	.strip li.rung {
		background: #fef08a;
	}
	.strip .name {
		overflow: hidden;
		text-overflow: ellipsis;
		text-transform: uppercase;
		white-space: nowrap;
	}
	.strip :global(.bell-mark) {
		flex: none;
		width: 1em;
		height: 1em;
		color: #a16207;
	}
	.strip .score {
		margin-left: auto;
		font-size: 1.15em;
		font-weight: 700;
	}
	.body {
		display: grid;
		grid-template-columns: minmax(0, 1fr) clamp(220px, 22vw, 420px);
		min-height: 0;
	}
	.left {
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
		border-right: 1px solid var(--line);
	}
	.main {
		flex: 1;
		min-height: 0;
		overflow: hidden;
	}
	.echo {
		display: flex;
		flex: none;
		align-items: baseline;
		gap: 0.8em;
		padding: 0.5em 0.9em;
		border-top: 1px solid var(--line);
	}
	.echo .label {
		flex: none;
		color: var(--muted);
		font-size: 0.8em;
	}
	.echo .recorded {
		flex: 1;
		overflow: hidden;
		font-size: 1.5em;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.echo .time {
		flex: none;
		font-size: 1.3em;
		font-weight: 700;
	}
	.right {
		display: grid;
		grid-template-rows: minmax(0, 1fr) minmax(0, 1fr);
		min-height: 0;
	}
</style>
