<script lang="ts">
	import { brand } from '$lib/brand';
    import blueLogo from '$lib/assets/image/4t-blue.webp';
    import whiteLogo from '$lib/assets/image/4t-white.webp';
	import type { HTMLImgAttributes } from 'svelte/elements';

    type Props = {
        color?: 'white' | 'blue';
    } & HTMLImgAttributes;

    let { color = 'white', ...rest }: Props = $props()

    const logoMap: Record<string, string> = {
        blue: blueLogo,
        white: whiteLogo
    }
</script>

<img src={logoMap[color]} alt="Logo {brand.appName}" {...rest} />