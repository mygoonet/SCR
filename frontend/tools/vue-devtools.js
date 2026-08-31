/**
 * Vue DevTools Inspector — подключается к запущенному Vue dev server
 * через Playwright + Chrome DevTools Protocol (CDP).
 *
 * Usage:
 *   node vue-devtools.js <command> [options]
 *
 * Commands:
 *   components          — дерево компонентов Vue
 *   state               — Pinia stores + reactive state
 *   router              — текущий роутер и маршруты
 *   errors              — console errors/warnings + runtime exceptions
 *   network             — сетевые запросы (XHR/fetch)
 *   perf                — performance metrics
 *   screenshot          — скриншот страницы
 *   info                — общая информация о приложении
 *   audit               — полный аудит (components + state + router + errors + network + perf)
 *   watch               — continuous monitoring mode
 */

import { chromium } from 'playwright';
import { resolve } from 'path';

const DEFAULT_URL = 'http://localhost:5173';
const DEFAULT_TIMEOUT = 15000;

// ─── Helpers ────────────────────────────────────────────────────────────────

async function createBrowser() {
  return chromium.launch({ headless: true });
}

async function navigate(page, url) {
  await page.goto(url, { waitUntil: 'networkidle', timeout: DEFAULT_TIMEOUT });
  
  // Wait for Vue app to boot
  try {
    await page.waitForSelector('[data-v-app]', { timeout: 8000 });
    return true;
  } catch {
    console.log('⚠️  [data-v-app] not found, continuing anyway');
    return false;
  }
}

// ─── Vue Introspection Scripts ──────────────────────────────────────────────

const vueScripts = {
  // Получить информацию о Vue приложении
  getAppInfo: `
    (() => {
      const info = {
        url: location.href,
        title: document.title,
        hasVueApp: !!document.querySelector('[data-v-app]'),
        domElements: document.querySelectorAll('*').length,
        scripts: Array.from(document.scripts).map(s => s.src?.split('/').pop()).filter(Boolean),
        stylesheets: document.styleSheets.length
      };
      
      // Detect Vue version
      if (window.__VUE__) {
        info.vueVersion = 'detected';
      }
      
      // Check for Vue DevTools
      info.devtoolsHook = typeof window.__VUE_DEVTOOLS_GLOBAL_HOOK__ !== 'undefined';
      
      return JSON.stringify(info);
    })()
  `,

  // Дерево компонентов
  getComponentTree: `
    (() => {
      const result = { components: [], componentCount: 0 };
      
      // Method 1: Scan DOM for Vue-specific attributes
      const vueNodes = [];
      function scanDOM(node, depth = 0) {
        if (!node || depth > 5) return;
        
        const attrs = node.attributes ? Array.from(node.attributes).map(a => a.name) : [];
        const isVueNode = attrs.some(a => 
          a.startsWith('__') || a.startsWith('data-v-') || 
          a === 'id' || a === 'class'
        );
        
        if (node.nodeType === 1 && node.tagName) {
          const tag = node.tagName.toLowerCase();
          const id = node.id || null;
          const classes = node.className?.split?.(' ').filter(c => c && !c.startsWith('el-') && !c.startsWith('el-icon')) || [];
          
          if (tag !== '#text' && tag !== 'html' && tag !== 'head' && tag !== 'body') {
            vueNodes.push({ tag, id, classes, depth });
          }
        }
        
        if (node.children) {
          for (const child of node.children) {
            scanDOM(child, depth + 1);
          }
        }
      }
      
      scanDOM(document.body);
      result.components = vueNodes;
      result.componentCount = vueNodes.length;
      
      return JSON.stringify(result);
    })()
  `,

  // Pinia stores
  getPiniaState: `
    (() => {
      const result = { stores: {}, found: false };
      
      // Try to find Pinia instance
      let pinia = null;
      
      // Method 1: Check common locations
      for (const key of Object.keys(window)) {
        const val = window[key];
        if (val && typeof val === 'object') {
          if (val._s && val._state) { pinia = val; break; }
          if (val._install && val._addPlugin) { pinia = val; break; }
        }
      }
      
      // Method 2: Check document for Vue app instances
      if (!pinia) {
        const apps = document.querySelectorAll('[data-v-app]');
        for (const el of apps) {
          const key = Object.keys(el).find(k => k.startsWith('__vue__'));
          if (key) {
            const app = el[key];
            if (app?._container?._instance?.store) {
              pinia = app._container._instance.store;
              break;
            }
          }
        }
      }
      
      if (pinia) {
        result.found = true;
        const storeIds = Object.keys(pinia._s || {});
        
        for (const id of storeIds) {
          const store = pinia._s[id];
          try {
            result.stores[id] = {
              state: JSON.parse(JSON.stringify(store.state || {})),
              getters: Object.keys(store.getters || {}),
              actions: Object.keys(store).filter(k => typeof store[k] === 'function')
            };
          } catch(e) {
            result.stores[id] = { error: e.message };
          }
        }
      } else {
        result.found = false;
        result.note = 'Pinia instance not accessible from page context';
      }
      
      return JSON.stringify(result);
    })()
  `,

  // Router info
  getRouterInfo: `
    (() => {
      const result = { routes: [], currentRoute: null, found: false };
      
      // Try to find router instance
      let router = null;
      
      for (const key of Object.keys(window)) {
        const val = window[key];
        if (val && typeof val === 'object') {
          if (val.matchers !== undefined || val.getRoutes !== undefined) {
            router = val;
            break;
          }
        }
      }
      
      // Alternative: check Vue app instances
      if (!router) {
        const apps = document.querySelectorAll('[data-v-app]');
        for (const el of apps) {
          const key = Object.keys(el).find(k => k.startsWith('__vue__'));
          if (key) {
            const app = el[key];
            if (app?._container?._instance?.router) {
              router = app._container._instance.router;
              break;
            }
          }
        }
      }
      
      if (router) {
        result.found = true;
        try {
          result.routes = router.getRoutes().map(r => ({
            path: r.path,
            name: r.name || '(unnamed)',
            children: r.children?.length || 0,
            component: r.component?.name || '(anonymous)'
          }));
          
          const current = router.currentRoute?.value;
          if (current) {
            result.currentRoute = {
              path: current.path,
              name: current.name,
              params: current.params,
              query: current.query
            };
          }
        } catch(e) {
          result.error = e.message;
        }
      } else {
        result.found = false;
        result.note = 'Router instance not accessible';
      }
      
      return JSON.stringify(result);
    })()
  `,

  // Performance metrics
  getPerformance: `
    (() => {
      const nav = performance.getEntriesByType('navigation')[0] || {};
      const resources = performance.getEntriesByType('resource');
      const apiCalls = resources.filter(e => 
        e.initiatorType === 'xmlhttprequest' || 
        e.initiatorType === 'fetch' ||
        e.name.includes('/api/')
      );
      
      return JSON.stringify({
        domContentLoaded: Math.round(nav.domContentLoadedEventEnd || 0),
        loadComplete: Math.round(nav.loadEventEnd || 0),
        domInteractive: Math.round(nav.domInteractive || 0),
        redirectCount: nav.redirectCount || 0,
        transferSize: nav.transferSize || 0,
        totalResources: resources.length,
        apiCalls: apiCalls.length,
        lastApiCalls: apiCalls.slice(-5).map(e => ({
          url: e.name.split('?')[0],
          duration: Math.round(e.duration),
          size: e.transferSize
        }))
      });
    })()
  `
};

// ─── Commands ───────────────────────────────────────────────────────────────

async function cmdComponents(page) {
  console.log('📦 Vue Components\n');
  
  // Page context evaluation
  const treeResult = JSON.parse(await page.evaluate(vueScripts.getComponentTree));
  console.log(`Found ${treeResult.componentCount} DOM nodes`);
  console.log('Sample components:', JSON.stringify(treeResult.components.slice(0, 15), null, 2));
  
  // CDP DOM inspection
  const client = await page.context().newCDPSession(page);
  const domDoc = await client.send('DOM.getDocument', { depth: 3 });
  
  function countNodes(node) {
    let count = 1;
    if (node.children) {
      for (const child of node.children) {
        count += countNodes(child);
      }
    }
    return count;
  }
  
  const totalDomNodes = countNodes(domDoc.root);
  console.log(`\n🔍 Total DOM nodes (via CDP): ${totalDomNodes}`);
}

async function cmdState(page) {
  console.log('🗂️  Pinia State\n');
  const result = JSON.parse(await page.evaluate(vueScripts.getPiniaState));
  console.log(JSON.stringify(result, null, 2));
}

async function cmdRouter(page) {
  console.log('🧭 Router\n');
  const result = JSON.parse(await page.evaluate(vueScripts.getRouterInfo));
  console.log(JSON.stringify(result, null, 2));
}

async function cmdErrors(page) {
  console.log('⚠️  Errors & Warnings\n');
  
  const consoleErrors = [];
  page.on('console', msg => {
    if (msg.type() === 'error' || msg.type() === 'warning') {
      consoleErrors.push({
        type: msg.type(),
        text: msg.text(),
        location: `${msg.location().url.split('/').pop()}:${msg.location().lineNumber}`
      });
    }
  });
  
  // Runtime exceptions via CDP
  const runtimeErrors = [];
  const client = await page.context().newCDPSession(page);
  client.on('Runtime.exceptionThrown', exc => {
    runtimeErrors.push({
      description: exc.exceptionDetails.exception?.description || exc.exceptionDetails.text,
      timestamp: exc.timestamp
    });
  });
  
  await page.waitForTimeout(2000);
  
  console.log(`Console errors/warnings: ${consoleErrors.length}`);
  consoleErrors.forEach(e => console.log(`  [${e.type}] ${e.text} (${e.location})`));
  
  console.log(`\nRuntime exceptions: ${runtimeErrors.length}`);
  runtimeErrors.forEach(e => console.log(`  ❌ ${e.description}`));
}

async function cmdNetwork(page) {
  console.log('🌐 Network\n');
  const perf = JSON.parse(await page.evaluate(vueScripts.getPerformance));
  console.log(JSON.stringify(perf, null, 2));
}

async function cmdPerf(page) {
  console.log('⚡ Performance\n');
  const perf = JSON.parse(await page.evaluate(vueScripts.getPerformance));
  console.log(JSON.stringify(perf, null, 2));
}

async function cmdScreenshot(page) {
  console.log('📸 Screenshot\n');
  const path = resolve(process.cwd(), 'screenshot.png');
  await page.screenshot({ path, fullPage: true });
  console.log(`✅ Saved to: ${path}`);
}

async function cmdInfo(page) {
  console.log('ℹ️  App Info\n');
  const info = JSON.parse(await page.evaluate(vueScripts.getAppInfo));
  console.log(JSON.stringify(info, null, 2));
}

async function cmdAudit(page) {
  console.log('🔍 Full Vue Audit\n');
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
  const command = process.argv[2] || 'audit';
  const url = process.argv[3] || DEFAULT_URL;
  
  console.log(`🚀 Vue DevTools Inspector — ${url}\n`);
  
  const browser = await createBrowser();
  const context = await browser.newContext();
  const page = await context.newPage();
  
  try {
    console.log(`⏳ Connecting to ${url}...`);
    const vueReady = await navigate(page, url);
    
    if (vueReady) {
      console.log('✅ Vue app detected\n');
    } else {
      console.log('⚠️  Proceeding without Vue detection\n');
    }
    
    switch(command) {
      case 'components': await cmdComponents(page); break;
      case 'state': await cmdState(page); break;
      case 'router': await cmdRouter(page); break;
      case 'errors': await cmdErrors(page); break;
      case 'network': await cmdNetwork(page); break;
      case 'perf': await cmdPerf(page); break;
      case 'screenshot': await cmdScreenshot(page); break;
      case 'info': await cmdInfo(page); break;
      case 'audit': await cmdAudit(page); break;
      default:
        console.error(`❌ Unknown command: ${command}`);
        console.log('Available: components, state, router, errors, network, perf, screenshot, info, audit');
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
