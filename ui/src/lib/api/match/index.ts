import type { Match, MatchEvent, MatchInput } from '$lib/types';
import type { HttpClient } from '../client';

export class MatchesApi {
	constructor(private readonly http: HttpClient) {}

	async list(query = ''): Promise<Match[]> {
		return (await this.http.request<{ matches: Match[] | null }>(`/matches?q=${encodeURIComponent(query)}`)).matches ?? [];
	}

	async get(id: string): Promise<Match> {
		return (await this.http.request<{ match: Match }>(`/matches/${id}`)).match;
	}

	async create(input: MatchInput): Promise<Match> {
		return (await this.http.request<{ match: Match }>('/matches', json('POST', input))).match;
	}

	async update(id: string, input: MatchInput): Promise<Match> {
		return (await this.http.request<{ match: Match }>(`/matches/${id}`, json('PUT', input))).match;
	}

	async archive(match: Match): Promise<void> {
		await this.http.request(`/matches/${match.id}?version=${match.version}`, { method: 'DELETE' });
	}

	async duplicate(id: string): Promise<Match> {
		return (await this.http.request<{ match: Match }>(`/matches/${id}/duplicate`, { method: 'POST' })).match;
	}

	async events(id: string, options: { afterSequence?: number; signal?: AbortSignal } = {}): Promise<MatchEvent[]> {
		const query = options.afterSequence === undefined ? '' : `?after_sequence=${options.afterSequence}`;
		return (await this.http.request<{ events: MatchEvent[] | null }>(
			`/matches/${encodeURIComponent(id)}/events${query}`, { signal: options.signal }
		)).events ?? [];
	}

	async amendResult(id: string, result: unknown, reason: string): Promise<void> {
		await this.http.request(`/matches/${id}/amendments`, json('POST', { result, reason }));
	}
}

function json(method: string, body: unknown): RequestInit {
	return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
}
