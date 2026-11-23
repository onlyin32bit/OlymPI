import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';
import ts from 'typescript';

function loadModule(path, globals = {}, imports = {}) {
	const source = readFileSync(new URL(`../src/lib/${path}`, import.meta.url), 'utf8').replaceAll(
		'import.meta.env.DEV',
		'false'
	);
	const compiled = ts.transpileModule(source, {
		compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS }
	}).outputText;
	const exports = {};
	vm.runInNewContext(compiled, {
		exports,
		require: (name) => {
			if (name === '../uuid' || name === '../../uuid') return loadModule('uuid.ts', { crypto: globals.crypto ?? globalThis.crypto });
			assert.ok(name in imports, `Unexpected runtime import: ${name}`);
			return imports[name];
		},
		$state: (value) => value,
		console,
		...globals
	});
	return exports;
}

function deferred() {
	let resolve;
	let reject;
	const promise = new Promise((yes, no) => {
		resolve = yes;
		reject = no;
	});
	return { promise, resolve, reject };
}

const settings = {
	enabled: true,
	master_volume: 1,
	music_volume: 1,
	effects_volume: 1,
	transition_fade_ms: 0,
	bell_sound_id: ''
};

function playback(id, startAt = 10_000) {
	return {
		playback_id: id,
		sound_id: id,
		url: `/${id}.wav`,
		bus: 'music',
		volume: 1,
		loop: true,
		fade_in_ms: 0,
		fade_out_ms: 0,
		start_at: startAt
	};
}

function audioHarness(fetcher = async () => new Response(new ArrayBuffer(0))) {
	const sources = [];
	const contexts = [];
	const outputs = [];
	const timers = new Map();
	let timerId = 0;
	function node() {
		return {
			gain: {
				value: 1,
				setTargetAtTime() {},
				setValueAtTime() {},
				linearRampToValueAtTime() {},
				cancelScheduledValues() {}
			},
			connect() {},
			disconnect() {}
		};
	}
	class Context {
		state = 'suspended';
		currentTime = 0;
		constructor() {
			contexts.push(this);
		}
		addEventListener() {}
		createGain() {
			return node();
		}
		createAnalyser() {
			return node();
		}
		createMediaStreamDestination() {
			return { stream: { getTracks: () => [] } };
		}
		createBufferSource() {
			const source = {
				...node(),
				stopped: false,
				stopCalls: [],
				addEventListener() {},
				start(at, offset) {
					this.at = at;
					this.offset = offset;
				},
				stop(at) {
					this.stopCalls.push(at);
					this.stopped = true;
				}
			};
			sources.push(source);
			return source;
		}
		async resume() {
			this.state = 'running';
		}
		async suspend() {
			this.state = 'suspended';
		}
		async close() {
			this.state = 'closed';
		}
		async decodeAudioData() {
			return { duration: 4, numberOfChannels: 2, sampleRate: 48_000 };
		}
	}
	class Output {
		paused = true;
		constructor() {
			outputs.push(this);
		}
		async play() {
			this.paused = false;
		}
		pause() {
			this.paused = true;
		}
	}
	const { AudioEngine } = loadModule('audio/engine.ts', {
		AudioContext: Context,
		Audio: Output,
		Date: { now: () => 10_000 },
		fetch: fetcher,
		setTimeout: (handler) => {
			timers.set(++timerId, handler);
			return timerId;
		},
		clearTimeout: (id) => timers.delete(id)
	});
	return { engine: new AudioEngine(), sources, contexts, outputs, timers };
}

test('authoritative reconnect snapshots stop or replace music without duplicate loops', async () => {
	const { engine, sources } = audioHarness();
	await engine.enable(settings, []);
	const first = playback('first', 4_000);
	await engine.play(first, first.start_at);
	assert.equal(sources[0].offset, 2, 'loop resumes at the server playhead');
	await engine.reconcile({ music: first }, (timestamp) => timestamp);
	assert.equal(sources.length, 1, 'same playback must not create another loop');
	const next = playback('next');
	await engine.reconcile({ music: next }, (timestamp) => timestamp);
	assert.equal(sources[0].stopped, true);
	assert.equal(sources.length, 2);
	await engine.reconcile({ music: null }, (timestamp) => timestamp);
	assert.equal(sources[1].stopped, true, 'a missed stop is repaired by the snapshot');
	await engine.dispose();
});

test('stop and disposal invalidate pending decode; disposed output cannot resume', async () => {
	const response = deferred();
	const { engine, sources, contexts, outputs, timers } = audioHarness(() => response.promise);
	await engine.enable(settings, []);
	const pending = engine.play(playback('pending'), 10_000);
	engine.stop({ all: true, fade_ms: 0 });
	await engine.dispose();
	response.resolve(new Response(new ArrayBuffer(0)));
	await pending;
	await engine.enable(settings, []);
	assert.equal(sources.length, 0);
	assert.equal(contexts.length, 1);
	assert.equal(contexts[0].state, 'closed');
	assert.equal(outputs[0].paused, true);
	assert.equal(outputs[0].srcObject, null);
	assert.equal(timers.size, 0);
});

test('disposal while enable is awaiting resume cannot start the detached output', async () => {
	const { engine, contexts, outputs } = audioHarness();
	const enabling = engine.enable(settings, []);
	await engine.dispose();
	await enabling;
	assert.equal(contexts[0].state, 'closed');
	assert.equal(outputs[0].paused, true);
	assert.equal(outputs[0].srcObject, null);
});

test('a replaced pending track cannot finish decoding after its replacement and take over', async () => {
	const firstResponse = deferred();
	const { engine, sources } = audioHarness((url) =>
		url === '/first.wav'
			? firstResponse.promise
			: Promise.resolve(new Response(new ArrayBuffer(0)))
	);
	await engine.enable(settings, []);
	const first = engine.play(playback('first'), 10_000);
	await engine.play(playback('next'), 10_000);
	firstResponse.resolve(new Response(new ArrayBuffer(0)));
	await first;
	assert.equal(sources.length, 1);
	assert.equal(sources[0].stopped, false);
	await engine.dispose();
});

test('replacement music preserves an authoritative fade deadline; disposal can still stop it', async () => {
	for (const replacementFade of [0, 10_000]) {
		const { engine, sources } = audioHarness();
		await engine.enable(settings, []);
		await engine.play(playback('first'), 10_000);
		engine.stop({ playback_id: 'first', fade_ms: 120 });
		const deadline = sources[0].stopCalls[0];
		assert.ok(Math.abs(deadline - 0.14) < 0.000001);
		await engine.play({ ...playback('next'), fade_out_ms: replacementFade }, 10_000);
		assert.equal(sources[0].stopCalls.length, 1, 'replacement must not reschedule a fading source');
		assert.equal(sources[0].stopCalls[0], deadline);
		await engine.dispose();
		assert.equal(sources[0].stopCalls.at(-1), 0, 'dispose must still stop retained fading sources');
	}
});

test('SoundSocket publishes reconnect snapshots and disable wins over pending enable', async () => {
	const handlers = {};
	const requests = [];
	let onOpen;
	const transport = {
		serverClockOffset: 0,
		on: (name, handler) => (handlers[name] = handler),
		onOpen: (handler) => (onOpen = handler),
		request: (operation) => {
			const pending = deferred();
			requests.push({ operation, ...pending });
			return pending.promise;
		}
	};
	const { SoundSocket } = loadModule('socket/sound/index.svelte.ts');
	const sound = new SoundSocket(transport);
	const snapshots = [];
	sound.onStatus((status) => snapshots.push(status));
	const enabling = sound.enableOutput();
	requests[0].resolve({ output_count: 1, music: playback('first') });
	await enabling;
	onOpen();
	assert.equal(requests[1].operation, 'Sound.EnableOutput');
	requests[1].resolve({ output_count: 1, music: null });
	await new Promise(setImmediate);
	assert.equal(snapshots.at(-1).music, null);
	const pendingEnable = sound.enableOutput();
	const disabling = sound.disableOutput();
	requests[3].resolve({ output_count: 0, music: null });
	await disabling;
	requests[2].resolve({ output_count: 1, music: playback('stale') });
	await pendingEnable;
	assert.equal(sound.status.output_count, 0);
	onOpen();
	assert.equal(requests[4].operation, 'Sound.GetStatus', 'unmounted output is not re-registered');
	requests[4].resolve({ output_count: 0 });
	await new Promise(setImmediate);
});

test('session invalidation stops reconnect and stale connection greetings are ignored', () => {
	const sockets = [];
	const timers = new Map();
	let timerId = 0;
	class WebSocket {
		static OPEN = 1;
		static CONNECTING = 0;
		readyState = 1;
		handlers = {};
		constructor() {
			sockets.push(this);
		}
		addEventListener(name, handler) {
			this.handlers[name] = handler;
		}
		close() {
			this.readyState = 3;
			this.handlers.close({ code: 1000 });
		}
	}
	const { SocketTransport } = loadModule(
		'socket/transport.ts',
		{
			WebSocket,
			Date,
			setTimeout: (handler) => {
				timers.set(++timerId, handler);
				return timerId;
			},
			clearTimeout: (id) => timers.delete(id)
		},
		{ './type': { Operation: { Greeting: 0, Event: 2, Response: 4, Request: 3 } } }
	);
	const transport = new SocketTransport();
	let checks = 0;
	transport.onSessionCheck = () => {
		checks++;
		transport.disconnect();
	};
	transport.connect('ws://test');
	sockets[0].handlers.close({ code: 1006 });
	assert.equal(checks, 1);
	assert.equal(timers.size, 0, 'invalidated sessions must not schedule another reconnect');
	transport.connect('ws://test');
	const currentOffset = transport.serverClockOffset;
	sockets[0].handlers.message({
		data: JSON.stringify({
			o: 0,
			d: { protocol_version: 1, server_time: Date.now() + 99_000, authenticated: true }
		})
	});
	assert.equal(transport.serverClockOffset, currentOffset);
	transport.disconnect();
});

test('401 clears cached identity and an older successful refresh cannot restore its role', async () => {
	class ApiError extends Error {
		constructor(status) {
			super('unauthorized');
			this.status = status;
		}
	}
	const requests = [];
	const http = {
		request: () => {
			const pending = deferred();
			requests.push(pending);
			return pending.promise;
		}
	};
	const { AuthApi } = loadModule('api/auth/index.svelte.ts', {}, {
		'../client': { ApiError },
		'svelte/reactivity': { SvelteURLSearchParams: URLSearchParams }
	});
	let invalidations = 0;
	const auth = new AuthApi(http, () => {}, () => invalidations++);
	const initial = auth.refresh();
	requests[0].resolve({ user: { id: 1, role: 'admin' } });
	await initial;
	const stale = auth.refresh();
	const revoked = auth.refresh();
	requests[2].reject(new ApiError(401));
	await revoked;
	assert.equal(auth.user, null);
	assert.equal(invalidations, 1);
	requests[1].resolve({ user: { id: 1, role: 'admin' } });
	await stale;
	assert.equal(auth.user, null);
});
