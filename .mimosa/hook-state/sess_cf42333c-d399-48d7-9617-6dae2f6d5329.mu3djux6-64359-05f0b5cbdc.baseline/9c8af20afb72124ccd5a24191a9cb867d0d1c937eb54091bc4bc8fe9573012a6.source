import { message as staticMessage, App as AntApp } from 'ant-design-vue'
import { ApiError } from '@/api/http'

// ant-design-vue 4 的静态 message API 不走 App 上下文,渲染不可靠(容器挂载但
// notice 不出现)。这里优先使用由 <a-app> 内部子组件注入的、与 App 上下文绑定
// 的 message 实例;未注入时(如单元测试)回退到静态 API。
type StaticMessage = typeof staticMessage

let appMessage: StaticMessage | null = null

/**
 * 由 <a-app> 内部子组件调用,注入与 App 上下文绑定的 message 实例。
 * useApp 实例与静态 message 在 4.2.x 方法签名略有差异,但运行时等价,故做一次等价转换。
 */
export function registerMessage(instance: ReturnType<typeof AntApp.useApp>['message']): void {
  appMessage = instance as unknown as StaticMessage
}

function api(): StaticMessage {
  return appMessage ?? staticMessage
}

/** 统一错误提示:可传入异常(自动提取 message)或直接传入文案字符串 */
export function notifyError(e: unknown, fallback = '操作失败'): void {
  let text: string
  if (typeof e === 'string') {
    text = e
  } else if (e instanceof ApiError) {
    text = e.message
  } else if (e instanceof Error) {
    text = e.message
  } else {
    text = fallback
  }
  api().error(text)
}

export function notifySuccess(text: string): void {
  api().success(text)
}

export function notifyWarning(text: string): void {
  api().warning(text)
}

export function notifyInfo(text: string): void {
  api().info(text)
}
