import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { render } from 'svelte/server';
import {
	createControllerTestServer,
	contestantPath,
	fixturePath
} from './controller-test-server.mjs';

const server = await createControllerTestServer();
after(() => server.close());
const { default: Contestant } = await server.ssrLoadModule(contestantPath);
const { fixture, socket, api } = await server.ssrLoadModule(fixturePath);
const { ContestantSocket } = await server.ssrLoadModule('/src/lib/socket/contestant/index.ts');

async function openQuestion(phase, screen, id, owner = 0) {
	fixture.reset(phase, screen);
	api.auth.user.fixed_slot = 1;
	await socket.controller.selectContestant(owner);
	await socket.controller.selectQuestion(id);
	await socket.controller.toggleQuestion(true);
}

function ringEnabled(slot) {
	api.auth.user.fixed_slot = slot;
	const body = render(Contestant).body;
	const button = body.match(/<button[^>]*aria-label="Nhấn chuông"[^>]*>/)?.[0];
	assert.ok(button, 'Bell stays visible on the full-screen contestant layout');
	return !/\sdisabled(?:[=\s>])/.test(button);
}

test('all four common-round contestant pages can buzz; one claim disables all four', async () => {
	await openQuestion('khoi_dong', 'ques_chung', 'kd_chung.0');
	for (const slot of [1, 2, 3, 4]) assert.equal(ringEnabled(slot), true);
	api.auth.user.fixed_slot = 2;
	await socket.contestant.ring();
	for (const slot of [1, 2, 3, 4]) assert.equal(ringEnabled(slot), false);
	await socket.timer.start();
	await socket.timer.reset();
	await socket.match.setContestantLock(2, false);
	for (const slot of [1, 2, 3, 4]) assert.equal(ringEnabled(slot), false);
	await socket.controller.selectQuestion('kd_chung.1');
	await socket.controller.toggleQuestion(true);
	for (const slot of [1, 2, 3, 4]) assert.equal(ringEnabled(slot), true);
});

for (const [phase, screen, id] of [
	['khoi_dong', 'ques_ts1', 'kd_rieng.0.0'],
	['tang_toc', 'ques', 'tt.0']
]) {
	test(`${phase}/${screen} keeps every contestant bell disabled`, async () => {
		await openQuestion(phase, screen, id, 1);
		fixture.runtime.controller.is_allowing_bell = true;
		for (const slot of [1, 2, 3, 4]) assert.equal(ringEnabled(slot), false);
	});
}

test('VCNV contestant bells work without a question and stay independent of submitted answers', async () => {
	fixture.reset('vcnv', 'intro');
	for (const slot of [1, 2, 3, 4]) assert.equal(ringEnabled(slot), true);
	await openQuestion('vcnv', 'ques', 'vcnv.rows.0');
	await socket.timer.start();
	fixture.runtime.timer.elapsed_ms = 1200;
	await socket.contestant.submitAnswer('MY ANSWER');
	assert.equal(fixture.runtime.contestants[0].locked, true);
	assert.equal(ringEnabled(1), true);
	await socket.contestant.ring();
	assert.equal(fixture.runtime.contestants[0].time_ms, 1200);
	assert.equal(ringEnabled(1), false);
	for (const slot of [2, 3, 4]) assert.equal(ringEnabled(slot), true);
	const epoch = fixture.runtime.bell_epoch;
	await socket.controller.selectQuestion('vcnv.rows.1');
	await socket.timer.start();
	assert.equal(fixture.runtime.bell_epoch, epoch);
	assert.equal(fixture.runtime.contestants[0].has_rung, true);
	for (const slot of [2, 3, 4]) assert.equal(ringEnabled(slot), true);
});

test('Về đích contestant bells open only for nonowners during the five-second steal', async () => {
	await openQuestion('ve_dich', 'ques_ts1', 'vd.0.30.0', 1);
	await socket.timer.start();
	for (const slot of [1, 2, 3, 4]) assert.equal(ringEnabled(slot), false);
	await socket.timer.startSteal();
	assert.equal(fixture.runtime.timer.duration_ms, 5000);
	assert.equal(ringEnabled(1), false);
	for (const slot of [2, 3, 4]) assert.equal(ringEnabled(slot), true);
	await socket.timer.pause();
	for (const slot of [1, 2, 3, 4]) assert.equal(ringEnabled(slot), false);
	await socket.timer.resume();
	for (const slot of [2, 3, 4]) assert.equal(ringEnabled(slot), true);
	api.auth.user.fixed_slot = 2;
	await socket.contestant.ring();
	for (const slot of [1, 2, 3, 4]) assert.equal(ringEnabled(slot), false);
	await socket.timer.reset();
	await assert.rejects(socket.timer.startSteal());
});

test('a stale selected-contestant value cannot change the Về đích question owner', async () => {
	await openQuestion('ve_dich', 'ques_ts3', 'vd.2.30.0', 1);
	await socket.timer.startSteal();
	assert.equal(ringEnabled(3), false);
	for (const slot of [1, 2, 4]) assert.equal(ringEnabled(slot), true);
});

test('expired steals and operator-locked contestant stations stay disabled', async () => {
	await openQuestion('ve_dich', 'ques_ts1', 'vd.0.20.0', 1);
	await socket.timer.startSteal();
	fixture.setTimer(0);
	for (const slot of [1, 2, 3, 4]) assert.equal(ringEnabled(slot), false);
	fixture.reset('vcnv', 'main_vcnv');
	await socket.match.setContestantLock(2, true);
	assert.equal(ringEnabled(2), false);
	for (const slot of [1, 3, 4]) assert.equal(ringEnabled(slot), true);
});

function clientHarness() {
	fixture.reset('vcnv', 'start');
	let current = structuredClone(fixture.runtime);
	let resolve;
	const calls = [];
	const client = new ContestantSocket(
		{
			request(method, payload) {
				calls.push([method, payload]);
				return new Promise((done) => {
					resolve = done;
				});
			}
		},
		{
			get current() {
				return current;
			},
			accept(value) {
				current = value;
				return value;
			}
		}
	);
	return {
		client,
		calls,
		get current() {
			return current;
		},
		change(patch) {
			current = { ...current, ...patch };
		},
		reply(value) {
			resolve(value);
		}
	};
}

test('a VCNV bell acknowledgement survives a question/timer InputEpoch change', async () => {
	const h = clientHarness();
	const pending = h.client.ring();
	assert.equal(h.calls[0][0], 'Contestant.Ring');
	assert.equal(h.calls[0][1].expected_version, undefined);
	h.change({ input_epoch: h.current.input_epoch + 1, version: 2 });
	const reply = { ...h.current, version: 3 };
	h.reply(reply);
	assert.equal(await pending, reply);
});

for (const boundary of ['bell_epoch', 'epoch', 'match_id']) {
	test(`a bell acknowledgement cannot cross a ${boundary} boundary`, async () => {
		const h = clientHarness();
		const old = { ...h.current };
		const pending = h.client.ring();
		h.change({ [boundary]: boundary === 'match_id' ? 'different-match' : h.current[boundary] + 1 });
		h.reply(old);
		await assert.rejects(pending, { name: 'AbortError' });
	});
}

test('answer acknowledgements ignore BellEpoch changes but still reject changed answer windows', async () => {
	let h = clientHarness();
	let pending = h.client.submitAnswer('ANSWER');
	h.change({ bell_epoch: h.current.bell_epoch + 1, version: 2 });
	h.reply(h.current);
	assert.equal(await pending, h.current);
	h = clientHarness();
	pending = h.client.submitAnswer('STALE ANSWER');
	const old = { ...h.current };
	h.change({ input_epoch: h.current.input_epoch + 1 });
	h.reply(old);
	await assert.rejects(pending, { name: 'AbortError' });
});

test('a winning bell response accepts the already-received winning broadcast without rolling back newer state', async () => {
	const h = clientHarness();
	const pending = h.client.ring();
	h.change({ version: 2, controller: { ...h.current.controller, is_allowing_bell: false } });
	const reply = { ...h.current };
	h.change({ version: 3 });
	h.reply(reply);
	assert.equal(await pending, h.current);
	assert.equal(h.current.version, 3);
});
