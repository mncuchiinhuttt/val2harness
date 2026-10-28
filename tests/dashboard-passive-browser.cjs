// Run against the opt-in TestBrowserFixture. All requests stay on loopback.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

async function main() {
  const fixturePath = process.env.VAL2HARNESS_BROWSER_FIXTURE;
  assert.ok(fixturePath, 'set VAL2HARNESS_BROWSER_FIXTURE to the Go fixture JSON');
  const fixture = JSON.parse(fs.readFileSync(fixturePath, 'utf8'));
  const playwright = require(process.env.VAL2HARNESS_PLAYWRIGHT_MODULE || 'playwright');
  const browser = await playwright.chromium.launch({ headless: true, channel: process.env.VAL2HARNESS_BROWSER_EXECUTABLE ? undefined : 'msedge', executablePath: process.env.VAL2HARNESS_BROWSER_EXECUTABLE });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1100 } });
  const requests = [], errors = [], external = [];
  await context.route('**/*', route => {
    const url = new URL(route.request().url());
    if (!['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname)) { external.push(url.origin); return route.abort(); }
    return route.continue();
  });
  const page = await context.newPage();
  page.on('pageerror', error => errors.push(error.message));
  page.on('request', request => requests.push({ url: request.url(), method: request.method(), auth: request.headers().authorization }));
  const stats = () => page.evaluate(async () => (await fetch('/api/local/fixture-stats', { headers: { Authorization: 'Bearer ' + sessionStorage.getItem('val2harness.session') } })).json());
  const screenshotDir = path.join(path.dirname(fixturePath), 'passive-ui');
  fs.mkdirSync(screenshotDir, { recursive: true });
  try {
    await page.goto(fixture.ticketURL);
    await page.waitForFunction(() => document.getElementById('modelsCount').textContent === '1 Models');
    await page.waitForFunction(() => document.getElementById('analyticsLiveText').textContent === 'Live observations');
    assert.equal(await page.locator('#localAuthPanel').isHidden(), true);
    assert.equal(await page.evaluate(() => location.hash), '');
    assert.equal(await page.locator('#quotaStatusText').textContent(), 'Unknown');
    assert.equal(await page.locator('#quotaRemainingText').textContent(), 'Unknown');
    assert.equal(await page.locator('#quotaResetText').textContent(), 'Unknown');
    assert.equal((await stats()).chat_requests, 0, 'page load sent inference');
    const live = requests.find(r => new URL(r.url).pathname === '/api/local/analytics/live');
    assert.ok(live?.auth?.startsWith('Bearer '), 'SSE lacked the owner session header');
    assert.ok(!requests.some(r => /\/api\/local\/(version|update)$/.test(new URL(r.url).pathname)), 'page load checked releases or updated');
    await page.locator('#checkQuotaBtn').click();
    assert.equal((await stats()).chat_requests, 0, 'refresh sent inference');
    await page.screenshot({ path: path.join(screenshotDir, 'desktop-unknown.png'), fullPage: true });
    for (let i = 0; i < 2; i++) {
      await page.getByRole('button', { name: 'Model Playground', exact: true }).click();
      await page.locator('#chatInput').fill('Synthetic browser observation ' + i);
      await page.locator('#sendChatBtn').click();
      await page.waitForFunction(() => !document.getElementById('sendChatBtn').disabled);
    }
    await page.getByRole('button', { name: 'Analytics & Usage', exact: true }).click();
    await page.waitForFunction(() => document.getElementById('analyticsMeasurementText').textContent.includes('1 measured · 1 without measured usage'));
    const recent = await page.locator('#recentLogsBody td').allTextContents();
    assert.ok(recent.includes('Unknown'), 'missing usage became zero');
    assert.ok(recent.includes('0'), 'measured zero disappeared');
    assert.equal(await page.locator('#heatmapSvg rect').count() > 300, true, 'heatmap missing');
    await page.screenshot({ path: path.join(screenshotDir, 'desktop-analytics.png'), fullPage: true });
    await page.getByRole('button', { name: 'Overview & Status', exact: true }).click();
    await page.waitForFunction(() => document.getElementById('quotaStatusText').textContent === 'Available at last observation');
    assert.equal((await stats()).chat_requests, 2);
    await page.setViewportSize({ width: 390, height: 844 });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'mobile layout overflows the viewport');
    await page.screenshot({ path: path.join(screenshotDir, 'mobile-observations.png'), fullPage: true });
    await page.getByRole('button', { name: 'Analytics & Usage', exact: true }).click();
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'mobile analytics overflows the viewport');
    await page.screenshot({ path: path.join(screenshotDir, 'mobile-analytics.png'), fullPage: true });
    await page.getByRole('button', { name: 'Overview & Status', exact: true }).click();
    await page.locator('#clearObservationsBtn').click();
    await page.waitForFunction(() => document.getElementById('quotaStatusText').textContent === 'Unknown');
    assert.equal((await stats()).chat_requests, 2, 'clear sent inference');
    assert.equal(await page.locator('#quotaObservedText').textContent(), 'No response observed yet');
    assert.deepEqual(errors, [], 'browser script errors');
    assert.deepEqual(external, [], 'browser attempted external requests');
    console.log('PASS: desktop/mobile passive status, authenticated live analytics, measured zero/missing usage, heatmap, refresh and clear; 0 incidental inference or external requests. Screenshots: ' + screenshotDir);
  } finally {
    await browser.close();
    fs.writeFileSync(fixturePath + '.stop', 'stop');
  }
}
main().catch(error => { console.error(error.stack); process.exitCode = 1; }).finally(() => {
  if (process.env.VAL2HARNESS_BROWSER_FIXTURE) fs.writeFileSync(process.env.VAL2HARNESS_BROWSER_FIXTURE + '.stop', 'stop');
});
