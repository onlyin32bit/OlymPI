<script lang="ts">
	import { brand } from '$lib/brand';
	import Particles, { particlesInit } from '@tsparticles/svelte';
	import { loadSlim } from '@tsparticles/slim';
	import type { DisplayContext } from '$lib/display/types';
	import Text3D from './text-3d.svelte';
	import { fade, fly } from 'svelte/transition';

	let { context }: { context: DisplayContext } = $props();

	const names: Record<string, string> = {
		khoi_dong: 'KHỞI ĐỘNG',
		vcnv: 'VƯỢT CHƯỚNG NGẠI VẬT',
		tang_toc: 'TĂNG TỐC',
		ve_dich: 'VỀ ĐÍCH'
	};
	let name = $derived(names[context.phase] ?? names.khoi_dong);
	let isVcnv = $derived(context.phase === 'vcnv');

	const particlesConfig = {
		particles: {
			number: { value: 120 },
			color: { value: '#ffffff' },
			shape: { type: 'circle', stroke: { width: 0, color: '#000000' }, image: { width: 100, height: 100 } },
			opacity: { value: { min: 0.2, max: 1 }, anim: { enable: true, speed: 2, sync: false } },
			size: { value: 4, random: true },
			move: {
				enable: true,
				speed: 3,
				direction: 'none' as const,
				random: true,
				straight: false,
				out_mode: 'out',
				bounce: true
			}
		}
	};

	void particlesInit(async (engine) => {
		await loadSlim(engine);
	});
</script>

<!-- Legacy ScreenStart.svelte. -->
<div class="fixed h-full w-full bg-bg-2 bg-cover bg-no-repeat" out:fade|global>
	<!-- Light vignette: darkens only the edges, under the particles and the title. -->
	<div class="pointer-events-none fixed inset-0" style="background: radial-gradient(ellipse at 50% 45%, transparent 55%, rgb(0 0 20 / 0.32) 100%);"></div>
	<Particles id="tsparticles" class="h-full w-full" style="" options={particlesConfig} />
	<img
		class="fixed top-[18cqh] left-[36cqw] w-[28cqw]"
		style="filter: drop-shadow(0.6cqh 1cqh 2.4cqh rgb(20 30 90 / 0.3));"
		src="/4t.png"
		alt="Logo {brand.appName}"
		in:fly|global={{ duration: 3000, y: -600, opacity: 0.7 }}
	/>

	<Text3D text={name} vcnv={isVcnv} />
</div>
