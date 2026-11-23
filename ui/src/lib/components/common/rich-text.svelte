<script lang="ts">
	import 'katex/dist/katex.min.css';
	import { renderRichText } from '$lib/rich-text';

	let { value, class: className = '' }: { value: string; class?: string } = $props();
	let html = $derived(renderRichText(value));
</script>

<!-- Source text is HTML-escaped before the formatter adds its own fixed tags and KaTeX output. -->
<!-- eslint-disable-next-line svelte/no-at-html-tags -->
<div class={className}>{@html html}</div>
