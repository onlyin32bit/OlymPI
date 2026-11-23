import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';

export async function createTextFitTestServer(listen = false) {
	const server = await createServer({
		configFile: false,
		publicDir: fileURLToPath(new URL('../static', import.meta.url)),
		plugins: [
			tailwindcss(),
			svelte({ configFile: false }),
			{
				name: 'text-fit-test-route',
				configureServer(vite) {
					vite.middlewares.use(async (request, response, next) => {
						if (request.url?.split('?')[0] !== '/text-fit-test') return next();
						const html = await vite.transformIndexHtml(
							'/text-fit-test',
							`<!doctype html>
					<html lang="vi"><head><meta name="viewport" content="width=device-width, initial-scale=1"><title>Display text fitting QA</title></head>
					<body><main id="display-root"></main><script type="module" src="/tests/fixtures/text-fit-entry.ts"></script></body></html>`
						);
						response.setHeader('Content-Type', 'text/html');
						response.end(html);
					});
				}
			}
		],
		resolve: {
			alias: [
				{
					find: /^\$lib\/socket$/,
					replacement: fileURLToPath(
						new URL('./fixtures/controller-state.svelte.ts', import.meta.url)
					)
				},
				{ find: '$lib', replacement: fileURLToPath(new URL('../src/lib', import.meta.url)) }
			]
		},
		server: listen
			? { host: '127.0.0.1', port: 5193, strictPort: true }
			: { middlewareMode: true, hmr: false, watch: null }
	});
	if (listen) await server.listen();
	return server;
}

if (process.argv.includes('--serve')) {
	await createTextFitTestServer(true);
	console.log('Display text fixture ready: http://127.0.0.1:5193/text-fit-test');
}
