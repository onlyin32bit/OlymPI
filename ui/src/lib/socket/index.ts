import { browser } from '$app/environment';
import { ContestantSocket } from './contestant';
import { ControllerSocket } from './controller';
import { MatchSocket } from './match/index.svelte';
import { TimerSocket } from './timer/index.svelte';
import { SocketTransport } from './transport';
import { SoundSocket } from './sound/index.svelte';
import { PresenceSocket } from './presence/index.svelte';

/** Query value the server treats as an anonymous audience connection. */
export const PUBLIC_VIEW = 'public';

export function publicViewURL(url: string): string {
	const target = new URL(url);
	target.searchParams.set('view', PUBLIC_VIEW);
	return target.toString();
}

export class SocketApi {
	readonly #transport: SocketTransport;
	readonly match: MatchSocket;
	readonly controller: ControllerSocket;
	readonly timer: TimerSocket;
	readonly contestant: ContestantSocket;
	readonly sound: SoundSocket;
	readonly presence: PresenceSocket;
	#baseUrl = '';
	#publicViewHolds = 0;
	#sessionCheck?: () => void;

	constructor(transport = new SocketTransport()) {
		this.#transport = transport;
		this.match = new MatchSocket(transport);
		this.controller = new ControllerSocket(transport, this.match);
		this.timer = new TimerSocket(transport, this.match);
		this.contestant = new ContestantSocket(transport, this.match);
		this.sound = new SoundSocket(transport);
		this.presence = new PresenceSocket(transport);
		// Audience displays are anonymous by design: never chase the session there.
		transport.onSessionCheck = () => {
			if (!this.publicView) this.#sessionCheck?.();
		};
	}

	get connected(): boolean {
		return this.#transport.ready;
	}

	get serverNow(): number {
		return Date.now() + this.#transport.serverClockOffset;
	}

	/** True while some mounted view holds the connection in public (audience) mode. */
	get publicView(): boolean {
		return this.#publicViewHolds > 0;
	}

	onSessionCheck(handler: () => void): void {
		this.#sessionCheck = handler;
	}

	/** Runs after every greeting (first connect and each reconnect). */
	onOpen(handler: () => void): () => void {
		return this.#transport.onOpen(handler);
	}

	/** `Settings.Updated` (no payload): site settings changed; re-fetch them. */
	onSettingsUpdated(handler: () => void): () => void {
		return this.#transport.on('Settings.Updated', () => handler());
	}

	/**
	 * Switches this tab's connection to the public view (anonymous principal,
	 * `public` room, public-filtered state) even when an operator is logged in.
	 * Reconnects if already connected. The returned release restores the normal
	 * connection once no holder remains.
	 */
	usePublicView(): () => void {
		this.#publicViewHolds++;
		if (this.#publicViewHolds === 1) this.#applyView();
		let released = false;
		return () => {
			if (released) return;
			released = true;
			this.#publicViewHolds--;
			if (this.#publicViewHolds === 0) this.#applyView();
		};
	}

	disconnect(): void {
		this.#transport.disconnect();
	}

	connect(url: string): void {
		this.#baseUrl = url;
		this.#transport.connect(this.publicView ? publicViewURL(url) : url);
	}

	reconnect(): void {
		this.#transport.reconnect();
	}

	/** Reconnects immediately if the link is down (network back, tab visible). */
	wake(): void {
		this.#transport.wake();
	}

	#applyView(): void {
		if (!this.#baseUrl) return;
		this.#transport.retarget(this.publicView ? publicViewURL(this.#baseUrl) : this.#baseUrl);
	}
}

export const socket = new SocketApi();

if (browser) {
	const url = new URL('/ws', window.location.href);
	url.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
	socket.connect(url.toString());
	window.addEventListener('online', () => socket.wake());
	document.addEventListener('visibilitychange', () => {
		if (document.visibilityState === 'visible') socket.wake();
	});
}
