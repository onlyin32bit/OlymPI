<script lang="ts">
	import type { DisplayContext } from '$lib/display/types';
	import PreQuestion from './pre-question.svelte';
	import Question from './question.svelte';
	import ContestantEnd from './contestant-end.svelte';
	import PhaseIntro from '$lib/components/display/shared/phase-intro.svelte';

	// Legacy Vd.svelte: start/rule/intro/end are the shared scenes (phase manifest).
	let { context }: { context: DisplayContext } = $props();
</script>

{#if context.screen === 'main_vd'}
	<PhaseIntro {context} still />
{:else if context.screen.startsWith('pre_ques')}
	<PreQuestion {context} />
{:else if context.screen.startsWith('ques_ts')}
	<Question {context} />
{:else if context.screen === 'end_ts'}
	<ContestantEnd />
{/if}
