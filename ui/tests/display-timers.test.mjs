import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { render } from 'svelte/server';
import { createTextFitTestServer } from './text-fit-test-server.mjs';

const server = await createTextFitTestServer();
after(() => server.close());
const { fixture } = await server.ssrLoadModule('/tests/fixtures/controller-state.svelte.ts');
const { emptySiteSettings } = await server.ssrLoadModule('/src/lib/display/types.ts');
const rounds = [
	[
		'khoi_dong',
		'ques_chung',
		'kd_chung.0',
		3000,
		'/src/lib/components/display/khoi_dong/khoi_dong.svelte'
	],
	['vcnv', 'ques', 'vcnv.rows.0', 15000, '/src/lib/components/display/vcnv/VCNV.svelte'],
	['tang_toc', 'ques', 'tt.0', 13000, '/src/lib/components/display/tang_toc/TangToc.svelte'],
	['ve_dich', 'ques_ts1', 'vd.0.20.0', 15000, '/src/lib/components/display/ve_dich/VeDich.svelte']
];
const components = new Map();
for (const [phase, , , , path] of rounds) {
	components.set(phase, (await server.ssrLoadModule(path)).default);
}

function renderScene(phase, screen, id, duration, fraction) {
	fixture.reset(phase, screen);
	fixture.setQuestion(id);
	const runtime = fixture.runtime;
	runtime.controller.is_showing_question = true;
	runtime.controller.selected_contestant = phase === 've_dich' ? 1 : 0;
	if (phase === 've_dich') {
		runtime.controller.ve_dich_contestants_state[0].selected_question_set = [
			'20_1',
			'30_1',
			'20_2'
		];
	}
	runtime.timer = {
		duration_ms: duration,
		remaining_ms: duration * (1 - fraction),
		elapsed_ms: duration * fraction,
		started_at: 0,
		is_running: false,
		is_paused: fraction < 1 && duration > 0
	};
	return render(components.get(phase), {
		props: { context: { match: runtime, site: emptySiteSettings, phase, screen, vcnv: null } }
	}).body;
}

for (const [phase, screen, id, duration] of rounds) {
	for (const fraction of [0, 0.5, 1]) {
		test(`${phase} renders ${fraction * 100}% elapsed progress, not the remaining fraction`, () => {
			const body = renderScene(phase, screen, id, duration, fraction);
			if (phase === 'vcnv') {
				// Legacy ScreenQuestionVCNV: the fill and dot appear only once time has elapsed.
				if (fraction === 0) assert.doesNotMatch(body, /class="progress-(?:fill|dot)/);
				else {
					assert.ok(body.includes(`width: ${fraction * 100}%`));
					assert.match(body, /class="progress-dot/);
				}
			} else if (phase === 'tang_toc') {
				assert.ok(body.includes(`#f00 ${fraction * 100}%`));
			} else if (phase === 'khoi_dong') {
				// Legacy ScreenQuestionKD ring and (3 - t).toFixed(0) digits.
				assert.ok(body.includes(`conic-gradient(red, ${fraction}turn,`));
				assert.match(body, new RegExp(`text-\\[13cqh\\] font-bold">${(3 - 3 * fraction).toFixed(0)}</div>`));
			} else if (phase === 've_dich') {
				// Legacy ScreenQuestionVD ring and (preset - t).toFixed(0) digits (preset 15 for 20 pt).
				assert.ok(body.includes(`conic-gradient(red, ${fraction}turn,`));
				assert.match(body, new RegExp(`text-\\[14cqh\\] font-bold">${(15 - 15 * fraction).toFixed(0)}</div>`));
			} else {
				assert.ok(body.includes(`red ${fraction}turn`));
				assert.match(
					body,
					new RegExp(`class="timer-inner[^"]*">${Math.floor((duration * fraction) / 1000)}</div>`)
				);
			}
		});
	}
}

for (const [id, label] of [
	['vcnv.rows.0', '1'],
	['vcnv.rows.1', '2'],
	['vcnv.rows.2', '3'],
	['vcnv.rows.3', '4'],
	['vcnv.center', 'TT']
]) {
	test(`VCNV ${id} preserves its original ${label} question tab`, () => {
		const body = renderScene('vcnv', 'ques', id, 15000, 0.5);
		assert.match(body, new RegExp(`class="question-number[^"]*"[^>]*>\\s*${label}\\s*</div>`));
	});
}

test('VCNV board gauge also fills upward with elapsed time', () => {
	fixture.reset('vcnv', 'main_vcnv');
	const runtime = fixture.runtime;
	const timer = { duration_ms: 15000, remaining_ms: 7500, elapsed_ms: 7500, started_at: 0, is_running: false, is_paused: true };
	const props = (kind) => ({ props: { context: { match: { ...runtime, timer: { ...timer, kind } }, site: emptySiteSettings, phase: 'vcnv', screen: 'main_vcnv', vcnv: null } } });
	assert.ok(render(components.get('vcnv'), props('obstacle')).body.includes('#f00 50%'));
	// Legacy board bar follows only the obstacle timer.
	assert.ok(render(components.get('vcnv'), props('question')).body.includes('#f00 0%'));
});

test('reset VCNV questions retain the rail and cap but remove the moving fill and dot', () => {
	const body = renderScene('vcnv', 'ques', 'vcnv.rows.0', 0, 0);
	assert.match(body, /class="progress-track/);
	assert.match(body, /class="progress-cap/);
	assert.doesNotMatch(body, /class="progress-(?:fill|dot)/);
});

test('Tăng tốc answer scene keeps its fully filled rail', () => {
	const body = renderScene('tang_toc', 'solve', 'tt.0', 13000, 0.5);
	// Legacy ScreenSolvedTT: solid red rail regardless of the timer.
	assert.ok(body.includes('background: #f00"'));
});

test('Về đích shows the legacy preset once shown, before the timer starts', () => {
	const body = renderScene('ve_dich', 'ques_ts1', 'vd.0.20.0', 0, 0);
	assert.match(body, /text-\[14cqh\] font-bold">15<\/div>/);
});
