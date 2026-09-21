import { ItemView, Notice, WorkspaceLeaf } from 'obsidian';
import type SyncPlugin from '../main';
import { describeOrganizerError } from './organizer-api-client';
import type { OrganizerCapture, OrganizerItem } from './organizer-models';

export const ORGANIZER_VIEW = 'faber-organizer-view';

type OrganizerGroup = { title: string; items: OrganizerItem[]; className?: string };

function itemTime(item: OrganizerItem): string {
	return item.startAt || item.dueAt || item.reminderAt || '';
}

function localDay(date = new Date()): Date {
	return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

function groupItems(items: OrganizerItem[]): OrganizerGroup[] {
	const today = localDay();
	const tomorrow = new Date(today);
	tomorrow.setDate(tomorrow.getDate() + 1);
	const afterTomorrow = new Date(tomorrow);
	afterTomorrow.setDate(afterTomorrow.getDate() + 1);
	const active = items.filter((item) => item.status !== 'done' && item.status !== 'cancelled');
	const overdue: OrganizerItem[] = [];
	const current: OrganizerItem[] = [];
	const next: OrganizerItem[] = [];
	const unscheduled: OrganizerItem[] = [];
	for (const item of active) {
		const value = itemTime(item);
		if (!value) unscheduled.push(item);
		else {
			const date = new Date(value);
			if (date < today) overdue.push(item);
			else if (date < tomorrow) current.push(item);
			else if (date < afterTomorrow) next.push(item);
		}
	}
	return [
		{ title: '今天', items: current },
		{ title: '已逾期', items: overdue, className: 'is-overdue' },
		{ title: '明天', items: next },
		{ title: '未安排', items: unscheduled },
	];
}

export class OrganizerView extends ItemView {
	private readonly plugin: SyncPlugin;
	private items: OrganizerItem[] = [];
	private captures: OrganizerCapture[] = [];
	private loading = false;

	constructor(leaf: WorkspaceLeaf, plugin: SyncPlugin) {
		super(leaf);
		this.plugin = plugin;
	}

	getViewType() { return ORGANIZER_VIEW; }
	getDisplayText() { return '一个闪念 · 今日事项'; }
	getIcon() { return 'calendar-check'; }

	async onOpen() { await this.refresh(); }

	async refresh() {
		if (this.loading) return;
		this.loading = true;
		this.render();
		try {
			const [itemResponse, captureResponse] = await Promise.all([
				this.plugin.organizerApi.listItems(),
				this.plugin.organizerApi.listCaptures(),
			]);
			this.items = itemResponse.items;
			this.captures = captureResponse.captures;
		} catch (error) {
			new Notice(describeOrganizerError(error));
		} finally {
			this.loading = false;
			this.render();
		}
	}

	private render() {
		const container = this.containerEl.children[1] as HTMLElement;
		container.empty();
		container.addClass('faber-organizer');
		const header = container.createDiv({ cls: 'faber-organizer-header' });
		header.createEl('h2', { text: '一个闪念 · 今日事项' });
		const refresh = header.createEl('button', { text: this.loading ? '刷新中…' : '刷新' });
		refresh.disabled = this.loading;
		refresh.addEventListener('click', () => void this.refresh());

		if (this.loading && !this.items.length) {
			container.createDiv({ text: '正在读取事项…', cls: 'faber-organizer-empty' });
			return;
		}

		for (const group of groupItems(this.items)) {
			if (!group.items.length) continue;
			const section = container.createEl('section', { cls: `faber-organizer-section ${group.className || ''}` });
			section.createEl('h3', { text: `${group.title} · ${group.items.length}` });
			for (const item of group.items) this.renderItem(section, item);
		}

		const pending = this.captures.filter((capture) => capture.status !== 'confirmed');
		const inbox = container.createEl('section', { cls: 'faber-organizer-section' });
		inbox.createEl('h3', { text: `待整理 · ${pending.length}` });
		if (!pending.length) inbox.createDiv({ text: '没有等待整理的内容。', cls: 'faber-organizer-empty compact' });
		for (const capture of pending) {
			const card = inbox.createDiv({ cls: 'faber-organizer-capture' });
			card.createDiv({ text: capture.rawText || '图片事项' });
			card.createEl('small', { text: capture.error ? `${capture.status} · ${capture.error}` : capture.status });
		}

		if (!this.items.length && !pending.length) container.createDiv({ text: '还没有事项。可从命令面板发送选中文字或当前笔记。', cls: 'faber-organizer-empty' });
	}

	private renderItem(parent: HTMLElement, item: OrganizerItem) {
		const row = parent.createDiv({ cls: 'faber-organizer-item' });
		const body = row.createDiv({ cls: 'faber-organizer-item-body' });
		body.createEl('strong', { text: item.title });
		const value = itemTime(item);
		if (value) body.createEl('small', { text: new Date(value).toLocaleString('zh-CN') });
		if (item.description) body.createDiv({ text: item.description, cls: 'faber-organizer-description' });
		const actions = row.createDiv({ cls: 'faber-organizer-actions' });
		const done = actions.createEl('button', { text: '完成' });
		done.addEventListener('click', () => void this.runItemAction(() => this.plugin.organizerApi.completeItem(item.id)));
		const snooze = actions.createEl('button', { text: '稍后 10 分钟' });
		snooze.addEventListener('click', () => void this.runItemAction(() => this.plugin.organizerApi.snoozeItem(item.id, 10)));
	}

	private async runItemAction(action: () => Promise<OrganizerItem>) {
		try {
			await action();
			await this.refresh();
		} catch (error) {
			new Notice(describeOrganizerError(error));
		}
	}
}

