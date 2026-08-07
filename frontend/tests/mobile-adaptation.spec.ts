import { test, expect, type Page } from '@playwright/test';

const ADMIN_USER = process.env.ADMIN_USER || 'admin';
const ADMIN_PASS = process.env.ADMIN_PASS || 'admin';

const PAGES = [
  { name: 'overview', path: '/overview' },
  { name: 'users', path: '/users' },
  { name: 'clients', path: '/clients' },
  { name: 'firewall', path: '/firewall' },
  { name: 'history', path: '/history' },
  { name: 'certs', path: '/certs' },
  { name: 'audit', path: '/audit' },
  { name: 'settings', path: '/settings' },
  { name: 'notifications', path: '/notifications' },
  { name: 'channels', path: '/channels' },
  { name: 'profile', path: '/profile' },
  { name: 'roles', path: '/roles' },
  { name: 'permissions', path: '/permissions' },
];

async function login(page: Page) {
  await page.goto('/login');
  await page.getByPlaceholder('请输入 OpenVPN 管理账号').fill(ADMIN_USER);
  await page.getByPlaceholder('请输入登录密码').fill(ADMIN_PASS);
  await page.getByRole('button', { name: '登 录' }).click();
  await page.waitForURL('**/overview', { timeout: 15_000 });
  await page.waitForLoadState('domcontentloaded');
}

async function checkNoHorizontalOverflow(page: Page, pageName: string) {
  const metrics = await page.evaluate(() => {
    const body = document.body;
    const html = document.documentElement;
    return {
      scrollWidth: Math.max(body.scrollWidth, html.scrollWidth),
      clientWidth: html.clientWidth,
    };
  });
  expect(
    metrics.scrollWidth <= metrics.clientWidth + 1,
    `[${pageName}] 水平溢出: scrollWidth=${metrics.scrollWidth} > clientWidth=${metrics.clientWidth}`,
  ).toBeTruthy();
}

async function checkTouchTargets(page: Page, pageName: string) {
  const smallTargets = await page.evaluate(() => {
    const buttons = Array.from(document.querySelectorAll('button, a, [role="button"], input[type="checkbox"], input[type="radio"]'));
    const visible = buttons.filter((el) => {
      const rect = el.getBoundingClientRect();
      const style = window.getComputedStyle(el);
      return (
        rect.width > 0 &&
        rect.height > 0 &&
        style.visibility !== 'hidden' &&
        style.display !== 'none' &&
        style.opacity !== '0'
      );
    });
    return visible
      .map((el) => {
        const rect = el.getBoundingClientRect();
        return {
          tag: el.tagName,
          text: (el.textContent || '').trim().substring(0, 30),
          w: Math.round(rect.width),
          h: Math.round(rect.height),
        };
      })
      .filter((t) => t.w > 0 && t.h > 0 && (t.w < 36 || t.h < 36));
  });
  expect(
    smallTargets.length === 0,
    `[${pageName}] ${smallTargets.length} 个触摸目标过小 (<36px): ${JSON.stringify(smallTargets.slice(0, 5))}`,
  ).toBeTruthy();
}

async function checkNoCutoffElements(page: Page, pageName: string) {
  const cutoffs = await page.evaluate(() => {
    const viewportW = window.innerWidth;
    const viewportH = window.innerHeight;
    const els = Array.from(document.querySelectorAll('main, [role="main"], .reference-login-card, [data-slot="dialog-content"]'));
    const issues: string[] = [];
    for (const el of els) {
      const rect = el.getBoundingClientRect();
      if (rect.right > viewportW + 2) {
        issues.push(`${el.tagName}.${el.className.substring(0, 40)} 右侧溢出 ${Math.round(rect.right - viewportW)}px`);
      }
      if (rect.left < -2) {
        issues.push(`${el.tagName}.${el.className.substring(0, 40)} 左侧溢出 ${Math.round(-rect.left)}px`);
      }
    }
    return issues;
  });
  expect(
    cutoffs.length === 0,
    `[${pageName}] 元素被截断: ${cutoffs.join('; ')}`,
  ).toBeTruthy();
}

function collectConsoleErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      const text = msg.text();
      if (!text.includes('ERR_ABORTED') && !text.includes('Download the React DevTools')) {
        errors.push(text);
      }
    }
  });
  return errors;
}

// ─── 测试组 1: 登录页 ───

test.describe('登录页移动端适配', () => {
  test('登录页无水平溢出且布局正确', async ({ page }) => {
    const errors = collectConsoleErrors(page);
    await page.goto('/login');
    await page.waitForLoadState('domcontentloaded');
    await checkNoHorizontalOverflow(page, 'login');
    await checkNoCutoffElements(page, 'login');
    await expect(page.getByPlaceholder('请输入 OpenVPN 管理账号')).toBeVisible();
    await expect(page.getByPlaceholder('请输入登录密码')).toBeVisible();
    await expect(page.getByRole('button', { name: '登 录' })).toBeVisible();
    expect(errors.length, `控制台错误: ${errors.join('; ')}`).toBe(0);
    await page.screenshot({ path: `screenshots/login-${test.info().project.name}.png`, fullPage: false });
  });
});

// ─── 测试组 2: 登录后各页面 ───

test.describe('认证页面移动端适配', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  for (const { name, path } of PAGES) {
    test(`${name} 页面适配检查`, async ({ page }) => {
      const errors = collectConsoleErrors(page);
      await page.goto(path);
      await page.waitForLoadState('domcontentloaded');
      await page.waitForTimeout(2000);

      await checkNoHorizontalOverflow(page, name);
      await checkNoCutoffElements(page, name);
      await checkTouchTargets(page, name);

      expect(errors.length, `[${name}] 控制台错误: ${errors.join('; ')}`).toBe(0);

      await page.screenshot({
        path: `screenshots/${name}-${test.info().project.name}.png`,
        fullPage: false,
      });
    });
  }
});

// ─── 测试组 3: 移动端交互 ───

test.describe('移动端交互行为', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('侧边栏汉堡菜单可展开收起', async ({ page }) => {
    await page.waitForLoadState('domcontentloaded');
    const menuButton = page.getByRole('button', { name: '打开菜单' });
    if (await menuButton.isVisible({ timeout: 3000 }).catch(() => false)) {
      await menuButton.click();
      await page.waitForTimeout(500);
      await expect(page.getByRole('link', { name: '账号管理' })).toBeVisible();
      await page.screenshot({ path: `screenshots/sidebar-open-${test.info().project.name}.png` });
      const closeBtn = page.getByRole('button', { name: /关闭|收起|关闭菜单/ });
      if (await closeBtn.isVisible({ timeout: 1000 }).catch(() => false)) {
        await closeBtn.click();
      } else {
        await page.keyboard.press('Escape');
      }
      await page.waitForTimeout(300);
    }
  });

  test('Overview 统计卡片正确渲染', async ({ page }) => {
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(2000);
    const cards = page.locator('main').locator('div').filter({ hasText: /在线连接|账号总数|客户端配置|今日上线/ });
    const count = await cards.count();
    expect(count, 'Overview 应有统计卡片').toBeGreaterThan(0);
    await checkNoHorizontalOverflow(page, 'overview-cards');
  });

  test('Settings 表单字段垂直堆叠', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(500);
    const viewportW = page.viewportSize()?.width || 375;
    const formLayout = await page.evaluate((vw) => {
      const labels = Array.from(document.querySelectorAll('label'));
      const inputs = Array.from(document.querySelectorAll('input[type="text"], input[type="password"], input[type="number"]'));
      const labelInfo = labels.slice(0, 3).map((l) => {
        const r = l.getBoundingClientRect();
        return { x: Math.round(r.x), w: Math.round(r.width), text: (l.textContent || '').substring(0, 20) };
      });
      const inputInfo = inputs.slice(0, 3).map((i) => {
        const r = i.getBoundingClientRect();
        return { x: Math.round(r.x), w: Math.round(r.width) };
      });
      return { vw, labelInfo, inputInfo };
    }, viewportW);

    if (viewportW < 640) {
      const labelsFullWidth = formLayout.labelInfo.every((l) => l.x < 20 && l.w > viewportW * 0.5);
      expect(labelsFullWidth, `小屏下标签应左对齐全宽: ${JSON.stringify(formLayout.labelInfo)}`).toBeTruthy();
      const inputsFullWidth = formLayout.inputInfo.every((i) => i.w > viewportW * 0.5);
      expect(inputsFullWidth, `小屏下输入框应全宽: ${JSON.stringify(formLayout.inputInfo)}`).toBeTruthy();
    }
  });

  test('页面间导航无报错', async ({ page }) => {
    const navLinks = ['概览', '账号管理', '客户端', '系统设置'];
    for (const linkName of navLinks) {
      const link = page.getByRole('link', { name: linkName }).first();
      if (await link.isVisible({ timeout: 2000 }).catch(() => false)) {
        await link.click();
        await page.waitForLoadState('domcontentloaded');
        await page.waitForTimeout(300);
        await checkNoHorizontalOverflow(page, `nav-${linkName}`);
      }
    }
  });
});

// ─── 测试组 4: DataTable 移动端卡片视图 ───

test.describe('DataTable 移动端卡片模式', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('Users 页表格切换为卡片视图', async ({ page }) => {
    await page.goto('/users');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1000);
    const viewportW = page.viewportSize()?.width || 375;
    if (viewportW < 1024) {
      const table = page.locator('table');
      const tableVisible = await table.isVisible({ timeout: 2000 }).catch(() => false);
      expect(!tableVisible, '移动端不应显示传统表格').toBeTruthy();
      await page.screenshot({ path: `screenshots/users-cardview-${test.info().project.name}.png`, fullPage: false });
    }
  });

  test('Clients 页表格切换为卡片视图', async ({ page }) => {
    await page.goto('/clients');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1000);
    const viewportW = page.viewportSize()?.width || 375;
    if (viewportW < 1024) {
      const table = page.locator('table');
      const tableVisible = await table.isVisible({ timeout: 2000 }).catch(() => false);
      expect(!tableVisible, '移动端不应显示传统表格').toBeTruthy();
      await page.screenshot({ path: `screenshots/clients-cardview-${test.info().project.name}.png`, fullPage: false });
    }
  });
});
