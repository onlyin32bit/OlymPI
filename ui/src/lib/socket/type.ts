export const Operation = {
	Greeting: 0,
	Authentication: 1,
	Event: 2,
	Request: 3,
	Response: 4
} as const;

export type GreetingPayload = {
	protocol_version: number;
	client_id: string;
	server_time: number;
	authenticated: boolean;
	subject?: string;
	roles?: string[];
	debug?: boolean;
};

export type Envelope = {
	o: number;
	d: unknown;
};

export type RequestPayload = {
	requestId: string;
	requestType: string;
	requestData?: unknown;
};

export type ResponsePayload<T = unknown> = {
	requestId: string;
	requestType: string;
	requestStatus: {
		ok: boolean;
		code?: string;
		message?: string;
		details?: Record<string, unknown>;
		retryable?: boolean;
		version?: number;
	};
	responseData?: T;
};

export type EventPayload<T = unknown> = {
	eventType: string;
	eventData?: T;
};
