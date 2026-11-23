import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { render } from 'svelte/server';
import { createTextFitTestServer } from './text-fit-test-server.mjs';

const server = await createTextFitTestServer();
after(() => server.close());
const { fixture } = await server.ssrLoadModule('/tests/fixtures/controller-state.svelte.ts');
const { emptySiteSettings } = await server.ssrLoadModule('/src/lib/display/types.ts');
const { DEFAULT_SCORING } = await server.ssrLoadModule('/src/lib/operator/scoring.ts');
const VeDich = (await server.ssrLoadModule('/src/lib/components/display/ve_dich/VeDich.svelte'))
	.default;
const Form = (await server.ssrLoadModule('/src/lib/components/operator/scoring-rules-form.svelte'))
	.default;

function renderVeDich(screen) {
	fixture.reset('ve_dich', screen);
	fixture.setQuestion('vd.0.30.0');
	const runtime = fixture.runtime;
	runtime.controller.is_showing_question = true;
	runtime.controller.selected_contestant = 1;
	runtime.controller.selected_question_id = 'vd.0.30.0';
	runtime.controller.ve_dich_contestants_state[0].selected_question_set = ['30_1', '20_2', '20_3'];
	const scoring = structuredClone(DEFAULT_SCORING);
	scoring.ve_dich.tiers = [20, 40];
	const site = { ...emptySiteSettings, scoring };
	return render(VeDich, {
		props: { context: { match: runtime, site, phase: 've_dich', screen, vcnv: null } }
	}).body;
}

test('question scene shows the configured tier value', () => {
	assert.ok(renderVeDich('ques_ts1').includes('Câu 40 điểm'));
});

test('package scene score boxes show configured tier values', () => {
	const boxes = [...renderVeDich('pre_ques_ts1').matchAll(/text-\[16cqh\] font-black"[^>]*>\s*(\d+)\s*</g)].map(
		(m) => m[1]
	);
	assert.deepEqual(boxes, ['20', '40']);
});

test('scoring form renders all sections', () => {
	const body = render(Form, { props: { value: DEFAULT_SCORING, onsave: () => {} } }).body;
	for (const heading of ['Khởi động', 'Vượt chướng ngại vật', 'Tăng tốc', 'Về đích', 'Bảng điều khiển']) {
		assert.ok(body.includes(heading), heading);
	}
});
