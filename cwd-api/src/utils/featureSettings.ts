import { Bindings } from '../bindings';

export const FEATURE_COMMENT_LIKE_KEY = 'comment_feature_comment_like';
export const FEATURE_ARTICLE_LIKE_KEY = 'comment_feature_article_like';

export type FeatureSettings = {
	enableCommentLike: boolean;
	enableArticleLike: boolean;
};

export async function loadFeatureSettings(env: Bindings): Promise<FeatureSettings> {
	await env.CWD_DB.prepare(
		'CREATE TABLE IF NOT EXISTS Settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)'
	).run();

	const keys = [FEATURE_COMMENT_LIKE_KEY, FEATURE_ARTICLE_LIKE_KEY];
	const { results } = await env.CWD_DB.prepare(
		'SELECT key, value FROM Settings WHERE key IN (?, ?)'
	)
		.bind(...keys)
		.all<{ key: string; value: string }>();

	const map = new Map<string, string>();
	for (const row of results) {
		if (row && row.key) {
			map.set(row.key, row.value);
		}
	}

	// Default to true if not set, or false?
	// Usually features might be enabled by default.
	// But let's check the user requirement. "New settings... whether to enable..."
	// If I default to false, existing users might lose features if they were implicit.
	// But these are "new" settings.
	// "comment likes" and "article likes" existed before?
	// The code shows `like.ts` and `likeComment.ts`.
	// `likePage` handler in `index.ts`.
	// So the features exist. To avoid breaking changes, I should probably default to TRUE.
	// But wait, if I default to true, then the user has to manually turn them off.
	// If I default to false, they disappear.
	// Given "whether to enable" implies they might be optional now.
	// I'll default to TRUE to maintain backward compatibility (features visible by default).

	const enableCommentLikeRaw = map.get(FEATURE_COMMENT_LIKE_KEY);
	const enableCommentLike = enableCommentLikeRaw !== '0'; // Default to true if missing or '1'

	const enableArticleLikeRaw = map.get(FEATURE_ARTICLE_LIKE_KEY);
	const enableArticleLike = enableArticleLikeRaw !== '0'; // Default to true if missing or '1'

	return {
		enableCommentLike,
		enableArticleLike
	};
}

export async function saveFeatureSettings(
	env: Bindings,
	settings: Partial<FeatureSettings>
) {
	await env.CWD_DB.prepare(
		'CREATE TABLE IF NOT EXISTS Settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)'
	).run();

	const entries: { key: string; value: string | undefined }[] = [
		{
			key: FEATURE_COMMENT_LIKE_KEY,
			value:
				typeof settings.enableCommentLike === 'boolean'
					? settings.enableCommentLike
						? '1'
						: '0'
					: undefined
		},
		{
			key: FEATURE_ARTICLE_LIKE_KEY,
			value:
				typeof settings.enableArticleLike === 'boolean'
					? settings.enableArticleLike
						? '1'
						: '0'
					: undefined
		}
	];

	for (const entry of entries) {
		if (entry.value !== undefined) {
			await env.CWD_DB.prepare('REPLACE INTO Settings (key, value) VALUES (?, ?)')
				.bind(entry.key, entry.value)
				.run();
		}
	}
}
