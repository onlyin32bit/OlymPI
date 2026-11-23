import type { SocketTransport } from '../transport';

/** Contestant slots with a connected station, as last reported by the server to operators. */
export class PresenceSocket {
	#slots = $state<number[]>([]);

	constructor(transport: SocketTransport) {
		transport.on('Contestant.Presence', (presence) => {
			this.#slots = presence.slots;
		});
	}

	isOnline(slot: number): boolean {
		return this.#slots.includes(slot);
	}
}
