<script lang="ts">
	import ScreenQuestionTT from './ScreenQuestionTT.svelte';
	import ScreenSolvedTT from './ScreenSolvedTT.svelte';
	import type { DisplayContext } from '$lib/display/types';

	let { context }: { context: DisplayContext } = $props();
</script>

{#if context.screen === 'ques'}
	<ScreenQuestionTT {context} />
{:else if context.screen === 'solve'}
	<ScreenSolvedTT {context} />
{/if}
