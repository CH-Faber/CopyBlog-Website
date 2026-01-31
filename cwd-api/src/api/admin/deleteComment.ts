import { Context } from 'hono';
import { Bindings } from '../../bindings';

export const deleteComment = async (c: Context<{ Bindings: Bindings }>) => {
  const id = c.req.query('id');

  if (!id) {
    return c.json({ message: "Missing id" }, 400);
  }

  // 从数据库中直接删除评论
  const { success } = await c.env.CWD_DB.prepare(
    "DELETE FROM Comment WHERE id = ?"
  ).bind(id).run();

  if (!success) {
    return c.json({ message: "Delete operation failed" }, 500);
  }

  return c.json({
    message: `Comment deleted, id: ${id}.`
  });
};