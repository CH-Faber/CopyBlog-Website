import { App, Notice, Plugin, PluginSettingTab, Setting, WorkspaceLeaf, requestUrl } from 'obsidian';
import { ApiClient, describeApiError } from './src/api-client';
import { ArticleManagerView, ARTICLE_MANAGER_VIEW } from './src/article-manager-view';
import type { SyncSettings } from './src/models';

const DEFAULT_AI_SYSTEM_PROMPT = '你是“一个闪念”的文章编辑助手。请准确、克制地整理文章信息，不要虚构文章中不存在的事实。';
const DEFAULT_AI_METADATA_PROMPT = '根据文章正文生成简洁摘要，并从已有分类和标签中选择最合适的项目。输出必须符合插件要求的 JSON 格式。';
const DEFAULT_AI_TAG_RULES = '优先选择已有标签。只有现有标签确实无法表达文章主题时才提出新标签；新标签应简短、稳定、可复用，避免同义词和过细的临时标签。';

const DEFAULT_SETTINGS: SyncSettings = {
	syncEndpoint: 'http://localhost:3001/api/sync',
	webhookSecret: '',
	localPostsFolder: '',
	activeJobId: '',
	aiBaseUrl: 'https://api.openai.com/v1',
	aiApiKey: '',
	aiModel: 'gpt-4o-mini',
	aiSystemPrompt: DEFAULT_AI_SYSTEM_PROMPT,
	aiMetadataPrompt: DEFAULT_AI_METADATA_PROMPT,
	aiTagRules: DEFAULT_AI_TAG_RULES,
	aiMaxProposedTags: 3,
	aiSuggestionJobId: '',
	aiSuggestions: {},
};

export default class SyncPlugin extends Plugin {
	settings!: SyncSettings;
	api!: ApiClient;

	async onload() {
		await this.loadSettings();
		this.api = new ApiClient(this.settings);
		this.registerView(ARTICLE_MANAGER_VIEW, (leaf) => new ArticleManagerView(leaf, this));

		this.addRibbonIcon('layout-dashboard', '一个闪念：内容管理', () => {
			void this.activateManagerView();
		});
		this.addCommand({
			id: 'open-flash-thought-content-manager',
			name: '打开“一个闪念”内容管理',
			callback: () => void this.activateManagerView(),
		});
		this.addCommand({
			id: 'prepare-flash-thought-sync',
			name: '获取并处理文章',
			callback: async () => {
				await this.activateManagerView();
				const view = this.app.workspace.getLeavesOfType(ARTICLE_MANAGER_VIEW)[0]?.view;
				if (view instanceof ArticleManagerView) await view.prepareSync();
			},
		});
		this.addSettingTab(new SyncSettingTab(this.app, this));
	}

	async onunload() {
		this.app.workspace.detachLeavesOfType(ARTICLE_MANAGER_VIEW);
	}

	async loadSettings() {
		this.settings = Object.assign({}, DEFAULT_SETTINGS, await this.loadData());
	}

	async saveSettings() {
		await this.saveData(this.settings);
		this.api = new ApiClient(this.settings);
	}

	async testServerConnection() {
		if (!this.settings.syncEndpoint.trim()) throw new Error('请先填写同步服务地址。');
		if (!this.settings.webhookSecret.trim()) throw new Error('请先填写 Webhook Secret。');
		const taxonomy = await this.api.testConnection();
		return `连接成功：已读取 ${taxonomy.tags.length} 个标签、${taxonomy.categories.length} 个分类。`;
	}

	async testAIConnection() {
		const baseUrl = this.settings.aiBaseUrl.trim().replace(/\/+$/, '');
		const apiKey = this.settings.aiApiKey.trim();
		const model = this.settings.aiModel.trim();
		if (!baseUrl) throw new Error('请先填写 AI API 地址。');
		if (!apiKey) throw new Error('请先填写 AI API Key。');
		if (!model) throw new Error('请先填写模型名称。');
		const endpoint = /\/chat\/completions$/i.test(baseUrl) ? baseUrl : `${baseUrl}/chat/completions`;
		const response = await requestUrl({
			url: endpoint,
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				Authorization: `Bearer ${apiKey}`,
			},
			body: JSON.stringify({
				model,
				messages: [
					{ role: 'system', content: this.settings.aiSystemPrompt || DEFAULT_AI_SYSTEM_PROMPT },
					{ role: 'user', content: '这是连接测试。请只回复 OK。' },
				],
				max_tokens: 8,
			}),
			throw: false,
		});
		if (response.status < 200 || response.status >= 300) {
			let message = response.text || `HTTP ${response.status}`;
			try {
				const data = JSON.parse(response.text) as { error?: { message?: string }; message?: string };
				message = data.error?.message ?? data.message ?? message;
			} catch {
				// Keep the provider's plain-text error response.
			}
			throw new Error(`HTTP ${response.status}：${message}`);
		}
		return `AI 连接成功，模型：${model}`;
	}

	async activateManagerView() {
		let leaf: WorkspaceLeaf | undefined = this.app.workspace.getLeavesOfType(ARTICLE_MANAGER_VIEW)[0];
		if (!leaf) {
			leaf = this.app.workspace.getLeaf(true);
			await leaf.setViewState({ type: ARTICLE_MANAGER_VIEW, active: true });
		}
		this.app.workspace.revealLeaf(leaf);
	}
}

class SyncSettingTab extends PluginSettingTab {
	plugin: SyncPlugin;

	constructor(app: App, plugin: SyncPlugin) {
		super(app, plugin);
		this.plugin = plugin;
	}

	display(): void {
		const { containerEl } = this;
		containerEl.empty();
		containerEl.createEl('h2', { text: '一个闪念 · 内容管理' });
		containerEl.createEl('h3', { text: '服务器连接' });

		new Setting(containerEl)
			.setName('同步服务地址')
			.setDesc('可以填写服务器根地址、/api/sync 或 /api/v1。')
			.addText((text) => text.setPlaceholder('https://example.com/api/sync').setValue(this.plugin.settings.syncEndpoint).onChange(async (value) => {
				this.plugin.settings.syncEndpoint = value.trim();
				await this.plugin.saveSettings();
			}));

		new Setting(containerEl)
			.setName('Webhook Secret')
			.setDesc('服务器 WEBHOOK_SECRET，用于验证管理和发布请求；它与 AI Key 相互独立。')
			.addText((text) => {
				text.inputEl.type = 'password';
				text.setPlaceholder('输入服务器密钥').setValue(this.plugin.settings.webhookSecret).onChange(async (value) => {
					this.plugin.settings.webhookSecret = value.trim();
					await this.plugin.saveSettings();
				});
			});

		new Setting(containerEl)
			.setName('连接测试')
			.setDesc('验证服务地址和 Webhook Secret，并读取标签库。')
			.addButton((button) => button.setButtonText('测试服务器连接').setCta().onClick(async () => {
				button.setDisabled(true).setButtonText('测试中…');
				try {
					new Notice(await this.plugin.testServerConnection());
				} catch (error) {
					new Notice(`服务器连接失败：${describeApiError(error)}`);
				} finally {
					button.setDisabled(false).setButtonText('测试服务器连接');
				}
			}));

		new Setting(containerEl)
			.setName('本地文章目录')
			.setDesc('Obsidian Vault 内保存博客文章的目录，例如 Blog/Posts。留空表示 Vault 根目录。')
			.addText((text) => text.setPlaceholder('Blog/Posts').setValue(this.plugin.settings.localPostsFolder).onChange(async (value) => {
				this.plugin.settings.localPostsFolder = value.replace(/^\/+|\/+$/g, '');
				await this.plugin.saveSettings();
			}));

		containerEl.createEl('h3', { text: '本地 AI 配置' });
		containerEl.createEl('p', {
			text: 'AI 请求将由 Obsidian 直接发送。API Key 只保存在本机插件 data.json 中，不会上传到服务器或 Git；该文件不是加密保险库，请确保设备和 Vault 可信。',
			cls: 'setting-item-description',
		});

		new Setting(containerEl)
			.setName('AI API 地址')
			.setDesc('OpenAI 兼容接口的 Base URL，例如 https://api.openai.com/v1。')
			.addText((text) => text.setPlaceholder('https://api.openai.com/v1').setValue(this.plugin.settings.aiBaseUrl).onChange(async (value) => {
				this.plugin.settings.aiBaseUrl = value.trim();
				await this.plugin.saveSettings();
			}));

		new Setting(containerEl)
			.setName('AI API Key')
			.setDesc('仅保存在 Obsidian 插件的本地配置中。')
			.addText((text) => {
				text.inputEl.type = 'password';
				text.setPlaceholder('输入 AI API Key').setValue(this.plugin.settings.aiApiKey).onChange(async (value) => {
					this.plugin.settings.aiApiKey = value.trim();
					await this.plugin.saveSettings();
				});
			});

		new Setting(containerEl)
			.setName('模型')
			.setDesc('填写服务商支持的模型 ID。')
			.addText((text) => text.setPlaceholder('gpt-4o-mini').setValue(this.plugin.settings.aiModel).onChange(async (value) => {
				this.plugin.settings.aiModel = value.trim();
				await this.plugin.saveSettings();
			}));

		this.addPromptSetting(containerEl, '系统提示词', '规定 AI 的身份、语气和基本边界。', 'aiSystemPrompt');
		this.addPromptSetting(containerEl, '文章信息提示词', '规定标题、摘要和分类等信息如何生成。', 'aiMetadataPrompt');
		this.addPromptSetting(containerEl, '标签规则', '限制 AI 如何选择已有标签及提出新标签。', 'aiTagRules');

		new Setting(containerEl)
			.setName('最多提出的新标签数')
			.setDesc('限制单篇文章中 AI 可提出的新标签数量，范围 0–10。')
			.addText((text) => {
				text.inputEl.type = 'number';
				text.inputEl.min = '0';
				text.inputEl.max = '10';
				text.setValue(String(this.plugin.settings.aiMaxProposedTags)).onChange(async (value) => {
					const parsed = Number.parseInt(value, 10);
					if (Number.isFinite(parsed)) {
						this.plugin.settings.aiMaxProposedTags = Math.max(0, Math.min(10, parsed));
						await this.plugin.saveSettings();
					}
				});
			});

		new Setting(containerEl)
			.setName('AI 配置操作')
			.setDesc('连接测试会发送一条极短的测试消息；恢复默认值不会清除 API Key。')
			.addButton((button) => button.setButtonText('测试 AI 连接').setCta().onClick(async () => {
				button.setDisabled(true).setButtonText('测试中…');
				try {
					new Notice(await this.plugin.testAIConnection());
				} catch (error) {
					new Notice(`AI 连接失败：${error instanceof Error ? error.message : String(error)}`);
				} finally {
					button.setDisabled(false).setButtonText('测试 AI 连接');
				}
			}))
			.addButton((button) => button.setButtonText('恢复默认提示词').setWarning().onClick(async () => {
				if (!window.confirm('确认恢复默认提示词和新标签数量吗？AI API Key 不会被清除。')) return;
				this.plugin.settings.aiSystemPrompt = DEFAULT_AI_SYSTEM_PROMPT;
				this.plugin.settings.aiMetadataPrompt = DEFAULT_AI_METADATA_PROMPT;
				this.plugin.settings.aiTagRules = DEFAULT_AI_TAG_RULES;
				this.plugin.settings.aiMaxProposedTags = DEFAULT_SETTINGS.aiMaxProposedTags;
				await this.plugin.saveSettings();
				this.display();
				new Notice('已恢复默认提示词。');
			}));
	}

	private addPromptSetting(containerEl: HTMLElement, name: string, description: string, key: 'aiSystemPrompt' | 'aiMetadataPrompt' | 'aiTagRules') {
		new Setting(containerEl)
			.setName(name)
			.setDesc(description)
			.addTextArea((text) => {
				text.inputEl.rows = 5;
				text.setValue(this.plugin.settings[key]).onChange(async (value) => {
					this.plugin.settings[key] = value;
					await this.plugin.saveSettings();
				});
			});
	}
}
