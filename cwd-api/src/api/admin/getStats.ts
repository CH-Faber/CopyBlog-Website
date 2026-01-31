import { Context } from 'hono';
import { Bindings } from '../../bindings';

type StatusCounts = {
	total: number;
	approved: number;
	pending: number;
	rejected: number;
};

type DomainCounts = StatusCounts & {
	domain: string;
};

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

export const getStats = async (c: Context<{ Bindings: Bindings }>) => {
	try {
		const rawDomain = c.req.query('domain') || '';
		const domainFilter = rawDomain.trim().toLowerCase();

		const { results } = await c.env.CWD_DB.prepare(
			'SELECT created, post_slug, url, status FROM Comment'
		).all<{
			created: number;
			post_slug: string;
			url: string | null;
			status: string;
		}>();

		const summaryAll: StatusCounts = {
			total: 0,
			approved: 0,
			pending: 0,
			rejected: 0
		};

		const summaryFiltered: StatusCounts = {
			total: 0,
			approved: 0,
			pending: 0,
			rejected: 0
		};

		const domainMap = new Map<string, StatusCounts>();

		const dailyMapAll = new Map<string, number>();
		const dailyMapFiltered = new Map<string, number>();

		const now = Date.now();
		const thirtyDaysAgo = now - 29 * 24 * 60 * 60 * 1000;

		for (const row of results) {
			const domain =
				extractDomain(row.post_slug) || extractDomain(row.url) || 'unknown';

			let counts = domainMap.get(domain);
			if (!counts) {
				counts = {
					total: 0,
					approved: 0,
					pending: 0,
					rejected: 0
				};
				domainMap.set(domain, counts);
			}
			counts.total += 1;
			if (row.status === 'approved') {
				counts.approved += 1;
			} else if (row.status === 'pending') {
				counts.pending += 1;
			} else if (row.status === 'rejected') {
				counts.rejected += 1;
			}

			summaryAll.total += 1;
			if (row.status === 'approved') {
				summaryAll.approved += 1;
			} else if (row.status === 'pending') {
				summaryAll.pending += 1;
			} else if (row.status === 'rejected') {
				summaryAll.rejected += 1;
			}

			const matchesFilter = domainFilter && domain === domainFilter;

			if (matchesFilter) {
				summaryFiltered.total += 1;
				if (row.status === 'approved') {
					summaryFiltered.approved += 1;
				} else if (row.status === 'pending') {
					summaryFiltered.pending += 1;
				} else if (row.status === 'rejected') {
					summaryFiltered.rejected += 1;
				}
			}

			if (row.created >= thirtyDaysAgo) {
				const d = new Date(row.created);
				const year = d.getUTCFullYear();
				const month = String(d.getUTCMonth() + 1).padStart(2, '0');
				const day = String(d.getUTCDate()).padStart(2, '0');
				const key = `${year}-${month}-${day}`;

				dailyMapAll.set(key, (dailyMapAll.get(key) || 0) + 1);

				if (matchesFilter) {
					dailyMapFiltered.set(
						key,
						(dailyMapFiltered.get(key) || 0) + 1
					);
				}
			}
		}

		const domains: DomainCounts[] = Array.from(domainMap.entries())
			.map(([domain, counts]) => ({
				domain,
				total: counts.total,
				approved: counts.approved,
				pending: counts.pending,
				rejected: counts.rejected
			}))
			.sort((a, b) => b.total - a.total);

		const dailyMap = domainFilter ? dailyMapFiltered : dailyMapAll;

		const last7Days: { date: string; total: number }[] = [];
		for (let i = 29; i >= 0; i--) {
			const d = new Date(now - i * 24 * 60 * 60 * 1000);
			const year = d.getUTCFullYear();
			const month = String(d.getUTCMonth() + 1).padStart(2, '0');
			const day = String(d.getUTCDate()).padStart(2, '0');
			const key = `${year}-${month}-${day}`;
			last7Days.push({
				date: key,
				total: dailyMap.get(key) || 0
			});
		}

		const summary = domainFilter ? summaryFiltered : summaryAll;

		return c.json({
			summary,
			domains,
			last7Days
		});
	} catch (e: any) {
		return c.json({ message: e.message || '获取统计数据失败' }, 500);
	}
};
