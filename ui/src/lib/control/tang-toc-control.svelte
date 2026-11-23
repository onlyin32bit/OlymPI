<script lang="ts">
	import AnswerReviewControl from '$lib/control/answer-review-control.svelte';
	import { socket } from '$lib/socket';
	import type { PhaseControlProps } from '$lib/display/phase-manifest';

	let { phase }: PhaseControlProps = $props();
	let busy = $state(false);
	let error = $state('');
	let controller = $derived(socket.controller.current);

	async function review(slot: number, correct: boolean): Promise<void> {
		if (busy) return;
		busy = true;
		error = '';
		try {
			await socket.controller.setAnswerStatus(slot, correct);
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Thao tác thất bại';
		} finally {
			busy = false;
		}
	}
</script>

<div class="speed-deck" data-phase={phase}>
	<AnswerReviewControl
		busy={busy || !controller.selected_question_id}
		onchange={(slot, correct) => void review(slot, correct)}
	/>
	{#if error}<p class="text-xs text-destructive" role="alert">{error}</p>{/if}
</div>
