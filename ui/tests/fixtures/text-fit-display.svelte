<script lang="ts">
	import DisplayFrame from '../../src/lib/components/display/display-frame.svelte';
	import KhoiDong from '../../src/lib/components/display/khoi_dong/khoi_dong.svelte';
	import VCNV from '../../src/lib/components/display/vcnv/VCNV.svelte';
	import TangToc from '../../src/lib/components/display/tang_toc/TangToc.svelte';
	import VeDich from '../../src/lib/components/display/ve_dich/VeDich.svelte';
	import { emptySiteSettings, type DisplayContext } from '../../src/lib/display/types';
	import { DEFAULT_SCORING } from '../../src/lib/operator/scoring';
	import { fixture } from './controller-state.svelte';

	let context = $derived<DisplayContext>({
		match: fixture.runtime,
		site: { ...emptySiteSettings, scoring: DEFAULT_SCORING },
		phase: fixture.runtime.controller.phase,
		screen: fixture.runtime.controller.screen,
		vcnv: null,
		preview: false
	});
</script>

<DisplayFrame fullScreen>
	{#if context.phase === 'khoi_dong'}
		<KhoiDong {context} />
	{:else if context.phase === 'vcnv'}
		<VCNV {context} />
	{:else if context.phase === 'tang_toc'}
		<TangToc {context} />
	{:else if context.phase === 've_dich'}
		<VeDich {context} />
	{/if}
</DisplayFrame>
