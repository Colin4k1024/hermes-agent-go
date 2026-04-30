const { defineConfig } = require('@playwright/test');

module.exports = defineConfig({
  testDir: './tests',
  timeout: 60000,
  retries: 0,
  reporter: [['list'], ['html', { outputFolder: 'playwright-report', open: 'never' }]],
  use: {
    baseURL: 'http://localhost:8080',
    headless: true,
  },
  // API tests don't need a browser project
  projects: [
    {
      name: 'api-isolation',
      testMatch: /isolation\.spec\.js/,
      use: { browserName: 'chromium' },
    },
  ],
});
