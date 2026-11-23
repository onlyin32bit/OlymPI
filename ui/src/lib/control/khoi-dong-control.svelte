<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { openLiveQuestion } from '$lib/control/question-workflow';
	import { socket } from '$lib/socket';
	import type { PhaseControlProps } from '$lib/display/phase-manifest';

	let { phase }: PhaseControlProps = $props();
	let controller = $derived(socket.controller.current);
	let contestants = $derived(socket.match.current?.contestants ?? []);
	let busy = $state(false);
	let error = $state('');

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

	function startTurn(slot: number): void {
		const prefix = slot === 0 ? 'kd_chung.' : `kd_rieng.${slot - 1}.`;
		const questionID = controller.selected_question_id.startsWith(prefix)
			? controller.selected_question_id
			: `${prefix}0`;
		void run(() => openLiveQuestion(phase, questionID));
	}
</script>

<div class="turn-deck" role="group" aria-label="Lượt thi và khóa thí sinh">
	<Button
		size="sm"
		class="common"
		variant={controller.screen === 'ques_chung' ? 'default' : 'outline'}
		aria-pressed={controller.screen === 'ques_chung'}
		disabled={busy || !socket.match.current?.question_revision_id}
		onclick={() => startTurn(0)}>Chung</Button
	>
	{#each [1, 2, 3, 4] as slot (slot)}
		{@const contestant = contestants.find((entry) => entry.slot === slot)}
		<div class="turn">
			<Button
				size="sm"
				variant={controller.screen === `ques_ts${slot}` ? 'default' : 'outline'}
				aria-pressed={controller.screen === `ques_ts${slot}`}
				disabled={busy || !contestant || !socket.match.current?.question_revision_id}
				onclick={() => startTurn(slot)}>{contestant?.name || `Thí sinh ${slot}`}</Button
			>
			<Button
				size="sm"
				variant={contestant?.locked ? 'secondary' : 'outline'}
				aria-label={`${contestant?.name || `Thí sinh ${slot}`}: ${contestant?.locked ? 'Mở khóa' : 'Khóa'}`}
				aria-pressed={contestant?.locked ?? false}
				disabled={busy || !contestant}
				onclick={() => void run(() => socket.match.setContestantLock(slot, !contestant?.locked))}
				>{contestant?.locked ? 'Mở khóa' : 'Khóa'}</Button
			>
		</div>
	{/each}
	{#if error}<p role="alert">{error}</p>{/if}
</div>

<style>
	.turn-deck {
		display: grid;
		grid-template-columns: minmax(64px, 0.65fr) repeat(4, minmax(0, 1fr));
		gap: 4px;
		min-width: 0;
	}
	.turn {
		display: grid;
		gap: 3px;
		min-width: 0;
	}
	.turn-deck :global(button) {
		height: 36px;
		min-height: 36px;
		padding: 0 6px;
		border-radius: 3px;
		font-size: 13px;
	}
	.turn :global(button:first-child) {
		height: auto;
		min-height: 36px;
		white-space: normal;
		line-height: 16px;
	}
	p {
		grid-column: 1 / -1;
		margin: 0;
		font-size: 12px;
		color: var(--destructive);
	}
</style>
