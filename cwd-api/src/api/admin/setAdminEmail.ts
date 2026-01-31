import { Context } from 'hono';
import { Bindings } from '../../bindings';
import { isValidEmail } from '../../utils/email';

export const setAdminEmail = async (c: Context<{ Bindings: Bindings }>) => {
  try {
    const { email } = await c.req.json();
    if (!email || !isValidEmail(email)) {
      return c.json({ message: '邮箱格式不正确' }, 400);
    }
    await c.env.CWD_DB.prepare(
      'CREATE TABLE IF NOT EXISTS Settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)'
    ).run();
    await c.env.CWD_DB.prepare('REPLACE INTO Settings (key, value) VALUES (?, ?)')
      .bind('admin_notify_email', email)
      .run();
    return c.json({ message: '保存成功' });
  } catch (e: any) {
    return c.json({ message: e.message }, 500);
  }
};

