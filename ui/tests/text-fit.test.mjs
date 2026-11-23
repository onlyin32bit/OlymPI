import assert from 'node:assert/strict';
import test from 'node:test';
import { fitText, largestFittingFontSize } from '../src/lib/display/fit-text.ts';

function harness({ resizeObserver = true } = {}) {
	const frames = new Map();
	const observers = {};
	let nextFrame = 0;
	let probes = 0;
	const ready = Promise.withResolvers();
	const fonts = Object.assign(new EventTarget(), { ready: ready.promise });
	const view = Object.assign(new EventTarget(), {
		requestAnimationFrame(callback) {
			frames.set(++nextFrame, callback);
			return nextFrame;
		},
		cancelAnimationFrame(id) {
			frames.delete(id);
		},
		ResizeObserver: resizeObserver
			? class {
					constructor(callback) {
						this.callback = callback;
						observers.resize = this;
					}
					observe(target) {
						this.target = target;
					}
					disconnect() {
						this.disconnected = true;
					}
				}
			: undefined,
		MutationObserver: class {
			constructor(callback) {
				this.callback = callback;
				observers.mutations = this;
			}
			observe(target, options) {
				this.target = target;
				this.options = options;
			}
			disconnect() {
				this.disconnected = true;
			}
		}
	});
	const node = {
		ownerDocument: { defaultView: view, fonts },
		isConnected: true,
		clientWidth: 200,
		clientHeight: 100,
		style: { fontSize: '20px', visibility: '' }
	};
	const state = {
		frames,
		observers,
		ready,
		fonts,
		node,
		view,
		get probes() {
			return probes;
		},
		measure(size) {
			return { width: node.clientWidth, height: size * 2 };
		},
		flush() {
			const pending = [...frames.values()];
			frames.clear();
			for (const callback of pending) callback();
		}
	};
	node.firstElementChild = {
		get scrollWidth() {
			probes++;
			return Math.ceil(state.measure(parseFloat(node.style.fontSize)).width);
		},
		get scrollHeight() {
			return Math.ceil(state.measure(parseFloat(node.style.fontSize)).height);
		},
		get offsetHeight() {
			return this.scrollHeight;
		}
	};
	return state;
}

test('long text shrinks to the largest fitting quarter pixel with a bounded probe count', () => {
	for (const threshold of [1, 12.13, 31.49, 51.63, 64.8, 119.9, 159.99]) {
		let calls = 0;
		const size = largestFittingFontSize(160, (candidate) => {
			calls++;
			return candidate <= threshold;
		});
		assert.equal(size, Math.floor(threshold * 4) / 4);
		assert.ok(calls <= 11, `Too many layout probes: ${calls}`);
	}
});

test('short text uses its exact cap in one probe rather than growing for dozens of frames', () => {
	let calls = 0;
	assert.equal(
		largestFittingFontSize(45.36, () => {
			calls++;
			return true;
		}),
		45.36
	);
	assert.equal(calls, 1);
});

test('invalid caps fail without entering an unbounded search', () => {
	for (const maximum of [0, -1, NaN, Infinity])
		assert.throws(() => largestFittingFontSize(maximum, () => true), RangeError);
});

test('fitting measures both axes in logical pixels and preserves the entire full-width child', () => {
	const h = harness();
	const action = fitText(h.node, 160);
	assert.equal(h.node.style.visibility, 'hidden');
	h.flush();
	assert.equal(h.node.style.fontSize, '49px');
	assert.equal(h.node.style.visibility, '');
	assert.ok(h.probes <= 11);
	assert.equal(h.observers.resize.target, h.node);
	assert.equal(h.observers.mutations.options.attributes, undefined);
	h.measure = (size) => ({ width: size * 20, height: 10 });
	h.observers.mutations.callback();
	h.flush();
	assert.equal(h.node.style.fontSize, '10px', 'horizontal overflow must also shrink the font');
	action.destroy();
});

test('content, resize and late-font invalidations are coalesced into one frame', async () => {
	const h = harness();
	const action = fitText(h.node, 160);
	h.flush();
	h.node.clientHeight = 82;
	h.observers.resize.callback();
	h.observers.mutations.callback();
	h.fonts.dispatchEvent(new Event('loadingdone'));
	h.ready.resolve(h.fonts);
	await Promise.resolve();
	assert.equal(h.frames.size, 1);
	h.flush();
	assert.equal(h.node.style.fontSize, '40px');
	assert.equal(h.frames.size, 0);
	action.destroy();
});

test('same parameters and idle time do not perpetually refit; a changed cap does refit', () => {
	const h = harness();
	const action = fitText(h.node, 160);
	h.flush();
	const probes = h.probes;
	action.update(160);
	h.flush();
	assert.equal(h.probes, probes);
	assert.equal(h.frames.size, 0);
	action.update(30);
	h.flush();
	assert.equal(h.node.style.fontSize, '30px');
	action.destroy();
});

test('zero-sized and detached scenes do not measure; a newly visible box can fit later', () => {
	const h = harness();
	h.node.clientHeight = 0;
	const action = fitText(h.node, 160);
	h.flush();
	assert.equal(h.probes, 0);
	h.node.clientHeight = 100;
	h.node.isConnected = false;
	h.observers.resize.callback();
	h.flush();
	assert.equal(h.probes, 0);
	h.node.isConnected = true;
	h.observers.resize.callback();
	h.flush();
	assert.equal(h.node.style.fontSize, '49px');
	action.destroy();
});

test('destroy cancels pending work, ignores late callbacks and restores owned styles', async () => {
	const h = harness();
	const action = fitText(h.node, 160);
	action.destroy();
	h.ready.resolve(h.fonts);
	await Promise.resolve();
	h.fonts.dispatchEvent(new Event('loadingdone'));
	h.observers.mutations.callback();
	h.observers.resize.callback();
	assert.equal(h.frames.size, 0);
	assert.equal(h.probes, 0);
	assert.equal(h.observers.resize.disconnected, true);
	assert.equal(h.observers.mutations.disconnected, true);
	assert.equal(h.node.style.fontSize, '20px');
	assert.equal(h.node.style.visibility, '');
});

test('window resize fallback is removed during teardown', () => {
	const h = harness({ resizeObserver: false });
	const action = fitText(h.node, 160);
	h.flush();
	h.view.dispatchEvent(new Event('resize'));
	assert.equal(h.frames.size, 1);
	action.destroy();
	h.view.dispatchEvent(new Event('resize'));
	assert.equal(h.frames.size, 0);
});
