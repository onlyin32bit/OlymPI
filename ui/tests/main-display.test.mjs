import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { render } from 'svelte/server';

const server = await createServer({
	configFile: false,
	plugins: [svelte({ configFile: false })],
	resolve: { alias: { $lib: fileURLToPath(new URL('../src/lib', import.meta.url)) } },
	server: { middlewareMode: true, hmr: false, watch: null }
});
after(() => server.close());
const { default: Main } = await server.ssrLoadModule('/src/lib/components/display/main/main.svelte');

function context(match = null) {
	return {
		match,
		site: { event_name: '' },
		phase: 'main',
		screen: 'main',
		vcnv: null
	};
}

test('main restores legacy logos, stage heading and name/class strips in slot order', () => {
	const match = {
		match_name: 'Test match', stage: 'Bán kết 2', season: 9,
		contestants: [
			{ slot: 2, name: 'Khả Vinh', class: '11A2' },
			{ slot: 1, name: 'Minh Tuấn', class: '12A1' },
			{ slot: 3, name: 'Hoàng Đức', class: '10A3' },
			{ slot: 4, name: 'Gia Hân', class: '11A4' }
		]
	};
	const { body } = render(Main, { props: { context: context(match) } });
	assert.match(body, /src="\/4t.png"/);
	assert.match(body, /school-logo/);
	assert.match(body, /bg-bg-2/);
	assert.match(body, /Bán kết 2/);
	assert.equal((body.match(/class="contestant-name /g) ?? []).length, 4);
	for (const contestant of match.contestants) {
		assert.ok(body.includes(contestant.name));
		assert.ok(body.includes(contestant.class));
	}
	assert.ok(body.indexOf('Minh Tuấn') < body.indexOf('Khả Vinh'));
	assert.deepEqual(match.contestants.map((value) => value.slot), [2, 1, 3, 4]);
	assert.doesNotMatch(body, /Mùa 9|SẴN SÀNG|THÍ SINH|Chưa chọn trận đấu/);
});

test('empty main retains only legacy branding rather than invented placeholder text', () => {
	const { body } = render(Main, { props: { context: context() } });
	assert.match(body, /src="\/4t.png"/);
	assert.match(body, /school-logo/);
	assert.doesNotMatch(body, /<h1|contestant-name|SẴN SÀNG|Chưa chọn trận đấu/);
});

test('main heading is the stage only and hidden without one', () => {
	const { body } = render(Main, {
		props: { context: context({ match_name: 'Giao hữu', stage: '', contestants: [] }) }
	});
	assert.doesNotMatch(body, /Giao hữu|<h1/);
});
