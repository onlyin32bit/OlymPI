<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { socket } from '$lib/socket';

	let { busy, onchange }: { busy: boolean; onchange: (slot: number, correct: boolean) => void } =
		$props();
	let controller = $derived(socket.controller.current);
	let contestants = $derived(socket.match.current?.contestants ?? []);
</script>

<div class="review-surface">
	<div class="review-grid" role="group" aria-label="Chấm đáp án">
		{#each [1, 2, 3, 4] as slot (slot)}
			{@const contestant = contestants.find((contestant) => contestant.slot === slot)}
			{@const correct = controller.answer_status[slot - 1] ?? true}
			<div class="contestant">
				<span>{contestant?.name || `Thí sinh ${slot}`}</span>
				<div class="verdict">
					<Button
						size="sm"
						variant={correct ? 'default' : 'outline'}
						aria-pressed={correct}
						aria-label={`Thí sinh ${slot}: Đúng`}
						disabled={busy}
						onclick={() => onchange(slot, true)}>Đúng</Button
					>
					<Button
						size="sm"
						variant={!correct ? 'destructive' : 'outline'}
						aria-pressed={!correct}
						aria-label={`Thí sinh ${slot}: Sai`}
						disabled={busy}
						onclick={() => onchange(slot, false)}>Sai</Button
					>
				</div>
			</div>
		{/each}
	</div>
</div>

<style>
	.review-surface {
		container-type: inline-size;
		min-width: 0;
	}
	.review-grid {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 4px;
	}
	.contestant {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		align-items: center;
		gap: 2px;
		min-width: 0;
		border-top: 1px solid var(--border);
		padding-top: 2px;
	}
	.contestant > span {
		display: block;
		font-size: 13px;
		line-height: 16px;
		font-weight: 500;
		overflow-wrap: anywhere;
	}
	.verdict {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 3px;
	}
	.verdict :global(button) {
		height: 36px;
		min-height: 36px;
		padding: 0 4px;
		border-radius: 3px;
		font-size: 13px;
	}
	@container (min-width: 500px) {
		.contestant {
			grid-template-columns: minmax(0, 1fr) 83px;
		}
	}
</style>
