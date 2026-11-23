import assert from 'node:assert/strict';
import { mkdtemp, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import ts from 'typescript';

class Param {
	value = 1;
	setTargetAtTime(value) {
		this.value = value;
	}
	setValueAtTime(value) {
		this.value = value;
	}
	linearRampToValueAtTime(value) {
		this.value = value;
	}
	cancelScheduledValues() {}
}

class Node {
	connections = [];
	connect(node) {
		this.connections.push(node);
		return node;
	}
	disconnect() {}
}

class Gain extends Node {
	gain = new Param();
}

class Source extends Node {
	started = null;
	stopped = null;
	addEventListener() {}
	start(at, offset) {
		this.started = { at, offset };
	}
	stop(at) {
		this.stopped = at;
	}
}

class FakeContext {
	state = 'running';
	currentTime = 0;
	baseLatency = 0;
	outputLatency = 0;
	sampleRate = 48000;
	gains = [];
	sources = [];
	addEventListener() {}
	createGain() {
		const gain = new Gain();
		this.gains.push(gain);
		return gain;
	}
	createAnalyser() {
		return Object.assign(new Node(), { fftSize: 0, getFloatTimeDomainData() {} });
	}
	createMediaStreamDestination() {
		return Object.assign(new Node(), { stream: { getTracks: () => [] } });
	}
	createBufferSource() {
		const source = new Source();
		this.sources.push(source);
		return source;
	}
	async decodeAudioData() {
		return { duration: 30, numberOfChannels: 1, sampleRate: 48000 };
	}
	async resume() {}
	async suspend() {}
	async close() {
		this.state = 'closed';
	}
}

const contexts = [];
globalThis.AudioContext = class extends FakeContext {
	constructor() {
		super();
		contexts.push(this);
	}
};
globalThis.Audio = class {
	async play() {}
	pause() {}
};
globalThis.fetch = async () => ({
	ok: true,
	status: 200,
	headers: { get: () => null },
	arrayBuffer: async () => new ArrayBuffer(1)
});

async function loadEngine() {
	const source = (await readFile(new URL('./engine.ts', import.meta.url), 'utf8')).replaceAll(
		'import.meta.env.DEV',
		'false'
	);
	const { outputText } = ts.transpileModule(source, {
		compilerOptions: { target: ts.ScriptTarget.ESNext, module: ts.ModuleKind.ESNext }
	});
	const dir = await mkdtemp(join(tmpdir(), 'engine-test-'));
	const file = join(dir, 'engine.mjs');
	await writeFile(file, outputText);
	return await import(file);
}

const { AudioEngine, mixerFor, singleSlot } = await loadEngine();
const settings = {
	enabled: true,
	master_volume: 1,
	music_volume: 1,
	effects_volume: 1,
	transition_fade_ms: 120,
	bell_sound_id: 'builtin.kd.bell'
};
let next = 0;
function playback(sound, bus, extra = {}) {
	return {
		playback_id: `p${++next}`,
		sound_id: sound,
		url: `/sounds/${sound}`,
		bus,
		volume: 1,
		loop: bus === 'music',
		fade_in_ms: 0,
		fade_out_ms: 0,
		start_at: Date.now(),
		...extra
	};
}

let context;
async function engine() {
	const value = new AudioEngine();
	await value.enable(settings, []);
	context = contexts.at(-1);
	return value;
}

test('bus helpers: timer is single-slot and mixed through the music gain', () => {
	assert.equal(singleSlot('timer'), true);
	assert.equal(singleSlot('music'), true);
	assert.equal(singleSlot('effects'), false);
	assert.equal(mixerFor('timer'), 'music');
	assert.equal(mixerFor('effects'), 'effects');
});

test('a new timer playback stops the previous one; effects overlap', async () => {
	const audio = await engine();
	const [, music, effects] = context.gains;
	await audio.play(playback('a', 'timer'), Date.now());
	await audio.play(playback('e1', 'effects'), Date.now());
	await audio.play(playback('e2', 'effects'), Date.now());
	await audio.play(playback('b', 'timer'), Date.now());
	const [timerA, effect1, effect2, timerB] = context.sources;
	assert.notEqual(timerA.stopped, null, 'previous timer sound is cut');
	assert.equal(effect1.stopped, null);
	assert.equal(effect2.stopped, null);
	assert.equal(timerB.stopped, null);
	// source -> per-playback gain -> bus mixer
	assert.equal(timerB.connections[0].connections[0], music);
	assert.equal(effect1.connections[0].connections[0], effects);
	await audio.dispose();
});

test('stopping the timer bus leaves music and effects playing', async () => {
	const audio = await engine();
	await audio.play(playback('loop', 'music'), Date.now());
	await audio.play(playback('tick', 'timer'), Date.now());
	await audio.play(playback('fx', 'effects'), Date.now());
	audio.stop({ bus: 'timer', fade_ms: 0 });
	const [loop, tick, fx] = context.sources;
	assert.notEqual(tick.stopped, null);
	assert.equal(loop.stopped, null);
	assert.equal(fx.stopped, null);
	await audio.dispose();
});

test('a resumed timer sound continues at its offset', async () => {
	const audio = await engine();
	await audio.play(playback('tt30', 'timer', { start_at: Date.now() - 9000 }), Date.now() - 9000);
	const [source] = context.sources;
	assert.ok(Math.abs(source.started.offset - 9) < 0.5, `offset ${source.started.offset}`);
	await audio.dispose();
});
