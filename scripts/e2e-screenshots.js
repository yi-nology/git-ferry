#!/usr/bin/env node
/**
 * 截图回归：登录 → 仪表盘 / 运维 / CLI·Agent / 健康 / 模板。
 * 用法：node scripts/e2e-screenshots.js [baseURL]
 * 默认 http://127.0.0.1:5175（vite dev），需先起前端与 API（或 mock 8890）。
 */
'use strict'

const path = require('path')
const fs = require('fs')

const CORE = '/Applications/Xiaomi MiMo.app/Contents/lib/node_modules/@playwright/cli/node_modules/playwright-core'
const pw = require(CORE)

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

  await page.goto(BASE + '/login', { waitUntil: 'networkidle', timeout: 20000 })
  await shot('01-login')
  await page.locator('input').first().fill(process.env.GITFERRY_E2E_KEY || 'test-api-key')
  await page.locator('button.ant-btn-primary').first().click()
  await page.waitForTimeout(1500)
  await page.evaluate(() => localStorage.setItem('git-sync-api-key', 'test-api-key'))
  await shot('02-after-login')

  const routes = [
    ['03-dashboard', '/dashboard'],
    ['04-ops', '/ops'],
    ['05-devhub', '/settings/dev'],
    ['06-sync-tasks', '/sync'],
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
