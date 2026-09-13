import { ApiError } from './api'

// ---- 模型档位 / 能力（内部 id 不变，仅展示更名）----
export type TierId = 'light' | 'standard' | 'power'

export const TIER_LABELS: Record<TierId, string> = {
  light: '快速',
  standard: '标准',
  power: '深度思考',
}

/** 内部档位 id -> 对外叫法；未知值按「标准」。 */
export function tierLabel(tier?: string): string {
  if (tier === 'light' || tier === 'standard' || tier === 'power') {
    return TIER_LABELS[tier]
  }
  return TIER_LABELS.standard
}

export const AUTO_LABEL = '智能选择'
export const VISION_LABEL = '视觉'

// ---- 聊天动作 ----
export const ACTIONS = {
  copy: '复制',
  regenerate: '重新回答',
  editAndReanswer: '编辑后重新回答',
  forkAsNew: '复制成新对话',
  rollbackHere: '回到这里',
  more: '更多操作',
} as const

// ---- HITL ----
export const HITL = {
  title: '需要你确认',
  approve: '同意',
  reject: '拒绝',
  confirmReject: '确认拒绝',
  viewParams: '看参数',
  failed: '操作失败，请重试',
  commentPlaceholder: '留言（选填）',
  approved: '已同意',
  rejected: '已拒绝',
} as const

// ---- 工作流 ----
export const WORKFLOW_PREPARING = '工作流准备中'

// ---- 欢迎区 ----
export const WELCOME = {
  title: '有什么可以帮你？',
  subtitle: '我可以查询数据、调用业务系统、处理文件与图片，直接说出你的需求即可。',
} as const

// ---- 聊天页反馈/门控/空态（任务 8 新增，集中收纳，组件不裸写新中文）----
export const CHAT = {
  // 模型持久化失效回退
  modelStaleFallback: '之前选择的模型已不可用，已切回智能选择。',
  // 图片能力门控
  visionWarningTitle: '暂时无法发送图片',
  visionWarningFallback: '当前选择不能处理图片。',
  visionWarningAck: '知道了',
  visionBlockedOperator: '当前无法处理图片，请联系管理员配置支持图片的模型。',
  // 空模型拦截（按角色分流，操作者不被导向无权限的设置页）
  noModelAdmin: '还没有可用的 AI 模型，请到「设置 → AI 模型」添加一个。',
  noModelOperator: '还没有可用的 AI 模型，请联系管理员配置。',
  // 删除对话确认
  deleteTitle: '删除这个对话？',
  deleteBody: '将永久删除该对话的消息与相关数据，且不可恢复。',
  deleteConfirm: '删除',
  deleteSuccess: '对话已删除',
  // 复制消息
  copySuccess: '已复制',
  copyFailed: '复制失败，请手动选择文本',
  // 空模型态
  addModel: '添加模型',
  noModelConfigured: '暂未配置模型',
  // SSE 降级轮询提示
  reconnecting: '正在重新连接…',
} as const

export interface FriendlyError {
  title: string
  /** 技术细节，默认收起，供「详情」展开。 */
  detail?: string
}

const CODE_TITLE: Record<string, string> = {
  conversation_busy: '上一条还在处理中，请稍候再发。',
  no_model_configured: '还没有可用的 AI 模型，请到「设置 → AI 模型」添加一个。',
  vision_unsupported: '当前模型看不了图片，请改用「智能选择」或带「视觉」标记的模型。',
  invalid_signature: '连接校验未通过，请刷新页面后重试。',
  not_found: '内容不存在或已被删除。',
  internal_error: '服务暂时出了点问题，请稍后重试。',
}

function isNetworkish(e: unknown): boolean {
  const msg = e instanceof Error ? e.message : String(e ?? '')
  return /failed to fetch|networkerror|load failed|network request/i.test(msg)
}

/** 把任意抛出值翻译为人话标题；未知错误附带可展开的技术 detail。 */
export function friendlyError(e: unknown): FriendlyError {
  if (e instanceof ApiError) {
    // invalid_signature 通常以 401 返回，但其指引是「刷新页面」，
    // 需先于通用 401/403「重新解锁」文案按错误码精确映射。
    const codeTitle = CODE_TITLE[e.code]
    if (codeTitle) return { title: codeTitle }
    if (e.status === 401 || e.status === 403) {
      return { title: '没有访问权限或登录已失效，请重新解锁后再试。' }
    }
    if (e.status >= 500) {
      return { title: CODE_TITLE.internal_error, detail: `${e.code}: ${e.message}` }
    }
    return { title: '操作未能完成，请稍后重试。', detail: `${e.code}: ${e.message}` }
  }
  if (isNetworkish(e)) {
    return { title: '网络连接失败，请检查服务是否正在运行。' }
  }
  const detail = e instanceof Error ? e.message : String(e ?? '')
  return { title: '出现了未知问题。', detail: detail || undefined }
}

// ---- 设置页：账号 ----
export const ACCOUNTS = {
  title: '账号',
  description:
    '这里显示助手当前已登录的业务系统账号。正常使用时，在对话中完成登录会自动出现在这里，无需手动填写。',
  emptyTitle: '暂无已登录的业务账号',
  emptyDesc: '在对话中登录业务系统后，会自动显示在这里。',
  setDefault: '设为默认',
  defaultBadge: '默认中',
  logout: '退出',
  clear: '清空登录账号',
  clearConfirmTitle: '清空已登录的账号？',
  clearConfirmBody: '将移除当前在对话中登录的全部业务账号（系统预设账号不受影响）。需要时可重新登录。',
  clearConfirmOk: '清空',
  toastLogout: '已退出账号',
  toastDefault: '已设为默认',
  toastCleared: '已清空登录账号',
  loadFailed: '无法加载账号',
  details: '详情',
  developer: '开发者信息',
} as const

/** 身份来源 -> 人话；未知来源原值兜底，不吞信息。 */
export function identitySourceLabel(source: string): string {
  switch (source) {
    case 'login_capture':
      return '对话中登录'
    case 'env':
      return '系统预设'
    case 'manual':
      return '临时提供'
    default:
      return source
  }
}

// ---- 设置页：数据存储 ----
export const STORAGE = {
  title: '数据存储',
  description:
    '选择助手数据的保存位置。更改并保存后服务会重启，且不会自动搬迁旧数据，请先自行备份。',
  driverField: '保存方式',
  sqlitePath: '数据库文件路径',
  sqliteHint: '默认 ./data/baize.db；换成新路径不会自动搬迁已有数据。',
  dsn: '连接地址（DSN）',
  ack: '我了解：切换存储不会自动迁移数据，旧库中的数据需自行处理',
  ackRequired: '请先勾选确认：切换存储不会自动迁移数据',
  postgresRequiresDSN: '使用 PostgreSQL 需要填写连接地址（DSN）',
  saveRestart: '保存并重启',
  saving: '正在保存…',
  confirmRestartTitle: '保存并重启服务？',
  confirmRestartBody:
    '服务将立即重启，进行中的对话会中断；数据不会从旧存储自动迁移。确认继续？',
  restarting: '正在重启…',
  developer: '技术信息',
} as const

const DRIVER_LABELS: Record<string, string> = {
  sqlite: '本地文件（SQLite）',
  postgres: 'PostgreSQL 数据库',
  memory: '内存（重启即清空，仅试用）',
}

/** 存储驱动 -> 人话选项；未知驱动原值兜底。提交值仍用英文 driver。 */
export function driverLabel(driver: string): string {
  return DRIVER_LABELS[driver] ?? driver
}

// ---- 设置页：消息回调 ----
export const WEBHOOKS = {
  title: '消息回调',
  description:
    '有新消息或运行结束时，主动推送到你指定的地址（与对话实时流并行，不堵引擎）。这是运行事件通知，不是连接里的「企业统一执行地址」。',
  urlLabel: '回调地址',
  urlHint: '留空表示不推送',
  headersLabel: '请求头',
  headersHint: '每行 KEY=VALUE，可选',
  save: '保存',
  saving: '保存中…',
  test: '发送测试',
  testing: '测试中…',
  deliveriesTitle: '最近投递',
  deliveriesHint:
    '展示待投递与死信；网络错误、5xx 或 429 会自动重试（最多 5 次），其余 4xx 进死信。',
  deliveriesEmpty: '暂无待投递或死信记录。',
  retry: '重投',
  retrying: '重投中…',
  statusDead: '死信',
  statusPending: '待投递',
  statusDelivered: '已投递',
  toastSaved: '已保存消息回调配置',
  toastTestOk: '测试投递成功',
  toastTestFail: '测试投递失败',
  toastRetryQueued: '已加入重投队列',
  errBadHeaderLine: '请求头格式不正确：请使用每行 KEY=VALUE',
  loadFailed: '无法加载消息回调配置',
} as const

// ---- 设置页：运行参数 ----
export const RUNTIME = {
  confirmResetTitle: '重置为基线口令？',
  confirmResetBody: '将清空全部热更新凭据，回落到 YAML/env 基线口令。引擎参数不受影响。',
  confirmResetOk: '重置',
} as const

// ---- 设置页：AI 模型 ----
export const MODELS = {
  title: '模型',
  description: '管理对话与理解用的模型；可按任务档位区分，并支持「智能选择」。',
  add: '添加模型',
  edit: '编辑',
  delete: '删除',
  save: '保存',
  cancel: '取消',
  emptyTitle: '还没有模型',
  emptyDescAdmin: '添加一个模型后，对话里就能选用。',
  emptyDescOperator: '暂无可用模型，请联系管理员添加。',
  advanced: '高级',
  fieldName: '名称',
  fieldBaseUrl: '服务地址',
  fieldModel: '模型名',
  fieldApiKey: 'API 密钥',
  fieldApiKeyEnv: 'API Key 环境变量名',
  fieldTier: '任务档位',
  fieldVision: '视觉（支持图片附件）',
  fieldDisableThinking: '禁用思考',
  fieldContextTokens: '上下文长度',
  confirmDeleteTitle: '删除这个模型？',
  confirmDeleteBody: '删除后不可恢复。',
  confirmDeleteLast: '这是当前唯一的模型，删除后对话将无法选择模型。',
  confirmDeleteOk: '删除',
  toastSaved: '已保存模型',
  toastDeleted: '已删除模型',
  errGeneric: '操作未能完成，请稍后重试。',
  errNotFound: '找不到该模型，可能已被删除。',
  errInvalidRequest: '填写内容不完整或不正确，请检查后重试。',
  errInternal: '服务暂时出了问题，请稍后重试。',
  errNameRequired: '请填写名称。',
  errBaseUrlRequired: '请填写服务地址。',
  errModelRequired: '请填写模型名。',
  errApiKeyRequired: 'API Key 与环境变量名至少填写一项',
} as const

/** 把模型设置页相关异常翻译为人话标题；未知错误附技术详情。 */
export function modelErrorText(e: unknown): FriendlyError {
  if (e instanceof ApiError) {
    if (e.code === 'not_found') return { title: MODELS.errNotFound }
    if (e.code === 'invalid_request') return { title: MODELS.errInvalidRequest }
    if (e.code === 'internal_error' || e.status >= 500) {
      return { title: MODELS.errInternal, detail: `${e.code}: ${e.message}` }
    }
    return { title: MODELS.errGeneric, detail: `${e.code}: ${e.message}` }
  }
  return friendlyError(e)
}

// ---- 设置页：技能 ----
export const SKILLS = {
  title: '技能',
  description: '管理对话默认技能；保存后仅对新开的对话生效。',
  upload: '上传技能',
  saveDefaults: '保存为默认技能',
  emptyTitle: '还没有技能',
  emptyDesc: '上传 .md 或 .zip 技能包后，即可在对话中选用。',
  confirmDeleteTitle: '删除这个技能？',
  confirmDeleteBody: '删除后不可恢复，且会从默认技能列表中移除。',
  confirmDeleteOk: '删除',
  toastUploaded: '技能已上传',
  toastSaved: '默认技能已保存',
  toastDeleted: '技能已删除',
  defaultBadge: '默认',
  notDefaultBadge: '非默认',
  sourceBuiltin: '内置',
  sourceUser: '用户',
  errGeneric: '操作未能完成，请稍后重试。',
  errNotFound: '找不到该技能，可能已被删除。',
  errInvalidRequest: '上传的文件无效或格式不正确，请检查后重试。',
  errInternal: '服务暂时出了问题，请稍后重试。',
} as const

/** 把技能设置页相关异常翻译为人话标题；未知错误附技术详情。 */
export function skillErrorText(e: unknown): FriendlyError {
  if (e instanceof ApiError) {
    if (e.code === 'not_found') return { title: SKILLS.errNotFound }
    if (e.code === 'invalid_request') return { title: SKILLS.errInvalidRequest }
    if (e.code === 'internal_error' || e.status >= 500) {
      return { title: SKILLS.errInternal, detail: `${e.code}: ${e.message}` }
    }
    return { title: SKILLS.errGeneric, detail: `${e.code}: ${e.message}` }
  }
  return friendlyError(e)
}

// ---- 设置页：助手功能 ----
export const TOOLS = {
  title: '助手功能',
  description: '管理助手可调用的功能：启停、需登录、需确认与显示名。',
  enable: '启用',
  requireLogin: '需登录',
  requireApprovalBadge: '需确认',
  editCopy: '编辑文案',
  techDetails: '技术详情',
  statusEnabled: '已启用',
  statusDisabled: '已停用',
  advanced: '高级',
  addTool: '添加',
  addModalTitle: '添加工具',
  cancel: '取消',
  fieldConnector: '所属连接',
  fieldName: '名称',
  fieldMethod: '请求方法',
  fieldPath: '路径',
  fieldTitle: '显示名（可选）',
  fieldDescription: '描述',
  fieldSchema: '参数说明',
  viewSchema: '查看参数说明',
  captureSection: '登录令牌捕获',
  captureIntro:
    '登录工具调用成功后，从返回 JSON 取出令牌写入当前会话身份，供后续「需登录」的工具使用。',
  captureToolGlob: '登录工具名匹配',
  captureToolGlobHint:
    '按工具名匹配，支持 * 通配。例：*login* 可匹配 login、user_login。填 __none__ 关闭捕获；留空则用系统默认（通常为 *login*）。',
  captureTokenPaths: '令牌 JSON 路径',
  captureTokenPathsHint: '从登录成功响应取令牌，每行一个点分路径，如 accessToken 或 data.token。',
  captureLabelPaths: '身份显示名路径',
  captureLabelPathsHint: '可选。从响应取展示用身份名，每行一个路径，如 email 或 user.name。',
  captureHeaderTemplate: '下游请求头模板',
  captureHeaderTemplateHint: '后续请求如何带上令牌。{{token}} 会替换为捕获值，例：Bearer {{token}}。',
  captureDefaultScheme: '默认认证方案',
  captureDefaultSchemeHint: '写入会话身份时的 scheme 名，常见为 bearer。',
  confirmDeleteTitle: '删除这个功能？',
  confirmDeleteBody: '删除后不可恢复。仅可删除手动添加的 extra 工具。',
  confirmDeleteOk: '删除',
  toastAdded: '功能已添加',
  toastDeleted: '功能已删除',
  errNoConnector: '请选择所属连接',
  errInvalidSchema: '参数说明不是合法的 JSON',
  errGeneric: '操作未能完成，请稍后重试。',
  errNotFound: '找不到该功能，可能已被删除。',
  errInvalidRequest: '填写内容不完整或不正确，请检查后重试。',
  errInternal: '服务暂时出了问题，请稍后重试。',
} as const

/** 把助手功能页相关异常翻译为人话标题；未知错误附技术详情。 */
export function toolErrorText(e: unknown): FriendlyError {
  if (e instanceof ApiError) {
    if (e.code === 'not_found') return { title: TOOLS.errNotFound }
    if (e.code === 'invalid_request') return { title: TOOLS.errInvalidRequest }
    if (e.code === 'internal_error' || e.status >= 500) {
      return { title: TOOLS.errInternal, detail: `${e.code}: ${e.message}` }
    }
    return { title: TOOLS.errGeneric, detail: `${e.code}: ${e.message}` }
  }
  return friendlyError(e)
}

// ---- 设置页：业务系统 / 插件（连接器） ----
export const CONNECTORS = {
  openapiTitle: '业务系统',
  openapiDesc:
    '上传一份接口文档，助手就能对接公司内部的各类业务系统。无需写代码。',
  pluginTitle: '插件服务',
  pluginDesc: '接入你们自行部署、按约定接口提供能力的程序，助手即可使用其能力。',
  addOpenapi: '接入业务系统',
  addPlugin: '接入插件服务',
  editOpenapi: '编辑业务系统',
  editPlugin: '编辑插件服务',
  openapiEmptyTitle: '还没有接入业务系统',
  openapiEmptyDesc: '上传接口文档后，助手即可对接公司内部的各类业务系统。',
  pluginEmptyTitle: '还没有接入插件服务',
  pluginEmptyDesc: '接入自行部署、按约定接口提供能力的程序后，助手即可使用其能力。',
  menuEdit: '编辑',
  menuDelete: '删除',
  deleteTitle: '删除这个连接？',
  deleteBody: '删除后，该连接提供的工具会从助手的能力中移除。',
  deleteOk: '删除',
  saved: '已保存',
  deleted: '已删除',
  toolsLink: '查看工具',
  toolCount: (n: number) => `${n} 个工具`,
  stepInfo: '连接信息',
  stepPermissions: '工具权限',
  noToolsDiscovered: '暂无已识别工具。',
  fieldId: '连接编号',
  fieldIdHint: '仅用于区分，保存后不可改；用小写字母、数字、- 或 _。',
  fieldBaseUrl: '服务地址',
  fieldSpec: '接口文档',
  fieldSpecHint: '上传文件（.json / .yaml / .yml），或在下方填写文档链接，二选一。',
  fieldSpecUrl: '文档链接',
  fieldFormat: '接口文档格式',
  fmtAuto: '自动识别',
  fmtOpenapi3: 'OpenAPI 3',
  fmtSwagger2: 'Swagger 2',
  fmtPostman: 'Postman 合集',
  specFileChosen: (name: string) => `已选择文件：${name}`,
  chooseSpec: '选择接口文档',
  specRemoveFile: '移除已选文件',
  specReadFailed: '读取文件失败',
  advanced: '高级',
  executionCallbackSection: '统一执行地址',
  executionCallback: '执行地址',
  executionCallbackHint:
    '选填。填写后，本连接下的工具调用会改为向该地址 POST，由企业网关代为执行（多数场景留空）。网关需按工具名与参数完成真实调用。',
  executionCallbackExample:
    'POST {统一执行地址}\n{\n  "tool": "create_ticket",\n  "arguments": { "title": "..." },\n  "run_id": "run_...",\n  "...": "..."\n}',
  permsIntro:
    '在此配置工具门闸：勾选「需登录」后，运营需先完成本人登录；勾选「需确认」后，每次调用前需人工确认。启停请到「助手功能」。',
  permsSearch: '搜索工具（名称 / 说明）',
  permsNoMatch: '无匹配工具',
  permLogin: '需登录',
  permApproval: '需确认',
  nextToPermissions: '下一步：设置工具权限',
  save: '保存连接',
  saving: '正在保存…',
  skip: '暂不设置',
  finish: '完成',
  back: '上一步',
  cancel: '取消',
  errIdRequired: '请填写连接编号',
  errIdPattern: '连接编号只能用小写字母开头，后跟小写字母、数字、- 或 _（最长 64 位）',
  errBaseUrlRequired: '请填写服务地址',
  errBaseUrlHttp: '服务地址需以 http:// 或 https:// 开头',
  errSpecRequired: '请上传接口文档或填写文档链接',
  // ---- MCP（外部工具服务） ----
  mcpTitle: '外部工具服务',
  mcpDesc: '接入标准 MCP 工具服务（本地子进程或远程 HTTP），扩展助手可用能力。',
  addMcp: '接入外部工具',
  editMcp: '编辑外部工具',
  mcpEmptyTitle: '还没有接入外部工具服务',
  mcpEmptyDesc: '接入遵循 MCP 标准的本地或远程工具服务后，助手即可调用其工具。',
  fieldTransport: '连接方式',
  transportStdio: '本地子进程（stdio）',
  transportHttp: '远程服务（Streamable HTTP）',
  fieldCommand: '启动命令',
  fieldCommandHint: '本地启动该工具服务的可执行命令，如 npx。',
  fieldArgs: '启动参数',
  fieldArgsHint: '空格分隔，或每行一个。',
  fieldEnv: '环境变量',
  fieldEnvHint: '每行一个 KEY=VALUE。密钥建议用 ${VAR}、env:VAR 或 file:路径 占位符，不要直接写死。',
  fieldUrl: '服务地址',
  fieldHeaders: '请求头',
  fieldHeadersHint: '每行一个 KEY=VALUE，例如 Authorization=Bearer ${TOKEN}。',
  errCommandRequired: '请填写启动命令',
  errMcpUrlRequired: '请填写服务地址',
  errMcpUrlHttp: '服务地址需以 http:// 或 https:// 开头',
  errEnvLine: (row: string) => `环境变量存在无法识别的行：${row}`,
  errHeadersLine: (row: string) => `请求头存在无法识别的行：${row}`,
  errMcpConnect: '无法连接到这个外部工具服务，请检查配置后重试。',
  permsIntroMcp: '勾选后，助手每次调用该工具前都会请你确认；不勾则直接执行。',
  mcpStdioSummary: (command: string) => `本地程序 · ${command}`,
  mcpHttpSummary: (url: string) => `远程服务 · ${url}`,
  errorGeneric: '操作未能完成，请稍后重试。',
} as const

// ---- 设置页：对外提供能力（MCP 导出） ----
export const MCP_EXPORTS = {
  title: '对外提供能力',
  intro:
    '把助手的能力以标准 MCP 服务对外开放，供 Cursor 等其他客户端调用。调用方凭下方创建的专用密钥访问。',
  introToolsLink: '下方可按功能覆盖默认导出规则；未覆盖的功能跟随系统默认。',
  toolsExportTitle: '按功能是否对外导出',
  toolsExportIntro: '选择每个功能在对外 MCP 中的导出策略。',
  toolsExportEmpty: '还没有可配置的功能。',
  toolsExportSearch: '搜索功能',
  toolsExportMcpWriteHint: 'MCP 写类工具即使设为必须导出也不会对外提供',
  exportDefault: '跟随默认规则',
  exportForceAllow: '必须导出',
  exportForceDeny: '禁止导出',
  toastExportSaved: '已更新导出策略',
  endpointTitle: '接入地址',
  endpointEnabled: '已启用',
  endpointDisabled: '已关闭（进程配置 mcp_export.enabled=false）',
  copyEndpoint: '复制地址',
  endpointCopied: '已复制接入地址',
  exampleTitle: '客户端配置示例',
  identityTitle: '调用方身份',
  identityIntro: '每把密钥必须绑定一个身份；调用时可附带统一的鉴权方式与请求头。',
  identityEmpty: '还没有调用方身份。',
  identityName: '名称',
  identityScheme: '鉴权方式（可选）',
  identityHeaders: '请求头（每行 KEY=VALUE）',
  createIdentity: '新建身份',
  keyTitle: '访问密钥',
  keyIntro: '密钥明文只在创建时显示一次，列表仅显示前缀；撤销后不可恢复。',
  keyEmpty: '还没有访问密钥。',
  keyName: '名称',
  keyBindIdentity: '绑定身份',
  keyNeedIdentityFirst: '请先创建身份',
  createKey: '新建密钥',
  revoke: '撤销',
  revoked: '已撤销',
  edit: '编辑',
  delete: '删除',
  save: '保存',
  cancel: '取消',
  tokenTitle: '密钥仅显示这一次',
  tokenBody: (name: string) => `请立即复制「${name}」的密钥并保存到安全位置，关闭后将无法再次查看。`,
  copyToken: '复制密钥',
  tokenCopied: '已复制密钥',
  tokenSaved: '我已保存',
  deleteIdentityTitle: '删除这个调用方身份？',
  deleteIdentityBody: (name: string) => `删除身份「${name}」后，其名下密钥也会一并删除。`,
  revokeKeyTitle: '撤销这把密钥？',
  revokeKeyBody: (name: string, prefix: string) => `撤销密钥「${name}」（${prefix}…）后不可恢复。`,
  confirmRevoke: '确认撤销',
  errNameRequired: '请填写名称',
  errKeyNameRequired: '请填写密钥名称',
  errKeyIdentityRequired: '请选择绑定的身份',
  errBadHeaderLine: (row: string) => `请求头存在无法识别的行：${row}`,
  savedIdentity: '已保存调用方身份',
  deletedIdentity: (name: string) => `已删除身份 ${name}`,
  revokedKey: (name: string) => `已撤销密钥 ${name}`,
  copyFailed: '复制失败，请手动复制',
  errGeneric: '操作未能完成，请稍后重试。',
  errInternal: '服务暂时出了点问题，请稍后重试。',
  errInvalidRequest: '提交的内容有误，请检查后重试。',
  errIdentityRequired: '请选择要绑定的调用方身份。',
  errIdentityNotFound: '调用方身份不存在或已被删除。',
  errKeyNotFound: '密钥不存在或已被撤销。',
} as const

const CONNECTOR_CODE_TITLES: Record<string, string> = {
  invalid_spec: '接口文档无法解析，请确认是有效的 OpenAPI / Swagger / Postman 文档。',
  invalid_spec_url: '文档链接格式不正确，请检查链接。',
  spec_fetch_blocked: '服务器不允许抓取该地址的文档（地址被安全策略拦截）。',
  spec_fetch_failed: '无法下载该文档链接，请确认地址可以访问。',
  unsupported_import_format: '不支持的文档格式。',
  invalid_plugin: '无法从该插件地址识别到可用能力，请检查服务是否正常。',
  tool_conflict: '有工具与其它连接重名，请调整对方系统里的操作名称后重试。',
  invalid_auth: '连接保存的凭证无效，请联系管理员通过配置处理。',
}

/** 把连接器相关异常翻译为 {title, detail?}；未知错误给出通用标题与可展开技术详情。 */
export function connectorErrorText(e: unknown): FriendlyError {
  if (e instanceof ApiError) {
    const byCode = CONNECTOR_CODE_TITLES[e.code]
    if (byCode) return { title: byCode }
    if (e.code === 'invalid_mcp') {
      // 后端契约（internal/connector/apply.go、mcp/errors.go）：可达 message 为
      // "invalid_mcp: mcp.command is required" / "...mcp.url is required" /
      // "...unsupported mcp transport: X"；连接失败为 "invalid_mcp: <底层错误>"；
      // 空工具与配置解析失败只有裸串 "invalid_mcp"（无法细分，走通用兜底）。
      if (/command is required|unsupported mcp transport/.test(e.message)) return { title: '连接配置不完整，请检查启动命令或连接方式。' }
      if (/url is required/.test(e.message)) return { title: '请填写远程服务地址。' }
      if (/401|403|unauthor|forbidden/i.test(e.message)) {
        return { title: '该服务需要鉴权，请在请求头中提供有效的 API Key；交互式 OAuth 登录暂不支持。' }
      }
      return { title: CONNECTORS.errMcpConnect, detail: `${e.code}: ${e.message}` }
    }
    if (/base_url is required/.test(e.message)) return { title: '请填写服务地址。' }
    if (/spec is required/.test(e.message)) return { title: '请提供接口文档。' }
    return { title: CONNECTORS.errorGeneric, detail: `${e.code}: ${e.message}` }
  }
  return friendlyError(e)
}

/** 把「对外提供能力」页身份/密钥接口异常翻译为人话标题；未知错误附技术详情。 */
export function mcpExportErrorText(e: unknown): FriendlyError {
  if (e instanceof ApiError) {
    // 后端契约（internal/api/server_mcp_export.go）：
    // 404 not_found: identity not found / key not found；
    // 400 invalid_request: name is required / missing identity id /
    //   identity_id is required / unknown identity_id / invalid json body；
    // 500 internal_error: 底层错误原文。
    if (e.code === 'not_found') {
      if (/key/i.test(e.message)) return { title: MCP_EXPORTS.errKeyNotFound }
      return { title: MCP_EXPORTS.errIdentityNotFound }
    }
    if (e.code === 'internal_error') {
      return { title: MCP_EXPORTS.errInternal, detail: `${e.code}: ${e.message}` }
    }
    if (e.code === 'invalid_request') {
      if (/name is required/.test(e.message)) return { title: MCP_EXPORTS.errNameRequired }
      if (/identity/.test(e.message)) return { title: MCP_EXPORTS.errIdentityRequired }
      return { title: MCP_EXPORTS.errInvalidRequest, detail: `${e.code}: ${e.message}` }
    }
    return { title: MCP_EXPORTS.errGeneric, detail: `${e.code}: ${e.message}` }
  }
  return friendlyError(e)
}

/** 连接卡上的一行权限摘要；两项皆空返回 null。 */
export function permissionSummary(loginNames: string[], approvalNames: string[]): string | null {
  const parts: string[] = []
  if (loginNames.length > 0) parts.push(`${loginNames.length} 个工具需本人登录`)
  if (approvalNames.length > 0) parts.push(`${approvalNames.length} 个需审批`)
  return parts.length > 0 ? parts.join(' · ') : null
}
