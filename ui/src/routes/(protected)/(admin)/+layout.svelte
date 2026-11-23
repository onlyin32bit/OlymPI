<script lang="ts">
    import { Button } from '$lib/components/ui/button';
	import ArrowLeft from '@lucide/svelte/icons/arrow-left';


	let { children } = $props();
</script>


<main class="mx-auto flex min-h-svh w-full max-w-7xl flex-col gap-6 p-4 md:p-8">
	<nav class="mb-2 flex items-center gap-2">
		<Button variant="outline" href="#/">
			<ArrowLeft />
		</Button>
		<a class="rounded-md px-3 py-2 text-sm font-medium hover:bg-muted" href="/master">Giám sát trận đấu</a>
	</nav>
	{@render children()}
</main>
