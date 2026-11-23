<script lang="ts">
	import { onMount } from 'svelte';
	import './panel.css';
	import { api } from '$lib/api';
	import { Display } from '$lib/components/display';
	import LogPanel from '$lib/components/operator/log-panel.svelte';
	import Soundboard from '$lib/components/operator/soundboard.svelte';
	import { APP_NAME } from '$lib/constant';
	import {
		ACTIVATION_KEYS,
		DoublePressGuard,
		ignoresShortcuts,
		resolveShortcut,
		type ShortcutAction
	} from '$lib/operator/keyboard';
	import { panel } from '$lib/operator/panel.svelte';
	import { PanelThemeState } from '$lib/operator/theme.svelte';
	import { socket } from '$lib/socket';
	import type { Sound } from '$lib/types';
	import ContestantCards from './contestant-cards.svelte';
	import Controller from './controller.svelte';
	import OnAirBar from './on-air-bar.svelte';
	import OperatorSlot from './operator-slot.svelte';
	import ScoringDialog from './scoring-dialog.svelte';

	type DetailTab = 'contestants' | 'sounds' | 'log';
	const detailTabs: { id: DetailTab; label: string }[] = [
		{ id: 'contestants', label: 'Thí sinh' },
		{ id: 'sounds', label: 'Âm thanh' },
		{ id: 'log', label: 'Nhật ký' }
	];

	const theme = new PanelThemeState();
	const guard = new DoublePressGuard(300);
	let detailTab = $state<DetailTab>('contestants');
	let sounds = $state<Sound[]>([]);
	let visibleSounds = $derived(sounds.filter((sound) => sound.enabled));

	onMount(() => {
		const detachTheme = theme.attach();
		const detachPanel = panel.attach();
		void loadSounds();
		const offCatalog = socket.sound.onCatalogUpdated(() => void loadSounds());
		const refreshSettings = () => void api.settings.get().catch(() => undefined);
		refreshSettings();
		window.addEventListener('focus', refreshSettings);
		return () => {
			detachTheme();
			detachPanel();
			offCatalog();
			window.removeEventListener('focus', refreshSettings);
		};
	});

	async function loadSounds(): Promise<void> {
		try {
			sounds = await api.sounds.manage();
		} catch {
			sounds = [];
		}
	}

	async function finishVeDichVideo(questionID: string, sessionID: number): Promise<void> {
		const current = socket.match.current;
		const video = current?.controller.ve_dich_video;
		if (current?.current_question?.id !== questionID || video?.session_id !== sessionID) return;
		await socket.controller.veDichVideo('finish', { question_id: questionID, session_id: sessionID });
	}

	function dispatch(action: ShortcutAction): void {
		switch (action.type) {
			case 'screen':
				return panel.stepScreen(action.direction);
			case 'question':
				return panel.stepQuestion(action.direction);
			case 'timer':
				return panel.toggleTimer();
			case 'toggle-question':
				return panel.toggleShowQuestion();
			case 'verdict':
				return panel.verdict(action.slot, action.verdict);
			case 'save':
				return panel.save();
			case 'steal':
				return panel.steal();
			case 'toggle-log':
				return panel.toggleView();
			case 'stop-sound':
				return panel.stopSound();
		}
	}

	function onKeydown(event: KeyboardEvent): void {
		if (ACTIVATION_KEYS[event.code] && (event.target as Element | null)?.closest?.('.quick-actions button')) {
			if (event.repeat) event.preventDefault();
			return;
		}
		if (ignoresShortcuts(event.target)) return;
		const action = resolveShortcut(event);
		if (!action) return;
		// The panel owns these keys; a focused button must not also activate.
		event.preventDefault();
		if (guard.accept(action, performance.now())) dispatch(action);
	}

	function onKeyup(event: KeyboardEvent): void {
		if (ACTIVATION_KEYS[event.code] && (event.target as Element | null)?.closest?.('.quick-actions button')) return;
		if (ACTIVATION_KEYS[event.code] && !ignoresShortcuts(event.target)) event.preventDefault();
	}
</script>

<svelte:head>
	<title>Bảng điều khiển - {APP_NAME}</title>
</svelte:head>

<svelte:window onkeydowncapture={onKeydown} onkeyupcapture={onKeyup} />

<main class="cp">
	<OnAirBar {theme} />
	<div class="left">
		<section class="program" aria-label="Màn hình đang phát">
			<Display preview onVeDichVideoFinished={finishVeDichVideo} />
		</section>
		<OperatorSlot />
	</div>
	<section class="details cp-panel" aria-label="Thí sinh, âm thanh và nhật ký">
		<div class="detail-tabs" role="tablist">
			{#each detailTabs as tab (tab.id)}
				<button
					type="button"
					role="tab"
					aria-selected={detailTab === tab.id}
					class:active={detailTab === tab.id}
					onclick={() => (detailTab = tab.id)}>{tab.label}</button
				>
			{/each}
			<div class="tab-tools"><ScoringDialog /></div>
		</div>
		<div class="detail-body">
			{#if detailTab === 'contestants'}
				<ContestantCards />
			{:else if detailTab === 'sounds'}
				<Soundboard
					sounds={visibleSounds}
					onUpdated={(updated) =>
						(sounds = sounds.map((sound) => (sound.id === updated.id ? updated : sound)))}
				/>
			{:else}
				<LogPanel />
			{/if}
		</div>
	</section>
	<Controller />
</main>

<style>
	.cp {
		display: grid;
		grid-template-columns: minmax(0, 0.72fr) minmax(0, 1fr);
		grid-template-rows: 44px minmax(0, 40fr) minmax(0, 60fr);
		gap: 6px;
		width: 100vw;
		height: 100vh;
		padding: 6px;
		overflow: hidden;
		background: var(--cp-ground);
		color: var(--cp-text);
		font-family: var(--cp-font);
	}
	.left {
		display: flex;
		grid-row: 2 / 4;
		flex-direction: column;
		gap: 6px;
		min-width: 0;
		min-height: 0;
	}
	.program {
		flex: none;
		aspect-ratio: 16 / 9;
		width: 100%;
		overflow: hidden;
		border-radius: 8px;
		background: #000;
	}
	.details {
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}
	.detail-tabs {
		display: flex;
		flex: none;
		align-items: stretch;
		height: 40px;
		padding: 0 6px;
		border-bottom: 1px solid var(--cp-line);
	}
	.detail-tabs > button {
		padding: 0 14px;
		border-bottom: 2px solid transparent;
		color: var(--cp-muted);
		font: 500 14px/1 var(--cp-font);
		cursor: pointer;
	}
	.detail-tabs > button.active {
		border-bottom-color: var(--cp-text);
		color: var(--cp-text);
		font-weight: 600;
	}
	.detail-tabs > button:focus-visible {
		outline: 2px solid var(--cp-text);
		outline-offset: -2px;
	}
	.tab-tools {
		display: flex;
		align-items: center;
		margin-left: auto;
	}
	.detail-body {
		flex: 1;
		min-height: 0;
		overflow: hidden;
	}
</style>
