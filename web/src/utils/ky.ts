import ky, { HTTPError } from 'ky'
import signStore from '@/stores/sign'

export { ky }

const instance = ky.create({
  prefixUrl: '/',
  timeout: 1000 * 30,
  retry: 0,
  hooks: {
    beforeRequest: [
      (request) => {
        const authorization = signStore().authorization()
        if (authorization && !request.headers.has('Authorization')) {
          request.headers.set('Authorization', authorization)
        }
      },
    ],
  },
})

export async function errorMessage(error: unknown) {
  if (error instanceof HTTPError) {
    try {
      const body = await error.response.clone().json<{ error?: string }>()
      if (body.error) {
        return body.error
      }
    } catch {
      // Fall through to the generic HTTP message.
    }
    return `请求失败（HTTP ${error.response.status}）`
  }
  if (error instanceof Error) {
    return error.message
  }
  return '发生未知错误'
}

export function isUnauthorized(error: unknown) {
  return error instanceof HTTPError && error.response.status === 401
}

export default instance
