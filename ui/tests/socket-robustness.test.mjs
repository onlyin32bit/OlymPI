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
		console: { ...console, warn: () => undefined },
		...globals
	});
	return exports;
}

const typeModule = { './type': { Operation: { Greeting: 0, Event: 2, Response: 4, Request: 3 } } };

/** Deterministic clock + timers + WebSocket fake for the transport. */
function harness() {
	const clock = { now: 1_000_000 };
	const timers = new Map();
	let timerId = 0;
	const sockets = [];
	class WebSocket {
		static OPEN = 1;
		static CONNECTING = 0;
		readyState = 1;
		handlers = {};
		sent = [];
		closedWith = null;
		constructor(url) {
			this.url = url;
			sockets.push(this);
		}
		addEventListener(name, handler) {
			this.handlers[name] = handler;
		}
		send(data) {
			this.sent.push(JSON.parse(data));
		}
		close(code) {
			this.closedWith = code ?? 1005;
			this.readyState = 3;
			this.handlers.close?.({ code: this.closedWith });
		}
		receive(envelope) {
			this.handlers.message({ data: JSON.stringify(envelope) });
		}
		greet(serverTime = clock.now) {
			this.receive({
				o: 0,
				d: { protocol_version: 1, server_time: serverTime, authenticated: false }
			});
		}
	}
	const globals = {
		WebSocket,
		Date: { now: () => clock.now },
		Math,
		setTimeout: (handler, delay = 0) => {
			timers.set(++timerId, { handler, at: clock.now + delay });
			return timerId;
		},
		clearTimeout: (id) => timers.delete(id),
		queueMicrotask
	};
	const advance = (ms) => {
		const target = clock.now + ms;
		for (;;) {
			const due = [...timers.entries()]
				.filter(([, timer]) => timer.at <= target)
				.sort((a, b) => a[1].at - b[1].at)[0];
			if (!due) break;
			timers.delete(due[0]);
			clock.now = due[1].at;
			due[1].handler();
		}
		clock.now = target;
	};
	const module = loadModule('socket/transport.ts', globals, typeModule);
	return { clock, timers, sockets, advance, ...module };
}

test('reconnect backoff is capped at 2 s including jitter', () => {
	const { reconnectDelay } = harness();
	assert.equal(
		reconnectDelay(1, () => 0),
		250
	);
	for (let attempt = 1; attempt < 50; attempt++) {
		assert.ok(reconnectDelay(attempt, () => 0.9999) <= 2_000, `attempt ${attempt}`);
	}
	assert.ok(reconnectDelay(20, () => 0) >= 1_500);
});

test('dropped connections reconnect within 2 s, repeatedly', () => {
	const { SocketTransport, sockets, advance } = harness();
	const transport = new SocketTransport();
	transport.connect('ws://test/ws');
	for (let drop = 0; drop < 8; drop++) {
		sockets.at(-1).handlers.close({ code: 1006 });
		const before = sockets.length;
		advance(2_000);
		assert.equal(sockets.length, before + 1, `drop ${drop} reconnected within 2 s`);
	}
	transport.disconnect();
});

test('heartbeat silence for 35 s forces a reconnect; traffic keeps the link', () => {
	const { SocketTransport, sockets, advance, timers } = harness();
	const transport = new SocketTransport();
	transport.connect('ws://test/ws');
	const first = sockets[0];
	first.greet();
	assert.ok(transport.ready);
	for (let beat = 0; beat < 5; beat++) {
		advance(25_000);
		first.receive({ o: 5, d: { server_time: 1 } });
	}
	assert.equal(sockets.length, 1, 'heartbeats keep the connection');
	advance(34_000);
	assert.equal(sockets.length, 1);
	advance(1_001);
	assert.equal(first.closedWith, 4000, 'silent socket is closed');
	assert.equal(transport.ready, false);
	advance(2_000);
	assert.equal(sockets.length, 2, 'a fresh connection replaces the silent one');
	transport.disconnect();
	assert.equal(timers.size, 0, 'manual disconnect leaves no timers');
});

test('a connection that never greets is abandoned after 35 s', () => {
	const { SocketTransport, sockets, advance } = harness();
	const transport = new SocketTransport();
	transport.connect('ws://test/ws');
	advance(35_001);
	assert.equal(sockets[0].closedWith, 4000);
	advance(2_000);
	assert.equal(sockets.length, 2);
	transport.disconnect();
});

test('clock offset is re-measured on each greeting and by round-trip probes', () => {
	const { SocketTransport, sockets, advance, clock } = harness();
	const transport = new SocketTransport();
	transport.connect('ws://test/ws');
	sockets[0].greet(clock.now + 5_000);
	assert.equal(transport.serverClockOffset, 5_000);
	const probe = sockets[0].sent.find((message) => message.o === 5);
	assert.ok(probe, 'greeting triggers a clock probe');
	// 100 ms round trip; server clock is really 3 s ahead at the midpoint.
	advance(100);
	sockets[0].receive({
		o: 5,
		d: { client_time: probe.d.client_time, server_time: probe.d.client_time + 50 + 3_000 }
	});
	assert.equal(transport.serverClockOffset, 3_000);
	advance(30_000);
	assert.equal(sockets[0].sent.filter((message) => message.o === 5).length, 2, 'periodic probe');

	// Reconnect to a server with a different clock: the greeting wins again.
	sockets[0].handlers.close({ code: 1006 });
	advance(2_000);
	sockets[1].greet(clock.now - 7_000);
	assert.equal(transport.serverClockOffset, -7_000);
	transport.disconnect();
});

test('retarget reconnects an active transport and only remembers on a stopped one', () => {
	const { SocketTransport, sockets } = harness();
	const transport = new SocketTransport();
	transport.connect('ws://test/ws');
	transport.retarget('ws://test/ws?view=public');
	assert.equal(sockets.length, 2);
	assert.equal(sockets[1].url, 'ws://test/ws?view=public');
	transport.retarget('ws://test/ws?view=public');
	assert.equal(sockets.length, 2, 'same URL is a no-op');
	transport.disconnect();
	transport.retarget('ws://test/ws');
	assert.equal(sockets.length, 2, 'stopped transport does not reconnect');
	assert.equal(transport.url, 'ws://test/ws');
});

function runtime(match_id, epoch, version) {
	return { match_id, epoch, version, controller: {} };
}

function matchHarness() {
	const handlers = {};
	let onOpen;
	const transport = {
		on: (name, handler) => (handlers[name] ??= []).push(handler),
		onOpen: (handler) => (onOpen = handler),
		request: () => new Promise(() => undefined)
	};
	const { MatchSocket, isStaleRuntime } = loadModule('socket/match/index.svelte.ts', {
		queueMicrotask,
		crypto
	});
	const match = new MatchSocket(transport);
	const emit = (name, value) => handlers[name].forEach((handler) => handler(value));
	return { match, emit, open: () => onOpen(), isStaleRuntime };
}

test('match socket ignores older versions of the same match and epoch', () => {
	const { match, emit, open, isStaleRuntime } = matchHarness();
	emit('Match.Updated', runtime('m1', 1, 5));
	emit('Match.PublicUpdated', runtime('m1', 1, 4));
	assert.equal(match.current.version, 5, 'late payload ignored');
	emit('Match.PublicUpdated', runtime('m1', 1, 5));
	assert.equal(match.current.version, 5, 'equal version accepted');
	assert.equal(match.accept(runtime('m1', 1, 3)).version, 5, 'accept returns the newer state');
	emit('Match.Updated', runtime('m1', 2, 1));
	assert.equal(match.current.epoch, 2, 'new epoch replaces');
	emit('Match.Updated', runtime('m1', 1, 99));
	assert.equal(match.current.epoch, 1, 'different epoch always replaces');
	emit('Match.Updated', runtime('m2', 1, 1));
	assert.equal(match.current.match_id, 'm2', 'different match replaces');
	emit('Match.Updated', runtime('', 0, 0));
	assert.equal(match.current, null, 'unload clears');

	emit('Match.Updated', runtime('m3', 1, 10));
	open();
	emit('Match.Updated', runtime('m3', 1, 2));
	assert.equal(match.current.version, 2, 'first snapshot after reconnect is the new baseline');
	emit('Match.Updated', runtime('m3', 1, 1));
	assert.equal(match.current.version, 2);

	assert.equal(isStaleRuntime(runtime('a', 1, 1), null), false);
	assert.equal(isStaleRuntime(runtime('a', 1, 1), runtime('a', 1, 2)), true);
});

test('Khởi động request captures one snapshot and only accepts successful runtime', async () => {
	let uuid = 0;
	const { MatchSocket } = loadModule('socket/match/index.svelte.ts', { crypto: { randomUUID: () => `id-${++uuid}` } });
	const calls = [];
	const transport = {
		on() {}, onOpen() {},
		request(operation, payload) {
			return new Promise((resolve, reject) => calls.push({ operation, payload, resolve, reject }));
		}
	};
	const match = new MatchSocket(transport);
	const snapshot = { match_id: 'm', epoch: 3, version: 4, input_epoch: 5, bell_epoch: 6, controller: { selected_question_id: 'kd_chung.0' }, contestants: [{ slot: 2, has_rung: true }] };
	match.accept(snapshot);
	const request = match.judgeKhoiDong(snapshot, 'wrong');
	match.accept({ ...snapshot, version: 5, controller: { selected_question_id: 'kd_chung.1' } });
	assert.equal(calls[0].operation, 'Match.JudgeKhoiDong');
	assert.equal(JSON.stringify(calls[0].payload), JSON.stringify({ command: { match_id: 'm', expected_epoch: 3, expected_version: 4, expected_input_epoch: 5, expected_bell_epoch: 6, command_id: 'id-1' }, question_id: 'kd_chung.0', verdict: 'wrong', expected_bell_slot: 2 }));
	calls[0].reject(new Error('stale'));
	await assert.rejects(request, /stale/);
	assert.equal(match.current.version, 5);
	const second = match.judgeKhoiDong({ ...snapshot, contestants: [] }, 'wrong');
	assert.equal(calls[1].payload.expected_bell_slot, 0);
	assert.notEqual(calls[1].payload.command.command_id, calls[0].payload.command.command_id);
	calls[1].resolve({ runtime: { ...snapshot, version: 6 }, question_id: 'kd_chung.0', verdict: 'wrong', slot: 0, delta: 0 });
	await second;
	assert.equal(match.current.version, 6);
});

test('score draft saves bind undo IDs, retain rejected drafts, and preserve KD sources', async () => {
	let uuid = 0;
	let rejectSave = true;
	let rejectUndo = false;
	const calls = [];
	const initial = {
		match_id: 'm', epoch: 1, version: 4,
		controller: { selected_question_id: 'kd_chung.0' },
		current_question: { id: 'kd_chung.0' },
		contestants: [{ slot: 1, score: 10 }, { slot: 2, score: 10 }]
	};
	const transport = {
		on() {}, onOpen() {},
		async request(operation, payload) {
			calls.push({ operation, payload });
			if (rejectSave) throw new Error('fast judgment already committed');
			if (rejectUndo && operation === 'Match.UndoScores') throw new Error('already undone');
			return { ...initial, version: 8 };
		}
	};
	const { MatchSocket } = loadModule('socket/match/index.svelte.ts', { crypto: { randomUUID: () => `id-${++uuid}` } });
	const match = new MatchSocket(transport);
	match.accept(initial);
	const scoring = loadModule('operator/scoring.ts');
	const drafts = loadModule('operator/drafts.ts', {}, { './scoring': scoring });
	const { ScoreDrafts } = loadModule(
		'operator/score-drafts.svelte.ts',
		{ $derived: (value) => value },
		{
			'$lib/api': { api: { settings: { current: null } } },
			'$lib/socket': { socket: { match } },
			'./drafts': drafts,
			'./scoring': scoring
		}
	);
	const store = new ScoreDrafts();
	store.toggle('kd_chung.0', 1, 'correct');
	store.toggle('tt.0', 2, 'correct');
	match.accept({ ...initial, version: 8 });
	const before = JSON.stringify(store.state);
	await assert.rejects(store.save(), /fast judgment/);
	assert.equal(JSON.stringify(store.state), before, 'failed save keeps every draft and undo entry');
	assert.equal(store.saving, false);
	assert.equal(calls[0].operation, 'Match.AdjustScores');
	assert.equal(JSON.stringify(calls[0].payload.sources), JSON.stringify([{ question_id: 'kd_chung.0', version: 4, kind: 'verdict' }]));

	rejectSave = false;
	assert.equal(await store.save(), true);
	assert.equal(JSON.stringify(calls[1].payload.sources), JSON.stringify([{ question_id: 'kd_chung.0', version: 4, kind: 'verdict' }]));
	assert.deepEqual(Object.keys(store.state.lastSave).sort(), ['adjustments', 'at', 'commandId', 'reason', 'sources']);
	assert.equal(JSON.stringify(store.state.lastSave.sources), JSON.stringify(calls[1].payload.sources));
	assert.equal(store.state.lastSave.commandId, calls[1].payload.command.command_id);
	assert.notEqual(calls[1].payload.command.command_id, calls[0].payload.command.command_id);
	const otherTab = new ScoreDrafts();
	otherTab.state = drafts.parseDraftState(JSON.stringify(store.state));
	otherTab.toggle('tt.2', 1, 'correct');
	const otherBefore = JSON.stringify(otherTab.state);
	store.toggle('tt.1', 2, 'correct');
	const pending = JSON.stringify(store.state.batches);
	assert.equal(await store.undo(), true);
	assert.equal(JSON.stringify(store.state.batches), pending);
	assert.equal(calls[2].operation, 'Match.UndoScores');
	assert.equal(calls[2].payload.undo_of, calls[1].payload.command.command_id);
	assert.equal(JSON.stringify(calls[2].payload.command), JSON.stringify({
		command_id: 'id-3', match_id: 'm', expected_epoch: 1, expected_version: 8
	}));
	assert.deepEqual(Object.keys(calls[2].payload).sort(), ['command', 'undo_of']);
	assert.equal(store.state.lastSave, null);
	rejectUndo = true;
	await assert.rejects(otherTab.undo(), /already undone/);
	assert.equal(JSON.stringify(otherTab.state), otherBefore, 'rejected cross-tab undo keeps saved identity and pending batches');
	assert.equal(otherTab.saving, false);
	assert.equal(calls[3].payload.undo_of, calls[2].payload.undo_of);
	assert.notEqual(calls[3].payload.command.command_id, calls[2].payload.command.command_id);
	rejectUndo = false;
	assert.equal(await store.save(), true);
	assert.equal('sources' in calls[4].payload, false);
	await match.adjustScores([{ slot: 1, delta: 1 }], 'manual', []);
	assert.equal('sources' in calls[5].payload, false);
	await match.adjustScores([{ slot: 1, delta: 1 }], 'manual');
	store.setTotal(1, -5);
	assert.equal(await store.save(), true);
	assert.equal(JSON.stringify(calls[7].payload.sources), JSON.stringify([{ question_id: 'kd_chung.0', version: 8, kind: 'correction' }]));
	assert.equal(JSON.stringify(store.state.lastSave.sources), JSON.stringify(calls[7].payload.sources));
	assert.equal('sources' in calls[6].payload, false);

	const legacy = {
		batches: JSON.parse(otherBefore).batches,
		lastSave: { adjustments: [{ slot: 1, delta: 10 }], reason: 'Khởi động · Câu 1', at: 1 }
	};
	otherTab.state = drafts.parseDraftState(JSON.stringify(legacy));
	rejectSave = true;
	await assert.rejects(otherTab.undo(), /fast judgment/);
	assert.equal(JSON.stringify(otherTab.state), JSON.stringify(legacy));
	assert.equal(otherTab.saving, false);
	assert.equal(calls[8].operation, 'Match.AdjustScores');
	assert.equal(JSON.stringify(calls[8].payload.adjustments), JSON.stringify([{ slot: 1, delta: -10 }]));
	assert.equal(calls[8].payload.reason, 'Hoàn tác · Khởi động · Câu 1');
	assert.equal('sources' in calls[8].payload, false);
	rejectSave = false;
	assert.equal(await otherTab.undo(), true);
	assert.equal(otherTab.state.lastSave, null);
	assert.equal(JSON.stringify(otherTab.state.batches), JSON.stringify(legacy.batches));
});

test('socket.usePublicView switches to ?view=public and restores on release', () => {
	const retargets = [];
	let connected = '';
	class SocketTransport {
		serverClockOffset = 0;
		connect(url) {
			connected = url;
		}
		retarget(url) {
			retargets.push(url);
		}
		on() {
			return () => undefined;
		}
		onOpen() {
			return () => undefined;
		}
	}
	const Stub = class {};
	const { SocketApi } = loadModule(
		'socket/index.ts',
		{ URL },
		{
			'$app/environment': { browser: false },
			'./contestant': { ContestantSocket: Stub },
			'./controller': { ControllerSocket: Stub },
			'./match/index.svelte': { MatchSocket: Stub },
			'./timer/index.svelte': { TimerSocket: Stub },
			'./transport': { SocketTransport },
			'./sound/index.svelte': { SoundSocket: Stub },
			'./presence/index.svelte': { PresenceSocket: Stub }
		}
	);
	const transport = new SocketTransport();
	const api = new SocketApi(transport);
	let sessionChecks = 0;
	api.onSessionCheck(() => sessionChecks++);
	api.connect('ws://host/ws');
	assert.equal(connected, 'ws://host/ws');
	const release = api.usePublicView();
	const second = api.usePublicView();
	assert.deepEqual(retargets, ['ws://host/ws?view=public']);
	assert.ok(api.publicView);
	transport.onSessionCheck();
	assert.equal(sessionChecks, 0, 'public view never chases the operator session');
	release();
	release();
	assert.equal(retargets.length, 1, 'still held by another view');
	second();
	assert.deepEqual(retargets, ['ws://host/ws?view=public', 'ws://host/ws']);
	transport.onSessionCheck();
	assert.equal(sessionChecks, 1);
});

test('initial settings load retries until it succeeds; newest response wins', async () => {
	let failures = 2;
	const pending = [];
	const http = {
		request: () => {
			if (failures-- > 0) return Promise.reject(new Error('offline'));
			const deferred = Promise.withResolvers();
			pending.push(deferred);
			return deferred.promise;
		}
	};
	const { SettingsApi } = loadModule('api/settings/index.svelte.ts', {
		setTimeout: (handler) => setTimeout(handler, 0),
		clearTimeout
	});
	const settings = new SettingsApi(http);
	const loading = settings.load({ initialDelayMs: 1 });
	while (!pending.length) await new Promise((resolve) => setTimeout(resolve, 1));
	pending[0].resolve({ settings: { event_name: 'loaded' } });
	assert.equal((await loading).event_name, 'loaded');
	assert.equal(settings.current.event_name, 'loaded');

	const older = settings.get();
	const newer = settings.get();
	pending[2].resolve({ settings: { event_name: 'newer' } });
	await newer;
	pending[1].resolve({ settings: { event_name: 'older' } });
	await older;
	assert.equal(settings.current.event_name, 'newer', 'late response cannot overwrite');

	const controller = new AbortController();
	failures = 1_000;
	const aborted = settings.load({ signal: controller.signal, initialDelayMs: 1 });
	setTimeout(() => controller.abort(), 5);
	await assert.rejects(aborted, (error) => error.name === 'AbortError');
});
