// Run against the dedicated local AlumniBridge stack. Creates and removes synthetic accounts only.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const { randomUUID } = require('node:crypto')
const { execFileSync } = require('node:child_process')
const { mkdirSync } = require('node:fs')
const base = process.env.WEB_URL || 'http://localhost:5173'
const issuer = process.env.KEYCLOAK_URL || 'http://localhost:18085'
const shots = process.env.SCREENSHOT_DIR || '.impeccable/review'
const assert = (value, message) => {
  if (!value) throw new Error(message)
}
async function capture(page, name, width) {
  await page.setViewportSize({ width, height: 1000 })
  await page.addStyleTag({
    content: 'html { scroll-behavior: auto !important; }',
  })
  await page.evaluate(() =>
    window.scrollTo({ top: 0, left: 0, behavior: 'instant' }),
  )
  // Capture a single tall viewport to avoid Chrome compositor tiling artifacts on mobile.
  const height = await page.evaluate(
    () => document.documentElement.scrollHeight,
  )
  await page.setViewportSize({ width, height })
  await page.evaluate(
    () =>
      new Promise((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(resolve)),
      ),
  )
  await page.waitForTimeout(150)
  await page.screenshot({
    animations: 'disabled',
    path: `${shots}/${name}.png`,
    fullPage: false,
  })
  await page.setViewportSize({ width, height: width < 700 ? 844 : 1000 })
}
async function saveDialog(page, label) {
  await page.getByRole('button', { name: label }).click()
  await page.getByRole('dialog').waitFor({ state: 'hidden' })
}
const cli = process.env.CONTAINER_CLI || 'docker'
async function run() {
  if (
    !process.env.KEYCLOAK_ADMIN_USERNAME ||
    !process.env.KEYCLOAK_ADMIN_PASSWORD
  )
    throw new Error(
      'Set local Keycloak admin credentials for synthetic-account cleanup.',
    )
  mkdirSync(shots, { recursive: true })
  const browser = await chromium.launch({
    executablePath:
      process.env.CHROME_EXECUTABLE ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    headless: true,
    args: ['--disable-gpu'],
  })
  const users = []
  try {
    for (const role of ['student', 'alumni']) {
      const username = `phase-d-${randomUUID()}`
      users.push(username)
      const password = `Test-${randomUUID()}!`
      const context = await browser.newContext({
        viewport: { width: 1440, height: 1000 },
      })
      const page = await context.newPage()
      page.setDefaultTimeout(15000)
      const errors = []
      page.on('pageerror', (error) => errors.push(error.message))
      await page.goto(base)
      await page.getByRole('button', { name: /Join the community/ }).click()
      await page.waitForURL(/registrations/)
      for (const [id, value] of Object.entries({
        username,
        password,
        'password-confirm': password,
        email: `${username}@example.invalid`,
        firstName: 'Ari',
        lastName: 'Demo',
      }))
        await page.locator(`#${id}`).fill(value)
      await page.locator('input[type=submit]').click()
      await page.waitForURL(`${base}/**`)
      await page
        .getByRole('link', { name: /Your professional profile/ })
        .click()
      await page.getByRole('heading', { name: /Tell us where/ }).waitFor()
      if (role === 'alumni') {
        await page.locator('input[value=alumni]').check()
        await page.locator('#graduation-year').fill('2020')
      }
      await page.locator('#display-name').fill('Ari Demo')
      await page.getByRole('button', { name: 'Complete onboarding' }).click()
      await page.waitForURL('**/account')
      await page.getByRole('link', { name: 'My Profile', exact: true }).click()
      await page.getByRole('heading', { name: 'Ari Demo' }).waitFor()
      await page.getByText('Private', { exact: true }).waitFor()
      if (role === 'alumni')
        await page.getByText('Alumni · verification pending').waitFor()
      // Keyboard dismissal and return to the invoking action.
      await page
        .getByRole('button', { name: 'Edit profile', exact: true })
        .click()
      await page.keyboard.press('Escape')
      assert(
        (await page.getByRole('dialog').count()) === 0,
        'Escape did not dismiss the editor',
      )
      assert(
        await page
          .getByRole('button', { name: 'Edit profile', exact: true })
          .evaluate((el) => el === document.activeElement),
        'Dialog did not restore focus',
      )
      await page
        .getByRole('button', { name: 'Edit profile', exact: true })
        .click()
      for (const [label, value] of Object.entries({
        'Display name': 'Ari Demo',
        'Professional headline':
          'Mathematics educator · learning through practice',
        'General location': 'Jakarta, Indonesia',
        'Current job title': 'Learning Designer',
        'Current company': 'Demo Learning Studio',
        'Public profile slug': username,
        About:
          'Synthetic browser test profile. I turn mathematical ideas into thoughtful learning experiences.',
      }))
        await page.getByLabel(label, { exact: true }).fill(value)
      await saveDialog(page, 'Save profile')
      await page.getByText('Profile saved.', { exact: true }).waitFor()
      await page.getByRole('button', { name: 'Add experience' }).click()
      for (const [label, value] of Object.entries({
        Company: 'Demo Learning Studio',
        'Job title': 'Learning Designer',
        'Employment type': 'Full-time',
        Location: 'Jakarta',
        'Start date': '2023-08-01',
        Description:
          'Designing accessible learning experiences. Synthetic test entry.',
      }))
        await page.getByLabel(label, { exact: true }).fill(value)
      await page.getByLabel('I currently work here').check()
      assert(
        await page.getByLabel('End date', { exact: true }).isDisabled(),
        'Current job end date must be disabled',
      )
      await saveDialog(page, 'Save experience')
      await page
        .getByRole('heading', { name: 'Learning Designer', exact: true })
        .waitFor()
      await page.getByRole('button', { name: 'Edit experience 1' }).click()
      await page
        .getByLabel('Job title', { exact: true })
        .fill('Senior Learning Designer')
      await saveDialog(page, 'Save experience')
      await page
        .getByRole('heading', { name: 'Senior Learning Designer' })
        .waitFor()
      await page.getByRole('button', { name: 'Add education' }).click()
      for (const [label, value] of Object.entries({
        Institution: 'Demo University',
        Degree: 'Bachelor of Education',
        'Field of study': 'Mathematics Education',
        'Start date': '2018-08-01',
        'End date': '2022-07-01',
        Description:
          'Synthetic education history, independent of membership verification.',
      }))
        await page.getByLabel(label, { exact: true }).fill(value)
      await saveDialog(page, 'Save education')
      await page.getByRole('heading', { name: 'Demo University' }).waitFor()
      await page.getByRole('button', { name: 'Edit education 1' }).click()
      await page
        .getByLabel('Degree', { exact: true })
        .fill('Bachelor of Education (Mathematics)')
      await saveDialog(page, 'Save education')
      await page
        .getByText(
          'Bachelor of Education (Mathematics) · Mathematics Education',
          { exact: true },
        )
        .waitFor()
      if (role === 'alumni') {
        await page.getByLabel('Search or add a skill').fill('Curriculum')
        await page
          .getByRole('button', { name: 'Curriculum Design', exact: true })
          .click()
      } else {
        await page.getByLabel('Search or add a skill').fill('Curriculum Design')
        await page
          .getByRole('button', { name: 'Add skill', exact: true })
          .click()
      }
      await page
        .getByRole('button', { name: 'Remove Curriculum Design' })
        .waitFor()
      await page.getByLabel('Search or add a skill').fill('curriculum design')
      assert(
        await page
          .getByRole('button', { name: 'Add skill', exact: true })
          .isDisabled(),
        'Duplicate skill allowed',
      )
      await page.getByLabel('Search or add a skill').fill('')
      const png = Buffer.from(
        await page.evaluate(() => {
          const canvas = document.createElement('canvas')
          canvas.width = 160
          canvas.height = 160
          const ctx = canvas.getContext('2d')
          ctx.fillStyle = '#dcece3'
          ctx.fillRect(0, 0, 160, 160)
          ctx.fillStyle = '#28644e'
          ctx.font = 'bold 56px sans-serif'
          ctx.textAlign = 'center'
          ctx.fillText('AD', 80, 101)
          return canvas.toDataURL('image/png').split(',')[1]
        }),
        'base64',
      )
      await page
        .getByLabel('Profile photo file')
        .setInputFiles({ name: 'demo.png', mimeType: 'image/png', buffer: png })
      await page.getByRole('button', { name: 'Save photo' }).click()
      await page.getByText('Photo saved.', { exact: true }).waitFor()
      if (role === 'student') {
        await page.addStyleTag({
          content: 'html { scroll-behavior: auto !important; }',
        })
        await page.evaluate(() =>
          window.scrollTo({ top: 0, left: 0, behavior: 'instant' }),
        )
        await capture(page, 'desktop', 1440)
        await page.setViewportSize({ width: 390, height: 844 })
        assert(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
          'Mobile profile overflows',
        )
        await capture(page, 'mobile', 390)
        await page.setViewportSize({ width: 1440, height: 1000 })
      }
      await page
        .getByRole('link', { name: 'Privacy Settings', exact: true })
        .click()
      await page.getByRole('radio', { name: /^Private/ }).waitFor()
      await page.getByRole('radio', { name: /^Public/ }).check()
      await page.getByRole('button', { name: 'Save privacy settings' }).click()
      await page.getByText(/Confirm publication consent/).waitFor()
      await page.getByRole('checkbox', { name: /I consent/ }).check()
      for (const name of [
        'Location',
        'Current company',
        'Current job title',
        'Work experience',
        'Education history',
        'Skills',
        'Profile photo',
      ])
        await page.getByRole('checkbox', { name, exact: true }).check()
      await page.getByRole('button', { name: 'Save privacy settings' }).click()
      await page.getByRole('link', { name: /View public profile/ }).waitFor()
      if (role === 'student') {
        await page.addStyleTag({
          content: 'html { scroll-behavior: auto !important; }',
        })
        await page.evaluate(() =>
          window.scrollTo({ top: 0, left: 0, behavior: 'instant' }),
        )
        await capture(page, 'privacy-desktop', 1440)
      }
      const anonymous = await browser.newContext({
        viewport: { width: 1440, height: 1000 },
      })
      const visitor = await anonymous.newPage()
      const protectedRequests = []
      visitor.on('request', (req) => {
        if (req.url().includes('/api/v1/users/'))
          protectedRequests.push(req.url())
      })
      await visitor.goto(`${base}/in/${username}`)
      await visitor.getByRole('heading', { name: 'Ari Demo' }).waitFor()
      await visitor
        .getByRole('heading', { name: 'Senior Learning Designer' })
        .waitFor()
      await visitor.getByRole('img', { name: "Ari Demo's profile" }).waitFor()
      assert(
        (await visitor
          .getByText('Not verified as alumni', { exact: true })
          .count()) === 1,
        'Unverified member falsely verified',
      )
      assert(
        (await visitor
          .getByRole('heading', { name: 'About', exact: true })
          .count()) === 0,
        'Hidden bio rendered publicly',
      )
      assert(
        !(await visitor.locator('main').innerText()).includes(
          '@example.invalid',
        ),
        'Email exposed publicly',
      )
      assert(!protectedRequests.length, 'Anonymous page requested owner data')
      if (role === 'student') {
        await capture(visitor, 'public-desktop', 1440)
        await visitor.setViewportSize({ width: 390, height: 844 })
        assert(
          await visitor.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
          'Mobile public page overflows',
        )
        await capture(visitor, 'public-mobile', 390)
      }
      await visitor.setViewportSize({ width: 390, height: 844 })
      await visitor
        .getByRole('heading', { name: 'Education', exact: true })
        .scrollIntoViewIfNeeded()
      const scrollBefore = await visitor.evaluate(() => window.scrollY)
      let releaseRefresh
      await visitor.route(
        `**/api/v1/profiles/${username}`,
        async (route) => {
          await new Promise((resolve) => {
            releaseRefresh = resolve
          })
          await route.continue()
        },
        { times: 1 },
      )
      try {
        await visitor
          .locator('.public-revalidation')
          .waitFor({ timeout: 25000 })
        assert(
          Math.abs(
            (await visitor.evaluate(() => window.scrollY)) - scrollBefore,
          ) <= 2,
          'Background privacy check collapsed the public story',
        )
        assert(
          (await visitor.getByRole('heading', { name: 'Ari Demo' }).count()) ===
            0,
          'Stale public data remained during validation',
        )
      } finally {
        if (releaseRefresh) releaseRefresh()
      }
      await visitor.getByRole('heading', { name: 'Ari Demo' }).waitFor()
      await visitor.unroute(`**/api/v1/profiles/${username}`)
      console.log(
        `${role}: mobile reading position preserved during background privacy validation PASSED`,
      )
      // Change slug while public; the old link must follow the backend's canonical result.
      await page.getByRole('link', { name: 'My Profile', exact: true }).click()
      await page
        .getByRole('button', { name: 'Edit profile', exact: true })
        .click()
      await page.getByLabel('Public profile slug').fill(`${username}-new`)
      await saveDialog(page, 'Save profile')
      await page.getByText('Profile saved.', { exact: true }).waitFor()
      await visitor.goto(`${base}/in/${username}`)
      await visitor.waitForURL(`**/in/${username}-new`)
      await visitor.getByRole('heading', { name: 'Ari Demo' }).waitFor()
      await page
        .getByRole('link', { name: 'Privacy Settings', exact: true })
        .click()
      await page.getByRole('radio', { name: /^Private/ }).check()
      await page.getByRole('button', { name: 'Save privacy settings' }).click()
      await page.getByText(/Profile is private. New anonymous/).waitFor()
      await visitor.reload()
      await visitor
        .getByRole('heading', { name: 'Profile unavailable' })
        .waitFor()
      assert(
        (await visitor.getByRole('heading', { name: 'Ari Demo' }).count()) ===
          0,
        'Cached private data displayed',
      )
      await visitor.goto(`${base}/in/${username}`)
      await visitor
        .getByRole('heading', { name: 'Profile unavailable' })
        .waitFor()
      await anonymous.close()
      await page.getByRole('link', { name: 'My Profile', exact: true }).click()
      await page.getByRole('button', { name: 'Delete profile photo' }).click()
      await page
        .getByRole('button', { name: 'Remove entry', exact: true })
        .click()
      await page.getByText('Photo removed.', { exact: true }).waitFor()
      await page.getByRole('button', { name: 'Delete experience 1' }).click()
      await page
        .getByRole('button', { name: 'Remove entry', exact: true })
        .click()
      await page.getByText(/Every path starts somewhere/).waitFor()
      await page.getByRole('button', { name: 'Delete education 1' }).click()
      await page
        .getByRole('button', { name: 'Remove entry', exact: true })
        .click()
      await page.getByText(/Add where you learned/).waitFor()
      await page
        .getByRole('button', { name: 'Remove Curriculum Design' })
        .click()
      await page.getByText(/Your skills will appear here/).waitFor()
      for (const route of ['/profile', '/dashboard', '/settings', '/account']) {
        await page.goto(base + route)
        await page.locator('#member-main h1').waitFor()
        await page.waitForURL(
          base + (route === '/settings' ? '/settings/privacy' : route),
        )
        await page.reload()
        await page.locator('#member-main h1').waitFor()
        assert(
          !(await page.getByText('Invalid parameter: redirect_uri').count()),
          'OIDC redirect failure',
        )
      }
      await page.getByRole('button', { name: 'Sign out' }).click()
      await page.waitForURL(`${base}/**`)
      await page.goto(`${base}/profile`)
      await page
        .getByRole('heading', { name: 'Sign in to continue.' })
        .waitFor()
      assert(
        (await page.getByText('Ari Demo', { exact: true }).count()) === 0,
        'Owner data visible after logout',
      )
      await page.getByRole('button', { name: 'Sign in', exact: true }).click()
      await page.locator('#username').fill(username)
      await page.locator('#password').fill(password)
      await page.locator('#kc-login').click()
      await page
        .getByRole('heading', { name: 'Ari Demo', exact: true })
        .waitFor()
      assert(!errors.length, `Browser errors: ${errors.join('; ')}`)
      console.log(
        `${role}: registration/onboarding, profile edits, experience/education CRUD, skills, photo, explicit consent, anonymous privacy, historical slug, refresh, logout and login restoration PASSED`,
      )
      await context.close()
    }
  } finally {
    await browser.close()
    const tokenResponse = await fetch(
      `${issuer}/realms/master/protocol/openid-connect/token`,
      {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'password',
          client_id: 'admin-cli',
          username: process.env.KEYCLOAK_ADMIN_USERNAME,
          password: process.env.KEYCLOAK_ADMIN_PASSWORD,
        }),
      },
    )
    assert(tokenResponse.ok, 'Cleanup admin authentication failed')
    const { access_token } = await tokenResponse.json()
    for (const username of users) {
      const result = await fetch(
        `${issuer}/admin/realms/alumnibridge/users?exact=true&username=${encodeURIComponent(username)}`,
        { headers: { Authorization: `Bearer ${access_token}` } },
      )
      assert(result.ok, 'Cleanup user lookup failed')
      for (const user of await result.json()) {
        assert(/^[0-9a-f-]{36}$/.test(user.id), 'Invalid synthetic identity')
        assert(
          (
            await fetch(
              `${issuer}/admin/realms/alumnibridge/users/${user.id}`,
              {
                method: 'DELETE',
                headers: { Authorization: `Bearer ${access_token}` },
              },
            )
          ).ok,
          'Cleanup user deletion failed',
        )
        execFileSync(
          cli,
          [
            'compose',
            '-p',
            'alumnibridge',
            'exec',
            '-T',
            'postgres',
            'psql',
            '-U',
            'alumnibridge',
            '-d',
            'alumnibridge',
            '-v',
            'ON_ERROR_STOP=1',
            '-c',
            `DELETE FROM users WHERE oidc_subject='${user.id}'`,
          ],
          { stdio: 'ignore' },
        )
      }
    }
    console.log(
      'Disposable accounts removed; photo cleanup remains with the durable worker; slug tombstones preserved.',
    )
  }
}
run().catch((error) => {
  console.error(error.stack)
  process.exitCode = 1
})
