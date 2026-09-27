import { platformApi, type Platform } from '@/api/platform'
import { notifyError, notifyInfo, notifySuccess, notifyWarning } from '@/utils/notify'

/**
 * 批量从各平台拉取仓库清单。
 * 并行同步 + 部分失败可见,避免一个平台拖垮整体或静默丢失败。
 */
export function usePlatformSync(onDone?: () => void) {
  async function syncAllPlatforms() {
    try {
      const data = await platformApi.list()
      const platforms = data.platforms || []
      if (!platforms.length) {
        notifyWarning('请先在系统-平台管理中配置平台')
        return
      }
      notifyInfo(`正在同步 ${platforms.length} 个平台的仓库...`)
      const results = await Promise.allSettled(
        platforms.map(async (p: Platform) => {
          const result = await platformApi.syncRepos(p.key)
          return { name: p.name, count: result.synced_count || 0 }
        }),
      )
      let total = 0
      const failed: string[] = []
      results.forEach((r, idx) => {
        if (r.status === 'fulfilled') {
          total += r.value.count
        } else {
          failed.push(platforms[idx]?.name || '未知')
        }
      })
      onDone?.()
      if (failed.length) {
        notifyError(`部分平台同步失败: ${failed.join('、')}`)
      } else {
        notifySuccess(`同步完成，共导入 ${total} 个仓库`)
      }
    } catch (e) {
      notifyError(e, '同步平台仓库失败')
    }
  }

  return { syncAllPlatforms }
}
