import { requestUrl } from 'obsidian';
import type { AISuggestion, ArticleDraft, ProposedCategory, ProposedTag, SyncSettings, Taxonomy } from './models';

type RawSuggestion = {
	description?: unknown;
	category?: unknown;
	proposedCategory?: { name?: unknown; reason?: unknown } | null;
	selectedTags?: unknown;
	proposedTags?: unknown;
};

function text(value: unknown): string {
	return typeof value === 'string' ? value.trim() : '';
}

function extractJSONObject(value: string): RawSuggestion {
	let cleaned = value.trim().replace(/^```(?:json)?\s*/i, '').replace(/\s*```$/, '');
	const start = cleaned.indexOf('{');
	const end = cleaned.lastIndexOf('}');
	if (start >= 0 && end >= start) cleaned = cleaned.slice(start, end + 1);
	try {
		return JSON.parse(cleaned) as RawSuggestion;
	} catch (error) {
		throw new Error(`AI 返回内容不是合法 JSON：${error instanceof Error ? error.message : String(error)}`);
	}
}

export function normalizeSuggestion(raw: RawSuggestion, taxonomy: Taxonomy, maxProposedTags: number): AISuggestion {
	const categoryMap = new Map(taxonomy.categories.filter((item) => item.enabled).map((item) => [item.name.toLowerCase(), item.name]));
	const tagMap = new Map<string, string>();
	for (const tag of taxonomy.tags.filter((item) => item.enabled && item.aiSelectable)) {
		tagMap.set(tag.name.toLowerCase(), tag.name);
		for (const alias of tag.aliases) tagMap.set(alias.trim().toLowerCase(), tag.name);
	}

	let category = text(raw.category);
	let proposedCategory: ProposedCategory | null = null;
	const canonicalCategory = categoryMap.get(category.toLowerCase());
	if (canonicalCategory) category = canonicalCategory;
	else if (category) {
		proposedCategory = { name: category, reason: 'AI 返回了分类库外的分类' };
		category = '';
	}
	if (raw.proposedCategory) {
		const name = text(raw.proposedCategory.name);
		const existing = categoryMap.get(name.toLowerCase());
		if (!category && existing) category = existing;
		else if (name && !existing) proposedCategory = { name, reason: text(raw.proposedCategory.reason) || '现有分类不适合这篇文章' };
	}

	const selectedTags: string[] = [];
	const proposedTags: ProposedTag[] = [];
	const selectedSeen = new Set<string>();
	const proposedSeen = new Set<string>();
	if (Array.isArray(raw.selectedTags)) {
		for (const item of raw.selectedTags) {
			const name = text(item);
			const canonical = tagMap.get(name.toLowerCase());
			if (canonical && !selectedSeen.has(canonical)) {
				selectedTags.push(canonical);
				selectedSeen.add(canonical);
			} else if (name && !canonical && !proposedSeen.has(name.toLowerCase())) {
				proposedTags.push({ name, reason: 'AI 返回了标签库外的标签' });
				proposedSeen.add(name.toLowerCase());
			}
		}
	}
	if (Array.isArray(raw.proposedTags)) {
		for (const item of raw.proposedTags) {
			if (!item || typeof item !== 'object') continue;
			const candidate = item as { name?: unknown; reason?: unknown };
			const name = text(candidate.name);
			const canonical = tagMap.get(name.toLowerCase());
			if (canonical && !selectedSeen.has(canonical)) {
				selectedTags.push(canonical);
				selectedSeen.add(canonical);
			} else if (name && !canonical && !proposedSeen.has(name.toLowerCase())) {
				proposedTags.push({ name, reason: text(candidate.reason) || '现有标签无法准确表达文章主题' });
				proposedSeen.add(name.toLowerCase());
			}
		}
	}

	return {
		description: text(raw.description),
		category,
		proposedCategory,
		selectedTags: selectedTags.slice(0, 6),
		proposedTags: proposedTags.slice(0, Math.max(0, Math.min(10, maxProposedTags))),
	};
}

export async function analyzeArticleLocally(settings: SyncSettings, taxonomy: Taxonomy, article: ArticleDraft): Promise<AISuggestion> {
	const baseUrl = settings.aiBaseUrl.trim().replace(/\/+$/, '');
	if (!baseUrl) throw new Error('请先配置 AI API 地址。');
	if (!settings.aiApiKey.trim()) throw new Error('请先配置 AI API Key。');
	if (!settings.aiModel.trim()) throw new Error('请先配置 AI 模型。');
	const endpoint = /\/chat\/completions$/i.test(baseUrl) ? baseUrl : `${baseUrl}/chat/completions`;
	const categories = taxonomy.categories.filter((item) => item.enabled).map((item) => `- ${item.name}：${item.description || '无说明'}`).join('\n') || '（无）';
	const tags = taxonomy.tags.filter((item) => item.enabled && item.aiSelectable).map((item) => `- ${item.name}：${item.description || '无说明'}`).join('\n') || '（无）';
	const prompt = `${settings.aiMetadataPrompt}\n\n${settings.aiTagRules}\n\n请只返回一个 JSON 对象，不要返回 Markdown。格式：\n{"description":"摘要","category":"只能是已有分类或空字符串","proposedCategory":{"name":"新分类","reason":"理由"},"selectedTags":["只能是已有标签"],"proposedTags":[{"name":"新标签","reason":"理由"}]}\n\n规则：\n1. category 只能从已有分类选择；没有合适分类时留空，并填写 proposedCategory，否则 proposedCategory 为 null。\n2. selectedTags 只能从已有标签选择；新标签只能放入 proposedTags。\n3. proposedTags 最多 ${settings.aiMaxProposedTags} 个。\n\n已有分类：\n${categories}\n\n已有标签：\n${tags}\n\n文件名：${article.filename}\n当前标题：${article.metadata.title || '无'}\n正文：\n${article.content.slice(0, 12000)}`;
	let response;
	try {
		response = await requestUrl({
			url: endpoint,
			method: 'POST',
			headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${settings.aiApiKey.trim()}` },
			body: JSON.stringify({
				model: settings.aiModel.trim(),
				messages: [
					{ role: 'system', content: settings.aiSystemPrompt },
					{ role: 'user', content: prompt },
				],
				max_tokens: 2048,
			}),
			throw: false,
		});
	} catch (error) {
		throw new Error(`无法连接 AI 服务：${error instanceof Error ? error.message : String(error)}`);
	}
	if (response.status < 200 || response.status >= 300) {
		let message = response.text || `HTTP ${response.status}`;
		try {
			const data = JSON.parse(response.text) as { error?: { message?: string }; message?: string };
			message = data.error?.message ?? data.message ?? message;
		} catch {
			// Keep plain-text provider errors.
		}
		throw new Error(`AI 服务返回 ${response.status}：${message}`);
	}
	const data = response.json as { choices?: Array<{ message?: { content?: unknown } }> };
	const content = data.choices?.[0]?.message?.content;
	if (typeof content !== 'string' || !content.trim()) throw new Error('AI 没有返回可用内容。');
	return normalizeSuggestion(extractJSONObject(content), taxonomy, settings.aiMaxProposedTags);
}
