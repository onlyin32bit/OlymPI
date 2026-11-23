import type { QuestionSet, QuestionSetData, QuestionSetRevision } from '$lib/types';
import type { HttpClient } from '../client';

export type QuestionSetInput = {
	name: string;
	category: string;
	data: QuestionSetData;
	version?: number;
};

export class QuestionSetsApi {
	constructor(private readonly http: HttpClient) {}

	async list(query = ''): Promise<QuestionSet[]> {
		return (
			(
				await this.http.request<{ question_sets: QuestionSet[] | null }>(
					`/question-sets?q=${encodeURIComponent(query)}`
				)
			).question_sets ?? []
		);
	}

	async get(id: string): Promise<{ questionSet: QuestionSet; revision: QuestionSetRevision }> {
		const response = await this.http.request<{
			question_set: QuestionSet;
			revision: QuestionSetRevision;
		}>(`/question-sets/${id}`);
		return { questionSet: response.question_set, revision: response.revision };
	}

	async revision(id: string): Promise<QuestionSetRevision> {
		return (
			await this.http.request<{ revision: QuestionSetRevision }>(`/question-set-revisions/${id}`)
		).revision;
	}

	async create(input: QuestionSetInput): Promise<QuestionSet> {
		return (
			await this.http.request<{ question_set: QuestionSet }>('/question-sets', json('POST', input))
		).question_set;
	}

	async update(id: string, input: QuestionSetInput): Promise<QuestionSet> {
		return (
			await this.http.request<{ question_set: QuestionSet }>(
				`/question-sets/${id}`,
				json('PUT', input)
			)
		).question_set;
	}

	async archive(questionSet: QuestionSet): Promise<void> {
		await this.http.request(`/question-sets/${questionSet.id}?version=${questionSet.version}`, {
			method: 'DELETE'
		});
	}

	async exportArchive(id: string): Promise<{ blob: Blob; filename: string }> {
		const response = await this.http.raw(`/question-sets/${id}/export`, { timeoutMs: 60_000 });
		const disposition = response.headers.get('content-disposition') ?? '';
		const filename = disposition.match(/filename="?([^";]+)"?/i)?.[1] ?? `question-set-${id}.zip`;
		return { blob: await response.blob(), filename };
	}

	async importArchive(file: File): Promise<QuestionSet> {
		const form = new FormData();
		form.append('file', file);
		return (
			await this.http.request<{ question_set: QuestionSet }>('/question-sets/import', {
				method: 'POST',
				body: form,
				timeoutMs: 120_000
			})
		).question_set;
	}
}

function json(method: string, body: unknown): RequestInit {
	return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
}
