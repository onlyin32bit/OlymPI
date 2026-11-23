<script lang="ts">
	import type { DisplayContext } from '$lib/display/types';
	import ContestantBoard from './contestant-board.svelte';

	let { context }: { context: DisplayContext } = $props();
</script>

<ContestantBoard {context} />
