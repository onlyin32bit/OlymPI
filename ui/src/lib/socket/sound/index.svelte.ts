import type {
	SoundPlayback,
	SoundSettingsUpdatedEvent,
	SoundStatus,
	SoundStoppedEvent
} from '../opsocket';
import type { SocketTransport } from '../transport';

type PlayedHandler = (playback: SoundPlayback) => void;
type StoppedHandler = (stop: SoundStoppedEvent) => void;
type SettingsHandler = (settings: SoundSettingsUpdatedEvent) => void;
type StatusHandler = (status: SoundStatus) => void;

export class SoundSocket {
	#status = $state<SoundStatus>({ output_count: 0 });
	#outputRequested = false;
	#statusRevision = 0;
	#requestRevision = 0;
	/* eslint-disable svelte/prefer-svelte-reactivity -- Callback registries are intentionally non-reactive and never rendered. */
	#played = new Set<PlayedHandler>();
	#stopped = new Set<StoppedHandler>();
	#catalog = new Set<() => void>();
	#settings = new Set<SettingsHandler>();
	#snapshots = new Set<StatusHandler>();
	/* eslint-enable svelte/prefer-svelte-reactivity */

	constructor(private readonly transport: SocketTransport) {
		transport.on('Sound.Played', (value) => {
			if (value.bus === 'music') {
				this.#statusRevision++;
				this.#status = { ...this.#status, music: value };
			}
			for (const handler of this.#played) handler(value);
		});
		transport.on('Sound.Stopped', (value) => {
			if (
				value.all ||
				value.bus === 'music' ||
				value.playback_id === this.#status.music?.playback_id
			) {
				this.#statusRevision++;
				this.#status = { ...this.#status, music: undefined };
			}
			for (const handler of this.#stopped) handler(value);
		});
		transport.on('Sound.CatalogUpdated', () => {
			for (const handler of this.#catalog) handler();
		});
		transport.on('Sound.SettingsUpdated', (value) => {
			for (const handler of this.#settings) handler(value);
		});
		transport.onOpen(() => {
			void (this.#outputRequested ? this.enableOutput() : this.getStatus()).catch(() => undefined);
		});
	}

	get status(): SoundStatus {
		return this.#status;
	}

	localStartTime(serverTimestamp: number): number {
		return serverTimestamp - this.transport.serverClockOffset;
	}

	async enableOutput(): Promise<SoundStatus> {
		this.#outputRequested = true;
		return this.#refreshStatus('Sound.EnableOutput');
	}

	async disableOutput(): Promise<SoundStatus> {
		this.#outputRequested = false;
		return this.#refreshStatus('Sound.DisableOutput');
	}

	async getStatus(): Promise<SoundStatus> {
		return this.#refreshStatus('Sound.GetStatus');
	}

	async #refreshStatus(
		operation: 'Sound.EnableOutput' | 'Sound.DisableOutput' | 'Sound.GetStatus'
	): Promise<SoundStatus> {
		const request = ++this.#requestRevision;
		const revision = this.#statusRevision;
		const status = await this.transport.request(operation, {});
		if (request !== this.#requestRevision) return this.#status;
		// Events received while the request was pending are newer than its snapshot.
		this.#status =
			revision === this.#statusRevision ? status : { ...status, music: this.#status.music };
		for (const handler of this.#snapshots) handler(this.#status);
		return this.#status;
	}

	onStatus(handler: StatusHandler): () => void {
		this.#snapshots.add(handler);
		return () => this.#snapshots.delete(handler);
	}

	async play(soundId: string): Promise<SoundPlayback> {
		return await this.transport.request('Sound.Play', { sound_id: soundId });
	}

	async stop(playbackId = '', fadeMs = 0): Promise<void> {
		await this.transport.request('Sound.Stop', { playback_id: playbackId, fade_ms: fadeMs });
	}

	async stopBus(bus: 'music' | 'effects' | 'timer', fadeMs = 0): Promise<void> {
		await this.transport.request('Sound.StopBus', { bus, fade_ms: fadeMs });
	}

	/** Phát media: plays the current question's audio through the TV sound output (re-press restarts). */
	async playQuestionMedia(): Promise<SoundPlayback> {
		return await this.transport.request('Sound.PlayQuestionMedia', {});
	}

	async stopAll(fadeMs = 0): Promise<void> {
		await this.transport.request('Sound.StopAll', { fade_ms: fadeMs });
	}

	async test(scope: 'this' | 'all'): Promise<SoundPlayback> {
		return await this.transport.request('Sound.TestOutput', { scope });
	}

	onPlayed(handler: PlayedHandler): () => void {
		this.#played.add(handler);
		return () => this.#played.delete(handler);
	}

	onStopped(handler: StoppedHandler): () => void {
		this.#stopped.add(handler);
		return () => this.#stopped.delete(handler);
	}

	onCatalogUpdated(handler: () => void): () => void {
		this.#catalog.add(handler);
		return () => this.#catalog.delete(handler);
	}

	onSettingsUpdated(handler: SettingsHandler): () => void {
		this.#settings.add(handler);
		return () => this.#settings.delete(handler);
	}
}
