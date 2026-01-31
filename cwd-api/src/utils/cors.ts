import { cors } from 'hono/cors'

export const customCors = () => {
  return cors({
    origin: '*',
    allowMethods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS'],
    allowHeaders: ['Content-Type', 'Authorization', 'X-CWD-Like-User'],
    exposeHeaders: ['Content-Length'],
    maxAge: 600,
    credentials: false,
  })
}
