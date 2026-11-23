<script lang="ts">
	import { brand } from '$lib/brand';
	import type { DisplayContext } from '$lib/display/types';

	// eslint-disable-next-line @typescript-eslint/no-unused-vars -- Idle accepts the shared display prop without rendering runtime state.
	let { context: _context }: { context: DisplayContext } = $props();
</script>

<!-- Legacy Screen.svelte fallback. -->
<div class="flex h-full flex-col items-center justify-center gap-8">
	<img src="/4t.png" alt="{brand.appName} logo" class="h-[150px]" />
</div>
