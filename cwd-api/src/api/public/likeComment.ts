import { Context } from 'hono';
import { Bindings } from '../../bindings';

export const likeComment = async (c: Context<{ Bindings: Bindings }>) => {
	let body: any = null;
	try {
		body = await c.req.json();
	} catch {
		body = null;
	}

	const rawId =
		(body && (body.id ?? body.commentId)) ??
		c.req.query('id') ??
		c.req.query('commentId') ??
		null;

	const parsed =
		typeof rawId === 'number'
			? rawId
			: typeof rawId === 'string' && rawId.trim()
			? Number.parseInt(rawId.trim(), 10)
			: NaN;

	if (!Number.isFinite(parsed) || parsed <= 0) {
		return c.json({ message: 'Missing or invalid id' }, 400);
	}

	const id = parsed;

	try {
		const existing = await c.env.CWD_DB.prepare(
			'SELECT id, likes FROM Comment WHERE id = ?'
		)
			.bind(id)
			.first<{ id: number; likes?: number }>();

		if (!existing) {
			return c.json({ message: 'Comment not found' }, 404);
		}

		await c.env.CWD_DB.prepare(
			'UPDATE Comment SET likes = COALESCE(likes, 0) + 1 WHERE id = ?'
		)
			.bind(id)
			.run();

		const updated = await c.env.CWD_DB.prepare(
			'SELECT COALESCE(likes, 0) as likes FROM Comment WHERE id = ?'
		)
			.bind(id)
			.first<{ likes?: number }>();

		const likes =
			updated && typeof updated.likes === 'number' && Number.isFinite(updated.likes) && updated.likes >= 0
				? updated.likes
				: ((existing.likes || 0) + 1);

		return c.json({ id, likes });
	} catch (e: any) {
		return c.json({ message: e?.message || '点赞失败' }, 500);
	}
};

