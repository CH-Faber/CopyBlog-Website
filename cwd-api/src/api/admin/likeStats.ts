import type { Context } from 'hono';
import type { Bindings } from '../../bindings';

type LikeStatsItem = {
	page_slug: string;
	page_title: string | null;
	page_url: string | null;
	likes: number;
};

export const getLikeStats = async (c: Context<{ Bindings: Bindings }>) => {
	try {
		await c.env.CWD_DB.prepare(
			'CREATE TABLE IF NOT EXISTS Likes (id INTEGER PRIMARY KEY AUTOINCREMENT, page_slug TEXT NOT NULL, user_id TEXT NOT NULL, created_at INTEGER NOT NULL, UNIQUE(page_slug, user_id))'
		).run();

		await c.env.CWD_DB.prepare(
			'CREATE TABLE IF NOT EXISTS page_stats (id INTEGER PRIMARY KEY AUTOINCREMENT, post_slug TEXT UNIQUE NOT NULL, post_title TEXT, post_url TEXT, pv INTEGER NOT NULL DEFAULT 0, last_visit_at INTEGER, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)'
		).run();

		const { results } = await c.env.CWD_DB.prepare(
			'SELECT l.page_slug, COALESCE(p.post_title, NULL) AS page_title, COALESCE(p.post_url, NULL) AS page_url, COUNT(*) AS likes FROM Likes l LEFT JOIN page_stats p ON p.post_slug = l.page_slug GROUP BY l.page_slug, p.post_title, p.post_url ORDER BY likes DESC LIMIT 50'
		).all<LikeStatsItem>();

		const items = results.map((row) => ({
			pageSlug: row.page_slug,
			pageTitle: row.page_title,
			pageUrl: row.page_url,
			likes: row.likes
		}));

		return c.json({ items });
	} catch (e: any) {
		return c.json({ message: e.message || '获取点赞统计失败' }, 500);
	}
};

