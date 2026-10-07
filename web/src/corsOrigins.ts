// 跨域来源的解析与校验。规则与服务端 settings.NormalizeOrigin 保持一致：
// 只接受 scheme://host 或 scheme://host:port，scheme 限 http/https；
// 允许一个可选的末尾斜杠；路径、查询、片段、用户信息一律拒绝。
const ORIGIN_RE = /^(https?):\/\/([A-Za-z0-9][A-Za-z0-9.-]*)(?::(\d{1,5}))?\/?$/i

// normalizeOrigin 校验并规整单个来源，非法时返回 null。
export function normalizeOrigin(raw: string): string | null {
  const m = ORIGIN_RE.exec(raw.trim())
  if (!m) return null
  const scheme = m[1].toLowerCase()
  const host = m[2].toLowerCase()
  const port = m[3]
  return port ? `${scheme}://${host}:${port}` : `${scheme}://${host}`
}

export interface ParsedOrigins {
  origins: string[]
  error: string | null
}

// parseCorsOrigins 按行解析文本域内容（每行也允许逗号分隔），返回规整去重后的列表。
// 任一非空项非法时返回错误，并指出是第几行（从 1 开始），不静默丢弃。
export function parseCorsOrigins(text: string): ParsedOrigins {
  const origins: string[] = []
  const seen = new Set<string>()
  const lines = text.split(/\r?\n/)
  for (let i = 0; i < lines.length; i++) {
    for (const part of lines[i].split(',')) {
      const token = part.trim()
      if (token === '') continue
      const norm = normalizeOrigin(token)
      if (norm === null) {
        return {
          origins: [],
          error: `第 ${i + 1} 行不合法：「${token}」。应为 scheme://host 或 scheme://host:port，scheme 限 http/https`,
        }
      }
      if (!seen.has(norm)) {
        seen.add(norm)
        origins.push(norm)
      }
    }
  }
  return { origins, error: null }
}
