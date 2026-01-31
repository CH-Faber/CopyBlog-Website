import type { Context } from 'hono';
import type { Bindings } from '../../bindings';

type LikeItem = {
	id: number;
	page_slug: string;
	user_id: string;
	created_at: number;
};

export const listLikes = async (c: Context<{ Bindings: Bindings }>) => {
	try {
		const page = parseInt(c.req.query('page') || '1', 10) || 1;
		const limit = 20;
		const offset = (page - 1) * limit;

		const rawPageSlug = c.req.query('page_slug') || c.req.query('pageSlug') || '';
		const pageSlug = rawPageSlug.trim();

		const rawUserId = c.req.query('user_id') || c.req.query('userId') || '';
		const userId = rawUserId.trim();

		const rawStart = c.req.query('start') || '';
		const rawEnd = c.req.query('end') || '';

		const whereSql: string[] = [];
		const params: (string | number)[] = [];

		if (pageSlug) {
			whereSql.push('page_slug = ?');
			params.push(pageSlug);
		}

		if (userId) {
			whereSql.push('user_id = ?');
			params.push(userId);
		}

		if (rawStart) {
			const startTs = Number(rawStart);
			if (Number.isFinite(startTs)) {
				whereSql.push('created_at >= ?');
				params.push(startTs);
			}
		}

		if (rawEnd) {
			const endTs = Number(rawEnd);
			if (Number.isFinite(endTs)) {
				whereSql.push('created_at <= ?');
				params.push(endTs);
			}
		}

		const whereClause = whereSql.length ? `WHERE ${whereSql.join(' AND ')}` : '';

		await c.env.CWD_DB.prepare(
			'CREATE TABLE IF NOT EXISTS Likes (id INTEGER PRIMARY KEY AUTOINCREMENT, page_slug TEXT NOT NULL, user_id TEXT NOT NULL, created_at INTEGER NOT NULL, UNIQUE(page_slug, user_id))'
		).run();

		const totalRow = await c.env.CWD_DB.prepare(
			`SELECT COUNT(*) AS count FROM Likes ${whereClause}`
		)
			.bind(...params)
			.first<{ count: number }>();

		const { results } = await c.env.CWD_DB.prepare(
			`SELECT id, page_slug, user_id, created_at FROM Likes ${whereClause} ORDER BY created_at DESC LIMIT ? OFFSET ?`
		)
			.bind(...params, limit, offset)
			.all<LikeItem>();

		const data = results.map((row) => ({
			id: row.id,
			pageSlug: row.page_slug,
			userId: row.user_id,
			createdAt: row.created_at
		}));

		const totalCount = totalRow?.count || 0;
		const totalPages = Math.max(1, Math.ceil(totalCount / limit));

		return c.json({
			data,
			pagination: {
				page,
				limit,
				total: totalPages
			}
		});
	} catch (e: any) {
		return c.json({ message: e.message || '获取点赞记录失败' }, 500);
	}
};

