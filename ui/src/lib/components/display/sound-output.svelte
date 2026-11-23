<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { AudioEngine } from '$lib/audio/engine';
	import { Button } from '$lib/components/ui/button';
	import { socket } from '$lib/socket';
	import type { SiteSettings } from '$lib/types';
	import Volume2 from '@lucide/svelte/icons/volume-2';

	const engine = new AudioEngine();
	let enabled = $state(false);
	let busy = $state(false);
	let error = $state('');
	let settings = $state<SiteSettings | null>(null);
	let outputName = $state('Mặc định hệ thống');
	let mounted = false;
	let generation = 0;

	onMount(() => {
		mounted = true;
		const offPlayed = socket.sound.onPlayed((playback) => {
			if (enabled)
				void engine.play(playback, socket.sound.localStartTime(playback.start_at)).catch(showError);
		});
		const offStopped = socket.sound.onStopped((stop) => engine.stop(stop));
		const offStatus = socket.sound.onStatus((status) => {
			if (enabled) {
				void engine
					.reconcile(status, (timestamp) => socket.sound.localStartTime(timestamp))
					.catch(showError);
			}
		});
		const offSettings = socket.sound.onSettingsUpdated((soundSettings) => {
			if (settings) settings = { ...settings, sound: soundSettings };
			engine.setSettings(soundSettings);
		});
		const offCatalog = socket.sound.onCatalogUpdated(() => {
			if (enabled) void reloadCatalog();
		});
		return () => {
			mounted = false;
			generation++;
			enabled = false;
			offPlayed();
			offStopped();
			offStatus();
			offSettings();
			offCatalog();
			void socket.sound.disableOutput().catch(() => undefined);
			void engine.dispose().catch(() => undefined);
		};
	});

	async function enableAudio(): Promise<void> {
		if (!mounted || busy) return;
		const operation = ++generation;
		busy = true;
		error = '';
		try {
			const [site, catalog] = await Promise.all([api.settings.get(), api.sounds.list()]);
			if (!mounted || operation !== generation) return;
			settings = site;
			await engine.enable(site.sound, catalog);
			if (!mounted || operation !== generation) return;
			const status = await socket.sound.enableOutput();
			if (!mounted || operation !== generation) return;
			enabled = true;
			await engine.reconcile(status, (timestamp) => socket.sound.localStartTime(timestamp));
		} catch (cause) {
			if (!mounted || operation !== generation) return;
			enabled = false;
			showError(cause);
			await Promise.allSettled([socket.sound.disableOutput(), engine.disable()]);
		} finally {
			if (mounted && operation === generation) busy = false;
		}
	}

	async function disableAudio(): Promise<void> {
		if (!mounted || busy) return;
		const operation = ++generation;
		busy = true;
		enabled = false;
		const results = await Promise.allSettled([socket.sound.disableOutput(), engine.disable()]);
		if (!mounted || operation !== generation) return;
		const failure = results.find((result) => result.status === 'rejected');
		if (failure?.status === 'rejected') showError(failure.reason);
		busy = false;
	}

	async function test(scope: 'this' | 'all'): Promise<void> {
		if (!mounted || !enabled || busy) return;
		const operation = generation;
		busy = true;
		error = '';
		try {
			const playback = await socket.sound.test(scope);
			if (!mounted || !enabled || operation !== generation) return;
			await engine.play(playback, socket.sound.localStartTime(playback.start_at));
		} catch (cause) {
			showError(cause);
		} finally {
			if (mounted && operation === generation) busy = false;
		}
	}

	async function chooseOutput(): Promise<void> {
		if (!mounted || busy) return;
		const operation = generation;
		busy = true;
		error = '';
		try {
			const name = await engine.chooseOutput();
			if (mounted && operation === generation) outputName = name;
		} catch (cause) {
			showError(cause);
		} finally {
			if (mounted && operation === generation) busy = false;
		}
	}

	async function reloadCatalog(): Promise<void> {
		try {
			const catalog = await api.sounds.list();
			if (!mounted || !enabled) return;
			await engine.setCatalog(catalog);
		} catch (cause) {
			showError(cause);
		}
	}

	function showError(cause: unknown): void {
		if (!mounted) return;
		error = cause instanceof Error ? cause.message : 'Không thể phát âm thanh';
	}
</script>

{#if !enabled}
	<!-- Legacy routes/display/+page.svelte button (daisyUI `btn btn-primary`). -->
	<button
		type="button"
		class="legacy-btn pointer-events-auto fixed right-6 bottom-6 z-[100]"
		aria-label="Bật âm thanh"
		disabled={busy}
		onclick={enableAudio}
	>
		Bật âm thanh
	</button>
{:else}
	<!-- Invisible to the audience; operators can hover the corner for output controls. -->
	<div class="group/audio fixed right-0 bottom-0 z-[100] flex h-28 w-80 items-end justify-end p-3">
		<div
			class="pointer-events-none flex max-w-72 translate-y-1 flex-col gap-2 rounded-lg bg-slate-950/65 p-3 text-sm text-white opacity-0 shadow-lg backdrop-blur-sm transition group-hover/audio:pointer-events-auto group-hover/audio:translate-y-0 group-hover/audio:opacity-100 focus-within:pointer-events-auto focus-within:translate-y-0 focus-within:opacity-100"
		>
			<div class="flex items-center gap-2 font-medium"><Volume2 />Âm thanh đã bật</div>
			<p class="max-w-64 truncate text-xs text-slate-200" title={outputName}>Loa: {outputName}</p>
			{#if error}<p class="text-xs text-amber-200" role="alert">{error}</p>{/if}
			<div class="flex flex-wrap justify-end gap-2">
				<Button size="sm" variant="secondary" disabled={busy} onclick={() => test('this')}
					>Thử máy này</Button
				>
				{#if api.auth.user?.role === 'admin'}
					<Button size="sm" variant="secondary" disabled={busy} onclick={() => test('all')}
						>Thử tất cả</Button
					>
				{/if}
				{#if engine.canChooseOutput}
					<Button size="sm" variant="secondary" disabled={busy} onclick={chooseOutput}
						>Chọn loa</Button
					>
				{/if}
				<Button size="sm" variant="outline" disabled={busy} onclick={disableAudio}>Tắt</Button>
			</div>
		</div>
	</div>
{/if}

<style>
	/* daisyUI 4 `.btn.btn-primary`, light theme, legacy font-sans. */
	.legacy-btn {
		display: inline-flex;
		height: 3rem;
		min-height: 3rem;
		flex-shrink: 0;
		cursor: pointer;
		user-select: none;
		flex-wrap: wrap;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		padding: 0 1rem;
		border: 1px solid oklch(49.12% 0.3096 275.75);
		border-radius: 0.5rem;
		background-color: oklch(49.12% 0.3096 275.75);
		color: oklch(89.824% 0.06192 275.75);
		font-family: 'Segoe UI', 'Selawik', system-ui, sans-serif;
		font-size: 0.875rem;
		font-weight: 600;
		line-height: 1em;
		text-align: center;
		text-decoration-line: none;
		outline-color: oklch(49.12% 0.3096 275.75);
		box-shadow: 0 1px 2px 0 rgb(0 0 0 / 0.05);
		transition-property: color, background-color, border-color, opacity, box-shadow, transform;
		transition-duration: 200ms;
		transition-timing-function: cubic-bezier(0, 0, 0.2, 1);
	}
	@media (hover: hover) {
		.legacy-btn:hover {
			background-color: color-mix(in oklab, oklch(49.12% 0.3096 275.75) 90%, black);
			border-color: color-mix(in oklab, oklch(49.12% 0.3096 275.75) 90%, black);
		}
	}
	.legacy-btn:active:hover,
	.legacy-btn:active:focus {
		transform: scale(0.95);
	}
	.legacy-btn:focus-visible {
		outline-style: solid;
		outline-width: 2px;
		outline-offset: 2px;
	}
	.legacy-btn:disabled {
		pointer-events: none;
	}
</style>
