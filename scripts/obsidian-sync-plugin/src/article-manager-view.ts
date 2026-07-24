import { ItemView, MarkdownRenderer, Notice, TFile, WorkspaceLeaf, normalizePath, stringifyYaml } from 'obsidian';
import type SyncPlugin from '../main';
import type { ArticleDraft, LocalFile, ManagedTag, SyncJob, Taxonomy } from './models';

export const ARTICLE_MANAGER_VIEW = 'vermilion-void-article-manager';

async function sha256(value: string): Promise<string> {
	const bytes = new TextEncoder().encode(value);
	const digest = await crypto.subtle.digest('SHA-256', bytes);
	return Array.from(new Uint8Array(digest)).map((item) => item.toString(16).padStart(2, '0')).join('');
}

export class ArticleManagerView extends ItemView {
	private readonly plugin: SyncPlugin;
	private job: SyncJob | null = null;
	private taxonomy: Taxonomy | null = null;
	private activeArticleId = '';
	private selected = new Set<string>();
	private dirtyArticles = new Set<string>();
	private showTaxonomy = false;
	private pollTimer: number | null = null;

	constructor(leaf: WorkspaceLeaf, plugin: SyncPlugin) {
		super(leaf);
		this.plugin = plugin;
	}

	getViewType() { return ARTICLE_MANAGER_VIEW; }
	getDisplayText() { return 'VermilionVoid 文章管理'; }
	getIcon() { return 'layout-dashboard'; }

	async onOpen() {
		this.contentEl.addClass('vermilion-manager');
		try {
			this.taxonomy = await this.plugin.api.getTaxonomy();
			if (this.plugin.settings.activeJobId) {
				this.job = await this.plugin.api.getJob(this.plugin.settings.activeJobId);
				this.activeArticleId = this.job.articles?.[0]?.id ?? '';
			}
		} catch (error) {
			new Notice(`加载管理数据失败：${(error as Error).message}`);
		}
		this.render();
		this.schedulePoll();
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
		toolbar.createEl('button', { text: '刷新' }).onclick = () => void this.refreshJob();
		toolbar.createEl('button', { text: this.showTaxonomy ? '返回文章' : '标签管理' }).onclick = () => {
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

	private renderEditor(container: HTMLElement, article: ArticleDraft) {
		container.createEl('h3', { text: '编辑' });
		if (article.error) container.createDiv({ cls: 'vermilion-error', text: article.error });
		this.labeledInput(container, '标题', article.metadata.title ?? '', (value) => { article.metadata.title = value; this.dirtyArticles.add(article.id); });
		this.labeledInput(container, '摘要', article.metadata.description ?? '', (value) => { article.metadata.description = value; this.dirtyArticles.add(article.id); }, true);
		this.labeledInput(container, '分类', article.metadata.category ?? '', (value) => { article.metadata.category = value; this.dirtyArticles.add(article.id); });
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
			aiBox.createEl('p', { text: `已有标签：${article.aiSuggestion.selectedTags.join('、') || '无'}` });
			aiBox.createEl('button', { text: '采用已有建议' }).onclick = () => {
				if (article.aiSuggestion?.description) article.metadata.description = article.aiSuggestion.description;
				if (article.aiSuggestion?.category) article.metadata.category = article.aiSuggestion.category;
				article.metadata.tags = Array.from(new Set([...(article.metadata.tags ?? []), ...(article.aiSuggestion?.selectedTags ?? [])]));
				this.dirtyArticles.add(article.id);
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
		section.createEl('h3', { text: '标签管理' });
		section.createEl('p', { text: '启用且允许 AI 使用的标签会进入 AI 白名单。' });
		if (!this.taxonomy) {
			section.createDiv({ text: '标签库尚未加载。' });
			return;
		}
		for (const tag of this.taxonomy.tags) this.renderTagRow(section, tag);
		const actions = section.createDiv({ cls: 'vermilion-actions' });
		actions.createEl('button', { text: '新增标签' }).onclick = () => {
			this.taxonomy?.tags.push({ id: '', name: '新标签', aliases: [], description: '', enabled: true, aiSelectable: true, createdAt: '', updatedAt: '' });
			this.render();
		};
		actions.createEl('button', { text: '保存标签库', cls: 'mod-cta' }).onclick = () => void this.saveTaxonomy();
	}

	private renderTagRow(container: HTMLElement, tag: ManagedTag) {
		const row = container.createDiv({ cls: 'vermilion-tag-row' });
		const name = row.createEl('input', { type: 'text', value: tag.name });
		name.oninput = () => tag.name = name.value;
		const description = row.createEl('input', { type: 'text', value: tag.description });
		description.placeholder = '告诉 AI 何时使用这个标签';
		description.oninput = () => tag.description = description.value;
		for (const [label, key] of [['启用', 'enabled'], ['允许 AI', 'aiSelectable']] as const) {
			const wrapper = row.createEl('label');
			const checkbox = wrapper.createEl('input', { type: 'checkbox' });
			checkbox.checked = tag[key];
			checkbox.onchange = () => tag[key] = checkbox.checked;
			wrapper.appendText(label);
		}
	}

	private async approveProposedTag(article: ArticleDraft, name: string) {
		if (!this.taxonomy) return;
		if (!this.taxonomy.tags.some((tag) => tag.name.toLowerCase() === name.toLowerCase())) {
			this.taxonomy.tags.push({ id: '', name, aliases: [], description: '', enabled: true, aiSelectable: true, createdAt: '', updatedAt: '' });
			await this.saveTaxonomy(false);
		}
		article.metadata.tags = Array.from(new Set([...(article.metadata.tags ?? []), name]));
		this.dirtyArticles.add(article.id);
		if (article.aiSuggestion) article.aiSuggestion.proposedTags = article.aiSuggestion.proposedTags.filter((tag) => tag.name !== name);
		this.render();
		new Notice(`已批准标签“${name}”，保存文章后生效。`);
	}

	private async saveTaxonomy(notify = true) {
		if (!this.taxonomy) return;
		try {
			this.taxonomy = await this.plugin.api.saveTaxonomy(this.taxonomy);
			if (notify) new Notice('标签库已保存，随下一次发布进入 Git。');
			this.render();
		} catch (error) {
			new Notice(`保存标签库失败：${(error as Error).message}`);
		}
	}

	private async saveArticle(article: ArticleDraft) {
		if (!this.job) return;
		try {
			await this.assertLocalNotChanged(article);
			const updated = await this.plugin.api.updateArticle(this.job.id, article);
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
