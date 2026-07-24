import { requestUrl } from 'obsidian';
import type { ArticleDraft, LocalFile, SyncJob, SyncSettings, Taxonomy, TaxonomyUsage } from './models';

export class ApiError extends Error {
	constructor(public readonly status: number, message: string) {
		super(message);
		this.name = 'ApiError';
	}
}

export function describeApiError(error: unknown): string {
	if (error instanceof ApiError) {
		if (error.status === 401) return '认证失败（401）：请检查 Obsidian 设置中的 Webhook Secret。';
		if (error.status === 404) return '接口不存在（404）：请检查同步服务地址，或确认服务器已更新。';
		if (error.status === 503) return `服务不可用（503）：${error.message}`;
		return `服务器返回 ${error.status}：${error.message}`;
	}
	const message = error instanceof Error ? error.message : String(error);
	if (/network|fetch|connect|socket|dns|timeout|certificate/i.test(message)) {
		return `无法连接服务器：${message}`;
	}
	return message || '未知错误';
}

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
		let response;
		try {
			response = await requestUrl({
				url: `${this.base}${path}`,
				method,
				headers: {
					'Content-Type': 'application/json',
					Authorization: `Bearer ${this.secret}`,
				},
				body: body === undefined ? undefined : JSON.stringify(body),
				throw: false,
			});
		} catch (error) {
			throw new Error(`请求 ${this.base}${path} 失败：${error instanceof Error ? error.message : String(error)}`);
		}
		if (response.status < 200 || response.status >= 300) {
			let message = response.text || `HTTP ${response.status}`;
			try {
				message = (JSON.parse(response.text) as { message?: string }).message ?? message;
			} catch {
				// Keep the plain-text response from proxies and non-JSON servers.
			}
			throw new ApiError(response.status, message);
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
	getTaxonomyUsage() { return this.request<TaxonomyUsage>('/taxonomy/usage'); }
	saveTaxonomy(value: Taxonomy) { return this.request<Taxonomy>('/taxonomy', 'PUT', value); }
	testConnection() { return this.getTaxonomy(); }
	publish(jobId: string, articles: ArticleDraft[]) {
		return this.request<{ commitSha: string }>(`/jobs/${jobId}/publish`, 'POST', {
			articles: articles.map((article) => ({ id: article.id, revision: article.revision, hash: article.currentHash })),
			includeTaxonomy: true,
		});
	}
}
