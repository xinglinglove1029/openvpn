const { chromium } = require('../frontend/node_modules/.pnpm/playwright@1.62.1/node_modules/playwright');

(async () => {
  const browser = await chromium.launch({ headless: true, channel: 'msedge' });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 2,
  });
  const page = await context.newPage();

  const baseUrl = 'http://127.0.0.1:5173';
  const shotsDir = 'F:/develop/openvpn/doc/screenshots';

  // 1. Login page screenshot
  console.log('1. Capturing login page...');
  await page.goto(`${baseUrl}/login`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(3000);
  await page.screenshot({ path: `${shotsDir}/login.png`, fullPage: false });
  console.log('   -> login.png');

  // 2. Login as admin
  console.log('2. Logging in as admin...');
  // Use placeholder-based selectors
  await page.locator('input[placeholder="请输入 OpenVPN 管理账号"]').fill('admin');
  await page.locator('input[placeholder="请输入登录密码"]').fill('admin');
  // Click the submit button (text is "登 录" with spaces)
  await page.locator('button[type="submit"]').click();
  // Wait for navigation to overview
  await page.waitForURL('**/overview', { timeout: 15000 }).catch(() => {
    console.log('   (URL wait timeout, checking current URL...)');
  });
  await page.waitForTimeout(3000);
  console.log('   -> Logged in, current URL:', page.url());

  // 3. Overview/Dashboard
  console.log('3. Capturing overview page...');
  await page.waitForTimeout(2000);
  await page.screenshot({ path: `${shotsDir}/overview.png`, fullPage: false });
  console.log('   -> overview.png');

  // 4. Users page
  console.log('4. Capturing users page...');
  await page.goto(`${baseUrl}/users`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2500);
  await page.screenshot({ path: `${shotsDir}/users.png`, fullPage: false });
  console.log('   -> users.png');

  // 5. Clients page
  console.log('5. Capturing clients page...');
  await page.goto(`${baseUrl}/clients`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2500);
  await page.screenshot({ path: `${shotsDir}/clients.png`, fullPage: false });
  console.log('   -> clients.png');

  // 6. Firewall page
  console.log('6. Capturing firewall page...');
  await page.goto(`${baseUrl}/firewall`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2500);
  await page.screenshot({ path: `${shotsDir}/firewall.png`, fullPage: false });
  console.log('   -> firewall.png');

  // 7. Certs page
  console.log('7. Capturing certs page...');
  await page.goto(`${baseUrl}/certs`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2500);
  await page.screenshot({ path: `${shotsDir}/certs.png`, fullPage: false });
  console.log('   -> certs.png');

  // 8. Audit page
  console.log('8. Capturing audit page...');
  await page.goto(`${baseUrl}/audit`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2500);
  await page.screenshot({ path: `${shotsDir}/audit.png`, fullPage: false });
  console.log('   -> audit.png');

  // 9. History page
  console.log('9. Capturing history page...');
  await page.goto(`${baseUrl}/history`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2500);
  await page.screenshot({ path: `${shotsDir}/history.png`, fullPage: false });
  console.log('   -> history.png');

  // 10. Settings page
  console.log('10. Capturing settings page...');
  await page.goto(`${baseUrl}/settings`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2500);
  await page.screenshot({ path: `${shotsDir}/settings.png`, fullPage: false });
  console.log('   -> settings.png');

  // 11. Notifications page
  console.log('11. Capturing notifications page...');
  await page.goto(`${baseUrl}/notifications`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2500);
  await page.screenshot({ path: `${shotsDir}/notifications.png`, fullPage: false });
  console.log('   -> notifications.png');

  // 12. Channel providers page
  console.log('12. Capturing channels page...');
  await page.goto(`${baseUrl}/channels`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2500);
  await page.screenshot({ path: `${shotsDir}/channels.png`, fullPage: false });
  console.log('   -> channels.png');

  // 13. Roles page
  console.log('13. Capturing roles page...');
  await page.goto(`${baseUrl}/roles`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2500);
  await page.screenshot({ path: `${shotsDir}/roles.png`, fullPage: false });
  console.log('   -> roles.png');

  // 14. AI Assistant widget
  console.log('14. Capturing AI assistant widget...');
  await page.goto(`${baseUrl}/overview`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  // The AI widget trigger is a floating button - look for it by icon or class
  const aiTrigger = page.locator('button:has(svg.lucide-bot), button:has(svg.lucide-sparkles), [class*="ai-trigger"], [class*="AIWidget"]').first();
  if (await aiTrigger.count() > 0) {
    await aiTrigger.click().catch(() => {});
    await page.waitForTimeout(3000);
  } else {
    // Try clicking by title or aria-label
    const altTrigger = page.locator('[title*="AI"], [aria-label*="AI"], [title*="助手"], [aria-label*="助手"]').first();
    if (await altTrigger.count() > 0) {
      await altTrigger.click().catch(() => {});
      await page.waitForTimeout(3000);
    }
  }
  await page.screenshot({ path: `${shotsDir}/ai-assistant.png`, fullPage: false });
  console.log('   -> ai-assistant.png');

  await browser.close();
  console.log('\nAll screenshots captured successfully!');
})().catch(err => {
  console.error('Error:', err.message);
  process.exit(1);
});
