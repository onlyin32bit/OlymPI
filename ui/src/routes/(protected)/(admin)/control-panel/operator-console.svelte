<script lang="ts">
	import { gameController } from '$lib/control/game-controller';
	import PhaseControlNav from '$lib/control/phase-control-nav.svelte';
	import { phaseFor } from '$lib/display/phase-manifest';
	import { socket } from '$lib/socket';
	import type { Phases } from '$lib/types';
	import { GAME_PHASES } from '.';
	import GamePhaseNav from './game-phase-nav.svelte';
	import RuntimeControl from './runtime-control.svelte';

	let controller = $derived(socket.controller.current);
	let activePhase = $derived(phaseFor(controller.phase));
	let currentScreenLabel = $derived(
		activePhase.screens.find((screen) => screen.id === controller.screen)?.label ??
			controller.screen
	);
	let applyingPhase = $state<Phases | null>(null);
	let phaseError = $state('');

	async function selectPhase(phase: Phases): Promise<void> {
		if (controller.phase === phase || applyingPhase !== null) return;
		phaseError = '';
		applyingPhase = phase;
		try {
			await gameController.changePhase(phase);
		} catch (cause) {
			phaseError = cause instanceof Error ? cause.message : 'Không thể đổi vòng thi';
		} finally {
			applyingPhase = null;
		}
	}
</script>

<section class="operator-console" aria-label="Điều khiển chương trình">
	<GamePhaseNav
		phases={GAME_PHASES}
		active={controller.phase}
		applying={applyingPhase}
		error={phaseError}
		onSelect={(phase) => void selectPhase(phase)}
	/>
	<div class="console-main">
		<h2 class="sr-only">{activePhase.label}</h2>
		<output class="sr-only">Đang phát: {currentScreenLabel}</output>
		<PhaseControlNav phase={activePhase.id} screens={activePhase.screens} />
		<RuntimeControl>
			{#if activePhase.control}
				{@const PhaseControl = activePhase.control}
				<PhaseControl phase={activePhase.id} screens={activePhase.screens} />
			{/if}
		</RuntimeControl>
	</div>
</section>

<style>
	.operator-console {
		display: flex;
		flex-direction: column;
		min-height: 0;
		min-width: 0;
		height: 100%;
		overflow: hidden;
		border: 1px solid #e2e8f0;
		border-radius: 6px;
		background: white;
		color: #0f172a;
	}
	.console-main {
		display: flex;
		flex: 1;
		flex-direction: column;
		min-height: 0;
		min-width: 0;
		overflow: hidden;
	}
	.operator-console :global(fieldset) {
		min-width: 0;
	}
	.operator-console :global(button) {
		cursor: pointer;
	}
	.operator-console :global(button:disabled) {
		cursor: not-allowed;
	}
</style>
