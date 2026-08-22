export function normalizePath(path: string) {
  return path.trim().replace(/\/{2,}/g, '/')
}

export function parentPath(path: string, rootPath: string) {
  const current = normalizePath(path).replace(/\/+$/, '')
  const root = normalizePath(rootPath).replace(/\/+$/, '')
  if (!current || current === root) {
    return ''
  }
  const index = current.lastIndexOf('/')
  return index <= 0 ? root : current.slice(0, index) || root
}

export function basename(path: string) {
  const normalized = normalizePath(path).replace(/\/+$/, '')
  const index = normalized.lastIndexOf('/')
  return index >= 0 ? normalized.slice(index + 1) : normalized
}
