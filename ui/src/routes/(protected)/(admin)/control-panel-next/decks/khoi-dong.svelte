<script lang="ts">
	import { panel } from '$lib/operator/panel.svelte';

	type Turn = { slot: number; label: string; prefix: string; screen: string };

	let contestants = $derived(panel.runtime?.contestants ?? []);
	let turns = $derived<Turn[]>([
		{ slot: 0, label: 'Chung', prefix: 'kd_chung.', screen: 'ques_chung' },
		...[1, 2, 3, 4].map((slot) => ({
			slot,
			label: contestants.find((entry) => entry.slot === slot)?.name || `Thí sinh ${slot}`,
			prefix: `kd_rieng.${slot - 1}.`,
			screen: `ques_ts${slot}`
		}))
	]);

	function questionsOf(turn: Turn) {
		return panel.options.filter((option) => option.id.startsWith(turn.prefix));
	}

	/** Resumes the turn: the selected question if it belongs here, else the first not yet asked. */
	function start(turn: Turn): void {
		const selected = panel.controller.selected_question_id;
		if (selected.startsWith(turn.prefix)) return panel.openQuestion(selected);
		const list = questionsOf(turn);
		const next = list.find((option) => !panel.asked.includes(option.id)) ?? list[0];
		if (next) panel.openQuestion(next.id);
	}
</script>

<div class="turns" role="group" aria-label="Lượt thi">
	{#each turns as turn (turn.slot)}
		{@const list = questionsOf(turn)}
		{@const done = list.filter((option) => panel.asked.includes(option.id)).length}
		<button
			type="button"
			class="cp-btn turn"
			class:is-live={panel.controller.screen === turn.screen}
			disabled={panel.busy || !list.length}
			onclick={() => start(turn)}
		>
			<span class="label">{turn.label}</span>
			<span class="progress cp-num">{done}/{list.length}</span>
		</button>
	{/each}
</div>

<style>
	.turns {
		display: grid;
		grid-template-columns: repeat(5, minmax(0, 1fr));
		gap: 6px;
	}
	.turn {
		flex-direction: column;
		gap: 4px;
		--cp-h: calc(var(--cp-h-deck) * 1.8);
		padding: 0 8px;
	}
	.label {
		overflow: hidden;
		max-width: 100%;
		font-size: 16px;
		font-weight: 600;
		text-overflow: ellipsis;
	}
	.progress {
		color: var(--cp-faint);
		font-size: 13px;
	}
	.turn.is-live .progress {
		color: inherit;
	}
</style>
