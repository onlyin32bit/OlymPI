import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';

export const consolePath = '/src/routes/(protected)/(admin)/control-panel/operator-console.svelte';
export const fixturePath = '/tests/fixtures/controller-state.svelte.ts';
export const contestantPath = '/src/routes/(protected)/(contestant)/contestant/+page.svelte';

export async function createControllerTestServer(listen = false) {
	const fixture = fileURLToPath(new URL('./fixtures/controller-state.svelte.ts', import.meta.url));
	const server = await createServer({
		configFile: false,
		publicDir: fileURLToPath(new URL('../static', import.meta.url)),
		plugins: [
			tailwindcss(),
			svelte({ configFile: false }),
			{
				name: 'controller-test-route',
				configureServer(vite) {
					vite.middlewares.use(async (request, response, next) => {
						const path = request.url?.split('?')[0];
						if (path !== '/controller-test' && path !== '/contestant-test') return next();
						const contestant = path === '/contestant-test';
						const html = await vite.transformIndexHtml(
							path,
							`<!doctype html>
							<html lang="vi"><head><meta name="viewport" content="width=device-width, initial-scale=1"><title>${contestant ? 'Contestant' : 'Controller'} QA</title></head>
							<body><main id="${contestant ? 'contestant' : 'controller'}-root" style="height:100dvh;width:100%;padding:${contestant ? 0 : 6}px;background:#dbeafe;box-sizing:border-box"></main>
							<script type="module" src="/tests/fixtures/${contestant ? 'contestant' : 'controller'}-entry.ts"></script></body></html>`
						);
						response.setHeader('Content-Type', 'text/html');
						response.end(html);
					});
				}
			}
		],
		resolve: {
			alias: [
				{ find: /^\$lib\/(?:socket|api)$/, replacement: fixture },
				{ find: '$lib', replacement: fileURLToPath(new URL('../src/lib', import.meta.url)) }
			]
		},
		ssr: { noExternal: ['bits-ui'] },
		server: listen
			? { host: '127.0.0.1', port: 5192, strictPort: true }
			: { middlewareMode: true, hmr: false, watch: null }
	});
	if (listen) await server.listen();
	return server;
}

if (process.argv.includes('--serve')) {
	await createControllerTestServer(true);
	console.log('Controller fixture ready: http://127.0.0.1:5192/controller-test');
}
