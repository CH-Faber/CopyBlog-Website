import { App, Plugin, PluginSettingTab, Setting, WorkspaceLeaf } from 'obsidian';
import { ApiClient } from './src/api-client';
import { ArticleManagerView, ARTICLE_MANAGER_VIEW } from './src/article-manager-view';
import type { SyncSettings } from './src/models';

const DEFAULT_SETTINGS: SyncSettings = {
	syncEndpoint: 'http://localhost:3001/api/sync',
	webhookSecret: '',
	localPostsFolder: '',
	activeJobId: '',
};

export default class SyncPlugin extends Plugin {
	settings!: SyncSettings;
	api!: ApiClient;

	async onload() {
		await this.loadSettings();
		this.api = new ApiClient(this.settings);
		this.registerView(ARTICLE_MANAGER_VIEW, (leaf) => new ArticleManagerView(leaf, this));

		this.addRibbonIcon('layout-dashboard', 'VermilionVoid: 文章管理', () => {
			void this.activateManagerView();
		});
		this.addCommand({
			id: 'open-vermilion-void-manager',
			name: '打开文章管理器',
			callback: () => void this.activateManagerView(),
		});
		this.addCommand({
			id: 'prepare-vermilion-void-sync',
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
		containerEl.createEl('h2', { text: 'VermilionVoid 管理设置' });

		new Setting(containerEl)
			.setName('同步服务地址')
			.setDesc('可以填写服务器根地址、/api/sync 或 /api/v1。')
			.addText((text) => text.setPlaceholder('https://example.com/api/sync').setValue(this.plugin.settings.syncEndpoint).onChange(async (value) => {
				this.plugin.settings.syncEndpoint = value.trim();
				await this.plugin.saveSettings();
			}));

		new Setting(containerEl)
			.setName('Webhook Secret')
			.setDesc('服务器 WEBHOOK_SECRET，所有管理和发布请求均使用它。')
			.addText((text) => {
				text.inputEl.type = 'password';
				text.setPlaceholder('输入密钥').setValue(this.plugin.settings.webhookSecret).onChange(async (value) => {
					this.plugin.settings.webhookSecret = value;
					await this.plugin.saveSettings();
				});
			});

		new Setting(containerEl)
			.setName('本地文章目录')
			.setDesc('Obsidian Vault 内保存博客文章的目录，例如 Blog/Posts。留空表示 Vault 根目录。')
			.addText((text) => text.setPlaceholder('Blog/Posts').setValue(this.plugin.settings.localPostsFolder).onChange(async (value) => {
				this.plugin.settings.localPostsFolder = value.replace(/^\/+|\/+$/g, '');
				await this.plugin.saveSettings();
			}));
	}
}
