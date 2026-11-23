<script lang="ts">
	import type { DisplayContext } from '$lib/display/types';
	import { ruleText } from '$lib/display/rule-text';
	import { typewriter } from '$lib/display/transitions';
	import { fly, scale, slide } from 'svelte/transition';

	let { context }: { context: DisplayContext } = $props();
	let rule = $derived(ruleText(context.phase, context.site?.scoring));
</script>

<!-- Legacy ScreenRule.svelte. -->
<div
	class="fixed h-full w-full bg-bg-rule bg-contain bg-center bg-no-repeat"
	in:slide|global={{ duration: 2000 }}
	out:scale|global={{ duration: 1000 }}
>
	<h1
		class="fixed top-0 left-0 z-50 w-full text-center font-header-text text-[8cqh] text-white"
		in:fly|global={{ delay: 1000, y: -100 }}
		out:scale|global={{ duration: 1000 }}
	>
		{rule.name}
	</h1>
	<div
		class="fixed top-[17cqh] left-1/2 z-50 w-[73cqw] -translate-x-1/2 text-left font-header font-medium text-white"
		style={`font-size: ${rule.size}cqh;`}
	>
		{#key rule.content}
			<p
				class="whitespace-pre-wrap"
				in:typewriter|global={{ delay: 2100, speed: 2.5 }}
				out:scale|global={{ duration: 1000 }}
			>{rule.content}</p>
		{/key}
	</div>
</div>
