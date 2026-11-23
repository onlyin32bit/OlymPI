import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import { SourceTextModule, SyntheticModule } from 'node:vm';
import ts from 'typescript';

async function harness() {
	const played = [];
	const controller = {
		current: { phase: 'idle', screen: 'intro' },
		async changePhase(phase) {
			this.current = { phase, screen: manifest.defaultScreen(phase).id };
		},
		async changeScreen(screen) { this.current.screen = screen; },
		async selectContestant(slot) { this.current.selected_contestant = slot; }
	};
	const socket = { controller, sound: { async play(id) { played.push(id); } } };
	const component = new SyntheticModule(['default'], function () {
		this.setExport('default', () => {});
	});
	const socketModule = new SyntheticModule(['socket'], function () {
		this.setExport('socket', socket);
	});
	const modules = new Map();
	async function load(relativePath) {
		if (modules.has(relativePath)) return modules.get(relativePath);
		const source = await readFile(new URL(relativePath, import.meta.url), 'utf8');
		const { outputText } = ts.transpileModule(source, {
			compilerOptions: { target: ts.ScriptTarget.ESNext, module: ts.ModuleKind.ESNext }
		});
		const module = new SourceTextModule(outputText);
		modules.set(relativePath, module);
		await module.link(async (specifier) => {
			if (specifier.endsWith('.svelte')) return component;
			if (specifier === '$lib/socket') return socketModule;
			if (specifier === '$lib/brand') return load('../src/lib/brand.ts');
			if (specifier === '$lib/display/phase-manifest') return load('../src/lib/display/phase-manifest.ts');
			throw new Error(`Unexpected dependency: ${specifier}`);
		});
		return module;
	}
	const manifestModule = await load('../src/lib/display/phase-manifest.ts');
	await manifestModule.evaluate();
	const manifest = manifestModule.namespace;
	const actionsModule = await load('../src/lib/control/game-controller.ts');
	await actionsModule.evaluate();
	return { played, controller, actions: actionsModule.namespace.gameController };
}

// Screen sounds are cued by the server (sound cue director); navigation never plays client-side.
for (const phase of ['khoi_dong', 'vcnv', 'tang_toc', 've_dich']) {
	test(`${phase}: Bắt đầu and Intro navigation plays nothing client-side`, async () => {
		const { actions, played, controller } = await harness();
		await actions.changePhase(phase);
		await actions.showScreen(phase, 'intro');
		await actions.showScreen(phase, 'start');
		assert.equal(controller.current.screen, 'start');
		assert.deepEqual(played, []);
	});
}

test('Màn hình chính, Điểm, and Tĩnh do not trigger any sound', async () => {
	const { actions, played } = await harness();
	for (const phase of ['main', 'score', 'idle', 'main']) await actions.changePhase(phase);
	await actions.changePhase('main');
	await actions.showScreen('main', 'main');
	assert.deepEqual(played, []);
});
