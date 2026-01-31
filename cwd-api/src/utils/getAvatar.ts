/**
 * 默认 gravatar.com 前缀
 */
const DEFAULT_AVATAR_PREFIX = 'https://gravatar.com/avatar';

/**
 * 当缺少邮箱或邮箱为空时使用的占位 hash
 * 32 个 0，符合 MD5 长度要求，Gravatar 会回退到默认头像样式
 */
const DEFAULT_AVATAR_HASH = '00000000000000000000000000000000';

/**
 * 辅助函数：生成 gravatar.com 头像地址 (MD5 算法)
 * 优先使用邮箱；当邮箱不存在或为空时，使用昵称计算 MD5。
 */
export const getCravatar = async (
	email: string | null | undefined,
	name?: string | null | undefined,
	prefix?: string
): Promise<string> => {
	const avatarPrefix = prefix || DEFAULT_AVATAR_PREFIX;

	const pickIdentifier = (value: string | null | undefined) => {
		if (!value || typeof value !== 'string') return null;
		const trimmed = value.trim().toLowerCase();
		return trimmed || null;
	};

	const identifier = pickIdentifier(email) || pickIdentifier(name);

	if (!identifier) {
		return `${avatarPrefix}/${DEFAULT_AVATAR_HASH}?s=120&d=retro`;
	}

	const msgUint8 = new TextEncoder().encode(identifier);
	const hashBuffer = await crypto.subtle.digest('MD5', msgUint8);
	const hashArray = Array.from(new Uint8Array(hashBuffer));
	const hashHex = hashArray.map((b) => b.toString(16).padStart(2, '0')).join('');

	return `${avatarPrefix}/${hashHex}?s=120&d=retro`;
};
