import type { Sound, SoundBus, SoundSettings } from '$lib/types';
import type { SoundPlayback, SoundStatus, SoundStoppedEvent } from '$lib/socket/opsocket';

/** Buses with a single slot: a new playback replaces the previous one. */
export function singleSlot(bus: string): boolean {
	return bus === 'music' || bus === 'timer';
}

/** The mixer node a bus plays through: timer sounds share the music gain. */
export function mixerFor(bus: string): 'music' | 'effects' {
	return bus === 'music' || bus === 'timer' ? 'music' : 'effects';
}

type ActiveSource = {
	source: AudioBufferSourceNode;
	gain: GainNode;
	bus: SoundBus;
	fading: boolean;
};

export class AudioEngine {
	#context: AudioContext | null = null;
	#master: GainNode | null = null;
	#music: GainNode | null = null;
	#effects: GainNode | null = null;
	#meter: AnalyserNode | null = null;
	#stream: MediaStreamAudioDestinationNode | null = null;
	#output: HTMLAudioElement | null = null;
	#buffers = new Map<string, AudioBuffer>();
	#sounds = new Map<string, Sound>();
	#active = new Map<string, ActiveSource>();
	#pending = new Map<string, { playback: SoundPlayback }>();
	#seen = new Set<string>();
	#settings: SoundSettings | null = null;
	#outputName = 'Mặc định hệ thống';
	#enabled = false;
	#disposed = false;
	#generation = 0;
	#timers = new Set<ReturnType<typeof setTimeout>>();

	get ready(): boolean {
		return this.#context?.state === 'running';
	}

	get outputName(): string {
		return this.#outputName;
	}

	get canChooseOutput(): boolean {
		if (typeof navigator === 'undefined') return false;
		const devices = navigator.mediaDevices as MediaDevices & {
			selectAudioOutput?: () => Promise<MediaDeviceInfo>;
		};
		const output = this.#output as
			| (HTMLAudioElement & { setSinkId?: (sinkId: string) => Promise<void> })
			| null;
		return (
			typeof devices?.selectAudioOutput === 'function' && typeof output?.setSinkId === 'function'
		);
	}

	async enable(settings: SoundSettings, sounds: Sound[]): Promise<void> {
		if (this.#disposed) return;
		const generation = ++this.#generation;
		if (!this.#context) {
			this.#context = new AudioContext({ latencyHint: 'interactive' });
			this.#context.addEventListener('statechange', () =>
				audioLog('context state', this.#context?.state)
			);
			this.#master = this.#context.createGain();
			this.#music = this.#context.createGain();
			this.#effects = this.#context.createGain();
			this.#meter = this.#context.createAnalyser();
			this.#stream = this.#context.createMediaStreamDestination();
			this.#output = new Audio();
			this.#output.autoplay = true;
			this.#output.srcObject = this.#stream.stream;
			this.#meter.fftSize = 2048;
			this.#music.connect(this.#master);
			this.#effects.connect(this.#master);
			this.#master.connect(this.#meter);
			this.#meter.connect(this.#stream);
		}
		const context = this.#context;
		const output = this.#output;
		await context.resume();
		if (generation !== this.#generation || this.#disposed) return;
		await output?.play();
		if (generation !== this.#generation || this.#disposed) {
			output?.pause();
			return;
		}
		this.#enabled = true;
		audioLog('enabled', {
			state: this.#context.state,
			sampleRate: this.#context.sampleRate,
			outputs: sounds.length,
			sinkId: getSinkId(this.#output),
			output: 'media element',
			baseLatency: this.#context.baseLatency,
			outputLatency: this.#context.outputLatency
		});
		this.setSettings(settings);
		await this.setCatalog(sounds);
	}

	async disable(): Promise<void> {
		this.#enabled = false;
		this.#generation++;
		this.stop({ all: true, fade_ms: 0 });
		this.#output?.pause();
		await this.#context?.suspend();
		this.#seen.clear();
	}

	async dispose(): Promise<void> {
		this.#disposed = true;
		this.#enabled = false;
		this.#generation++;
		this.stop({ all: true, fade_ms: 0 });
		for (const timer of this.#timers) clearTimeout(timer);
		this.#timers.clear();
		this.#output?.pause();
		if (this.#output) this.#output.srcObject = null;
		for (const track of this.#stream?.stream.getTracks() ?? []) track.stop();
		this.#master?.disconnect();
		this.#music?.disconnect();
		this.#effects?.disconnect();
		this.#meter?.disconnect();
		const context = this.#context;
		this.#context = null;
		this.#master = null;
		this.#music = null;
		this.#effects = null;
		this.#meter = null;
		this.#stream = null;
		this.#output = null;
		this.#buffers.clear();
		this.#sounds.clear();
		this.#seen.clear();
		if (context && context.state !== 'closed') await context.close();
	}

	async reconcile(
		status: SoundStatus,
		localStartTime: (timestamp: number) => number
	): Promise<void> {
		const music = status.music;
		for (const [id, pending] of this.#pending) {
			if (pending.playback.bus === 'music' && id !== music?.playback_id) {
				this.#pending.delete(id);
				this.#seen.delete(id);
			}
		}
		for (const [id, active] of this.#active) {
			if (active.bus === 'music' && id !== music?.playback_id) {
				this.#fadeStop(id, active, 0);
				this.#seen.delete(id);
			}
		}
		if (music) await this.play(music, localStartTime(music.start_at));
	}

	setSettings(settings: SoundSettings): void {
		this.#settings = settings;
		if (!this.#context || !this.#master || !this.#music || !this.#effects) return;
		const now = this.#context.currentTime;
		this.#master.gain.setTargetAtTime(settings.enabled ? settings.master_volume : 0, now, 0.015);
		this.#music.gain.setTargetAtTime(settings.music_volume, now, 0.015);
		this.#effects.gain.setTargetAtTime(settings.effects_volume, now, 0.015);
		audioLog('mixer', settings);
	}

	async setCatalog(sounds: Sound[]): Promise<void> {
		this.#sounds = new Map(sounds.map((sound) => [sound.id, sound]));
		await Promise.allSettled(sounds.map((sound) => this.#load(sound.id, sound.url)));
	}

	async play(playback: SoundPlayback, localStartTime: number): Promise<void> {
		if (
			this.#disposed ||
			!this.#enabled ||
			!this.#context ||
			!this.#master ||
			!this.#music ||
			!this.#effects ||
			!this.#settings?.enabled
		) {
			audioError('play ignored: engine is not ready', {
				context: this.#context?.state,
				hasSettings: Boolean(this.#settings),
				enabled: this.#settings?.enabled
			});
			return;
		}
		if (this.#seen.has(playback.playback_id)) return;
		if (singleSlot(playback.bus)) {
			for (const [id, pending] of this.#pending) {
				if (pending.playback.bus === playback.bus) {
					this.#pending.delete(id);
					this.#seen.delete(id);
				}
			}
		}
		this.#seen.add(playback.playback_id);
		const pending = { playback };
		this.#pending.set(playback.playback_id, pending);
		const generation = this.#generation;
		const known = this.#sounds.get(playback.sound_id);
		let buffer: AudioBuffer | null;
		try {
			buffer = await this.#load(playback.sound_id, playback.url || known?.url || '', true);
		} catch (error) {
			if (this.#pending.get(playback.playback_id) === pending) {
				this.#pending.delete(playback.playback_id);
				this.#seen.delete(playback.playback_id);
			}
			throw error;
		}
		if (generation !== this.#generation || this.#pending.get(playback.playback_id) !== pending) {
			return;
		}
		this.#pending.delete(playback.playback_id);
		if (!buffer || !this.#context || this.#context.state !== 'running') {
			audioError('play ignored after decode', {
				soundId: playback.sound_id,
				hasBuffer: Boolean(buffer),
				context: this.#context?.state
			});
			this.#seen.delete(playback.playback_id);
			return;
		}
		if (singleSlot(playback.bus)) {
			for (const [id, active] of this.#active) {
				if (active.bus === playback.bus && !active.fading) {
					this.#fadeStop(id, active, playback.fade_out_ms);
				}
			}
		}
		const elapsed = Math.max(0, (Date.now() - localStartTime) / 1000);
		if (!playback.loop && elapsed >= buffer.duration) return;
		const offset = playback.loop ? elapsed % buffer.duration : elapsed;

		const source = this.#context.createBufferSource();
		const gain = this.#context.createGain();
		const bus: SoundBus =
			playback.bus === 'music' || playback.bus === 'timer' ? playback.bus : 'effects';
		const mixer = mixerFor(bus) === 'music' ? this.#music : this.#effects;
		source.buffer = buffer;
		source.loop = playback.loop;
		source.connect(gain);
		gain.connect(mixer);

		const delay = Math.max(0, (localStartTime - Date.now()) / 1000);
		const startAt = this.#context.currentTime + delay;
		const target = Math.max(0, Math.min(1, playback.volume));
		if (playback.fade_in_ms > 0) {
			gain.gain.setValueAtTime(0, startAt);
			gain.gain.linearRampToValueAtTime(target, startAt + playback.fade_in_ms / 1000);
		} else {
			gain.gain.setValueAtTime(target, startAt);
		}
		this.#active.set(playback.playback_id, { source, gain, bus, fading: false });
		audioLog('play scheduled', {
			playbackId: playback.playback_id,
			soundId: playback.sound_id,
			bus,
			startInMs: Math.round(delay * 1000),
			volume: target,
			master: this.#master.gain.value,
			busVolume: mixer.gain.value,
			duration: buffer.duration
		});
		source.addEventListener(
			'ended',
			() => {
				audioLog('play ended', {
					playbackId: playback.playback_id,
					soundId: playback.sound_id
				});
				if (this.#active.get(playback.playback_id)?.source === source) {
					this.#active.delete(playback.playback_id);
				}
				source.disconnect();
				gain.disconnect();
				if (!this.#disposed) {
					const timer = setTimeout(() => {
						this.#timers.delete(timer);
						if (generation === this.#generation && !this.#active.has(playback.playback_id)) {
							this.#seen.delete(playback.playback_id);
						}
					}, 5_000);
					this.#timers.add(timer);
				}
			},
			{ once: true }
		);
		source.start(startAt, offset);
		const timer = setTimeout(
			() => {
				this.#timers.delete(timer);
				this.#logSignal(playback);
			},
			delay * 1000 + 250
		);
		this.#timers.add(timer);
	}

	async chooseOutput(): Promise<string> {
		if (!this.#output) throw new Error('Bật âm thanh trước khi chọn loa');
		const devices = navigator.mediaDevices as MediaDevices & {
			selectAudioOutput?: () => Promise<MediaDeviceInfo>;
		};
		const outputElement = this.#output as HTMLAudioElement & {
			setSinkId?: (sinkId: string) => Promise<void>;
		};
		if (!devices.selectAudioOutput || !outputElement.setSinkId) {
			throw new Error('Trình duyệt này không hỗ trợ chọn loa; hãy chọn trong cài đặt hệ điều hành');
		}
		const output = await devices.selectAudioOutput();
		await outputElement.setSinkId(output.deviceId);
		// ve-dich-video follows this speaker ($lib/audio/output.ts listens).
		window.dispatchEvent(new CustomEvent('4t:audio-output', { detail: output.deviceId }));
		this.#outputName = output.label || 'Thiết bị đã chọn';
		audioLog('output selected', { name: this.#outputName, sinkId: output.deviceId });
		return this.#outputName;
	}

	stop(event: SoundStoppedEvent): void {
		for (const [id, pending] of this.#pending) {
			if (!event.all && event.playback_id && event.playback_id !== id) continue;
			if (!event.all && event.bus && event.bus !== pending.playback.bus) continue;
			if (!event.all && !event.playback_id && !event.bus) continue;
			this.#pending.delete(id);
			this.#seen.delete(id);
		}
		if (!this.#context) return;
		for (const [id, active] of this.#active) {
			if (!event.all && event.playback_id && event.playback_id !== id) continue;
			if (!event.all && event.bus && event.bus !== active.bus) continue;
			if (!event.all && !event.playback_id && !event.bus) continue;
			this.#fadeStop(id, active, event.fade_ms);
		}
	}

	async #load(id: string, url: string, trace = false): Promise<AudioBuffer | null> {
		const cached = this.#buffers.get(id);
		if (cached) {
			if (trace)
				audioLog('buffer ready', {
					id,
					duration: cached.duration,
					channels: cached.numberOfChannels,
					sampleRate: cached.sampleRate,
					cached: true
				});
			return cached;
		}
		if (!url || !this.#context) return null;
		const context = this.#context;
		const response = await fetch(url, { credentials: 'include' });
		if (trace)
			audioLog('fetched', {
				id,
				url,
				status: response.status,
				contentType: response.headers.get('content-type'),
				contentLength: response.headers.get('content-length')
			});
		if (!response.ok) throw new Error(`Không thể tải âm thanh (${response.status})`);
		const data = await response.arrayBuffer();
		if (this.#context !== context || this.#disposed) return null;
		const buffer = await context.decodeAudioData(data);
		if (this.#context !== context || this.#disposed) return null;
		if (trace)
			audioLog('decoded', {
				id,
				duration: buffer.duration,
				channels: buffer.numberOfChannels,
				sampleRate: buffer.sampleRate
			});
		this.#buffers.set(id, buffer);
		return buffer;
	}

	#fadeStop(id: string, active: ActiveSource, fadeMS: number): void {
		if (!this.#context) return;
		const now = this.#context.currentTime;
		const seconds = Math.max(0, fadeMS) / 1000;
		active.fading = seconds > 0;
		active.gain.gain.cancelScheduledValues(now);
		active.gain.gain.setValueAtTime(active.gain.gain.value, now);
		active.gain.gain.linearRampToValueAtTime(0, now + seconds);
		try {
			active.source.stop(seconds > 0 ? now + seconds + 0.02 : now);
		} catch {
			// The source may already have ended.
		}
		if (seconds === 0) this.#active.delete(id);
	}

	#logSignal(playback: SoundPlayback): void {
		if (!this.#meter) return;
		const samples = new Float32Array(this.#meter.fftSize);
		this.#meter.getFloatTimeDomainData(samples);
		let peak = 0;
		let sumSquares = 0;
		for (const sample of samples) {
			peak = Math.max(peak, Math.abs(sample));
			sumSquares += sample * sample;
		}
		audioLog('signal', {
			playbackId: playback.playback_id,
			soundId: playback.sound_id,
			peak: Number(peak.toFixed(4)),
			rms: Number(Math.sqrt(sumSquares / samples.length).toFixed(4))
		});
	}
}

function getSinkId(output: HTMLAudioElement | null): string {
	return (output as (HTMLAudioElement & { sinkId?: string }) | null)?.sinkId ?? 'browser default';
}

function audioLog(message: string, detail?: unknown): void {
	if (!import.meta.env.DEV) return;
	if (detail === undefined) console.info(`[audio] ${message}`);
	else console.info(`[audio] ${message}`, detail);
}

function audioError(message: string, detail?: unknown): void {
	if (!import.meta.env.DEV) return;
	if (detail === undefined) console.error(`[audio] ${message}`);
	else console.error(`[audio] ${message}`, detail);
}
