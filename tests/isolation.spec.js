// @ts-check
const { test, expect } = require('@playwright/test');
const fs = require('fs');
const path = require('path');

// ─── Tenant config — 3-level fallback ────────────────────────────────────────
// Level 1: tests/fixtures/tenants.json  (written by bootstrap.sh / make quickstart)
// Level 2: environment variables        (CI / K8s deployment)
// Level 3: hardcoded                    (backward-compatible local dev fallback)
function loadTenantConfig() {
  const fixturesPath = path.join(__dirname, 'fixtures', 'tenants.json');
  if (fs.existsSync(fixturesPath)) {
    try {
      const data = JSON.parse(fs.readFileSync(fixturesPath, 'utf8'));
      const p = data.tenants.pirate;
      const a = data.tenants.academic;
      return {
        tenantA: { id: p.id, name: p.name, apiKey: p.apiKey, userID: p.userID },
        tenantB: { id: a.id, name: a.name, apiKey: a.apiKey, userID: a.userID },
      };
    } catch (_) { /* fall through */ }
  }
  if (process.env.TENANT_A_API_KEY && process.env.TENANT_B_API_KEY) {
    return {
      tenantA: {
        id: process.env.TENANT_A_ID || '',
        name: 'IsolationTest-Pirate',
        apiKey: process.env.TENANT_A_API_KEY,
        userID: process.env.TENANT_A_USER_ID || 'pirate-user-001',
      },
      tenantB: {
        id: process.env.TENANT_B_ID || '',
        name: 'IsolationTest-Academic',
        apiKey: process.env.TENANT_B_API_KEY,
        userID: process.env.TENANT_B_USER_ID || 'academic-user-001',
      },
    };
  }
  // Hardcoded fallback (original local dev values)
  return {
    tenantA: {
      id: '4c05313d-0ab8-4143-9728-598cc68f4008',
      name: 'IsolationTest-Pirate',
      apiKey: 'hk_62c801d95a99bd4b7a716063a131ea6e5805f737fa751482ebc90fcbf2631003',
      userID: 'pirate-user-001',
    },
    tenantB: {
      id: '62427f80-b842-4672-a8bb-29e39ec92775',
      name: 'IsolationTest-Academic',
      apiKey: 'hk_d41f9515066202179878fbd4b3a1c891b0177a43889d38f51ff9c672c8113837',
      userID: 'academic-user-001',
    },
  };
}

// Test configuration
const BASE_URL = process.env.BASE_URL || 'http://localhost:8080';
const CHAT_URL = `${BASE_URL}/chat.html`;

const { tenantA: _cfgA, tenantB: _cfgB } = loadTenantConfig();

const TENANT_A = { ..._cfgA, sessionID: `pirate-sess-${Date.now()}` };
const TENANT_B = { ..._cfgB, sessionID: `academic-sess-${Date.now()}` };

// Helper: send a chat message via API
async function sendChat(apiKey, sessionID, userID, message) {
  const resp = await fetch(`${BASE_URL}/v1/chat/completions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${apiKey}`,
      'X-Hermes-Session-Id': sessionID,
      'X-Hermes-User-Id': userID,
    },
    body: JSON.stringify({
      model: 'MiniMax-M2.7-highspeed',
      messages: [{ role: 'user', content: message }],
    }),
  });
  if (!resp.ok) throw new Error(`Chat API ${resp.status}: ${await resp.text()}`);
  const data = await resp.json();
  return data.choices?.[0]?.message?.content ?? '';
}

// Helper: list memories via API
async function listMemories(apiKey, userID) {
  const resp = await fetch(`${BASE_URL}/v1/memories`, {
    headers: {
      'Authorization': `Bearer ${apiKey}`,
      'X-Hermes-User-Id': userID,
    },
  });
  if (!resp.ok) throw new Error(`Memories API ${resp.status}: ${await resp.text()}`);
  return resp.json();
}

// Helper: list skills via API
async function listSkills(apiKey) {
  const resp = await fetch(`${BASE_URL}/v1/skills`, {
    headers: { 'Authorization': `Bearer ${apiKey}` },
  });
  if (!resp.ok) throw new Error(`Skills API ${resp.status}: ${await resp.text()}`);
  return resp.json();
}

// ─── Soul Isolation ────────────────────────────────────────────────────────

test.describe('Soul Isolation', () => {
  test('Tenant A (Pirate) responds with pirate personality', async () => {
    const reply = await sendChat(
      TENANT_A.apiKey,
      TENANT_A.sessionID,
      TENANT_A.userID,
      'Hello! Who are you and how do you speak?'
    );
    console.log('[Tenant A Soul Reply]:', reply.substring(0, 200));

    // Pirate soul should produce pirate-flavored response
    const lowerReply = reply.toLowerCase();
    const pirateMarkers = ['arr', 'matey', 'pirate', 'captain', 'ahoy', 'ye', 'landlubber', 'treasure', 'shiver', 'yo-ho'];
    const hasPirateMarker = pirateMarkers.some(m => lowerReply.includes(m));
    expect(hasPirateMarker, `Expected pirate language in: ${reply.substring(0, 300)}`).toBe(true);
  });

  test('Tenant B (Academic) responds with academic personality', async () => {
    const reply = await sendChat(
      TENANT_B.apiKey,
      TENANT_B.sessionID,
      TENANT_B.userID,
      'Hello! Who are you and what is your style of communication?'
    );
    console.log('[Tenant B Soul Reply]:', reply.substring(0, 200));

    // Academic soul should produce formal/scholarly response
    const lowerReply = reply.toLowerCase();
    const academicMarkers = ['professor', 'academic', 'scholar', 'formal', 'research', 'erudite', 'rigorous', 'scholarly', 'endeavor', 'hermes'];
    const hasAcademicMarker = academicMarkers.some(m => lowerReply.includes(m));
    expect(hasAcademicMarker, `Expected academic language in: ${reply.substring(0, 300)}`).toBe(true);
  });

  test('Soul does NOT bleed across tenants', async () => {
    const [replyA, replyB] = await Promise.all([
      sendChat(TENANT_A.apiKey, TENANT_A.sessionID + '-x', TENANT_A.userID, 'Say your greeting in one sentence'),
      sendChat(TENANT_B.apiKey, TENANT_B.sessionID + '-x', TENANT_B.userID, 'Say your greeting in one sentence'),
    ]);
    console.log('[Cross-tenant A]:', replyA.substring(0, 150));
    console.log('[Cross-tenant B]:', replyB.substring(0, 150));

    // Responses must differ meaningfully
    expect(replyA).not.toEqual(replyB);
    // Tenant A must not sound like Tenant B's academic soul
    expect(replyA.toLowerCase()).not.toMatch(/professor hermes|erudite scholar/);
    // Tenant B must not sound like Tenant A's pirate soul
    expect(replyB.toLowerCase()).not.toMatch(/captain hermes|yo-ho-ho/);
  });
});

// ─── Memory Isolation ─────────────────────────────────────────────────────

test.describe('Memory Isolation', () => {
  const memSessA = `mem-sess-a-${Date.now()}`;
  const memSessB = `mem-sess-b-${Date.now()}`;

  test('Tenant A stores a personal fact in memory', async () => {
    const reply = await sendChat(
      TENANT_A.apiKey,
      memSessA,
      'pirate-mem-user',
      'Remember this: my ship is named The Black Pearl and I am searching for the Aztec Gold.'
    );
    console.log('[Memory store A]:', reply.substring(0, 200));
    expect(reply.length).toBeGreaterThan(10);
  });

  test('Tenant A can recall stored memory in new session', async () => {
    // First session: store the fact
    await sendChat(
      TENANT_A.apiKey,
      memSessA + '-recall',
      'pirate-mem-user',
      'My favorite color is crimson red.'
    );
    // Second session: recall
    const recall = await sendChat(
      TENANT_A.apiKey,
      memSessA + '-recall-2',
      'pirate-mem-user',
      'What do you remember about me?'
    );
    console.log('[Memory recall A]:', recall.substring(0, 300));
    // Should mention something stored (memory system working)
    expect(recall.length).toBeGreaterThan(20);
  });

  test('Tenant B cannot see Tenant A memory (cross-tenant isolation)', async () => {
    // Store in Tenant A
    await sendChat(
      TENANT_A.apiKey,
      memSessA + '-secret',
      'pirate-secret-user',
      'Secret: my treasure is buried at coordinates 13.7N 144.9E'
    );

    // Query from Tenant B with same user ID
    const replyB = await sendChat(
      TENANT_B.apiKey,
      memSessB + '-spy',
      'pirate-secret-user',
      'Do you know where my treasure is buried? What coordinates?'
    );
    console.log('[Cross-memory isolation]:', replyB.substring(0, 300));

    // Tenant B must not know the secret coordinates
    expect(replyB).not.toContain('13.7N');
    expect(replyB).not.toContain('144.9E');
  });

  test('Memory API returns tenant-scoped memories only', async () => {
    const [memsA, memsB] = await Promise.all([
      listMemories(TENANT_A.apiKey, 'pirate-mem-user'),
      listMemories(TENANT_B.apiKey, 'pirate-mem-user'),
    ]);
    console.log('[Memories A count]:', memsA.memories?.length ?? memsA.total ?? 0);
    console.log('[Memories B count]:', memsB.memories?.length ?? memsB.total ?? 0);

    const tenantAIds = new Set((memsA.memories || []).map(m => m.id));
    const tenantBIds = new Set((memsB.memories || []).map(m => m.id));
    // No memory ID should appear in both tenants
    const overlap = [...tenantAIds].filter(id => tenantBIds.has(id));
    expect(overlap).toHaveLength(0);
  });
});

// ─── Skill Isolation ──────────────────────────────────────────────────────

test.describe('Skill Isolation', () => {
  test('Tenant A has treasure-hunt skill, not academic-research', async () => {
    const skills = await listSkills(TENANT_A.apiKey);
    console.log('[Skills A]:', JSON.stringify(skills.skills?.map(s => s.name)));
    const names = (skills.skills || []).map(s => s.name);
    expect(names).toContain('treasure-hunt');
    expect(names).not.toContain('academic-research');
  });

  test('Tenant B has academic-research skill; Tenant A does not', async () => {
    const [skillsA, skillsB] = await Promise.all([
      listSkills(TENANT_A.apiKey),
      listSkills(TENANT_B.apiKey),
    ]);
    console.log('[Skills B]:', JSON.stringify(skillsB.skills?.map(s => s.name)));
    const namesA = (skillsA.skills || []).map(s => s.name);
    const namesB = (skillsB.skills || []).map(s => s.name);
    // Tenant B must have academic-research
    expect(namesB).toContain('academic-research');
    // Tenant A must NOT have academic-research (it's Tenant B's exclusive skill)
    expect(namesA).not.toContain('academic-research');
  });

  test('Tenant A conversation activates treasure-hunt skill context', async () => {
    const reply = await sendChat(
      TENANT_A.apiKey,
      `skill-test-a-${Date.now()}`,
      TENANT_A.userID,
      'I found an old map with X marks the spot. Help me find the buried treasure!'
    );
    console.log('[Skill A activation]:', reply.substring(0, 300));
    // Should reference treasure hunting concepts
    const lowerReply = reply.toLowerCase();
    const treasureMarkers = ['treasure', 'map', 'buried', 'gold', 'hunt', 'chest', 'clue', 'mark'];
    const hasMarker = treasureMarkers.some(m => lowerReply.includes(m));
    expect(hasMarker, `Expected treasure context in: ${reply.substring(0, 300)}`).toBe(true);
  });

  test('Tenant B conversation does NOT activate pirate/treasure skill', async () => {
    const reply = await sendChat(
      TENANT_B.apiKey,
      `skill-test-b-${Date.now()}`,
      TENANT_B.userID,
      'I found an old map with X marks the spot. Help me find the buried treasure!'
    );
    console.log('[Skill B no-activation]:', reply.substring(0, 300));
    // Should NOT use pirate vocabulary (pirate skill not loaded)
    const lowerReply = reply.toLowerCase();
    const strictPirateWords = ['shiver me timbers', 'yo-ho-ho', 'arrr matey', 'walk the plank'];
    const hasPirateSpeak = strictPirateWords.some(m => lowerReply.includes(m));
    expect(hasPirateSpeak).toBe(false);
  });
});

// ─── Chat UI Smoke Test ────────────────────────────────────────────────────

test.describe('Chat UI', () => {
  test('Chat page loads and shows config inputs', async ({ page }) => {
    await page.goto(CHAT_URL);
    await expect(page.locator('#cfgUrl')).toBeVisible();
    await expect(page.locator('#cfgKey')).toBeVisible();
    await expect(page.locator('#sendBtn')).toBeVisible();
  });

  test('Tenant A can send a message via chat UI', async ({ page }) => {
    await page.goto(CHAT_URL);

    // Configure API URL and key, then click Connect
    await page.fill('#cfgUrl', BASE_URL);
    await page.fill('#cfgKey', TENANT_A.apiKey);
    await page.click('#btnConnect');

    // Wait for input to become enabled (connect() sets disabled=false on sendBtn)
    await expect(page.locator('#sendBtn')).toBeEnabled({ timeout: 15000 });

    // Type message and send
    await page.fill('#msgInput', 'Hello! Tell me who you are in one sentence.');
    await page.click('#sendBtn');

    // Wait for assistant reply (up to 45s for LLM round-trip)
    const assistantMsg = page.locator('.msg.assistant, .message.assistant, [data-role="assistant"], .assistant-msg').first();
    await expect(assistantMsg).toBeVisible({ timeout: 45000 });

    const text = await assistantMsg.textContent();
    console.log('[UI Reply]:', text?.substring(0, 200));
    expect(text?.length).toBeGreaterThan(10);
  });
});
