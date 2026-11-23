<script lang="ts">
	import type { DisplayContext } from '$lib/display/types';
	import Board from './Board.svelte';
	import Image from './Image.svelte';
	import Question from './Question.svelte';

	let { context }: { context: DisplayContext } = $props();
</script>

{#if context.screen === 'main_vcnv'}
	<Board {context} />
{:else if context.screen === 'image_vcnv'}
	<Image {context} />
{:else if context.screen === 'ques'}
	<Question {context} />
{/if}
