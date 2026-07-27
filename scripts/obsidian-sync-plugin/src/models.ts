export interface SyncSettings {
	syncEndpoint: string;
	webhookSecret: string;
	localPostsFolder: string;
	activeJobId: string;
	aiBaseUrl: string;
	aiApiKey: string;
	aiModel: string;
	aiSystemPrompt: string;
	aiMetadataPrompt: string;
	aiTagRules: string;
	aiMaxProposedTags: number;
	aiSuggestionJobId: string;
	aiSuggestions: Record<string, AISuggestion>;
}

export interface ArticleMetadata {
	title: string;
	published: string;
	updated?: string;
	draft?: boolean;
	description?: string;
	image?: string;
	tags: string[];
	category?: string;
	lang?: string;
	pinned?: boolean;
	encrypted?: boolean;
	password?: string;
	disclaimer?: unknown;
	extra?: Record<string, unknown>;
}

export interface ProposedTag { name: string; reason: string; }
export interface ProposedCategory { name: string; reason: string; }
export interface AISuggestion {
	description: string;
	category: string;
	proposedCategory?: ProposedCategory | null;
	selectedTags: string[];
	proposedTags: ProposedTag[];
}

export type ArticleStatus = 'new' | 'modified' | 'unchanged' | 'deleted' | 'conflict' | 'approved' | 'rejected' | 'published';

export interface ArticleDraft {
	id: string;
	path: string;
	filename: string;
	status: ArticleStatus;
	metadata: ArticleMetadata;
	content: string;
	originalMetadata: ArticleMetadata;
	originalContent: string;
	aiSuggestion?: AISuggestion;
	sourceHash: string;
	originalHash?: string;
	clientHash?: string;
	currentHash: string;
	revision: number;
	error?: string;
}

export interface SyncJob {
	id: string;
	status: 'queued' | 'syncing' | 'analyzing' | 'awaiting_review' | 'publishing' | 'published' | 'failed';
	progress: number;
	message: string;
	createdAt: string;
	updatedAt: string;
	publishedSha?: string;
	articles?: ArticleDraft[];
	errors?: string[];
}

export interface ManagedTag {
	id: string;
	name: string;
	aliases: string[];
	description: string;
	enabled: boolean;
	aiSelectable: boolean;
	createdAt: string;
	updatedAt: string;
}

export interface Category { id: string; name: string; description: string; enabled: boolean; aiSelectable: boolean; }
export interface Taxonomy { version: number; categories: Category[]; tags: ManagedTag[]; }
export interface TaxonomyUsage { tags: Record<string, number>; categories: Record<string, number>; }
export interface LocalFile { path: string; hash: string; }
