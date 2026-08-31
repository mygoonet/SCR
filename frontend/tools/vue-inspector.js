/**
 * Vue Inspector — подключается к запущенному Vue dev server через Playwright + CDP
 * и собирает информацию о приложении: компоненты, стейт, роутер, ошибки, сеть.
 *
 * Usage:
 *   node vue-inspector.js <command> [options]
 *
 * Commands:
 *   components        — дерево компонентов Vue
 *   state             — Pinia stores + reactive state
 *   router            — текущий роутер, маршруты
 *   errors            — console errors/warnings
 *   network           — сетевые запросы (последние N)
 *   perf              — performance metrics
 *   screenshot        — скриншот страницы
 *   info              — общая информация о приложении
 *   all               — всё сразу (components + state + router + errors)
 */

import { chromium } from 'playwright';
import { resolve } from 'path';

const BASE_URL = process.env.VUE_URL || 'http://localhost:5173';
const TIMEOUT = parseInt(process.env.VUE_TIMEOUT || '15000', 10);

// ─── Helpers ────────────────────────────────────────────────────────────────

async function getPage() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();
  return { browser, context, page };
}

async function collectConsoleErrors(page) {
  const errors = [];
  page.on('console', msg => {
    if (msg.type() === 'error' || msg.type() === 'warning') {
      errors.push({ type: msg.type(), text: msg.text(), location: msg.location() });
    }
  });
  return errors;
}

// ─── Vue Introspection Scripts ──────────────────────────────────────────────

const scripts = {
  // Получить дерево компонентов Vue
  components: `
    (() => {
      const results = [];
      // Vue 3: try to find app instance
      const apps = document.querySelectorAll('[data-v-app]');
      if (!apps.length) return JSON.stringify({ error: 'No Vue app found on page' });

      // Try to get root component via Vue DevTools internals
      let app = null;
      for (const key in window) {
        if (key.startsWith('__VUE__')) {
          app = window[key];
          break;
        }
      }

      // Alternative: use the globalThis.__VUE_DEVTOOLS_COMPONENT_INSPECTOR_ENABLED__
      // or access via document._vueApp if available
      
      // Most reliable: check if Vue is available
      if (typeof window.__VUE__ !== 'undefined') {
        return JSON.stringify({ vue: 'detected', version: 'unknown' });
      }

      // Fallback: scan DOM and report structure
      const root = document.querySelector('[data-v-app]') || document.body;
      const tree = { tag: root.tagName.toLowerCase(), children: [] };
      
      function walk(node, depth = 0) {
        if (depth > 4) return;
        if (node.children) {
          node.children = Array.from(node.children).map(child => ({
            tag: child.tagName?.toLowerCase() || '#text',
            classes: child.className?.split?.(' ').filter(Boolean) || [],
            id: child.id || null,
            children: []
          })).map(c => walk(c, depth + 1));
        }
        return node;
      }
      
      return JSON.stringify({ domTree: walk(tree) });
    })()
  `,

  // Pinia stores
  state: `
    (() => {
      const result = { stores: {}, errors: [] };
      
      // Method 1: Check for Pinia instances
      try {
        const pinia = window.__VUEX_DEFAULT_STORE__ || 
                      Object.values(window).find(v => v?._options?._isPinia);
        if (pinia) {
          const storeIds = Object.keys(pinia._s || {});
          for (const id of storeIds) {
            const store = pinia._s[id];
            result.stores[id] = {
              state: JSON.parse(JSON.stringify(store.state)),
              getters: Object.keys(store.getters || {})
            };
          }
        }
      } catch(e) { result.errors.push(e.message); }

      // Method 2: Check for Vue devtools state
      try {
        if (window.__VUE_DEVTOOLS_GLOBAL_HOOK__) {
          result.devtools = 'available';
        }
      } catch(e) {}

      // Method 3: Generic reactive state detection
      try {
        const reactiveKeys = [];
        for (const key of Object.keys(window)) {
          if (key.startsWith('__') && typeof window[key] === 'object') {
            reactiveKeys.push(key);
          }
        }
        result.reactiveKeys = reactiveKeys.slice(0, 20);
      } catch(e) {}

      return JSON.stringify(result);
    })()
  `,

  // Router info
  router: `
    (() => {
      const result = { routes: [], currentRoute: null, historyType: null };
      
      try {
        // Try to find router instance
        let router = null;
        for (const key in window) {
          const val = window[key];
          if (val && typeof val === 'object' && val.matchers !== undefined) {
            router = val;
            break;
          }
        }
        
        if (router) {
          result.routes = router.getRoutes?.().map(r => ({
            path: r.path,
            name: r.name,
            children: r.children?.length || 0
          })) || [];
          
          const current = router.currentRoute?.value;
          if (current) {
            result.currentRoute = {
              path: current.path,
              name: current.name,
              params: current.params,
              query: current.query
            };
          }
        }
      } catch(e) { result.error = e.message; }

      return JSON.stringify(result);
    })()
  `,

  // Console errors
  errors: `
    (() => {
      const errors = [];
      // Get captured errors from performance entries
      const entries = performance.getEntriesByType('resource');
      const failedResources = entries.filter(e => e.transferSize === 0 || e.decodedBodySize === 0);
      
      return JSON.stringify({
        type: 'console-errors',
        note: 'Run with page.on(console) handler for live errors',
        failedResources: failedResources.map(e => ({ url: e.name, size: e.transferSize }))
      });
    })()
  `,

  // Network requests
  network: `
    (() => {
      const entries = performance.getEntriesByType('resource');
      const apiCalls = entries.filter(e => 
        e.initiatorType === 'xmlhttprequest' || 
        e.initiatorType === 'fetch' ||
        e.name.includes('/api/')
      );
      
      return JSON.stringify({
        totalRequests: entries.length,
        apiCalls: apiCalls.length,
        lastApiCalls: apiCalls.slice(-10).map(e => ({
          url: e.name,
          duration: Math.round(e.duration),
          size: e.transferSize
        }))
      });
    })()
  `,

  // Performance
  perf: `
    (() => {
      const perf = performance.timing || {};
      const nav = performance.getEntriesByType('navigation')[0] || {};
      
      return JSON.stringify({
        domContentLoaded: nav.domContentLoadedEventEnd || 0,
        loadComplete: nav.loadEventEnd || 0,
        domInteractive: nav.domInteractive || 0,
        redirectCount: nav.redirectCount || 0,
        transferSize: nav.transferSize || 0,
        domElements: document.querySelectorAll('*').length,
        resources: performance.getEntriesByType('resource').length
      });
    })()
  `,

  // Vue-specific component tree via CDP
  vueComponentsCDP: `
    (() => {
      // This script is designed to run in the page context
      // It tries to access Vue internals through various methods
      
      const result = { components: [], methods: [] };
      
      // Check if Vue DevTools extension is connected
      if (window.__VUE_DEVTOOLS_COMPONENT_INSPECTOR_ENABLED__) {
        result.devtoolsInspector = true;
      }
      
      // Try to find Vue app instances
      const vueApps = [];
      for (const key of Object.keys(window)) {
        if (key.includes('Vue') || key.includes('vue')) {
          try {
            const val = window[key];
            if (val && typeof val === 'object') {
              vueApps.push({ key, type: Object.prototype.toString.call(val) });
            }
          } catch(e) {}
        }
      }
      result.foundKeys = vueApps;
      
      // Scan for Vue component registrations
      const components = [];
      function scanForComponents(obj, path = '') {
        if (!obj || typeof obj !== 'object') return;
        if (obj.__isComponent || obj._component) {
          components.push({ path, name: obj.name || 'anonymous' });
        }
        for (const key of Object.keys(obj)) {
          scanForComponents(obj[key], path ? path + '.' + key : key);
        }
      }
      
      // Limit scan depth
      try {
        scanForComponents(document, 'document');
        result.components = components.slice(0, 50);
      } catch(e) {}
      
      return JSON.stringify(result);
    })()
  `,

  // Full diagnostic
  all: `
    (() => {
      return JSON.stringify({
        url: location.href,
        title: document.title,
        domElements: document.querySelectorAll('*').length,
        hasVueApp: !!document.querySelector('[data-v-app]'),
        scripts: Array.from(document.scripts).map(s => s.src?.split('/').pop()).filter(Boolean),
        styles: document.styleSheets.length,
        cookies: document.cookie.split(';').length
      });
    })()
  `
};

// ─── Commands ───────────────────────────────────────────────────────────────

async function cmdComponents(page) {
  console.log('📦 Inspecting Vue components...\n');
  
  // Method 1: Run introspection script
  const result = await page.evaluate(scripts.components);
  console.log('DOM/Component info:', result);
  
  // Method 2: Use CDP to get DOM tree with Vue-specific attributes
  const client = await page.context().newCDPSession(page);
  const domResult = await client.send('DOM.getDocument', { depth: -1 });
  
  // Parse and look for Vue-specific markers
  function extractVueNodes(node) {
    const vueNodes = [];
    if (node.children) {
      for (const child of node.children) {
        const attrs = (node.attributes || []).join(' ');
        if (attrs.includes('__vnode') || attrs.includes('__vue__') || 
            attrs.includes('data-v-') || child.nodeType === 1) {
          const tag = child.nodeName?.toLowerCase() || '#text';
          const id = child.attributes?.[child.attributes.indexOf('id') + 1] || null;
          const classes = child.attributes?.filter((_, i) => i % 2 === 0 && i > 0) || [];
          vueNodes.push({ tag, id, classes: classes.join(' ') });
        }
        vueNodes.push(...extractVueNodes(child));
      }
    }
    return vueNodes;
  }
  
  const vueNodes = extractVueNodes(domResult.root);
  if (vueNodes.length > 0) {
    console.log(`\n🔍 Found ${vueNodes.length} DOM nodes`);
    console.log('Sample nodes:', vueNodes.slice(0, 10));
  }
  
  // Method 3: Check for Vue DevTools connection
  const devtoolsStatus = await page.evaluate(() => {
    return {
      __VUE_DEVTOOLS__: typeof window.__VUE_DEVTOOLS__,
      __VUE_DEVTOOLS_GLOBAL_HOOK__: typeof window.__VUE_DEVTOOLS_GLOBAL_HOOK__,
      __VUE__: typeof window.__VUE__
    };
  });
  console.log('\n🔌 DevTools status:', devtoolsStatus);
}

async function cmdState(page) {
  console.log('🗂️  Inspecting Pinia/state...\n');
  const result = JSON.parse(await page.evaluate(scripts.state));
  console.log(JSON.stringify(result, null, 2));
}

async function cmdRouter(page) {
  console.log('🧭 Inspecting router...\n');
  const result = JSON.parse(await page.evaluate(scripts.router));
  console.log(JSON.stringify(result, null, 2));
}

async function cmdErrors(page) {
  console.log('⚠️  Checking for errors...\n');
  
  const consoleErrors = [];
  page.on('console', msg => {
    if (msg.type() === 'error' || msg.type() === 'warning') {
      consoleErrors.push({
        type: msg.type(),
        text: msg.text(),
        location: `${msg.location().url}:${msg.location().lineNumber}`
      });
    }
  });
  
  // Also check via CDP for runtime errors
  const client = await page.context().newCDPSession(page);
  const runtimeErrors = [];
  client.on('Runtime.exceptionThrown', exception => {
    runtimeErrors.push({
      description: exception.exceptionDetails.exception.description || 
                   exception.exceptionDetails.text,
      timestamp: exception.timestamp
    });
  });
  
  await page.waitForTimeout(2000);
  
  console.log(`Console errors/warnings: ${consoleErrors.length}`);
  consoleErrors.forEach(e => console.log(`  [${e.type}] ${e.text} (${e.location})`));
  
  console.log(`\nRuntime exceptions: ${runtimeErrors.length}`);
  runtimeErrors.forEach(e => console.log(`  ❌ ${e.description}`));
}

async function cmdNetwork(page) {
  console.log('🌐 Inspecting network...\n');
  const result = JSON.parse(await page.evaluate(scripts.network));
  console.log(JSON.stringify(result, null, 2));
}

async function cmdPerf(page) {
  console.log('⚡ Performance metrics...\n');
  const result = JSON.parse(await page.evaluate(scripts.perf));
  console.log(JSON.stringify(result, null, 2));
}

async function cmdScreenshot(page) {
  console.log('📸 Taking screenshot...\n');
  const path = resolve(process.cwd(), 'screenshot.png');
  await page.screenshot({ path, fullPage: true });
  console.log(`✅ Screenshot saved to: ${path}`);
}

async function cmdInfo(page) {
  console.log('ℹ️  App info...\n');
  const result = JSON.parse(await page.evaluate(scripts.all));
  console.log(JSON.stringify(result, null, 2));
}

async function cmdAll(page) {
  console.log('🔍 Full Vue diagnostic\n');
  console.log('═'.repeat(60));
  await cmdInfo(page);
  console.log('═'.repeat(60));
  await cmdComponents(page);
  console.log('═'.repeat(60));
  await cmdState(page);
  console.log('═'.repeat(60));
  await cmdRouter(page);
  console.log('═'.repeat(60));
  await cmdErrors(page);
  console.log('═'.repeat(60));
  await cmdNetwork(page);
  console.log('═'.repeat(60));
  await cmdPerf(page);
  console.log('═'.repeat(60));
}

// ─── Main ───────────────────────────────────────────────────────────────────

async function main() {
  const command = process.argv[2] || 'all';
  const url = process.argv[3] || BASE_URL;
  
  console.log(`🚀 Vue Inspector — connecting to ${url}\n`);
  
  let { browser, context, page } = await getPage();
  
  try {
    // Navigate and wait for Vue to boot
    console.log(`⏳ Navigating to ${url}...`);
    await page.goto(url, { waitUntil: 'networkidle', timeout: TIMEOUT });
    
    // Wait for Vue app to be ready
    try {
      await page.waitForSelector('[data-v-app]', { timeout: 10000 });
      console.log('✅ Vue app detected\n');
    } catch(e) {
      console.log('⚠️  No [data-v-app] selector found, continuing anyway\n');
    }
    
    // Execute command
    switch(command) {
      case 'components': await cmdComponents(page); break;
      case 'state': await cmdState(page); break;
      case 'router': await cmdRouter(page); break;
      case 'errors': await cmdErrors(page); break;
      case 'network': await cmdNetwork(page); break;
      case 'perf': await cmdPerf(page); break;
      case 'screenshot': await cmdScreenshot(page); break;
      case 'info': await cmdInfo(page); break;
      case 'all': await cmdAll(page); break;
      default:
        console.error(`❌ Unknown command: ${command}`);
        console.log('Available: components, state, router, errors, network, perf, screenshot, info, all');
        process.exit(1);
    }
  } catch(err) {
    console.error(`\n❌ Error: ${err.message}`);
    process.exit(1);
  } finally {
    await browser.close();
  }
}

main().catch(err => {
  console.error('Fatal:', err);
  process.exit(1);
});
