<script lang="ts">
	import { brand } from '$lib/brand';
	import type { DisplayContext } from '$lib/display/types';
	import { scale, slide } from 'svelte/transition';

	// eslint-disable-next-line @typescript-eslint/no-unused-vars -- Logo accepts the shared display prop without rendering runtime state.
	let { context: _context }: { context: DisplayContext } = $props();
</script>

<!-- Legacy Logo.svelte (bg-bg-4 was never defined in the legacy theme). -->
<div class="bg-bg-4 h-full w-full bg-cover bg-no-repeat text-white">
	<img
		class="fixed top-[17cqh] left-[35cqw] w-[30cqw]"
		style="filter: drop-shadow(8px 10px 10px #335);"
		src="/4t.png"
		alt="Logo {brand.appName}"
		in:scale|global
	/>
	<h1
		class="fixed top-[53cqh] w-full text-center text-[14cqh] font-extrabold"
		style="text-shadow: 0.1cqh 0.5cqh 1cqh rgba(53, 53, 80, 1);"
		in:slide|global
	>
		{brand.eventName.toUpperCase()}
	</h1>
</div>
