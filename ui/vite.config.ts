import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		host: '127.0.0.1',
		strictPort: true,
		proxy: {
			'/api': 'http://127.0.0.1:8090',
			'/ws': {
				target: 'ws://127.0.0.1:8090',
				ws: true
			}
		}
	},
	ssr: {
		noExternal: ['bits-ui']
	}
});
