import { ItemView, MarkdownRenderer, Notice, TFile, WorkspaceLeaf, normalizePath, stringifyYaml } from 'obsidian';
import type SyncPlugin from '../main';
import { ApiError, describeApiError } from './api-client';
import { analyzeArticleLocally } from './local-ai';
import type { ArticleDraft, Category, LocalFile, ManagedTag, ProposedCategory, ProposedTag, SyncJob, Taxonomy } from './models';

export const ARTICLE_MANAGER_VIEW = 'flash-thought-article-manager';

type ManagementTab = 'articles' | 'taxonomy' | 'suggestions';
type TaxonomyItem = ManagedTag | Category;
type TaxonomyStatusFilter = 'all' | 'enabled' | 'disabled';
type TaxonomyAIFilter = 'all' | 'allowed' | 'blocked';
type TaxonomyUsageFilter = 'all' | 'used' | 'unused';
type SuggestionEntry = {
	key: string;
	kind: 'tag' | 'category';
	article: ArticleDraft;
	proposal: ProposedTag | ProposedCategory;
};

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
	private managementTab: ManagementTab = 'articles';
	private taxonomyTab: 'categories' | 'tags' = 'tags';
	private taxonomySearch = '';
	private taxonomyStatusFilter: TaxonomyStatusFilter = 'all';
	private taxonomyAIFilter: TaxonomyAIFilter = 'all';
	private taxonomyUsageFilter: TaxonomyUsageFilter = 'all';
	private selectedTaxonomyItems = new Set<TaxonomyItem>();
	private taxonomyOriginalNames = new WeakMap<TaxonomyItem, string>();
	private taxonomyDirty = false;
	private taxonomyBaseline = '';
	private suggestionFilter: 'all' | 'tags' | 'categories' = 'all';
	private selectedAISuggestions = new Set<string>();
	private activeArticleId = '';
	private selected = new Set<string>();
	private dirtyArticles = new Set<string>();
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
			for (const category of this.taxonomy.categories) {
				if (typeof category.aiSelectable !== 'boolean') category.aiSelectable = category.enabled;
				if (!category.enabled) category.aiSelectable = false;
			}
			this.taxonomyOriginalNames = new WeakMap<TaxonomyItem, string>();
			for (const item of [...this.taxonomy.categories, ...this.taxonomy.tags]) this.taxonomyOriginalNames.set(item, item.name);
			this.taxonomyBaseline = JSON.stringify(this.taxonomy);
			this.taxonomyDirty = false;
			this.selectedTaxonomyItems.clear();
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
			if (showNotice) new Notice(`分类体系加载成功：${this.taxonomy.categories.length} 个分类，${this.taxonomy.tags.length} 个标签。`);
		} catch (error) {
			this.taxonomy = null;
			this.taxonomyState = 'error';
			this.taxonomyError = describeApiError(error);
			if (showNotice) new Notice(`分类体系加载失败：${this.taxonomyError}`);
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
			this.managementTab = 'articles';
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
		const navigation = root.createDiv({ cls: 'vermilion-management-tabs' });
		const pendingSuggestions = this.collectProposals().length + this.collectCategoryProposals().length;
		for (const [label, tab] of [
			['文章管理', 'articles'],
			['分类体系', 'taxonomy'],
			[`AI 建议${pendingSuggestions ? ` (${pendingSuggestions})` : ''}`, 'suggestions'],
		] as Array<[string, ManagementTab]>) {
			const button = navigation.createEl('button', { text: label, cls: this.managementTab === tab ? 'mod-cta' : '' });
			button.onclick = () => this.switchManagementTab(tab);
		}

		if (this.managementTab === 'taxonomy') {
			this.renderTaxonomy(root);
			return;
		}
		if (this.managementTab === 'suggestions') {
			this.renderAISuggestions(root);
			return;
		}

		const toolbar = root.createDiv({ cls: 'vermilion-toolbar' });
		toolbar.createEl('button', { text: '获取并处理文章', cls: 'mod-cta' }).onclick = () => void this.prepareSync();
		const refreshButton = toolbar.createEl('button', { text: '刷新状态' });
		refreshButton.title = '仅重新读取服务器任务状态，不会运行 AI 分析';
		refreshButton.onclick = () => void this.refreshJob();
		const aiButton = toolbar.createEl('button', { text: this.aiRunning ? 'AI 分析中…' : 'AI 分析待处理' });
		aiButton.disabled = this.aiRunning || !this.job?.articles?.length;
		aiButton.onclick = () => void this.analyzePendingArticles();
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

	private switchManagementTab(tab: ManagementTab) {
		if (tab === this.managementTab) return;
		if (this.managementTab === 'taxonomy' && this.taxonomyDirty) {
			new Notice('分类体系存在未保存修改，请先保存或放弃修改。');
			return;
		}
		this.managementTab = tab;
		this.render();
	}

	private renderArticleList(container: HTMLElement) {
		container.createEl('h3', { text: '文章' });
		container.createEl('small', { text: '勾选文章表示加入本次发布；保存草稿不会自动勾选或发布。', cls: 'vermilion-list-hint' });
		for (const article of this.job?.articles ?? []) {
			const row = container.createDiv({ cls: `vermilion-list-item ${article.id === this.activeArticleId ? 'is-active' : ''}` });
			const checkbox = row.createEl('input', { type: 'checkbox' });
			checkbox.title = '加入本次发布';
			checkbox.setAttr('aria-label', `将“${article.metadata.title || article.filename}”加入本次发布`);
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
		const saveDraft = actions.createEl('button', { text: '保存审核草稿', cls: 'mod-cta' });
		saveDraft.title = '保存到 Obsidian 本地文件和服务器审核任务；不会写回 S3、加入发布列表或发布网站';
		saveDraft.onclick = () => void this.saveArticle(article);
		actions.createEl('button', { text: '在 Obsidian 中打开' }).onclick = () => void this.openLocalArticle(article);
		container.createEl('small', { text: '保存到 Obsidian 本地文件和服务器审核任务，不会写回 S3，也不会自动加入本次发布。', cls: 'vermilion-action-help' });
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
		const heading = section.createDiv({ cls: 'vermilion-section-heading' });
		heading.createEl('h3', { text: '分类体系' });
		heading.createEl('span', { text: this.taxonomyDirty ? '● 有未保存修改' : '已与服务器同步', cls: this.taxonomyDirty ? 'vermilion-dirty' : 'vermilion-synced' });
		if (this.taxonomyState === 'loading') {
			section.createDiv({ cls: 'vermilion-state-card', text: '正在加载分类体系…' });
			return;
		}
		if (this.taxonomyState === 'error' || !this.taxonomy) {
			const card = section.createDiv({ cls: 'vermilion-state-card is-error' });
			card.createEl('strong', { text: '分类体系加载失败' });
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
		for (const [label, value] of [[`标签 (${this.taxonomy.tags.length})`, 'tags'], [`分类 (${this.taxonomy.categories.length})`, 'categories']] as const) {
			const button = tabs.createEl('button', { text: label, cls: this.taxonomyTab === value ? 'mod-cta' : '' });
			button.onclick = () => { this.taxonomyTab = value; this.selectedTaxonomyItems.clear(); this.render(); };
		}
		const allItems = this.currentTaxonomyItems();
		const enabledCount = allItems.filter((item) => item.enabled).length;
		const aiCount = allItems.filter((item) => item.enabled && item.aiSelectable).length;
		section.createDiv({ cls: 'vermilion-taxonomy-summary', text: `共 ${allItems.length} 项 · ${enabledCount} 项启用 · ${aiCount} 项允许 AI · 使用中的项目不能直接删除` });
		const toolbar = section.createDiv({ cls: 'vermilion-taxonomy-toolbar' });
		const search = toolbar.createEl('input', { type: 'search', value: this.taxonomySearch });
		search.placeholder = '搜索名称、别名或说明';
		const statusFilter = this.createFilter(toolbar, [['全部状态', 'all'], ['已启用', 'enabled'], ['已停用', 'disabled']], this.taxonomyStatusFilter);
		const aiFilter = this.createFilter(toolbar, [['全部 AI 状态', 'all'], ['允许 AI', 'allowed'], ['禁止 AI', 'blocked']], this.taxonomyAIFilter);
		const usageFilter = this.createFilter(toolbar, [['全部使用状态', 'all'], ['正在使用', 'used'], ['未使用', 'unused']], this.taxonomyUsageFilter);
		const addButton = toolbar.createEl('button', { text: this.taxonomyTab === 'tags' ? '新增标签' : '新增分类' });
		const reloadButton = toolbar.createEl('button', { text: '重新加载' });
		search.oninput = () => { this.taxonomySearch = search.value; this.render(); };
		statusFilter.onchange = () => { this.taxonomyStatusFilter = statusFilter.value as TaxonomyStatusFilter; this.render(); };
		aiFilter.onchange = () => { this.taxonomyAIFilter = aiFilter.value as TaxonomyAIFilter; this.render(); };
		usageFilter.onchange = () => { this.taxonomyUsageFilter = usageFilter.value as TaxonomyUsageFilter; this.render(); };
		addButton.onclick = () => {
			if (!this.taxonomy) return;
			if (this.taxonomyTab === 'tags') this.taxonomy.tags.unshift({ id: '', name: '', aliases: [], description: '', enabled: true, aiSelectable: true, createdAt: '', updatedAt: '' });
			else this.taxonomy.categories.unshift({ id: '', name: '', description: '', enabled: true, aiSelectable: true });
			this.taxonomySearch = '';
			this.taxonomyStatusFilter = 'all';
			this.markTaxonomyDirty();
			this.render();
		};
		reloadButton.onclick = () => {
			if (!this.taxonomyDirty || window.confirm('重新加载会丢弃尚未保存的修改，确认继续吗？')) void this.loadTaxonomy(true);
		};

		const filtered = this.filteredTaxonomyItems();
		this.renderTaxonomyBulkActions(section, filtered);
		const table = section.createDiv({ cls: 'vermilion-taxonomy-table' });
		const header = table.createDiv({ cls: 'vermilion-taxonomy-row is-header' });
		header.createSpan({ text: '选择' });
		header.createSpan({ text: '名称' });
		header.createSpan({ text: '说明 / 别名' });
		header.createSpan({ text: '使用量' });
		header.createSpan({ text: '启用' });
		header.createSpan({ text: '允许 AI' });
		header.createSpan({ text: '操作' });
		if (!filtered.length) table.createDiv({ cls: 'vermilion-empty compact', text: allItems.length ? '没有符合筛选条件的项目。' : `暂无${this.taxonomyTab === 'tags' ? '标签' : '分类'}，可以先新增一个。` });
		for (const item of filtered) this.renderTaxonomyRow(table, item);

		const footer = section.createDiv({ cls: `vermilion-save-bar ${this.taxonomyDirty ? 'is-dirty' : ''}` });
		footer.createSpan({ text: this.taxonomyDirty ? '● 分类体系有尚未保存的修改' : '没有尚未保存的修改' });
		footer.createEl('button', { text: '放弃修改' }).onclick = () => {
			if (this.taxonomyDirty && window.confirm('确认放弃全部未保存修改吗？')) void this.loadTaxonomy();
		};
		const saveButton = footer.createEl('button', { text: '保存全部修改', cls: 'mod-cta' });
		saveButton.disabled = !this.taxonomyDirty;
		saveButton.onclick = () => void this.saveTaxonomy();
	}

	private createFilter<T extends string>(container: HTMLElement, options: Array<[string, T]>, value: T) {
		const select = container.createEl('select');
		for (const [label, optionValue] of options) {
			const option = select.createEl('option', { text: label, value: optionValue });
			option.selected = optionValue === value;
		}
		return select;
	}

	private currentTaxonomyItems(): TaxonomyItem[] {
		if (!this.taxonomy) return [];
		return this.taxonomyTab === 'tags' ? this.taxonomy.tags : this.taxonomy.categories;
	}

	private isTag(item: TaxonomyItem): item is ManagedTag {
		return 'aliases' in item;
	}

	private itemUsage(item: TaxonomyItem) {
		const names = Array.from(new Set([item.name, this.taxonomyOriginalNames.get(item) ?? ''].filter(Boolean)));
		const draftValues = names.map((name) => this.isTag(item) ? this.countDraftTagUsage(name) : this.countDraftCategoryUsage(name)).filter((value): value is number => value !== null);
		const publishedValues = names.map((name) => this.isTag(item) ? (this.taxonomyUsage[name] ?? 0) : (this.categoryUsage[name] ?? 0));
		const draft = draftValues.length ? Math.max(...draftValues) : null;
		const published = publishedValues.length ? Math.max(...publishedValues) : 0;
		const effective = draft ?? (this.taxonomyUsageLoaded ? published : null);
		return { draft, published, effective };
	}

	private filteredTaxonomyItems() {
		const query = this.taxonomySearch.trim().toLowerCase();
		return this.currentTaxonomyItems().filter((item) => {
			const searchable = [item.name, item.description, ...(this.isTag(item) ? item.aliases : [])];
			if (query && !searchable.some((value) => value.toLowerCase().includes(query))) return false;
			if (this.taxonomyStatusFilter === 'enabled' && !item.enabled) return false;
			if (this.taxonomyStatusFilter === 'disabled' && item.enabled) return false;
			if (this.taxonomyAIFilter === 'allowed' && (!item.enabled || !item.aiSelectable)) return false;
			if (this.taxonomyAIFilter === 'blocked' && item.enabled && item.aiSelectable) return false;
			const usage = this.itemUsage(item).effective;
			if (this.taxonomyUsageFilter === 'used' && (usage === null || usage === 0)) return false;
			if (this.taxonomyUsageFilter === 'unused' && usage !== 0) return false;
			return true;
		});
	}

	private renderTaxonomyBulkActions(container: HTMLElement, filtered: TaxonomyItem[]) {
		const bar = container.createDiv({ cls: 'vermilion-bulk-bar' });
		const selectAll = bar.createEl('input', { type: 'checkbox' });
		selectAll.checked = filtered.length > 0 && filtered.every((item) => this.selectedTaxonomyItems.has(item));
		selectAll.indeterminate = filtered.some((item) => this.selectedTaxonomyItems.has(item)) && !selectAll.checked;
		selectAll.onchange = () => {
			for (const item of filtered) selectAll.checked ? this.selectedTaxonomyItems.add(item) : this.selectedTaxonomyItems.delete(item);
			this.render();
		};
		bar.createSpan({ text: `全选当前结果 · 已选择 ${this.selectedTaxonomyItems.size} 项` });
		for (const [label, action] of [['启用', 'enable'], ['停用', 'disable'], ['允许 AI', 'allow-ai'], ['禁止 AI', 'block-ai'], ['删除', 'delete']] as const) {
			const button = bar.createEl('button', { text: label, cls: action === 'delete' ? 'mod-warning' : '' });
			button.disabled = this.selectedTaxonomyItems.size === 0;
			button.onclick = () => this.applyTaxonomyBulkAction(action);
		}
	}

	private renderTaxonomyRow(container: HTMLElement, item: TaxonomyItem) {
		const usage = this.itemUsage(item);
		const row = container.createDiv({ cls: 'vermilion-taxonomy-row' });
		const selected = row.createEl('input', { type: 'checkbox' });
		selected.checked = this.selectedTaxonomyItems.has(item);
		selected.onchange = () => { selected.checked ? this.selectedTaxonomyItems.add(item) : this.selectedTaxonomyItems.delete(item); this.render(); };
		const identity = row.createDiv({ cls: 'vermilion-taxonomy-identity' });
		const name = identity.createEl('input', { type: 'text', value: item.name });
		name.placeholder = this.isTag(item) ? '标签名称' : '分类名称';
		name.oninput = () => { item.name = name.value; this.markTaxonomyDirty(); };
		const details = row.createDiv({ cls: 'vermilion-taxonomy-details' });
		const description = details.createEl('input', { type: 'text', value: item.description });
		description.placeholder = '告诉 AI 何时使用';
		description.oninput = () => { item.description = description.value; this.markTaxonomyDirty(); };
		if (this.isTag(item)) {
			const aliases = details.createEl('input', { type: 'text', value: item.aliases.join(', ') });
			aliases.placeholder = '别名（逗号分隔）';
			aliases.oninput = () => { item.aliases = aliases.value.split(/[,，]/).map((value) => value.trim()).filter(Boolean); this.markTaxonomyDirty(); };
		}
		row.createDiv({ cls: 'vermilion-usage-cell', text: this.usageLabel(usage.draft, usage.published) });
		const enabled = row.createEl('input', { type: 'checkbox' });
		enabled.checked = item.enabled;
		enabled.title = '启用后可由用户手动选择';
		enabled.onchange = () => { item.enabled = enabled.checked; if (!item.enabled) item.aiSelectable = false; this.markTaxonomyDirty(); this.render(); };
		const aiSelectable = row.createEl('input', { type: 'checkbox' });
		aiSelectable.checked = item.enabled && item.aiSelectable;
		aiSelectable.disabled = !item.enabled;
		aiSelectable.title = item.enabled ? '允许 AI 主动选择' : '请先启用该项目';
		aiSelectable.onchange = () => { item.aiSelectable = aiSelectable.checked; this.markTaxonomyDirty(); };
		const remove = row.createEl('button', { text: '删除' });
		remove.disabled = usage.effective === null || usage.effective > 0;
		remove.title = usage.effective === null ? '无法确认使用次数，已禁止删除。' : usage.effective > 0 ? '当前仍有文章使用，建议先停用。' : '删除未使用项目';
		remove.onclick = () => this.removeTaxonomyItem(item);
	}

	private markTaxonomyDirty() {
		this.taxonomyDirty = Boolean(this.taxonomy && JSON.stringify(this.taxonomy) !== this.taxonomyBaseline);
		const indicator = this.contentEl.querySelector('.vermilion-section-heading > span');
		if (indicator instanceof HTMLElement) {
			indicator.setText(this.taxonomyDirty ? '● 有未保存修改' : '已与服务器同步');
			indicator.classList.toggle('vermilion-dirty', this.taxonomyDirty);
			indicator.classList.toggle('vermilion-synced', !this.taxonomyDirty);
		}
		const saveBar = this.contentEl.querySelector('.vermilion-save-bar');
		if (saveBar instanceof HTMLElement) {
			saveBar.classList.toggle('is-dirty', this.taxonomyDirty);
			const message = saveBar.querySelector('span');
			if (message instanceof HTMLElement) message.setText(this.taxonomyDirty ? '● 分类体系有尚未保存的修改' : '没有尚未保存的修改');
			const save = saveBar.querySelector('button.mod-cta');
			if (save instanceof HTMLButtonElement) save.disabled = !this.taxonomyDirty;
		}
	}

	private removeTaxonomyItem(item: TaxonomyItem) {
		if (!this.taxonomy || this.itemUsage(item).effective !== 0 || !window.confirm(`确认删除“${item.name || '未命名项目'}”吗？`)) return;
		if (this.isTag(item)) this.taxonomy.tags = this.taxonomy.tags.filter((value) => value !== item);
		else this.taxonomy.categories = this.taxonomy.categories.filter((value) => value !== item);
		this.selectedTaxonomyItems.delete(item);
		this.markTaxonomyDirty();
		this.render();
	}

	private applyTaxonomyBulkAction(action: 'enable' | 'disable' | 'allow-ai' | 'block-ai' | 'delete') {
		if (!this.taxonomy) return;
		const items = Array.from(this.selectedTaxonomyItems);
		let changed = 0;
		let blocked = 0;
		if (action === 'delete' && !window.confirm(`准备删除 ${items.length} 个项目。仍被文章使用的项目会保留，是否继续？`)) return;
		for (const item of items) {
			if (action === 'enable') { item.enabled = true; changed++; }
			if (action === 'disable') { item.enabled = false; item.aiSelectable = false; changed++; }
			if (action === 'allow-ai') { if (item.enabled) { item.aiSelectable = true; changed++; } else blocked++; }
			if (action === 'block-ai') { item.aiSelectable = false; changed++; }
			if (action === 'delete') {
				if (this.itemUsage(item).effective !== 0) { blocked++; continue; }
				if (this.isTag(item)) this.taxonomy.tags = this.taxonomy.tags.filter((value) => value !== item);
				else this.taxonomy.categories = this.taxonomy.categories.filter((value) => value !== item);
				changed++;
			}
		}
		this.selectedTaxonomyItems.clear();
		this.markTaxonomyDirty();
		this.render();
		new Notice(`批量操作完成：修改 ${changed} 项${blocked ? `，跳过 ${blocked} 项` : ''}。`);
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

	private suggestionEntries(): SuggestionEntry[] {
		const entries: SuggestionEntry[] = [];
		for (const { article, proposal } of this.collectProposals()) entries.push({ key: `tag:${article.id}:${proposal.name}`, kind: 'tag', article, proposal });
		for (const { article, proposal } of this.collectCategoryProposals()) entries.push({ key: `category:${article.id}:${proposal.name}`, kind: 'category', article, proposal });
		return entries;
	}

	private renderAISuggestions(container: HTMLElement) {
		const section = container.createDiv({ cls: 'vermilion-taxonomy vermilion-suggestions' });
		section.createEl('h3', { text: 'AI 建议审批' });
		section.createEl('p', { text: '这里只审批 AI 提出的新分类和新标签。运行 AI 分析仍需在文章管理中主动点击。' });
		const entries = this.suggestionEntries();
		const tabs = section.createDiv({ cls: 'vermilion-taxonomy-tabs' });
		for (const [label, value] of [['全部', 'all'], ['新标签', 'tags'], ['新分类', 'categories']] as const) {
			const count = value === 'all' ? entries.length : entries.filter((entry) => entry.kind === (value === 'tags' ? 'tag' : 'category')).length;
			const button = tabs.createEl('button', { text: `${label} (${count})`, cls: this.suggestionFilter === value ? 'mod-cta' : '' });
			button.onclick = () => { this.suggestionFilter = value; this.selectedAISuggestions.clear(); this.render(); };
		}
		const filtered = entries.filter((entry) => this.suggestionFilter === 'all' || entry.kind === (this.suggestionFilter === 'tags' ? 'tag' : 'category'));
		if (!filtered.length) {
			section.createDiv({ cls: 'vermilion-empty', text: entries.length ? '当前筛选没有待审批建议。' : '目前没有 AI 提出的新分类或新标签。' });
			return;
		}
		const bulk = section.createDiv({ cls: 'vermilion-bulk-bar' });
		const selectAll = bulk.createEl('input', { type: 'checkbox' });
		selectAll.checked = filtered.every((entry) => this.selectedAISuggestions.has(entry.key));
		selectAll.indeterminate = filtered.some((entry) => this.selectedAISuggestions.has(entry.key)) && !selectAll.checked;
		selectAll.onchange = () => {
			for (const entry of filtered) selectAll.checked ? this.selectedAISuggestions.add(entry.key) : this.selectedAISuggestions.delete(entry.key);
			this.render();
		};
		bulk.createSpan({ text: `全选当前结果 · 已选择 ${this.selectedAISuggestions.size} 项` });
		const approveAll = bulk.createEl('button', { text: '批量批准', cls: 'mod-cta' });
		approveAll.disabled = this.selectedAISuggestions.size === 0;
		approveAll.onclick = () => void this.applySuggestionBulkAction('approve');
		const rejectAll = bulk.createEl('button', { text: '批量拒绝' });
		rejectAll.disabled = this.selectedAISuggestions.size === 0;
		rejectAll.onclick = () => void this.applySuggestionBulkAction('reject');

		const table = section.createDiv({ cls: 'vermilion-suggestion-table' });
		const header = table.createDiv({ cls: 'vermilion-suggestion-row is-header' });
		for (const label of ['选择', '类型', '建议名称', '来源与理由', '替换为已有项目', '操作']) header.createSpan({ text: label });
		for (const entry of filtered) this.renderSuggestionRow(table, entry);
	}

	private renderSuggestionRow(container: HTMLElement, entry: SuggestionEntry) {
		const row = container.createDiv({ cls: 'vermilion-suggestion-row' });
		const selected = row.createEl('input', { type: 'checkbox' });
		selected.checked = this.selectedAISuggestions.has(entry.key);
		selected.onchange = () => { selected.checked ? this.selectedAISuggestions.add(entry.key) : this.selectedAISuggestions.delete(entry.key); this.render(); };
		row.createSpan({ text: entry.kind === 'tag' ? '标签' : '分类', cls: 'vermilion-type-badge' });
		row.createEl('strong', { text: entry.proposal.name });
		const source = row.createDiv({ cls: 'vermilion-suggestion-source' });
		source.createSpan({ text: entry.article.metadata.title || entry.article.filename });
		source.createEl('small', { text: entry.proposal.reason || '未提供理由' });
		const replacement = row.createEl('select');
		replacement.createEl('option', { text: '选择已有项目', value: '' });
		if (entry.kind === 'tag') {
			for (const tag of this.taxonomy?.tags.filter((item) => item.enabled) ?? []) replacement.createEl('option', { text: tag.name, value: tag.name });
		} else {
			for (const category of this.taxonomy?.categories.filter((item) => item.enabled) ?? []) replacement.createEl('option', { text: category.name, value: category.name });
		}
		const actions = row.createDiv({ cls: 'vermilion-row-actions' });
		actions.createEl('button', { text: '批准', cls: 'mod-cta' }).onclick = () => void this.approveSuggestionEntry(entry);
		const replace = actions.createEl('button', { text: '替换' });
		replace.disabled = replacement.options.length <= 1;
		replace.onclick = () => {
			if (!replacement.value) { new Notice('请先选择一个已有项目。'); return; }
			this.selectedAISuggestions.delete(entry.key);
			if (entry.kind === 'tag') this.resolveProposal(entry.article, entry.proposal.name, replacement.value);
			else this.resolveCategoryProposal(entry.article, replacement.value);
		};
		actions.createEl('button', { text: '拒绝' }).onclick = () => {
			this.selectedAISuggestions.delete(entry.key);
			if (entry.kind === 'tag') this.resolveProposal(entry.article, entry.proposal.name);
			else this.resolveCategoryProposal(entry.article);
		};
	}

	private async approveSuggestionEntry(entry: SuggestionEntry) {
		if (entry.kind === 'tag') await this.approveProposedTag(entry.article, entry.proposal.name);
		else await this.approveProposedCategory(entry.article, entry.proposal as ProposedCategory);
		this.selectedAISuggestions.delete(entry.key);
	}

	private async applySuggestionBulkAction(action: 'approve' | 'reject') {
		const entries = this.suggestionEntries().filter((entry) => this.selectedAISuggestions.has(entry.key));
		if (!entries.length) return;
		if (!window.confirm(`${action === 'approve' ? '批准' : '拒绝'}所选 ${entries.length} 条 AI 建议吗？`)) return;
		for (const entry of entries) {
			if (action === 'approve') await this.approveSuggestionEntry(entry);
			else if (entry.kind === 'tag') this.resolveProposal(entry.article, entry.proposal.name);
			else this.resolveCategoryProposal(entry.article);
		}
		this.selectedAISuggestions.clear();
		this.render();
		new Notice(`已${action === 'approve' ? '批准' : '拒绝'} ${entries.length} 条 AI 建议。`);
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
			category = { id: '', name: proposal.name, description: proposal.reason, enabled: true, aiSelectable: true };
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
		this.taxonomy.version = Math.max(2, this.taxonomy.version || 0);
		for (const item of [...this.taxonomy.categories, ...this.taxonomy.tags]) {
			if (!item.enabled) item.aiSelectable = false;
		}
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
			this.taxonomyBaseline = JSON.stringify(this.taxonomy);
			this.taxonomyDirty = false;
			this.selectedTaxonomyItems.clear();
			this.taxonomyOriginalNames = new WeakMap<TaxonomyItem, string>();
			for (const item of [...this.taxonomy.categories, ...this.taxonomy.tags]) this.taxonomyOriginalNames.set(item, item.name);
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
			new Notice(`保存分类体系失败：${describeApiError(error)}`);
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
			new Notice(`审核草稿已保存到本地和服务器任务：${updated.metadata.title}`);
		} catch (error) {
			new Notice(`保存失败：${(error as Error).message}`);
		}
	}

	private async publishSelected() {
		if (!this.job?.articles) return;
		const articles = this.job.articles.filter((article) => this.selected.has(article.id));
		if (articles.some((article) => this.dirtyArticles.has(article.id))) {
			new Notice('所选文章存在未保存修改，请先保存审核草稿。');
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
