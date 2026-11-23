import {
	Operation,
	type Envelope,
	type EventPayload,
	type GreetingPayload,
	type ResponsePayload
} from './type';
import type { OpsocketEvents, OpsocketOperations } from './opsocket';
import { uuid } from '../uuid';

let runtimeDebug = import.meta.env.DEV;

/** Application heartbeat op (both directions); browsers cannot observe WS pings. */
const HEARTBEAT_OPERATION = 5;
/** Server heartbeats every 25 s; silence this long means the link is dead. */
export const HEARTBEAT_TIMEOUT_MS = 35_000;
/** Reconnect delay before jitter is capped so total delay stays within 2 s. */
export const RECONNECT_MAX_DELAY_MS = 1_750;
export const RECONNECT_JITTER_MS = 250;
const RECONNECT_BASE_DELAY_MS = 250;
/** Round-trip clock probes after each greeting and then periodically. */
export const CLOCK_PROBE_INTERVAL_MS = 30_000;
const CLOCK_SAMPLES = 5;

type HeartbeatPayload = { server_time?: number; client_time?: number };
type ClockSample = { offset: number; rtt: number };

type EventHandler = (data: unknown) => void;
type OpenHandler = () => void;
type PendingRequest = {
	operation: string;
	startedAt: number;
	resolve: (value: unknown) => void;
	reject: (reason: Error) => void;
	timeout: ReturnType<typeof setTimeout>;
	cleanup: () => void;
};

export type SocketRequestOptions = { timeoutMs?: number; signal?: AbortSignal };

export class OpsocketError extends Error {
	constructor(
		public readonly code: string,
		message: string,
		public readonly details?: Record<string, unknown>,
		public readonly retryable = false,
		public readonly version?: number
	) {
		super(message);
		this.name = 'OpsocketError';
	}
}

export class SocketTransport {
	#socket: WebSocket | null = null;
	#url = '';
	#reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	#reconnectAttempt = 0;
	#manuallyClosed = false;
	#ready = false;
	#lastReceivedAt = 0;
	#watchdogTimer: ReturnType<typeof setTimeout> | null = null;
	#probeTimer: ReturnType<typeof setTimeout> | null = null;
	#clockSamples: ClockSample[] = [];
	#events = new Map<string, Set<EventHandler>>();
	#openHandlers = new Set<OpenHandler>();
	#pending = new Map<string, PendingRequest>();

	serverClockOffset = 0;
	onSessionCheck?: () => void;

	/** True once the greeting arrived on an open socket. */
	get ready(): boolean {
		return this.#ready && this.#socket?.readyState === WebSocket.OPEN;
	}

	/** The URL the next connection uses. */
	get url(): string {
		return this.#url;
	}

	connect(url: string): void {
		this.#url = url;
		this.#manuallyClosed = false;
		if (
			this.#socket?.readyState === WebSocket.OPEN ||
			this.#socket?.readyState === WebSocket.CONNECTING
		)
			return;
		this.#clearReconnectTimer();
		const socket = new WebSocket(url);
		this.#socket = socket;
		devLog('connect', { url });
		// A connection that never greets is as dead as one that went silent.
		this.#lastReceivedAt = Date.now();
		this.#armWatchdog(socket, HEARTBEAT_TIMEOUT_MS);
		socket.addEventListener('open', () => devLog('open'));
		socket.addEventListener('message', (event) => {
			if (this.#socket !== socket || this.#manuallyClosed) return;
			this.#lastReceivedAt = Date.now();
			this.#handleMessage(event);
		});
		socket.addEventListener('close', (event) => this.#handleClose(socket, event));
		socket.addEventListener('error', (event) => {
			devError('transport error', event);
			socket.close();
		});
	}

	disconnect(): void {
		devLog('disconnect', { manual: true });
		this.#manuallyClosed = true;
		this.#clearReconnectTimer();
		this.#clearLinkTimers();
		this.#ready = false;
		this.#socket?.close(1000, 'client disconnect');
		this.#socket = null;
		this.#rejectPending(
			new OpsocketError('DISCONNECTED', 'WebSocket disconnected', undefined, true)
		);
	}

	reconnect(): void {
		devLog('reconnect requested');
		this.disconnect();
		this.#manuallyClosed = false;
		this.#reconnectAttempt = 0;
		if (this.#url) this.connect(this.#url);
	}

	/**
	 * Changes the connection URL. An active (or reconnecting) transport
	 * reconnects immediately; a disconnected one only remembers it.
	 */
	retarget(url: string): void {
		if (url === this.#url) return;
		const active = this.#url !== '' && !this.#manuallyClosed;
		this.#url = url;
		if (active) this.reconnect();
	}

	/** Reconnects now if the link is down and not deliberately closed. */
	wake(): void {
		if (this.#manuallyClosed || !this.#url || this.#socket) return;
		devLog('wake');
		this.connect(this.#url);
	}

	request<K extends keyof OpsocketOperations>(
		operation: K,
		payload: OpsocketOperations[K]['request'],
		options: SocketRequestOptions = {}
	): Promise<OpsocketOperations[K]['response']> {
		if (this.#socket?.readyState !== WebSocket.OPEN) {
			return Promise.reject(
				new OpsocketError('NOT_CONNECTED', 'WebSocket is not connected', undefined, true)
			);
		}
		if (options.signal?.aborted)
			return Promise.reject(new DOMException('Request aborted', 'AbortError'));

		const requestId = uuid();
		devLog(`→ ${String(operation)}`, { requestId, payload });
		return new Promise((resolve, reject) => {
			const abort = () => {
				devLog(`× ${String(operation)} aborted`, { requestId });
				this.#finishPending(requestId);
				reject(new DOMException('Request aborted', 'AbortError'));
			};
			options.signal?.addEventListener('abort', abort, { once: true });
			const timeout = setTimeout(() => {
				devError(`× ${String(operation)} timed out`, { requestId });
				this.#finishPending(requestId);
				reject(new OpsocketError('TIMEOUT', `${operation} timed out`, undefined, true));
			}, options.timeoutMs ?? 10_000);
			this.#pending.set(requestId, {
				operation: String(operation),
				startedAt: performance.now(),
				resolve: (value) => resolve(value as OpsocketOperations[K]['response']),
				reject,
				timeout,
				cleanup: () => options.signal?.removeEventListener('abort', abort)
			});
			this.#socket?.send(
				JSON.stringify({
					o: Operation.Request,
					d: { requestId, requestType: operation, requestData: payload }
				})
			);
		});
	}

	on<K extends keyof OpsocketEvents>(
		event: K,
		handler: (data: OpsocketEvents[K]) => void
	): () => void {
		let handlers = this.#events.get(event);
		if (!handlers) {
			handlers = new Set();
			this.#events.set(event, handlers);
		}
		handlers.add(handler as EventHandler);
		return () => {
			handlers?.delete(handler as EventHandler);
			if (!handlers?.size) this.#events.delete(event);
		};
	}

	onOpen(handler: OpenHandler): () => void {
		this.#openHandlers.add(handler);
		if (this.#ready) queueMicrotask(handler);
		return () => this.#openHandlers.delete(handler);
	}

	#handleMessage(event: MessageEvent): void {
		let envelope: Envelope;
		try {
			envelope = JSON.parse(String(event.data)) as Envelope;
		} catch (error) {
			devError('invalid message', { error, data: event.data });
			return;
		}

		if (envelope.o === Operation.Greeting) {
			const greeting = envelope.d as GreetingPayload;
			runtimeDebug ||= greeting.debug === true;
			if (greeting.protocol_version !== 1) {
				devError('unsupported protocol', greeting);
				this.disconnect();
				return;
			}
			// One-way estimate first; round-trip probes refine it. The server may
			// have restarted, so earlier samples no longer apply.
			this.serverClockOffset = greeting.server_time - Date.now();
			this.#clockSamples = [];
			this.#reconnectAttempt = 0;
			this.#ready = true;
			this.#probeClock();
			devLog('ready', {
				clientId: greeting.client_id,
				authenticated: greeting.authenticated,
				roles: greeting.roles ?? [],
				clockOffsetMs: this.serverClockOffset
			});
			if (!greeting.authenticated) this.onSessionCheck?.();
			for (const handler of this.#openHandlers) handler();
			return;
		}

		if (envelope.o === HEARTBEAT_OPERATION) {
			this.#handleHeartbeat((envelope.d ?? {}) as HeartbeatPayload);
			return;
		}

		if (envelope.o === Operation.Event) {
			const payload = envelope.d as EventPayload;
			devLog(`← event ${payload.eventType}`, payload.eventData);
			for (const handler of this.#events.get(payload.eventType) ?? []) handler(payload.eventData);
			return;
		}

		if (envelope.o === Operation.Response) {
			const payload = envelope.d as ResponsePayload;
			const pending = this.#pending.get(payload.requestId);
			if (!pending) {
				devLog(`← response ${payload.requestType} (no pending request)`, payload);
				return;
			}
			const durationMs = Math.round(performance.now() - pending.startedAt);
			this.#finishPending(payload.requestId);
			if (payload.requestStatus?.ok) {
				devLog(`← ${pending.operation} ${durationMs}ms`, {
					requestId: payload.requestId,
					data: payload.responseData
				});
				pending.resolve(payload.responseData);
			} else {
				devError(`← ${pending.operation} failed ${durationMs}ms`, {
					requestId: payload.requestId,
					status: payload.requestStatus
				});
				if (payload.requestStatus?.code === 'UNAUTHORIZED') this.onSessionCheck?.();
				pending.reject(
					new OpsocketError(
						payload.requestStatus?.code || 'FAILED',
						payload.requestStatus?.message || 'Request failed',
						payload.requestStatus?.details,
						payload.requestStatus?.retryable,
						payload.requestStatus?.version
					)
				);
			}
		}
	}

	#handleClose(socket: WebSocket, event: CloseEvent): void {
		if (this.#socket !== socket) return;
		devLog('close', { code: event.code, reason: event.reason, clean: event.wasClean });
		this.#dropSocket();
		if (this.#manuallyClosed) return;
		this.onSessionCheck?.();
		if (this.#manuallyClosed) return;
		this.#scheduleReconnect();
	}

	/** Detaches the current socket; its late events are ignored afterwards. */
	#dropSocket(): void {
		this.#socket = null;
		this.#ready = false;
		this.#clearLinkTimers();
		this.#rejectPending(
			new OpsocketError('DISCONNECTED', 'WebSocket disconnected', undefined, true)
		);
	}

	#scheduleReconnect(): void {
		this.#clearReconnectTimer();
		this.#reconnectAttempt++;
		const delay = reconnectDelay(this.#reconnectAttempt);
		devLog('reconnect scheduled', { attempt: this.#reconnectAttempt, delayMs: delay });
		this.#reconnectTimer = setTimeout(() => {
			this.#reconnectTimer = null;
			this.connect(this.#url);
		}, delay);
	}

	#clearReconnectTimer(): void {
		if (this.#reconnectTimer) clearTimeout(this.#reconnectTimer);
		this.#reconnectTimer = null;
	}

	#clearLinkTimers(): void {
		if (this.#watchdogTimer) clearTimeout(this.#watchdogTimer);
		if (this.#probeTimer) clearTimeout(this.#probeTimer);
		this.#watchdogTimer = null;
		this.#probeTimer = null;
	}

	/** Forces a reconnect when nothing (not even a heartbeat) arrives in time. */
	#armWatchdog(socket: WebSocket, delay: number): void {
		if (this.#watchdogTimer) clearTimeout(this.#watchdogTimer);
		this.#watchdogTimer = setTimeout(() => {
			this.#watchdogTimer = null;
			if (this.#socket !== socket) return;
			const silentFor = Date.now() - this.#lastReceivedAt;
			if (silentFor < HEARTBEAT_TIMEOUT_MS) {
				this.#armWatchdog(socket, HEARTBEAT_TIMEOUT_MS - silentFor);
				return;
			}
			devError('heartbeat timeout; reconnecting', { silentForMs: silentFor });
			// Do not wait for the close handshake of a half-dead link.
			this.#dropSocket();
			try {
				socket.close(4000, 'heartbeat timeout');
			} catch {
				// Already closing.
			}
			if (!this.#manuallyClosed) this.#scheduleReconnect();
		}, delay);
	}

	#probeClock(): void {
		if (this.#probeTimer) clearTimeout(this.#probeTimer);
		this.#probeTimer = null;
		const socket = this.#socket;
		if (!socket || socket.readyState !== WebSocket.OPEN) return;
		try {
			socket.send(JSON.stringify({ o: HEARTBEAT_OPERATION, d: { client_time: Date.now() } }));
		} catch (error) {
			devError('clock probe failed', error);
		}
		this.#probeTimer = setTimeout(() => this.#probeClock(), CLOCK_PROBE_INTERVAL_MS);
	}

	#handleHeartbeat(beat: HeartbeatPayload): void {
		if (!beat.client_time || !beat.server_time) return;
		const now = Date.now();
		const rtt = now - beat.client_time;
		if (rtt < 0 || rtt > 10_000) return;
		this.#clockSamples = [
			...this.#clockSamples,
			{ offset: beat.server_time + rtt / 2 - now, rtt }
		].slice(-CLOCK_SAMPLES);
		// The fastest round trip has the least asymmetric-delay error.
		const best = this.#clockSamples.reduce((a, b) => (b.rtt < a.rtt ? b : a));
		this.serverClockOffset = Math.round(best.offset);
		devLog('clock sync', { rttMs: rtt, clockOffsetMs: this.serverClockOffset });
	}

	#finishPending(id: string): void {
		const pending = this.#pending.get(id);
		if (!pending) return;
		clearTimeout(pending.timeout);
		pending.cleanup();
		this.#pending.delete(id);
	}

	#rejectPending(error: Error): void {
		for (const [id, request] of this.#pending) {
			this.#finishPending(id);
			request.reject(error);
		}
	}
}

/** Exponential backoff from 250 ms, capped so delay + jitter never exceeds 2 s. */
export function reconnectDelay(attempt: number, random = Math.random): number {
	const base = Math.min(
		RECONNECT_MAX_DELAY_MS,
		RECONNECT_BASE_DELAY_MS * 2 ** Math.max(0, Math.min(attempt - 1, 8))
	);
	return base + Math.floor(random() * RECONNECT_JITTER_MS);
}

function devLog(message: string, detail?: unknown): void {
	if (!runtimeDebug) return;
	if (detail === undefined) console.info(`[opsocket] ${message}`);
	else console.info(`[opsocket] ${message}`, detail);
}

function devError(message: string, detail?: unknown): void {
	if (!runtimeDebug) return;
	if (detail === undefined) console.error(`[opsocket] ${message}`);
	else console.error(`[opsocket] ${message}`, detail);
}
