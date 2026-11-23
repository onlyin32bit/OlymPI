<script lang="ts">
	import { gameController } from '$lib/control/game-controller';
	import { panel } from '$lib/operator/panel.svelte';
	import { veDichSlot } from '$lib/operator/questions';
	import { SLOTS, type Slot } from '$lib/operator/scoring';
	import { socket } from '$lib/socket';
	import type { Screens, VDQuestionChoices } from '$lib/types';
	import Play from '@lucide/svelte/icons/play';
	import Star from '@lucide/svelte/icons/star';

	type Tier = '20' | '30';
	const TIERS: readonly Tier[] = ['20', '30'];

	let slot = $derived(veDichSlot(panel.runtime));
	let contestantState = $derived(panel.controller.ve_dich_contestants_state[slot - 1]);
	let selected = $derived(contestantState?.selected_question_set ?? []);
	let packageChosen = $derived(selected.length === 3);
	/** Follows the saved package; local edits stay until the next save or turn. */
	let choices = $derived<Tier[]>(
		selected.length === 3 ? selected.map((choice) => choice.slice(0, 2) as Tier) : ['20', '20', '20']
	);
	let tierValues = $derived(panel.drafts.rules.ve_dich.tiers);

	function pick(next: Slot): void {
		void panel.run(async () => {
			await socket.controller.selectContestant(next);
			await gameController.showScreen('ve_dich', `pre_ques_ts${next}` as Screens);
		});
	}

	function confirmPackage(): void {
		const owner = slot;
		if (!owner) return;
		const ids = choices.map((tier, index) => `${tier}_${index + 1}` as VDQuestionChoices);
		// Stays on the package screen; question 1 is opened with the question controls.
		void panel.run(() => socket.controller.selectVeDichQuestionSet(owner, ids));
	}
</script>

<div class="finish">
	<div class="picker" role="group" aria-label="Thí sinh về đích">
		{#each SLOTS as value (value)}
			{@const entry = panel.runtime?.contestants.find((item) => item.slot === value)}
			{@const onAir = slot === value && /^(?:pre_)?ques_ts[1-4]$/.test(panel.controller.screen)}
			<button
				type="button"
				class="cp-btn"
				class:is-live={onAir}
				class:is-selected={slot === value && !onAir}
				disabled={panel.busy || !entry}
				onclick={() => pick(value)}
			>
				<span class="name">{entry?.name || `TS${value}`}</span>
				<span class="score cp-num">{entry?.score ?? 0}</span>
			</button>
		{/each}
	</div>

	<div class="package" role="group" aria-label="Gói câu hỏi">
		{#each [0, 1, 2] as index (index)}
			{@const starred = contestantState?.starred_on === index + 1}
			<div class="choice">
				<span class="cp-label">Câu {index + 1}</span>
				<div class="tiers">
					{#each TIERS as tier, tierIndex (tier)}
						<button
							type="button"
							class="cp-btn"
							aria-pressed={choices[index] === tier}
							aria-label={`Câu ${index + 1}: ${tierValues[tierIndex]} điểm`}
							disabled={panel.busy || !slot}
							onclick={() => (choices = choices.map((value, i) => (i === index ? tier : value)))}
							>{tierValues[tierIndex]}</button
						>
					{/each}
					<button
						type="button"
						class="cp-btn star"
						class:is-armed={starred}
						aria-pressed={starred}
						aria-label={`Ngôi sao hy vọng câu ${index + 1}`}
						title="Ngôi sao hy vọng"
						disabled={panel.busy || !packageChosen}
						onclick={() => void panel.run(() => socket.controller.setVeDichStar(slot, index + 1))}><Star /></button
					>
				</div>
			</div>
		{/each}
		<button
			type="button"
			class="cp-btn is-primary confirm"
			disabled={panel.busy || !slot || !panel.runtime?.question_revision_id}
			onclick={confirmPackage}><Play />Chốt gói</button
		>
	</div>
</div>

<style>
	.finish {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.picker {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 6px;
	}
	.picker .cp-btn {
		justify-content: space-between;
	}
	.name {
		overflow: hidden;
		font-size: 15px;
		font-weight: 600;
		text-overflow: ellipsis;
	}
	.score {
		color: var(--cp-faint);
		font-size: 14px;
	}
	.package {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr)) minmax(0, 0.8fr);
		align-items: end;
		gap: 10px;
	}
	.choice {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.tiers {
		display: grid;
		grid-template-columns: 1fr 1fr auto;
		gap: 4px;
	}
	.tiers .cp-btn {
		padding: 0 6px;
		font-size: 16px;
		font-weight: 600;
	}
	.star {
		width: var(--cp-h);
		padding: 0;
	}
	.confirm {
		height: var(--cp-h);
	}
</style>
