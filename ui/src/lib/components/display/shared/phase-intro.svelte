<script lang="ts">
	import type { DisplayContext } from '$lib/display/types';
	import Text3D from './text-3d.svelte';
	import { fade } from 'svelte/transition';

	let { context, still = false }: { context: DisplayContext; still?: boolean } = $props();

	const names: Record<string, string> = {
		khoi_dong: 'KHỞI ĐỘNG',
		vcnv: 'VƯỢT\nCHƯỚNG NGẠI VẬT',
		tang_toc: 'TĂNG TỐC',
		ve_dich: 'VỀ ĐÍCH'
	};
	let name = $derived(names[context.phase] ?? names.khoi_dong);
	let isVcnv = $derived(context.phase === 'vcnv');
</script>

<!-- Intro card: same backdrop as the start slide, title centred, no logo and no particles. -->
<div class="fixed h-full w-full bg-bg-2 bg-cover bg-no-repeat" out:fade|global>
	<div class="pointer-events-none fixed inset-0" style="background: radial-gradient(ellipse at 50% 45%, transparent 55%, rgb(0 0 20 / 0.32) 100%);"></div>
	<Text3D text={name} vcnv={isVcnv} centered {still} shadow="0.8cqh 1.2cqh 0.7cqh rgb(0 0 0 / 0.6)" />
</div>
