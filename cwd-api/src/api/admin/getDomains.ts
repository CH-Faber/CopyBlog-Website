import type { Context } from 'hono';
import type { Bindings } from '../../bindings';

function extractDomain(source: string | null | undefined): string | null {
	if (!source) {
		return null;
	}
	const value = source.trim();
	if (!value) {
		return null;
	}
	if (!/^https?:\/\//i.test(value)) {
		return null;
	}
	try {
		const url = new URL(value);
		return url.hostname.toLowerCase();
	} catch {
		return null;
	}
}

export const getDomains = async (c: Context<{ Bindings: Bindings }>) => {
	try {
		const domains = new Set<string>();

		const { results: commentRows } = await c.env.CWD_DB.prepare(
			'SELECT post_slug, url FROM Comment'
		).all<{
			post_slug: string;
			url: string | null;
		}>();

		for (const row of commentRows) {
			const domain =
				extractDomain(row.post_slug) || extractDomain(row.url);
			if (domain) {
				domains.add(domain);
			}
		}

		await c.env.CWD_DB.prepare(
			'CREATE TABLE IF NOT EXISTS page_stats (id INTEGER PRIMARY KEY AUTOINCREMENT, post_slug TEXT UNIQUE NOT NULL, post_title TEXT, post_url TEXT, pv INTEGER NOT NULL DEFAULT 0, last_visit_at INTEGER, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)'
		).run();

		const { results: pageRows } = await c.env.CWD_DB.prepare(
			'SELECT post_slug, post_url FROM page_stats'
		).all<{
			post_slug: string;
			post_url: string | null;
		}>();

		for (const row of pageRows) {
			const domain =
				extractDomain(row.post_url) || extractDomain(row.post_slug);
			if (domain) {
				domains.add(domain);
			}
		}

		const list = Array.from(domains);
		list.sort();

		return c.json({
			domains: list
		});
	} catch (e: any) {
		return c.json(
			{ message: e.message || '获取域名列表失败' },
			500
		);
	}
};

