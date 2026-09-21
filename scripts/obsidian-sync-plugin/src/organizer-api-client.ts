import { requestUrl } from 'obsidian';
import type { SyncSettings } from './models';
import type { OrganizerCapture, OrganizerCapturesResponse, OrganizerItem, OrganizerItemsResponse } from './organizer-models';

export class OrganizerApiError extends Error {
	constructor(public readonly status: number, message: string) {
		super(message);
		this.name = 'OrganizerApiError';
	}
}

export function describeOrganizerError(error: unknown): string {
	if (error instanceof OrganizerApiError) {
		if (error.status === 401) return '认证失败（401）：请检查事项服务设备令牌。';
		if (error.status === 404) return '事项接口不存在（404）：请检查服务地址。';
		if (error.status === 409) return '事项已在其他设备修改，请刷新后重试。';
		return `事项服务返回 ${error.status}：${error.message}`;
	}
	return error instanceof Error ? error.message : String(error);
}

function normalizeEndpoint(endpoint: string): string {
	const raw = endpoint.trim().replace(/\/+$/, '');
	if (!raw) return '';
	if (/\/api\/organizer\/v1$/i.test(raw)) return raw;
	return `${raw}/api/organizer/v1`;
}

export class OrganizerApiClient {
	private readonly base: string;
	private readonly token: string;

	constructor(settings: SyncSettings) {
		this.base = normalizeEndpoint(settings.organizerEndpoint);
		this.token = settings.organizerToken.trim();
	}

	private async request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
		if (!this.base) throw new Error('请先填写事项服务地址。');
		if (!this.token) throw new Error('请先填写事项服务设备令牌。');
		let response;
		try {
			response = await requestUrl({
				url: `${this.base}${path}`,
				method,
				headers: {
					'Content-Type': 'application/json',
					Authorization: `Bearer ${this.token}`,
				},
				body: body === undefined ? undefined : JSON.stringify(body),
				throw: false,
			});
		} catch (error) {
			throw new Error(`无法连接事项服务：${error instanceof Error ? error.message : String(error)}`);
		}
		if (response.status < 200 || response.status >= 300) {
			let message = response.text || `HTTP ${response.status}`;
			try {
				message = (JSON.parse(response.text) as { message?: string }).message || message;
			} catch {
				// Keep a non-JSON proxy response.
			}
			throw new OrganizerApiError(response.status, message);
		}
		return response.json as T;
	}

	testConnection() { return this.request<{ authenticated: boolean; timezone: string }>('/session'); }
	listItems() { return this.request<OrganizerItemsResponse>('/items?limit=1000'); }
	listCaptures() { return this.request<OrganizerCapturesResponse>('/captures?limit=100'); }
	createCapture(rawText: string) {
		return this.request<OrganizerCapture>('/captures', 'POST', { rawText, sourceType: 'obsidian' });
	}
	parseCapture(id: string) { return this.request<{ items: unknown[] }>(`/captures/${encodeURIComponent(id)}/parse`, 'POST', {}); }
	completeItem(id: string) { return this.request<OrganizerItem>(`/items/${encodeURIComponent(id)}/complete`, 'POST', {}); }
	snoozeItem(id: string, minutes = 10) { return this.request<OrganizerItem>(`/items/${encodeURIComponent(id)}/snooze`, 'POST', { minutes }); }
}

