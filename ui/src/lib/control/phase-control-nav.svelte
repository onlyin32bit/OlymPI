<script lang="ts">
	import { gameController } from '$lib/control/game-controller';
	import { socket } from '$lib/socket';
	import type { Phases } from '$lib/types';
	import { cn } from '$lib/utils';
	import type { ScreenDefinition } from '$lib/display/phase-manifest';

	let { phase, screens }: { phase: Phases; screens: readonly ScreenDefinition[] } = $props();
	let selected = $derived(socket.controller.current.screen);
	let selectedIndex = $derived(screens.findIndex((screen) => screen.id === selected));
	let previousScreen = $derived(selectedIndex > 0 ? screens[selectedIndex - 1] : undefined);
	let nextScreen = $derived(
		selectedIndex >= 0 && selectedIndex < screens.length - 1
			? screens[selectedIndex + 1]
			: undefined
	);
	let busy = $state(false);
	let error = $state('');
	let sceneScreens = $derived(
		screens.filter(
			(screen) => screen.id !== 'ques_chung' && !/^(?:pre_)?ques_ts[1-4]$/.test(screen.id)
		)
	);

	function label(screen: ScreenDefinition): string {
		const labels: Record<string, string> = {
			main_kd: 'Chính',
			main_vcnv: 'Bảng',
			main_vd: 'Thí sinh',
			image_vcnv: 'Hình',
			answers_vcnv: 'Đáp án TS',
			answers_tt: 'Đáp án TS',
			solve: 'Lời giải',
			end_ts: 'Xong lượt',
			end: 'Kết thúc'
		};
		return labels[screen.id] ?? screen.label;
	}

	async function select(screen: string): Promise<void> {
		if (busy || selected === screen) return;
		error = '';
		busy = true;
		try {
			await gameController.showScreen(phase, screen);
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Không thể đổi màn hình';
		} finally {
			busy = false;
		}
	}
</script>

<nav class="scene-nav" aria-label="Màn hình của vòng">
	<div class="scene-step">
		<button
			disabled={busy || !previousScreen}
			aria-label="Màn hình trước"
			title={previousScreen ? `Màn hình trước: ${previousScreen.label}` : 'Màn hình trước'}
			onclick={() => previousScreen && void select(previousScreen.id)}>&lt;</button
		>
		<button
			disabled={busy || !nextScreen}
			aria-label="Màn hình tiếp theo"
			title={nextScreen ? `Màn hình tiếp theo: ${nextScreen.label}` : 'Màn hình tiếp theo'}
			onclick={() => nextScreen && void select(nextScreen.id)}>&gt;</button
		>
	</div>
	<div class="scene-buttons">
		{#each sceneScreens as screen (screen.id)}
			<button
				disabled={busy}
				aria-current={selected === screen.id ? 'step' : undefined}
				onclick={() => void select(screen.id)}
				class={cn(selected === screen.id && 'is-current')}
				title={screen.label}>{label(screen)}</button
			>
		{/each}
	</div>
	{#if error}<p class="scene-error" role="alert">{error}</p>{/if}
</nav>

<style>
	.scene-nav {
		display: flex;
		flex: none;
		flex-wrap: wrap;
		align-items: flex-start;
		gap: 6px;
		padding: 2px 8px;
		border-bottom: 1px solid #e2e8f0;
		background: #f8fafc;
	}
	.scene-step {
		display: flex;
		flex: none;
		gap: 4px;
	}
	.scene-buttons {
		display: flex;
		flex: 1;
		flex-wrap: wrap;
		min-width: 0;
		gap: 2px;
	}
	button {
		display: inline-flex;
		min-height: 32px;
		align-items: center;
		justify-content: center;
		padding: 3px 7px;
		border: 1px solid transparent;
		border-radius: 4px;
		color: #475569;
		font-size: 13px;
		font-weight: 500;
	}
	.scene-step button {
		width: 32px;
		height: 32px;
		padding: 0;
		border-color: #cbd5e1;
		background: white;
	}
	button:hover:not(:disabled) {
		background: #e2e8f0;
		color: #0f172a;
	}
	button.is-current {
		border-color: #bfdbfe;
		background: #eff6ff;
		color: #1d4ed8;
	}
	button:focus-visible {
		outline: 2px solid #2563eb;
		outline-offset: 2px;
	}
	button:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}
	.scene-error {
		flex-basis: 100%;
		color: #b91c1c;
		font-size: 13px;
	}
	@media (pointer: coarse) {
		button {
			min-height: 44px;
		}
		.scene-step button {
			width: 44px;
			height: 44px;
		}
	}
</style>
