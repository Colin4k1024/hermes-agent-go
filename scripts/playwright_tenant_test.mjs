import { chromium } from 'playwright';
import { readFileSync } from 'fs';

const BASE = 'http://localhost:8080';
const KEY_A = readFileSync('/tmp/hermes_key_a', 'utf-8').trim();
const KEY_B = readFileSync('/tmp/hermes_key_b', 'utf-8').trim();
const TID_A = readFileSync('/tmp/hermes_tenant_a_id', 'utf-8').trim();
const TID_B = readFileSync('/tmp/hermes_tenant_b_id', 'utf-8').trim();

let pass = 0, fail = 0;

function assert(desc, condition) {
  if (condition) { console.log(`  \x1b[32mPASS\x1b[0m: ${desc}`); pass++; }
  else { console.log(`  \x1b[31mFAIL\x1b[0m: ${desc}`); fail++; }
}

async function apiCall(key, path, opts = {}) {
  const resp = await fetch(BASE + path, {
    ...opts,
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + key,
      ...(opts.headers || {}),
    },
  });
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  return resp.json();
}

(async () => {
  const browser = await chromium.launch({ headless: true });

  // ============================================================
  console.log('\x1b[34m=== Phase 1: Chat Page Login — Tenant A ===\x1b[0m');
  // ============================================================
  const ctxA = await browser.newContext();
  const pageA = await ctxA.newPage();
  await pageA.goto(BASE + '/chat.html');

  assert('chat page loads', await pageA.title() === 'Hermes Chat');
  assert('login form visible', await pageA.isVisible('#input-apikey'));

  await pageA.fill('#input-apikey', KEY_A);
  await pageA.fill('#input-baseurl', BASE);
  await pageA.fill('#input-userid', 'alice');
  await pageA.click('#btn-login');
  await pageA.waitForSelector('#chat-app:not(.hidden)', { timeout: 10000 });

  const tenantA = await pageA.textContent('#info-tenant');
  assert('Tenant A ID displayed', tenantA === TID_A);
  assert('Tenant A roles shown', (await pageA.textContent('#info-roles')).includes('user'));

  await pageA.waitForFunction(() => parseInt(document.getElementById('skills-count').textContent) > 0, { timeout: 10000 });
  const skillsCountA = await pageA.textContent('#skills-count');
  assert('Tenant A has skills loaded', parseInt(skillsCountA) > 0);
  console.log(`  Skills count A: ${skillsCountA}`);

  // ============================================================
  console.log('\x1b[34m=== Phase 2: Chat Page Login — Tenant B ===\x1b[0m');
  // ============================================================
  const ctxB = await browser.newContext();
  const pageB = await ctxB.newPage();
  await pageB.goto(BASE + '/chat.html');

  await pageB.fill('#input-apikey', KEY_B);
  await pageB.fill('#input-baseurl', BASE);
  await pageB.fill('#input-userid', 'bob');
  await pageB.click('#btn-login');
  await pageB.waitForSelector('#chat-app:not(.hidden)', { timeout: 10000 });

  const tenantB = await pageB.textContent('#info-tenant');
  assert('Tenant B ID displayed', tenantB === TID_B);

  await pageB.waitForFunction(() => parseInt(document.getElementById('skills-count').textContent) > 0, { timeout: 10000 });
  const skillsCountB = await pageB.textContent('#skills-count');
  assert('Tenant B has skills loaded', parseInt(skillsCountB) > 0);
  console.log(`  Skills count B: ${skillsCountB}`);

  // ============================================================
  console.log('\x1b[34m=== Phase 3: Tenant A Chat — Soul Injection ===\x1b[0m');
  // ============================================================
  await pageA.fill('#chat-input', 'What is your name and who are you? Answer in one sentence.');
  await pageA.click('#btn-send');

  await pageA.waitForSelector('.message.assistant', { timeout: 60000 });
  const replyA1 = await pageA.textContent('.message.assistant');
  assert('Tenant A gets chat response', replyA1.length > 10);
  console.log(`  Response A (first 120): ${replyA1.slice(0, 120)}`);

  // ============================================================
  console.log('\x1b[34m=== Phase 4: Tenant B Chat — Soul Injection ===\x1b[0m');
  // ============================================================
  await pageB.fill('#chat-input', 'What is your name and who are you? Answer in one sentence.');
  await pageB.click('#btn-send');

  await pageB.waitForSelector('.message.assistant', { timeout: 60000 });
  const replyB1 = await pageB.textContent('.message.assistant');
  assert('Tenant B gets chat response', replyB1.length > 10);
  console.log(`  Response B (first 120): ${replyB1.slice(0, 120)}`);

  // ============================================================
  console.log('\x1b[34m=== Phase 5: Memory Persistence — Tenant A ===\x1b[0m');
  // ============================================================
  await pageA.fill('#chat-input', 'Remember: my favorite fruit is mango. Confirm briefly.');
  await pageA.click('#btn-send');
  await pageA.waitForFunction(() => document.querySelectorAll('.message.assistant').length >= 2, { timeout: 60000 });

  const replies_A = await pageA.$$('.message.assistant');
  assert('Tenant A memory store response', replies_A.length >= 2);

  await pageA.fill('#chat-input', 'What is my favorite fruit? Answer in one word.');
  await pageA.click('#btn-send');
  await pageA.waitForFunction(() => document.querySelectorAll('.message.assistant').length >= 3, { timeout: 60000 });

  const memReplyA = await (await pageA.$$('.message.assistant')).at(-1).textContent();
  const memOkA = memReplyA.toLowerCase().includes('mango');
  assert('Tenant A remembers mango', memOkA);
  console.log(`  Memory recall A: ${memReplyA.slice(0, 100)}`);

  // ============================================================
  console.log('\x1b[34m=== Phase 6: Memory Persistence — Tenant B ===\x1b[0m');
  // ============================================================
  await pageB.fill('#chat-input', 'Remember: my favorite city is Tokyo. Confirm briefly.');
  await pageB.click('#btn-send');
  await pageB.waitForFunction(() => document.querySelectorAll('.message.assistant').length >= 2, { timeout: 60000 });

  await pageB.fill('#chat-input', 'What is my favorite city? Answer in one word.');
  await pageB.click('#btn-send');
  await pageB.waitForFunction(() => document.querySelectorAll('.message.assistant').length >= 3, { timeout: 60000 });

  const memReplyB = await (await pageB.$$('.message.assistant')).at(-1).textContent();
  const memOkB = memReplyB.toLowerCase().includes('tokyo');
  assert('Tenant B remembers Tokyo', memOkB);
  console.log(`  Memory recall B: ${memReplyB.slice(0, 100)}`);

  // ============================================================
  console.log('\x1b[34m=== Phase 7: Cross-Tenant Session Isolation ===\x1b[0m');
  // ============================================================
  const sessionsA = await apiCall(KEY_A, '/v1/mock-sessions');
  const sessionsB = await apiCall(KEY_B, '/v1/mock-sessions');

  assert('Tenant A sessions scoped', sessionsA.tenant_id === TID_A);
  assert('Tenant B sessions scoped', sessionsB.tenant_id === TID_B);

  const aSessionIds = (sessionsA.sessions || []).map(s => s.session_id);
  const bSessionIds = (sessionsB.sessions || []).map(s => s.session_id);
  const overlap = aSessionIds.filter(id => bSessionIds.includes(id));
  assert('No session overlap between tenants', overlap.length === 0);

  // ============================================================
  console.log('\x1b[34m=== Phase 8: Cross-Tenant Skill Isolation ===\x1b[0m');
  // ============================================================
  await fetch(BASE + '/v1/skills/alpha-exclusive', {
    method: 'PUT',
    headers: { 'Authorization': 'Bearer ' + KEY_A, 'Content-Type': 'text/plain' },
    body: '---\nname: "alpha-exclusive"\ndescription: "Only for Alpha"\nversion: "1.0.0"\n---\nAlpha only skill.',
  });

  const skillsAfterA = await apiCall(KEY_A, '/v1/skills');
  const skillsAfterB = await apiCall(KEY_B, '/v1/skills');

  const aHasAlpha = skillsAfterA.skills.some(s => s.name === 'alpha-exclusive');
  const bHasAlpha = skillsAfterB.skills.some(s => s.name === 'alpha-exclusive');
  assert('Tenant A sees alpha-exclusive skill', aHasAlpha);
  assert('Tenant B cannot see alpha-exclusive skill', !bHasAlpha);

  await fetch(BASE + '/v1/skills/alpha-exclusive', {
    method: 'DELETE',
    headers: { 'Authorization': 'Bearer ' + KEY_A },
  });

  // ============================================================
  console.log('\x1b[34m=== Phase 9: New Session Isolation ===\x1b[0m');
  // ============================================================
  await pageB.click('text=New Session');
  await pageB.fill('#chat-input', 'Do you know my favorite fruit? If yes name it, if not say "I don\'t know".');
  await pageB.click('#btn-send');
  await pageB.waitForFunction(() => {
    const msgs = document.querySelectorAll('.message.assistant');
    return msgs.length >= 1;
  }, { timeout: 60000 });

  const newSessionMsgs = await pageB.$$('.message.assistant');
  const crossCheck = await newSessionMsgs.at(-1).textContent();
  const leakedMango = crossCheck.toLowerCase().includes('mango');
  assert('Tenant B does NOT know Tenant A\'s mango', !leakedMango);
  console.log(`  Cross-tenant check: ${crossCheck.slice(0, 120)}`);

  // ============================================================
  console.log('\x1b[34m=== Phase 10: Cross-Session Memory — Tenant A ===\x1b[0m');
  // ============================================================
  await pageA.click('text=New Session');
  await pageA.fill('#chat-input', 'What is my favorite fruit? Answer in one word if you know it.');
  await pageA.click('#btn-send');
  await pageA.waitForFunction(() => {
    const msgs = document.querySelectorAll('.message.assistant');
    return msgs.length >= 1;
  }, { timeout: 60000 });

  const crossSessionMsgs = await pageA.$$('.message.assistant');
  const crossSessionReply = await crossSessionMsgs.at(-1).textContent();
  const remembersMango = crossSessionReply.toLowerCase().includes('mango');
  assert('Tenant A cross-session memory: remembers mango', remembersMango);
  console.log(`  Cross-session recall: ${crossSessionReply.slice(0, 120)}`);

  // ============================================================
  console.log('\x1b[34m=== Phase 11: Memory API Verification ===\x1b[0m');
  // ============================================================
  const memA = await apiCall(KEY_A, '/v1/memories', {
    headers: { 'X-Hermes-User-Id': 'alice' }
  });
  assert('Tenant A has memories', (memA.memories || []).length > 0);
  const hasFruitMemory = (memA.memories || []).some(
    m => m.content && m.content.toLowerCase().includes('mango')
  );
  assert('Tenant A memory contains mango', hasFruitMemory);
  console.log(`  Tenant A memories: ${(memA.memories || []).length} entries`);

  const memB = await apiCall(KEY_B, '/v1/memories', {
    headers: { 'X-Hermes-User-Id': 'bob' }
  });
  const bHasMango = (memB.memories || []).some(
    m => m.content && m.content.toLowerCase().includes('mango')
  );
  assert('Tenant B memory does NOT contain mango', !bHasMango);

  // ============================================================
  console.log('\x1b[34m=== Phase 12: Cross-User Memory Isolation ===\x1b[0m');
  // ============================================================
  const memBob2 = await apiCall(KEY_A, '/v1/memories', {
    headers: { 'X-Hermes-User-Id': 'bob2' }
  });
  const bob2HasMango = (memBob2.memories || []).some(
    m => m.content && m.content.toLowerCase().includes('mango')
  );
  assert('Same-tenant different user (bob2) does NOT see alice mango', !bob2HasMango);

  const sessionsAPI = await apiCall(KEY_A, '/v1/sessions', {
    headers: { 'X-Hermes-User-Id': 'alice' }
  });
  assert('Session history API returns sessions', (sessionsAPI.sessions || []).length > 0);
  console.log(`  Alice sessions: ${(sessionsAPI.sessions || []).length}`);

  // ============================================================
  // Summary
  // ============================================================
  await ctxA.close();
  await ctxB.close();
  await browser.close();

  console.log('');
  console.log('\x1b[34m============================================\x1b[0m');
  const total = pass + fail;
  if (fail === 0) {
    console.log(`\x1b[32mALL ${total} TESTS PASSED (${pass} passed, ${fail} failed)\x1b[0m`);
  } else {
    console.log(`\x1b[31m${fail} of ${total} TESTS FAILED (${pass} passed, ${fail} failed)\x1b[0m`);
  }
  console.log('\x1b[34m============================================\x1b[0m');

  process.exit(fail);
})();
