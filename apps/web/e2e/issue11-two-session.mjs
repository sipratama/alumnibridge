import assert from 'node:assert/strict'
import { randomBytes } from 'node:crypto'
import { execFileSync, spawnSync } from 'node:child_process'
import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { chromium } from 'playwright'

const repo = resolve(import.meta.dirname, '../../..')
const project = process.env.ISSUE11_PROJECT
const webPort = process.env.WEB_PORT
const keycloakPort = process.env.KEYCLOAK_PORT
const reportPath = process.env.ISSUE11_REPORT
if (!project?.startsWith('abissue11e2e') || !webPort || !keycloakPort || !reportPath)
  throw new Error('Run via scripts/issue11-browser-e2e.sh with a disposable project')

const web = `http://localhost:${webPort}`
const keycloak = `http://localhost:${keycloakPort}`
const sourceSHA = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim()
const checks = []
let phase = 'setup'
let browser
let adminToken
let userID

function checked(name, status, expected) {
  assert.equal(status, expected, `${name}: unexpected HTTP status`)
  checks.push({ name, status })
}

async function admin(path, options = {}) {
  const response = await fetch(`${keycloak}/admin/realms/alumnibridge${path}`, {
    ...options,
    headers: { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json', ...options.headers },
  })
  return response
}

async function prepareIdentity() {
  const tokenResponse = await fetch(`${keycloak}/realms/master/protocol/openid-connect/token`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      grant_type: 'password', client_id: 'admin-cli',
      username: process.env.KEYCLOAK_ADMIN_USERNAME || 'localadmin',
      password: process.env.KEYCLOAK_ADMIN_PASSWORD || 'local_admin_only',
    }),
  })
  assert.equal(tokenResponse.status, 200, 'disposable Keycloak admin login')
  adminToken = (await tokenResponse.json()).access_token
  const clientsResponse = await admin('/clients?clientId=alumnibridge-web')
  assert.equal(clientsResponse.status, 200)
  const client = (await clientsResponse.json()).find((item) => item.clientId === 'alumnibridge-web')
  assert.ok(client, 'disposable web client exists')
  client.redirectUris = [...new Set([...client.redirectUris, `${web}/`])]
  client.webOrigins = [...new Set([...client.webOrigins, web])]
  assert.equal((await admin(`/clients/${client.id}`, { method: 'PUT', body: JSON.stringify(client) })).status, 204)

  const suffix = randomBytes(8).toString('hex')
  const username = `issue11e2e${suffix}`
  const email = `${username}@example.invalid`
  const password = `Disposable-${randomBytes(20).toString('base64url')}`
  const created = await admin('/users', {
    method: 'POST',
    body: JSON.stringify({
      username, email, enabled: true, emailVerified: true,
      firstName: 'Issue', lastName: 'Eleven',
      credentials: [{ type: 'password', value: password, temporary: false }],
    }),
  })
  assert.equal(created.status, 201, 'disposable Keycloak user creation')
  userID = created.headers.get('location')?.split('/').at(-1)
  return { username, email, password }
}

function verifyDisposableAlumni(email) {
  // The test alone owns this Compose project. This trusted moderation step
  // exercises the actual membership trigger without changing development DBs.
  const sql = `UPDATE memberships SET status='verified' WHERE user_id=(SELECT id FROM users WHERE email='${email}') AND role='alumni' AND status='pending';\n`
  const result = spawnSync('docker', [
    'exec', '-i', `${project}-postgres-1`, 'psql', '-v', 'ON_ERROR_STOP=1',
    '-U', 'alumnibridge', '-d', 'alumnibridge',
  ], { input: sql, encoding: 'utf8', timeout: 15000 })
  assert.equal(result.status, 0, 'disposable PostgreSQL verification step')
  assert.match(result.stdout, /UPDATE 1\b/, 'exactly one disposable membership verified')
}

async function signInAndOnboard(identity) {
  const context = await browser.newContext()
  try {
    const page = await context.newPage()
    await page.goto(web, { waitUntil: 'networkidle' })
    await page.getByRole('button', { name: 'Sign in' }).first().click()
    await page.locator('#username').fill(identity.username)
    await page.locator('#password').fill(identity.password)
    await page.locator('#kc-login').click()
    await page.waitForURL(new RegExp(`localhost:${webPort}`))
    await page.goto(`${web}/onboarding`, { waitUntil: 'networkidle' })
    await page.getByRole('radio', { name: /Alumni/ }).check()
    await page.getByLabel('Display name').fill('Issue Eleven Browser Fixture')
    await page.getByLabel('Graduation year').fill('2020')
    await page.getByRole('button', { name: 'Complete onboarding' }).click()
    await page.waitForURL(/\/(account|dashboard)(\?|$)/)
    return await context.storageState()
  } finally {
    await context.close()
  }
}

async function session(storageState, path) {
  const context = await browser.newContext({ storageState })
  const page = await context.newPage()
  await page.goto(`${web}${path}`, { waitUntil: 'networkidle' })
  return { context, page }
}

function mutation(page, suffix, method) {
  return page.waitForResponse((response) =>
    response.url().endsWith(suffix) && response.request().method() === method)
}

async function runScenarios(storageState) {
  phase = 'private-withdrawal'
  const seed = await session(storageState, '/settings/privacy')
  await seed.page.getByRole('radio', { name: /^Public/ }).check()
  await seed.page.getByRole('checkbox', { name: /I consent/ }).check()
  let response = mutation(seed.page, '/profile/privacy', 'PATCH')
  await seed.page.getByRole('button', { name: 'Save privacy settings' }).click()
  checked('seed_public', (await response).status(), 200)
  await seed.context.close()

  const a = await session(storageState, '/settings/privacy')
  const b = await session(storageState, '/settings/privacy')
  try {
    assert.equal(await a.page.locator('input[name="visibility"]:checked').getAttribute('value'), 'public')
    await b.page.getByRole('checkbox', { name: 'About', exact: true }).check()
    await a.page.getByRole('radio', { name: /^Private/ }).check()
    response = mutation(a.page, '/profile/privacy', 'PATCH')
    await a.page.getByRole('button', { name: 'Save privacy settings' }).click()
    checked('private_withdrawal', (await response).status(), 200)
    const anonymous = await browser.newContext()
    try {
      checked('private_public_read', (await anonymous.request.get(`${web}/api/v1/profiles/issue-eleven-browser-fixture`)).status(), 404)
    } finally { await anonymous.close() }
    response = mutation(b.page, '/profile/privacy', 'PATCH')
    await b.page.getByRole('button', { name: 'Save privacy settings' }).click()
    checked('stale_public_draft', (await response).status(), 412)
    await b.page.getByRole('region', { name: 'Review current server values' }).waitFor()
    assert.equal(await b.page.getByRole('checkbox', { name: 'About', exact: true }).isChecked(), true)
    assert.equal(await b.page.getByRole('button', { name: 'Save privacy settings' }).isDisabled(), true)

    await a.page.reload({ waitUntil: 'networkidle' })
    await a.page.getByRole('radio', { name: /^Public/ }).check()
    assert.equal(await a.page.getByRole('checkbox', { name: /I consent/ }).isChecked(), false)
    await a.page.getByRole('checkbox', { name: /I consent/ }).check()
    response = mutation(a.page, '/profile/privacy', 'PATCH')
    await a.page.getByRole('button', { name: 'Save privacy settings' }).click()
    checked('fresh_explicit_public', (await response).status(), 200)
    await a.page.getByRole('checkbox', { name: /Include my profile in Alumni Directory/ }).check()
    response = mutation(a.page, '/profile/discovery', 'PATCH')
    await a.page.getByRole('button', { name: 'Save discovery settings' }).click()
    checked('seed_directory_consent', (await response).status(), 200)
  } finally { await a.context.close(); await b.context.close() }

  phase = 'directory-withdrawal'
  const c = await session(storageState, '/settings/privacy')
  const d = await session(storageState, '/settings/privacy')
  try {
    c.page.on('dialog', (dialog) => dialog.accept())
    await c.page.getByRole('checkbox', { name: /Include my profile in Alumni Directory/ }).uncheck()
    await d.page.getByRole('checkbox', { name: 'Show institution', exact: true }).check()
    response = mutation(c.page, '/profile/discovery', 'PATCH')
    await c.page.getByRole('button', { name: 'Save discovery settings' }).click()
    checked('directory_withdrawal', (await response).status(), 200)
    response = mutation(d.page, '/profile/discovery', 'PATCH')
    await d.page.getByRole('button', { name: 'Save discovery settings' }).click()
    checked('stale_academic_draft', (await response).status(), 412)
    await d.page.getByRole('region', { name: 'Review current server values' }).waitFor()
    assert.equal(await d.page.getByRole('checkbox', { name: 'Show institution', exact: true }).isChecked(), true)
    assert.equal(await d.page.getByRole('button', { name: 'Save discovery settings' }).isDisabled(), true)
    const fresh = await session(storageState, '/settings/privacy')
    try {
      assert.equal(await fresh.page.getByRole('checkbox', { name: /Include my profile in Alumni Directory/ }).isChecked(), false)
      assert.equal(await fresh.page.getByRole('checkbox', { name: 'Show institution', exact: true }).isChecked(), false)
    } finally { await fresh.context.close() }
  } finally { await c.context.close(); await d.context.close() }

  phase = 'stale-history-delete'
  const e = await session(storageState, '/profile')
  try {
    await e.page.getByRole('button', { name: 'Add experience' }).click()
    await e.page.getByLabel('Company').fill('Issue Eleven Browser Lab')
    await e.page.getByLabel('Job title').fill('Researcher')
    await e.page.getByLabel('Start date').fill('2024-02-01')
    response = mutation(e.page, '/experiences', 'POST')
    await e.page.getByRole('button', { name: 'Save experience' }).click()
    checked('experience_create', (await response).status(), 201)
    await e.page.getByRole('heading', { name: 'Researcher' }).waitFor()
    const f = await session(storageState, '/profile')
    try {
      await f.page.getByRole('button', { name: 'Delete experience 1' }).click()
      await e.page.getByRole('button', { name: 'Edit experience 1' }).click()
      await e.page.getByLabel('Job title').fill('Senior Researcher')
      response = e.page.waitForResponse((item) => item.url().includes('/experiences/') && item.request().method() === 'PATCH')
      await e.page.getByRole('button', { name: 'Save experience' }).click()
      checked('experience_change', (await response).status(), 200)
      response = f.page.waitForResponse((item) => item.url().includes('/experiences/') && item.request().method() === 'DELETE')
      await f.page.getByRole('button', { name: 'Remove entry' }).click()
      checked('stale_experience_delete', (await response).status(), 412)
      await f.page.getByText(/Review the current entry before deleting/).waitFor()
      await f.page.getByText(/Current: Senior Researcher/).waitFor()
      assert.equal(await f.page.getByRole('button', { name: 'Remove entry' }).isDisabled(), true)
      await f.page.getByRole('button', { name: 'Use current entry revision for deletion' }).click()
      response = f.page.waitForResponse((item) => item.url().includes('/experiences/') && item.request().method() === 'DELETE')
      await f.page.getByRole('button', { name: 'Remove entry' }).click()
      checked('reviewed_experience_delete', (await response).status(), 204)
    } finally { await f.context.close() }
  } finally { await e.context.close() }
}

function saveReport(status) {
  const lines = [
    '# Issue #11 disposable two-session browser evidence', '',
    `- Result: **${status}**`,
    `- Application source HEAD: \`${sourceSHA}\``,
    `- UTC: ${new Date().toISOString()}`,
    `- Command: \`ISSUE11_POSTGRES_PORT=${process.env.POSTGRES_PORT} ISSUE11_MINIO_PORT=${process.env.MINIO_PORT} ISSUE11_MINIO_CONSOLE_PORT=${process.env.MINIO_CONSOLE_PORT} CHROME_PATH=<local Chrome executable> bash scripts/issue11-browser-e2e.sh\``,
    '- Environment: isolated Docker Compose project; PostgreSQL 17, MinIO, Keycloak, API and web; disposable verified alumni identity.',
    '- Authentication: two independent Playwright browser contexts with one disposable Keycloak account; an additional anonymous context checks public denial.',
    '- Artifact policy: HTTP status and UI assertions only. No trace, response body, token, cookie, password, account identifier or personal data is stored.',
    '', '## Assertions', '',
    '| Check | HTTP |', '| --- | ---: |',
    ...checks.map((check) => `| ${check.name} | ${check.status} |`),
    '',
    status === 'PASS'
      ? 'UI assertions passed: original PUBLIC/academic drafts remain visible on 412; conflict review blocks blind Save; PRIVATE public read is 404; a fresh owner view keeps Directory disclosure off; stale history DELETE shows the new title and requires explicit review before 204.'
      : `Stopped at phase \`${phase}\`. No browser-pass claim is made.`,
    '',
    'The wrapper removes the disposable Compose project and all of its volumes after this run. The result is local evidence; GitHub CI on a later commit must be reported separately.',
  ]
  mkdirSync(dirname(reportPath), { recursive: true })
  writeFileSync(reportPath, `${lines.join('\n')}\n`, { mode: 0o600 })
}

try {
  const identity = await prepareIdentity()
  browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || undefined, headless: true })
  const storageState = await signInAndOnboard(identity)
  verifyDisposableAlumni(identity.email)
  await runScenarios(storageState)
  saveReport('PASS')
  console.log('Issue #11 disposable two-session browser checks: PASS')
} catch (error) {
  saveReport('FAIL')
  console.error(`Issue #11 browser checks failed at ${phase} (${error.name || 'Error'}). See the sanitized report.`)
  process.exitCode = 1
} finally {
  if (browser) await browser.close()
  if (adminToken && userID) {
    try { await admin(`/users/${userID}`, { method: 'DELETE' }) } catch { /* The disposable volume is removed by the wrapper. */ }
  }
}
