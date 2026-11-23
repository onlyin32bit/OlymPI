<script lang="ts">
	import { brand } from '$lib/brand';
	import { slide } from 'svelte/transition';
</script>

<!-- Legacy ScreenEnd.svelte (isContestantEnd). -->
<div class="fixed h-full w-full bg-bg-3 bg-cover bg-no-repeat">
	<img
		class="center-element fixed h-[50cqh]"
		src="/4t.png"
		alt="Logo {brand.appName}"
		in:slide|global={{ duration: 4000 }}
	/>
</div>
