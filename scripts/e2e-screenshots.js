#!/usr/bin/env node
/**
 * 截图回归：登录 → 仪表盘 / 运维 / CLI·Agent / 健康 / 模板。
 * 用法：node scripts/e2e-screenshots.js [baseURL]
 * 默认 http://127.0.0.1:5175（vite dev），需先起前端与 API（或 mock 8890）。
 */
'use strict'

const path = require('path')
const fs = require('fs')

// playwright-core 解析:**不能硬编码某台机器的路径** ——
// 之前写死了作者机器上 MiMo.app 里的副本,别的机器上这脚本直接跑不起来。
// 顺序:显式覆盖(PLAYWRIGHT_CORE) → 本地 node_modules → 全局 npm 根。
function resolvePlaywright() {
  if (process.env.PLAYWRIGHT_CORE) return require(process.env.PLAYWRIGHT_CORE)
  for (const id of ['playwright-core', 'playwright', '@playwright/test']) {
    try {
      return require(id)
    } catch (_) {
      /* 继续找下一个 */
    }
  }
  try {
    const { execSync } = require('child_process')
    const root = execSync('npm root -g', { encoding: 'utf8' }).trim()
    return require(root + '/playwright-core')
  } catch (_) {
    throw new Error(
      '找不到 playwright-core。装一个:npm i -g playwright-core\n' +
      '或用 PLAYWRIGHT_CORE=/path/to/playwright-core 指定路径。'
    )
  }
}
const pw = resolvePlaywright()

const BASE = process.argv[2] || 'http://127.0.0.1:5175'
const OUT = process.env.SHOT_DIR ||
  path.join(__dirname, '..', 'gui-test-screenshots', new Date().toISOString().slice(0, 10) + '-e2e')

async function main() {
  fs.mkdirSync(OUT, { recursive: true })
  const browser = await pw.chromium.launch({ headless: true })
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
  const shot = async (name) => {
    await page.screenshot({ path: path.join(OUT, name + '.png'), fullPage: true })
    console.log('shot', name)
  }

  // **同一个 key 要用在两处**:输入框填的与写进 localStorage 的必须是同一个值。
  // 之前输入框用环境变量、localStorage 硬编码 'test-api-key' ⇒ 两者不一致
  // ⇒ 401 ⇒ 被踢回登录页 ⇒ 后面截的全都是登录页(截图看着"成功"其实是废片)。
  const API_KEY = process.env.GITFERRY_E2E_KEY || 'test-api-key'
  await page.goto(BASE + '/login', { waitUntil: 'networkidle', timeout: 20000 })
  await shot('01-login')
  await page.locator('input').first().fill(API_KEY)
  await page.locator('button.ant-btn-primary').first().click()
  await page.waitForTimeout(1500)
  await page.evaluate((k) => localStorage.setItem('git-sync-api-key', k), API_KEY)
  await shot('02-after-login')

  const routes = [
    ['03-dashboard', '/dashboard'],
    ['04-ops', '/ops'],
    ['05-devhub', '/settings/dev'],
    ['06-sync-tasks', '/sync'],
    // AI 设置页:后端可插拔后这里多出后端选择器与外部 agent 参数,必须进回归截图
    ['10-ai-settings', '/settings/ai'],
  ]
  for (const [name, p] of routes) {
    await page.goto(BASE + p, { waitUntil: 'networkidle', timeout: 20000 })
    await page.waitForTimeout(1000)
    await shot(name)
    console.log(name, 'text_len=', (await page.locator('body').innerText()).length)
  }

  await page.goto(BASE + '/ops', { waitUntil: 'networkidle' })
  await page.waitForTimeout(800)
  const tabs = page.locator('.ant-tabs-tab')
  const n = await tabs.count()
  for (let i = 0; i < n; i++) {
    const label = (await tabs.nth(i).innerText().catch(() => '')).replace(/\s+/g, ' ').trim()
    if (/健康/.test(label)) {
      await tabs.nth(i).click(); await page.waitForTimeout(700); await shot('07-health-panel')
    }
    if (/模板/.test(label)) {
      await tabs.nth(i).click(); await page.waitForTimeout(700); await shot('08-templates')
    }
    if (/概览|总览/.test(label)) {
      await tabs.nth(i).click(); await page.waitForTimeout(700); await shot('09-ops-overview')
    }
  }

  await browser.close()
  console.log('DONE →', OUT)
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
