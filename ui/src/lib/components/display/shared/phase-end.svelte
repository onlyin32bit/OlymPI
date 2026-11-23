<script lang="ts">
	import { brand } from '$lib/brand';
	import type { DisplayContext } from '$lib/display/types';
	import { slide } from 'svelte/transition';

	// eslint-disable-next-line @typescript-eslint/no-unused-vars -- End accepts the shared display prop without rendering runtime state.
	let { context: _context }: { context: DisplayContext } = $props();
</script>

<!-- Legacy ScreenEnd.svelte. -->
<div class="fixed h-full w-full bg-bg-3 bg-cover bg-no-repeat">
	<img class="center-element fixed h-[50cqh]" src="/4t.png" alt="Logo {brand.appName}" in:slide|global={{ duration: 4000 }} />
</div>
