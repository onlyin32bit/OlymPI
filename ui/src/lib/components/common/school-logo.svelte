<script lang="ts">
	import { brand } from '$lib/brand';
    import schoolLogo from '$lib/assets/image/school.webp';
	import type { HTMLImgAttributes } from 'svelte/elements';

    type Props = {} & HTMLImgAttributes;

    let {  ...rest }: Props = $props()
</script>

<img src={schoolLogo} alt="{brand.appName} Logo" {...rest} />