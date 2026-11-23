<script lang="ts">
	import Question from './question.svelte';
	import PhaseEnd from '$lib/components/display/shared/phase-end.svelte';
	import PhaseIntro from '$lib/components/display/shared/phase-intro.svelte';
	import type { DisplayContext } from '$lib/display/types';

	// Legacy Kd.svelte branches not covered by the shared slides.
	let { context }: { context: DisplayContext } = $props();
</script>

{#if context.screen === 'main_kd'}
	<PhaseIntro {context} still />
{:else if context.screen === 'ques_chung' || /^ques_ts[1-4]$/.test(context.screen)}
	<Question {context} />
{:else if context.screen === 'end_ts'}
	<PhaseEnd {context} />
{/if}
