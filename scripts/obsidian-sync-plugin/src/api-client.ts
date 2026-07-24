import { requestUrl } from 'obsidian';
import type { ArticleDraft, LocalFile, SyncJob, SyncSettings, Taxonomy } from './models';

function apiBase(endpoint: string): string {
	const raw = endpoint.trim();
	try {
		const url = new URL(raw);
		if (url.pathname.endsWith('/api/sync')) url.pathname = '/api/v1';
		else if (!url.pathname.includes('/api/v1')) url.pathname = '/api/v1';
		url.pathname = url.pathname.replace(/\/+$/, '');
		return url.toString().replace(/\/$/, '');
	} catch {
		return raw.replace(/\/api\/sync\/?$/, '/api/v1').replace(/\/$/, '');
	}
}

export class ApiClient {
	private readonly base: string;
	private readonly secret: string;

	constructor(settings: SyncSettings) {
		this.base = apiBase(settings.syncEndpoint);
		this.secret = settings.webhookSecret;
	}

	private async request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
		const response = await requestUrl({
			url: `${this.base}${path}`,
			method,
			headers: {
				'Content-Type': 'application/json',
				Authorization: `Bearer ${this.secret}`,
			},
			body: body === undefined ? undefined : JSON.stringify(body),
			throw: false,
		});
		if (response.status < 200 || response.status >= 300) {
			const message = (response.json as { message?: string } | undefined)?.message ?? response.text ?? `HTTP ${response.status}`;
			throw new Error(message);
		}
		return response.json as T;
	}

	createJob(localManifest: LocalFile[]) { return this.request<SyncJob>('/jobs', 'POST', { clientId: 'obsidian', localManifest }); }
	getJob(id: string) { return this.request<SyncJob>(`/jobs/${id}`); }
	updateArticle(jobId: string, article: ArticleDraft) {
		return this.request<ArticleDraft>(`/jobs/${jobId}/articles/${article.id}`, 'PUT', {
			revision: article.revision,
			metadata: article.metadata,
			content: article.content,
			status: article.status,
		});
	}
	getTaxonomy() { return this.request<Taxonomy>('/taxonomy'); }
	saveTaxonomy(value: Taxonomy) { return this.request<Taxonomy>('/taxonomy', 'PUT', value); }
	publish(jobId: string, articles: ArticleDraft[]) {
		return this.request<{ commitSha: string }>(`/jobs/${jobId}/publish`, 'POST', {
			articles: articles.map((article) => ({ id: article.id, revision: article.revision, hash: article.currentHash })),
			includeTaxonomy: true,
		});
	}
}
