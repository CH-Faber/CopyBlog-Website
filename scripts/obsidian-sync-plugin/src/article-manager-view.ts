import { ItemView, MarkdownRenderer, Notice, TFile, WorkspaceLeaf, normalizePath, stringifyYaml } from 'obsidian';
import type SyncPlugin from '../main';
import { ApiError, describeApiError } from './api-client';
import { analyzeArticleLocally } from './local-ai';
import type { ArticleDraft, Category, LocalFile, ManagedTag, ProposedCategory, ProposedTag, SyncJob, Taxonomy } from './models';

export const ARTICLE_MANAGER_VIEW = 'flash-thought-article-manager';

async function sha256(value: string): Promise<string> {
	const bytes = new TextEncoder().encode(value);
	const digest = await crypto.subtle.digest('SHA-256', bytes);
	return Array.from(new Uint8Array(digest)).map((item) => item.toString(16).padStart(2, '0')).join('');
}

export class ArticleManagerView extends ItemView {
	private readonly plugin: SyncPlugin;
	private job: SyncJob | null = null;
	private taxonomy: Taxonomy | null = null;
	private taxonomyState: 'loading' | 'loaded' | 'error' = 'loading';
	private taxonomyError = '';
	private taxonomyUsage: Record<string, number> = {};
	private categoryUsage: Record<string, number> = {};
	private taxonomyUsageLoaded = false;
	private taxonomyTab: 'categories' | 'tags' = 'tags';
	private tagSearch = '';
	private tagFilter: 'all' | 'enabled' | 'disabled' = 'all';
	private activeArticleId = '';
	private selected = new Set<string>();
	private dirtyArticles = new Set<string>();
	private showTaxonomy = false;
	private pollTimer: number | null = null;
	private aiRunning = false;
	private aiProgress = '';

	constructor(leaf: WorkspaceLeaf, plugin: SyncPlugin) {
		super(leaf);
		this.plugin = plugin;
	}

	getViewType() { return ARTICLE_MANAGER_VIEW; }
	getDisplayText() { return '一个闪念 · 内容管理'; }
	getIcon() { return 'layout-dashboard'; }

	async onOpen() {
		this.contentEl.addClass('vermilion-manager');
		this.render();
		await Promise.all([
			this.loadTaxonomy(),
			(async () => {
				if (!this.plugin.settings.activeJobId) return;
				try {
				this.job = await this.plugin.api.getJob(this.plugin.settings.activeJobId);
				this.applyCachedSuggestions();
				this.activeArticleId = this.job.articles?.[0]?.id ?? '';
				} catch (error) {
					new Notice(`加载文章任务失败：${describeApiError(error)}`);
				}
			})(),
		]);
		this.render();
		this.schedulePoll();
	}

	private async loadTaxonomy(showNotice = false) {
		this.taxonomyState = 'loading';
		this.taxonomyError = '';
		this.render();
		try {
			this.taxonomy = await this.plugin.api.getTaxonomy();
			this.taxonomyState = 'loaded';
			try {
				const usage = await this.plugin.api.getTaxonomyUsage();
				this.taxonomyUsage = usage.tags ?? {};
				this.categoryUsage = usage.categories ?? {};
				this.taxonomyUsageLoaded = true;
			} catch (error) {
				// Older servers do not expose usage counts. Tag management remains usable.
				this.taxonomyUsage = {};
				this.taxonomyUsageLoaded = false;
				if (!(error instanceof ApiError && error.status === 404)) console.warn('Failed to load taxonomy usage', error);
			}
			if (showNotice) new Notice(`标签库加载成功：${this.taxonomy.tags.length} 个标签。`);
		} catch (error) {
			this.taxonomy = null;
			this.taxonomyState = 'error';
			this.taxonomyError = describeApiError(error);
			if (showNotice) new Notice(`标签库加载失败：${this.taxonomyError}`);
		} finally {
			this.render();
		}
	}

	async onClose() {
		if (this.pollTimer !== null) window.clearTimeout(this.pollTimer);
	}

	async prepareSync() {
		try {
			const manifest = await this.collectLocalManifest();
			this.job = await this.plugin.api.createJob(manifest);
			this.plugin.settings.activeJobId = this.job.id;
			await this.plugin.saveSettings();
			this.activeArticleId = '';
			this.selected.clear();
			this.dirtyArticles.clear();
			this.plugin.settings.aiSuggestionJobId = this.job.id;
			this.plugin.settings.aiSuggestions = {};
			await this.plugin.saveSettings();
			this.showTaxonomy = false;
			this.render();
			this.schedulePoll(true);
			new Notice('处理任务已创建，不会自动发布。');
		} catch (error) {
			new Notice(`创建任务失败：${(error as Error).message}`);
		}
	}

	private async refreshJob() {
		if (!this.plugin.settings.activeJobId) return;
		if (this.dirtyArticles.size > 0) {
			new Notice('存在未保存的文章，已跳过刷新。');
			return;
		}
		try {
			this.job = await this.plugin.api.getJob(this.plugin.settings.activeJobId);
			this.applyCachedSuggestions();
			if (!this.activeArticleId) this.activeArticleId = this.job.articles?.[0]?.id ?? '';
			this.render();
		} catch (error) {
			new Notice(`刷新任务失败：${(error as Error).message}`);
		}
	}

	private schedulePoll(force = false) {
		if (this.pollTimer !== null) window.clearTimeout(this.pollTimer);
		const running = this.job && ['queued', 'syncing', 'analyzing', 'publishing'].includes(this.job.status);
		if (!force && !running) return;
		this.pollTimer = window.setTimeout(async () => {
			await this.refreshJob();
			this.schedulePoll();
		}, 1500);
	}

	private render() {
		const root = this.contentEl;
		root.empty();
		const toolbar = root.createDiv({ cls: 'vermilion-toolbar' });
		toolbar.createEl('button', { text: '获取并处理文章', cls: 'mod-cta' }).onclick = () => void this.prepareSync();
		const refreshButton = toolbar.createEl('button', { text: '刷新状态' });
		refreshButton.title = '仅重新读取服务器任务状态，不会运行 AI 分析';
		refreshButton.onclick = () => void this.refreshJob();
		const aiButton = toolbar.createEl('button', { text: this.aiRunning ? 'AI 分析中…' : 'AI 分析待处理' });
		aiButton.disabled = this.aiRunning || !this.job?.articles?.length;
		aiButton.onclick = () => void this.analyzePendingArticles();
		toolbar.createEl('button', { text: this.showTaxonomy ? '返回文章' : '分类与标签' }).onclick = () => {
			this.showTaxonomy = !this.showTaxonomy;
			this.render();
		};
		const publishButton = toolbar.createEl('button', { text: `发布所选 (${this.selected.size})`, cls: 'mod-cta' });
		publishButton.disabled = this.selected.size === 0 || !this.job || this.job.status === 'publishing';
		publishButton.onclick = () => void this.publishSelected();

		if (this.job) {
			const status = root.createDiv({ cls: 'vermilion-job-status' });
			status.createSpan({ text: `${this.job.message} · ${this.job.progress}%` });
			const progress = status.createEl('progress');
			progress.max = 100;
			progress.value = this.job.progress;
			if (this.job.publishedSha) status.createEl('code', { text: this.job.publishedSha.slice(0, 12) });
		}
		if (this.aiProgress) root.createDiv({ cls: 'vermilion-ai-progress', text: this.aiProgress });

		if (this.showTaxonomy) {
			this.renderTaxonomy(root);
			return;
		}
		if (!this.job) {
			root.createDiv({ cls: 'vermilion-empty', text: '点击“获取并处理文章”创建一个待审核任务。' });
			return;
		}
		if (!this.job.articles?.length) {
			root.createDiv({ cls: 'vermilion-empty', text: this.job.status === 'failed' ? this.job.message : '服务器正在处理文章……' });
			return;
		}

		const layout = root.createDiv({ cls: 'vermilion-layout' });
		this.renderArticleList(layout.createDiv({ cls: 'vermilion-list' }));
		const active = this.job.articles.find((article) => article.id === this.activeArticleId) ?? this.job.articles[0];
		this.activeArticleId = active.id;
		this.renderEditor(layout.createDiv({ cls: 'vermilion-editor' }), active);
		void this.renderPreview(layout.createDiv({ cls: 'vermilion-preview' }), active);
	}

	private renderArticleList(container: HTMLElement) {
		container.createEl('h3', { text: '文章' });
		for (const article of this.job?.articles ?? []) {
			const row = container.createDiv({ cls: `vermilion-list-item ${article.id === this.activeArticleId ? 'is-active' : ''}` });
			const checkbox = row.createEl('input', { type: 'checkbox' });
			checkbox.checked = this.selected.has(article.id);
			checkbox.onchange = () => {
				if (checkbox.checked) this.selected.add(article.id); else this.selected.delete(article.id);
				this.render();
			};
			const text = row.createDiv({ cls: 'vermilion-list-label' });
			text.createDiv({ text: article.metadata.title || article.filename });
			text.createEl('small', { text: article.status });
			text.onclick = () => {
				this.activeArticleId = article.id;
				this.render();
			};
		}
	}

	private labeledInput(container: HTMLElement, label: string, value: string, onChange: (value: string) => void, multiline = false) {
		const field = container.createDiv({ cls: 'vermilion-field' });
		field.createEl('label', { text: label });
		const input = multiline ? field.createEl('textarea') : field.createEl('input', { type: 'text' });
		input.value = value;
		input.oninput = () => onChange(input.value);
		return input;
	}

	private renderCategorySelect(container: HTMLElement, article: ArticleDraft) {
		const field = container.createDiv({ cls: 'vermilion-field' });
		field.createEl('label', { text: '分类' });
		const select = field.createEl('select');
		select.createEl('option', { text: '未分类', value: '' });
		const enabled = this.taxonomy?.categories.filter((category) => category.enabled) ?? [];
		const current = article.metadata.category ?? '';
		if (current && !enabled.some((category) => category.name === current)) {
			select.createEl('option', { text: `${current}（未批准或已停用）`, value: current });
		}
		for (const category of enabled) select.createEl('option', { text: category.name, value: category.name });
		select.value = current;
		select.onchange = () => {
			article.metadata.category = select.value;
			this.dirtyArticles.add(article.id);
		};
	}

	private renderEditor(container: HTMLElement, article: ArticleDraft) {
		const heading = container.createDiv({ cls: 'vermilion-editor-heading' });
		heading.createEl('h3', { text: '编辑' });
		const analyzeButton = heading.createEl('button', { text: 'AI 分析当前' });
		analyzeButton.disabled = this.aiRunning || article.status === 'deleted' || article.status === 'conflict';
		analyzeButton.onclick = () => void this.analyzeArticle(article);
		if (article.error) container.createDiv({ cls: 'vermilion-error', text: article.error });
		this.labeledInput(container, '标题', article.metadata.title ?? '', (value) => { article.metadata.title = value; this.dirtyArticles.add(article.id); });
		this.labeledInput(container, '摘要', article.metadata.description ?? '', (value) => { article.metadata.description = value; this.dirtyArticles.add(article.id); }, true);
		this.renderCategorySelect(container, article);
		this.labeledInput(container, '标签（逗号分隔）', (article.metadata.tags ?? []).join(', '), (value) => {
			article.metadata.tags = value.split(/[,，]/).map((tag) => tag.trim()).filter(Boolean);
			this.dirtyArticles.add(article.id);
		});
		this.labeledInput(container, '发布时间', article.metadata.published ?? '', (value) => { article.metadata.published = value; this.dirtyArticles.add(article.id); });
		const flags = container.createDiv({ cls: 'vermilion-flags' });
		for (const [label, key] of [['草稿', 'draft'], ['置顶', 'pinned']] as const) {
			const wrapper = flags.createEl('label');
			const checkbox = wrapper.createEl('input', { type: 'checkbox' });
			checkbox.checked = Boolean(article.metadata[key]);
			checkbox.onchange = () => { article.metadata[key] = checkbox.checked; this.dirtyArticles.add(article.id); };
			wrapper.appendText(label);
		}
		const bodyField = container.createDiv({ cls: 'vermilion-field' });
		bodyField.createEl('label', { text: '正文' });
		const body = bodyField.createEl('textarea', { cls: 'vermilion-body-editor' });
		body.value = article.content;
		body.oninput = () => { article.content = body.value; this.dirtyArticles.add(article.id); };

		if (article.aiSuggestion) {
			const aiBox = container.createDiv({ cls: 'vermilion-ai-box' });
			aiBox.createEl('h4', { text: 'AI 建议' });
			aiBox.createEl('p', { text: article.aiSuggestion.description || '没有摘要建议' });
			aiBox.createEl('p', { text: `分类：${article.aiSuggestion.category || '无'}` });
			if (article.aiSuggestion.proposedCategory) {
				aiBox.createEl('p', { text: `新分类建议：${article.aiSuggestion.proposedCategory.name}（${article.aiSuggestion.proposedCategory.reason || '未提供理由'}）` });
			}
			aiBox.createEl('p', { text: `已有标签：${article.aiSuggestion.selectedTags.join('、') || '无'}` });
			aiBox.createEl('button', { text: '采用已有建议' }).onclick = () => {
				if (article.aiSuggestion?.description) article.metadata.description = article.aiSuggestion.description;
				if (article.aiSuggestion?.category) article.metadata.category = article.aiSuggestion.category;
				article.metadata.tags = Array.from(new Set([...(article.metadata.tags ?? []), ...(article.aiSuggestion?.selectedTags ?? [])]));
				this.dirtyArticles.add(article.id);
				void this.cacheAISuggestion(article);
				this.render();
			};
			for (const proposal of article.aiSuggestion.proposedTags) {
				const proposalRow = aiBox.createDiv({ cls: 'vermilion-proposal' });
				proposalRow.createSpan({ text: `${proposal.name}：${proposal.reason}` });
				proposalRow.createEl('button', { text: '批准新标签' }).onclick = () => void this.approveProposedTag(article, proposal.name);
			}
		}

		const actions = container.createDiv({ cls: 'vermilion-actions' });
		actions.createEl('button', { text: '保存到服务器和本地', cls: 'mod-cta' }).onclick = () => void this.saveArticle(article);
		actions.createEl('button', { text: '在 Obsidian 中打开' }).onclick = () => void this.openLocalArticle(article);
	}

	private async renderPreview(container: HTMLElement, article: ArticleDraft) {
		container.createEl('h3', { text: '预览' });
		const meta = container.createDiv({ cls: 'vermilion-preview-meta' });
		meta.createEl('strong', { text: article.metadata.title });
		meta.createEl('p', { text: article.metadata.description ?? '' });
		meta.createEl('small', { text: `${article.metadata.category ?? '未分类'} · ${(article.metadata.tags ?? []).join('、')}` });
		const markdown = container.createDiv({ cls: 'markdown-preview-view' });
		await MarkdownRenderer.render(this.app, article.content, markdown, this.localPath(article), this);
	}

	private renderTaxonomy(container: HTMLElement) {
		const section = container.createDiv({ cls: 'vermilion-taxonomy' });
		section.createEl('h3', { text: '分类与标签管理' });
		if (this.taxonomyState === 'loading') {
			section.createDiv({ cls: 'vermilion-state-card', text: '正在加载标签库…' });
			return;
		}
		if (this.taxonomyState === 'error' || !this.taxonomy) {
			const card = section.createDiv({ cls: 'vermilion-state-card is-error' });
			card.createEl('strong', { text: '标签库加载失败' });
			card.createEl('p', { text: this.taxonomyError || '未知错误' });
			const actions = card.createDiv({ cls: 'vermilion-actions' });
			actions.createEl('button', { text: '重新加载', cls: 'mod-cta' }).onclick = () => void this.loadTaxonomy(true);
			actions.createEl('button', { text: '检查插件设置' }).onclick = () => {
				const setting = (this.app as unknown as { setting?: { open(): void; openTabById(id: string): void } }).setting;
				setting?.open();
				setting?.openTabById(this.plugin.manifest.id);
			};
			return;
		}
		const tabs = section.createDiv({ cls: 'vermilion-taxonomy-tabs' });
		for (const [label, value] of [['分类', 'categories'], ['标签', 'tags']] as const) {
			const button = tabs.createEl('button', { text: label, cls: this.taxonomyTab === value ? 'mod-cta' : '' });
			button.onclick = () => { this.taxonomyTab = value; this.render(); };
		}
		if (this.taxonomyTab === 'categories') {
			this.renderCategories(section);
			return;
		}
		section.createEl('p', { text: '启用且允许 AI 使用的标签会进入 AI 白名单。已被文章使用的标签请停用或改名，不直接删除。' });

		const enabledCount = this.taxonomy.tags.filter((tag) => tag.enabled).length;
		section.createDiv({ cls: 'vermilion-taxonomy-summary', text: `共 ${this.taxonomy.tags.length} 个标签 · ${enabledCount} 个启用 · ${this.collectProposals().length} 个待审批建议` });
		this.renderProposalQueue(section);

		const toolbar = section.createDiv({ cls: 'vermilion-taxonomy-toolbar' });
		const search = toolbar.createEl('input', { type: 'search', value: this.tagSearch });
		search.placeholder = '搜索名称、别名或说明';
		const filter = toolbar.createEl('select');
		for (const [label, value] of [['全部标签', 'all'], ['仅启用', 'enabled'], ['仅停用', 'disabled']] as const) {
			const option = filter.createEl('option', { text: label, value });
			option.selected = value === this.tagFilter;
		}
		const addButton = toolbar.createEl('button', { text: '新增标签' });
		const reloadButton = toolbar.createEl('button', { text: '重新加载' });
		const saveButton = toolbar.createEl('button', { text: '保存标签库', cls: 'mod-cta' });
		const list = section.createDiv({ cls: 'vermilion-tag-list' });
		const updateList = () => this.renderTagList(list);
		search.oninput = () => { this.tagSearch = search.value; updateList(); };
		filter.onchange = () => { this.tagFilter = filter.value as typeof this.tagFilter; updateList(); };
		addButton.onclick = () => {
			this.taxonomy?.tags.unshift({ id: '', name: '', aliases: [], description: '', enabled: true, aiSelectable: true, createdAt: '', updatedAt: '' });
			this.tagSearch = '';
			this.tagFilter = 'all';
			this.render();
		};
		reloadButton.onclick = () => {
			if (window.confirm('重新加载会丢弃尚未保存的标签修改，确认继续吗？')) void this.loadTaxonomy(true);
		};
		saveButton.onclick = () => void this.saveTaxonomy();
		updateList();
	}

	private renderCategories(container: HTMLElement) {
		if (!this.taxonomy) return;
		container.createEl('p', { text: '每篇文章只能选择一个已启用分类。已被文章使用的分类不能直接删除。' });
		const proposals = this.collectCategoryProposals();
		container.createDiv({ cls: 'vermilion-taxonomy-summary', text: `共 ${this.taxonomy.categories.length} 个分类 · ${this.taxonomy.categories.filter((item) => item.enabled).length} 个启用 · ${proposals.length} 个待审批建议` });
		if (proposals.length) {
			const box = container.createDiv({ cls: 'vermilion-proposal-queue' });
			box.createEl('h4', { text: 'AI 新分类待审批' });
			for (const { article, proposal } of proposals) {
				const row = box.createDiv({ cls: 'vermilion-proposal-review' });
				const description = row.createDiv();
				description.createEl('strong', { text: proposal.name });
				description.createEl('small', { text: `${article.metadata.title || article.filename} · ${proposal.reason || '未提供理由'}` });
				const replacement = row.createEl('select');
				for (const category of this.taxonomy.categories.filter((item) => item.enabled)) replacement.createEl('option', { text: category.name, value: category.name });
				row.createEl('button', { text: '批准' }).onclick = () => void this.approveProposedCategory(article, proposal);
				const replace = row.createEl('button', { text: '替换' });
				replace.disabled = replacement.options.length === 0;
				replace.onclick = () => this.resolveCategoryProposal(article, replacement.value);
				row.createEl('button', { text: '拒绝' }).onclick = () => this.resolveCategoryProposal(article);
			}
		}
		const actions = container.createDiv({ cls: 'vermilion-taxonomy-toolbar compact' });
		actions.createEl('button', { text: '新增分类' }).onclick = () => {
			this.taxonomy?.categories.unshift({ id: '', name: '', description: '', enabled: true });
			this.render();
		};
		actions.createEl('button', { text: '重新加载' }).onclick = () => {
			if (window.confirm('重新加载会丢弃尚未保存的分类修改，确认继续吗？')) void this.loadTaxonomy(true);
		};
		actions.createEl('button', { text: '保存分类库', cls: 'mod-cta' }).onclick = () => void this.saveTaxonomy();
		const list = container.createDiv({ cls: 'vermilion-tag-list' });
		if (!this.taxonomy.categories.length) list.createDiv({ cls: 'vermilion-empty compact', text: '分类库为空，可以创建第一个分类。' });
		for (const category of this.taxonomy.categories) this.renderCategoryRow(list, category);
	}

	private renderCategoryRow(container: HTMLElement, category: Category) {
		const row = container.createDiv({ cls: 'vermilion-category-row' });
		const identity = row.createDiv({ cls: 'vermilion-tag-identity' });
		const name = identity.createEl('input', { type: 'text', value: category.name });
		name.placeholder = '分类名称';
		name.oninput = () => category.name = name.value;
		const draftUsage = this.countDraftCategoryUsage(category.name);
		const publishedUsage = this.categoryUsage[category.name] ?? 0;
		identity.createEl('small', { text: this.usageLabel(draftUsage, publishedUsage) });
		const description = row.createEl('input', { type: 'text', value: category.description });
		description.placeholder = '告诉 AI 何时选择这个分类';
		description.oninput = () => category.description = description.value;
		const enabledLabel = row.createEl('label');
		const enabled = enabledLabel.createEl('input', { type: 'checkbox' });
		enabled.checked = category.enabled;
		enabled.onchange = () => category.enabled = enabled.checked;
		enabledLabel.appendText('启用');
		const remove = row.createEl('button', { text: '删除' });
		const effectiveUsage = draftUsage ?? (this.taxonomyUsageLoaded ? publishedUsage : null);
		remove.disabled = effectiveUsage === null || effectiveUsage > 0;
		remove.title = effectiveUsage === null ? '无法确认使用次数，已禁止删除。' : effectiveUsage > 0 ? '当前审核状态仍有文章使用该分类。' : '删除分类，并将相关文章加入本次发布';
		remove.onclick = () => {
			if (!this.taxonomy || !window.confirm(`确认删除分类“${category.name || '未命名分类'}”吗？`)) return;
			this.selectArticlesAffectedByCategory(category.name);
			this.taxonomy.categories = this.taxonomy.categories.filter((item) => item !== category);
			this.render();
		};
	}

	private renderTagList(container: HTMLElement) {
		container.empty();
		if (!this.taxonomy) return;
		const query = this.tagSearch.trim().toLowerCase();
		const tags = this.taxonomy.tags.filter((tag) => {
			if (this.tagFilter === 'enabled' && !tag.enabled) return false;
			if (this.tagFilter === 'disabled' && tag.enabled) return false;
			return !query || [tag.name, tag.description, ...tag.aliases].some((value) => value.toLowerCase().includes(query));
		});
		if (!tags.length) {
			container.createDiv({ cls: 'vermilion-empty compact', text: this.taxonomy.tags.length ? '没有符合条件的标签。' : '标签库为空，可以创建第一个标签。' });
			return;
		}
		for (const tag of tags) this.renderTagRow(container, tag);
	}

	private renderTagRow(container: HTMLElement, tag: ManagedTag) {
		const row = container.createDiv({ cls: 'vermilion-tag-row' });
		const identity = row.createDiv({ cls: 'vermilion-tag-identity' });
		const name = identity.createEl('input', { type: 'text', value: tag.name });
		name.placeholder = '标签名称';
		name.oninput = () => tag.name = name.value;
		const draftUsage = this.countDraftTagUsage(tag.name);
		const publishedUsage = this.taxonomyUsage[tag.name] ?? 0;
		identity.createEl('small', { text: this.usageLabel(draftUsage, publishedUsage) });
		const details = row.createDiv({ cls: 'vermilion-tag-details' });
		const description = details.createEl('input', { type: 'text', value: tag.description });
		description.placeholder = '告诉 AI 何时使用这个标签';
		description.oninput = () => tag.description = description.value;
		const aliases = details.createEl('input', { type: 'text', value: tag.aliases.join(', ') });
		aliases.placeholder = '别名（逗号分隔）';
		aliases.oninput = () => tag.aliases = aliases.value.split(/[,，]/).map((value) => value.trim()).filter(Boolean);
		const switches = row.createDiv({ cls: 'vermilion-tag-switches' });
		for (const [label, key] of [['启用', 'enabled'], ['允许 AI', 'aiSelectable']] as const) {
			const wrapper = switches.createEl('label');
			const checkbox = wrapper.createEl('input', { type: 'checkbox' });
			checkbox.checked = tag[key];
			checkbox.onchange = () => tag[key] = checkbox.checked;
			wrapper.appendText(label);
		}
		const remove = row.createEl('button', { text: '删除' });
		const effectiveUsage = draftUsage ?? (this.taxonomyUsageLoaded ? publishedUsage : null);
		remove.disabled = effectiveUsage === null || effectiveUsage > 0;
		remove.title = effectiveUsage === null ? '无法确认使用次数，已禁止删除。' : effectiveUsage > 0 ? '当前审核状态仍有文章使用该标签。' : '删除标签，并将相关文章加入本次发布';
		remove.onclick = () => {
			if (!this.taxonomy || !window.confirm(`确认删除标签“${tag.name || '未命名标签'}”吗？`)) return;
			this.selectArticlesAffectedByTag(tag.name);
			this.taxonomy.tags = this.taxonomy.tags.filter((item) => item !== tag);
			this.render();
		};
	}

	private usageLabel(draftUsage: number | null, publishedUsage: number) {
		const published = this.taxonomyUsageLoaded ? `${publishedUsage}` : '未知';
		return draftUsage === null ? `已发布 ${published} 篇使用` : `当前任务 ${draftUsage} 篇 · 已发布 ${published} 篇`;
	}

	private countDraftTagUsage(name: string): number | null {
		if (!this.job?.articles) return null;
		const target = name.trim().toLowerCase();
		return this.job.articles.filter((article) => article.status !== 'deleted' && (article.metadata.tags ?? []).some((tag) => tag.trim().toLowerCase() === target)).length;
	}

	private countDraftCategoryUsage(name: string): number | null {
		if (!this.job?.articles) return null;
		const target = name.trim().toLowerCase();
		return this.job.articles.filter((article) => article.status !== 'deleted' && (article.metadata.category ?? '').trim().toLowerCase() === target).length;
	}

	private selectArticlesAffectedByTag(name: string) {
		if (!this.job?.articles) return;
		const target = name.trim().toLowerCase();
		let added = 0;
		for (const article of this.job.articles) {
			const previouslyUsed = (article.originalMetadata.tags ?? []).some((tag) => tag.trim().toLowerCase() === target);
			const stillUsed = article.status !== 'deleted' && (article.metadata.tags ?? []).some((tag) => tag.trim().toLowerCase() === target);
			if (previouslyUsed && !stillUsed && !this.selected.has(article.id)) {
				this.selected.add(article.id);
				added++;
			}
		}
		if (added) new Notice(`已自动选择 ${added} 篇受标签“${name}”影响的文章，请确认保存后一起发布。`);
	}

	private selectArticlesAffectedByCategory(name: string) {
		if (!this.job?.articles) return;
		const target = name.trim().toLowerCase();
		let added = 0;
		for (const article of this.job.articles) {
			const previouslyUsed = (article.originalMetadata.category ?? '').trim().toLowerCase() === target;
			const stillUsed = article.status !== 'deleted' && (article.metadata.category ?? '').trim().toLowerCase() === target;
			if (previouslyUsed && !stillUsed && !this.selected.has(article.id)) {
				this.selected.add(article.id);
				added++;
			}
		}
		if (added) new Notice(`已自动选择 ${added} 篇受分类“${name}”影响的文章，请确认保存后一起发布。`);
	}

	private collectProposals(): Array<{ article: ArticleDraft; proposal: ProposedTag }> {
		const result: Array<{ article: ArticleDraft; proposal: ProposedTag }> = [];
		for (const article of this.job?.articles ?? []) {
			for (const proposal of article.aiSuggestion?.proposedTags ?? []) result.push({ article, proposal });
		}
		return result;
	}

	private collectCategoryProposals(): Array<{ article: ArticleDraft; proposal: ProposedCategory }> {
		const result: Array<{ article: ArticleDraft; proposal: ProposedCategory }> = [];
		for (const article of this.job?.articles ?? []) {
			if (article.aiSuggestion?.proposedCategory) result.push({ article, proposal: article.aiSuggestion.proposedCategory });
		}
		return result;
	}

	private async approveProposedCategory(article: ArticleDraft, proposal: ProposedCategory) {
		if (!this.taxonomy) return;
		let category = this.taxonomy.categories.find((item) => item.name.toLowerCase() === proposal.name.toLowerCase());
		if (!category) {
			category = { id: '', name: proposal.name, description: proposal.reason, enabled: true };
			this.taxonomy.categories.push(category);
			if (!await this.saveTaxonomy(false)) return;
		}
		article.metadata.category = category.name;
		if (article.aiSuggestion) article.aiSuggestion.proposedCategory = null;
		void this.cacheAISuggestion(article);
		this.dirtyArticles.add(article.id);
		this.render();
		new Notice(`已批准分类“${category.name}”，保存文章后生效。`);
	}

	private resolveCategoryProposal(article: ArticleDraft, replacement?: string) {
		const proposedName = article.aiSuggestion?.proposedCategory?.name ?? '新分类';
		if (replacement) {
			article.metadata.category = replacement;
			this.dirtyArticles.add(article.id);
		}
		if (article.aiSuggestion) article.aiSuggestion.proposedCategory = null;
		void this.cacheAISuggestion(article);
		this.render();
		new Notice(replacement ? `已将“${proposedName}”替换为“${replacement}”。` : `已拒绝新分类“${proposedName}”。`);
	}

	private renderProposalQueue(container: HTMLElement) {
		const proposals = this.collectProposals();
		if (!proposals.length) return;
		const box = container.createDiv({ cls: 'vermilion-proposal-queue' });
		box.createEl('h4', { text: 'AI 新标签待审批' });
		for (const { article, proposal } of proposals) {
			const row = box.createDiv({ cls: 'vermilion-proposal-review' });
			const description = row.createDiv();
			description.createEl('strong', { text: proposal.name });
			description.createEl('small', { text: `${article.metadata.title || article.filename} · ${proposal.reason || '未提供理由'}` });
			const replacement = row.createEl('select');
			for (const tag of this.taxonomy?.tags.filter((item) => item.enabled) ?? []) replacement.createEl('option', { text: tag.name, value: tag.name });
			row.createEl('button', { text: '批准' }).onclick = () => void this.approveProposedTag(article, proposal.name);
			const replace = row.createEl('button', { text: '替换' });
			replace.disabled = replacement.options.length === 0;
			replace.onclick = () => this.resolveProposal(article, proposal.name, replacement.value);
			row.createEl('button', { text: '拒绝' }).onclick = () => this.resolveProposal(article, proposal.name);
		}
	}

	private resolveProposal(article: ArticleDraft, proposedName: string, replacement?: string) {
		if (replacement) {
			article.metadata.tags = Array.from(new Set([...(article.metadata.tags ?? []), replacement]));
			this.dirtyArticles.add(article.id);
		}
		if (article.aiSuggestion) article.aiSuggestion.proposedTags = article.aiSuggestion.proposedTags.filter((tag) => tag.name !== proposedName);
		void this.cacheAISuggestion(article);
		this.render();
		new Notice(replacement ? `已将“${proposedName}”替换为“${replacement}”。` : `已拒绝新标签“${proposedName}”。`);
	}

	private async approveProposedTag(article: ArticleDraft, name: string) {
		if (!this.taxonomy) {
			new Notice('标签库未加载，暂时无法批准新标签。');
			return;
		}
		if (!this.taxonomy.tags.some((tag) => tag.name.toLowerCase() === name.toLowerCase())) {
			this.taxonomy.tags.push({ id: '', name, aliases: [], description: '', enabled: true, aiSelectable: true, createdAt: '', updatedAt: '' });
			if (!await this.saveTaxonomy(false)) return;
		}
		article.metadata.tags = Array.from(new Set([...(article.metadata.tags ?? []), name]));
		this.dirtyArticles.add(article.id);
		if (article.aiSuggestion) article.aiSuggestion.proposedTags = article.aiSuggestion.proposedTags.filter((tag) => tag.name !== name);
		void this.cacheAISuggestion(article);
		this.render();
		new Notice(`已批准标签“${name}”，保存文章后生效。`);
	}

	private async saveTaxonomy(notify = true) {
		if (!this.taxonomy) return false;
		const names = this.taxonomy.tags.map((tag) => tag.name.trim());
		if (names.some((name) => !name)) {
			new Notice('标签名称不能为空。');
			return false;
		}
		const normalized = names.map((name) => name.toLowerCase());
		if (new Set(normalized).size !== normalized.length) {
			new Notice('标签名称不能重复，请先合并或改名。');
			return false;
		}
		const categoryNames = this.taxonomy.categories.map((category) => category.name.trim());
		if (categoryNames.some((name) => !name)) {
			new Notice('分类名称不能为空。');
			return false;
		}
		const normalizedCategories = categoryNames.map((name) => name.toLowerCase());
		if (new Set(normalizedCategories).size !== normalizedCategories.length) {
			new Notice('分类名称不能重复，请先合并或改名。');
			return false;
		}
		try {
			this.taxonomy = await this.plugin.api.saveTaxonomy(this.taxonomy);
			try {
				const usage = await this.plugin.api.getTaxonomyUsage();
				this.taxonomyUsage = usage.tags ?? {};
				this.categoryUsage = usage.categories ?? {};
				this.taxonomyUsageLoaded = true;
			} catch {
				// Usage counts are optional for compatibility with an older server.
				this.taxonomyUsageLoaded = false;
			}
			if (notify) new Notice('分类与标签库已保存，随下一次发布进入 Git。');
			this.render();
			return true;
		} catch (error) {
			new Notice(`保存标签库失败：${describeApiError(error)}`);
			return false;
		}
	}

	private applyCachedSuggestions() {
		if (!this.job || this.plugin.settings.aiSuggestionJobId !== this.job.id) return;
		for (const article of this.job.articles ?? []) {
			const cached = this.plugin.settings.aiSuggestions[article.id];
			if (cached) article.aiSuggestion = cached;
		}
	}

	private needsLocalAI(article: ArticleDraft) {
		return article.status !== 'deleted' && article.status !== 'conflict' && !article.aiSuggestion && (
			article.status === 'new' ||
			!article.metadata.description?.trim() ||
			!article.metadata.category?.trim() ||
			(article.metadata.tags ?? []).length === 0
		);
	}

	private async cacheAISuggestion(article: ArticleDraft) {
		if (!this.job || !article.aiSuggestion) return;
		if (this.plugin.settings.aiSuggestionJobId !== this.job.id) {
			this.plugin.settings.aiSuggestionJobId = this.job.id;
			this.plugin.settings.aiSuggestions = {};
		}
		this.plugin.settings.aiSuggestions[article.id] = article.aiSuggestion;
		await this.plugin.saveSettings();
	}

	private async generateSuggestion(article: ArticleDraft) {
		if (!this.taxonomy) throw new Error('分类与标签库尚未加载。');
		article.aiSuggestion = await analyzeArticleLocally(this.plugin.settings, this.taxonomy, article);
		await this.cacheAISuggestion(article);
	}

	private async analyzeArticle(article: ArticleDraft) {
		if (this.aiRunning) return;
		this.aiRunning = true;
		this.aiProgress = `正在分析：${article.metadata.title || article.filename}`;
		this.render();
		try {
			await this.generateSuggestion(article);
			new Notice(`AI 分析完成：${article.metadata.title || article.filename}`);
		} catch (error) {
			new Notice(`AI 分析失败：${error instanceof Error ? error.message : String(error)}`);
		} finally {
			this.aiRunning = false;
			this.aiProgress = '';
			this.render();
		}
	}

	private async analyzePendingArticles() {
		if (this.aiRunning || !this.job?.articles) return;
		const articles = this.job.articles.filter((article) => this.needsLocalAI(article));
		if (!articles.length) {
			new Notice('没有需要 AI 分析的文章；可以在文章编辑区手动重新分析当前文章。');
			return;
		}
		if (!this.taxonomy) {
			new Notice('分类与标签库尚未加载。');
			return;
		}
		if (!this.plugin.settings.aiApiKey.trim()) {
			new Notice('请先在插件设置中配置 AI API Key。');
			return;
		}
		this.aiRunning = true;
		const errors: string[] = [];
		this.render();
		for (let index = 0; index < articles.length; index++) {
			const article = articles[index];
			this.aiProgress = `本地 AI 分析 ${index + 1}/${articles.length}：${article.metadata.title || article.filename}`;
			this.render();
			try {
				await this.generateSuggestion(article);
			} catch (error) {
				errors.push(`${article.metadata.title || article.filename}：${error instanceof Error ? error.message : String(error)}`);
			}
		}
		this.aiRunning = false;
		this.aiProgress = '';
		this.render();
		if (errors.length) new Notice(`AI 分析完成，但有 ${errors.length} 篇失败：${errors[0]}`);
		else new Notice(`本地 AI 分析完成：${articles.length} 篇文章。`);
	}

	private async saveArticle(article: ArticleDraft) {
		if (!this.job) return;
		try {
			await this.assertLocalNotChanged(article);
			const suggestion = article.aiSuggestion;
			const updated = await this.plugin.api.updateArticle(this.job.id, article);
			updated.aiSuggestion = suggestion;
			const index = this.job.articles?.findIndex((item) => item.id === article.id) ?? -1;
			if (index >= 0 && this.job.articles) this.job.articles[index] = updated;
			this.activeArticleId = updated.id;
			await this.writeLocalArticle(updated);
			updated.clientHash = await sha256(this.composeMarkdown(updated));
			this.dirtyArticles.delete(updated.id);
			this.render();
			new Notice(`已保存：${updated.metadata.title}`);
		} catch (error) {
			new Notice(`保存失败：${(error as Error).message}`);
		}
	}

	private async publishSelected() {
		if (!this.job?.articles) return;
		const articles = this.job.articles.filter((article) => this.selected.has(article.id));
		if (articles.some((article) => this.dirtyArticles.has(article.id))) {
			new Notice('所选文章存在未保存修改，请先保存到服务器和本地。');
			return;
		}
		if (articles.some((article) => article.status === 'deleted') && !window.confirm('所选内容包含待删除文章，确认从网站删除吗？')) return;
		if (!window.confirm(`确认直接发布 ${articles.length} 篇文章到 deploy 吗？`)) return;
		try {
			if (!await this.saveTaxonomy(false)) return;
			const response = await this.plugin.api.publish(this.job.id, articles);
			new Notice(`发布成功：${response.commitSha.slice(0, 12)}`);
			this.selected.clear();
			await this.refreshJob();
		} catch (error) {
			new Notice(`发布失败：${(error as Error).message}`);
		}
	}

	private localPath(article: ArticleDraft) {
		return normalizePath([this.plugin.settings.localPostsFolder, article.filename].filter(Boolean).join('/'));
	}

	private async collectLocalManifest(): Promise<LocalFile[]> {
		const prefix = this.plugin.settings.localPostsFolder ? normalizePath(this.plugin.settings.localPostsFolder) + '/' : '';
		const files = this.app.vault.getFiles().filter((file) => file.path.startsWith(prefix) && /\.mdx?$/i.test(file.path));
		return Promise.all(files.map(async (file) => ({ path: file.path, hash: await sha256(await this.app.vault.read(file)) })));
	}

	private composeMarkdown(article: ArticleDraft) {
		const metadata: Record<string, unknown> = { ...(article.metadata.extra ?? {}) };
		for (const [key, value] of Object.entries(article.metadata)) {
			if (key !== 'extra' && value !== undefined && value !== '') metadata[key] = value;
		}
		return `---\n${stringifyYaml(metadata).trim()}\n---\n\n${article.content.trim()}\n`;
	}

	private async ensureFolder(path: string) {
		const parts = path.split('/').slice(0, -1);
		let current = '';
		for (const part of parts) {
			current = current ? `${current}/${part}` : part;
			if (!this.app.vault.getAbstractFileByPath(current)) await this.app.vault.createFolder(current);
		}
	}

	private async assertLocalNotChanged(article: ArticleDraft) {
		if (!article.clientHash) return;
		const file = this.app.vault.getAbstractFileByPath(this.localPath(article));
		if (!(file instanceof TFile)) throw new Error('本地文件已被删除，请重新处理任务。');
		const currentHash = await sha256(await this.app.vault.read(file));
		if (currentHash !== article.clientHash) throw new Error('本地文件在任务开始后发生了变化，已阻止覆盖，请重新处理任务。');
	}

	private async writeLocalArticle(article: ArticleDraft) {
		const path = this.localPath(article);
		await this.ensureFolder(path);
		const markdown = this.composeMarkdown(article);
		const existing = this.app.vault.getAbstractFileByPath(path);
		if (existing instanceof TFile) await this.app.vault.modify(existing, markdown);
		else await this.app.vault.create(path, markdown);
	}

	private async openLocalArticle(article: ArticleDraft) {
		const file = this.app.vault.getAbstractFileByPath(this.localPath(article));
		if (file instanceof TFile) await this.app.workspace.getLeaf(true).openFile(file);
		else new Notice('本地文件不存在，请先保存到本地。');
	}
}
