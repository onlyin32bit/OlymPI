export type Fetch = typeof fetch;

type RequestOptions = RequestInit & {
	fetch?: Fetch;
	timeoutMs?: number;
};

type ErrorBody = {
	code?: string;
	message?: string;
	details?: Record<string, unknown>;
	retryable?: boolean;
	version?: number;
};

export class ApiError extends Error {
	constructor(
		public readonly status: number,
		message: string,
		public readonly code = 'HTTP_ERROR',
		public readonly details?: Record<string, unknown>,
		public readonly retryable = false,
		public readonly version?: number
	) {
		super(message);
		this.name = 'ApiError';
	}
}

export class HttpClient {
	onUnauthorized?: () => void;

	constructor(private readonly baseURL = '/api') {}

	async raw(path: string, options: RequestOptions = {}): Promise<Response> {
		const { fetch: fetcher = fetch, timeoutMs = 15_000, signal, ...init } = options;
		if (signal?.aborted) {
			throw signal.reason ?? new DOMException('The operation was aborted', 'AbortError');
		}
		const controller = new AbortController();
		const abort = () => controller.abort(signal?.reason);
		const timeout = setTimeout(() => controller.abort(new DOMException('The operation timed out', 'TimeoutError')), timeoutMs);
		signal?.addEventListener('abort', abort, { once: true });

		try {
			const response = await fetcher(`${this.baseURL}${path}`, {
				credentials: 'include',
				...init,
				signal: controller.signal
			});
			if (!response.ok) {
				const text = await response.text();
				const body = parseBody(text);
				if (response.status === 401) this.onUnauthorized?.();
				const error = isErrorBody(body) ? body : undefined;
				throw new ApiError(
					response.status,
					error?.message || text || response.statusText || `Request failed (${response.status})`,
					error?.code,
					error?.details,
					error?.retryable,
					error?.version
				);
			}
			return response;
		} finally {
			clearTimeout(timeout);
			signal?.removeEventListener('abort', abort);
		}
	}


	async request<T = void>(path: string, options: RequestOptions = {}): Promise<T> {
		const response = await this.raw(path, options);
		const text = await response.text();
		const body = parseBody(text);
		return body as T;
	}
}

function parseBody(text: string): unknown {
	if (!text) return undefined;
	try {
		return JSON.parse(text);
	} catch {
		return text;
	}
}

function isErrorBody(value: unknown): value is ErrorBody {
	return typeof value === 'object' && value !== null;
}
