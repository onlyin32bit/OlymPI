<script lang="ts">
	import { animate } from 'animejs';
	import { onMount, type Snippet } from 'svelte';

	type Preset = 'fade' | 'rise' | 'pop' | 'left' | 'right';
	type Props = {
		as?: string;
		preset?: Preset;
		delay?: number;
		duration?: number;
		class?: string;
		style?: string;
		children: Snippet;
	};

	let { as = 'div', preset = 'rise', delay = 0, duration = 620, class: className = '', style = '', children }: Props = $props();
	let element = $state<HTMLElement>();

	const entrance: Record<Preset, Record<string, unknown>> = {
		fade: { opacity: { from: 0 } },
		rise: { opacity: { from: 0 }, y: { from: 44 } },
		pop: { opacity: { from: 0 }, y: { from: 28 }, scale: { from: 0.88 } },
		left: { opacity: { from: 0 }, x: { from: -90 } },
		right: { opacity: { from: 0 }, x: { from: 90 } }
	};

	onMount(() => {
		if (!element) return;
		const animation = animate(element, {
			...entrance[preset],
			duration,
			delay,
			ease: 'outExpo'
		});
		return () => animation.cancel();
	});
</script>

<svelte:element this={as} bind:this={element} class={`display-motion ${className}`} {style}>
	{@render children()}
</svelte:element>
