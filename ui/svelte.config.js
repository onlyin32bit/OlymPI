import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	// Consult https://svelte.dev/docs/kit/integrations
	// for more information about preprocessors
	preprocess: vitePreprocess(),
	kit: {
		env: {
			publicPrefix: 'APP4T_'
		},
		router: {
			type: 'hash'
		},
		adapter: adapter({
			pages: 'dist',
			assets: 'dist',
			strict: true,
			precompress: false,
		})
	}
};

export default config;
