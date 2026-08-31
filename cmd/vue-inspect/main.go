// vue-inspect — подключается к запущенному Vue dev server через chromedp
// и собирает информацию о приложении: компоненты, стейт, роутер, ошибки, сеть.
//
// Usage:
//   go run cmd/vue-inspect/main.go <command> [url]
//
// Commands:
//   components    — дерево компонентов Vue
//   state         — Pinia stores + reactive state
//   router        — текущий роутер, маршруты
//   errors        — console errors/warnings
//   network       — сетевые запросы (последние N)
//   perf          — performance metrics
//   screenshot    — скриншот страницы
//   info          — общая информация о приложении
//   audit         — всё сразу

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

const defaultURL = "http://localhost:5173"
const defaultTimeout = 30 * time.Second

// ─── JS introspection scripts ────────────────────────────────────────────────

const scriptAppInfo = `() => {
	const info = {
		url: location.href,
		title: document.title,
		hasVueApp: !!document.querySelector('[data-v-app]'),
		domElements: document.querySelectorAll('*').length,
		scripts: Array.from(document.scripts).map(s => s.src?.split('/').pop()).filter(Boolean),
		stylesheets: document.styleSheets.length
	};
	return JSON.stringify(info);
}`

const scriptComponentTree = `() => {
	const result = { components: [], component_count: 0 };
	function scan(node, depth = 0) {
		if (!node || depth > 5) return;
		if (node.nodeType === 1 && node.tagName) {
			const tag = node.tagName.toLowerCase();
			if (tag !== '#text' && tag !== 'html' && tag !== 'head' && tag !== 'body') {
				const id = node.id || '';
				const classes = (node.className || '').toString().split(/\s+/).filter(c => c && !c.startsWith('el-'));
				result.components.push({ tag, id, classes, depth });
				result.component_count++;
			}
		}
		if (node.children) for (const child of node.children) scan(child, depth + 1);
	}
	scan(document.body);
	return JSON.stringify(result);
}`

const scriptPiniaState = `() => {
	const result = { stores: {}, found: false };
	let pinia = null;
	for (const key of Object.keys(window)) {
		const val = window[key];
		if (val && typeof val === 'object') {
			if (val._s && val._state) { pinia = val; break; }
		}
	}
	if (!pinia) {
		const apps = document.querySelectorAll('[data-v-app]');
		for (const el of apps) {
			for (const key of Object.keys(el)) {
				if (key.startsWith('__vue__')) {
					const app = el[key];
					if (app?._container?._instance?.store) { pinia = app._container._instance.store; break; }
				}
			}
			if (pinia) break;
		}
	}
	if (pinia) {
		result.found = true;
		for (const id of Object.keys(pinia._s || {})) {
			const store = pinia._s[id];
			try { result.stores[id] = JSON.parse(JSON.stringify({ state: store.state || {}, getters: Object.keys(store.getters || {}) })); }
			catch(e) { result.stores[id] = { error: e.message }; }
		}
	} else {
		result.note = 'Pinia instance not accessible from page context';
	}
	return JSON.stringify(result);
}`

const scriptRouterInfo = `() => {
	const result = { routes: [], found: false };
	let router = null;
	for (const key of Object.keys(window)) {
		const val = window[key];
		if (val && typeof val === 'object' && (val.matchers !== undefined || val.getRoutes !== undefined)) { router = val; break; }
	}
	if (!router) {
		const apps = document.querySelectorAll('[data-v-app]');
		for (const el of apps) {
			for (const key of Object.keys(el)) {
				if (key.startsWith('__vue__')) {
					const app = el[key];
					if (app?._container?._instance?.router) { router = app._container._instance.router; break; }
				}
			}
			if (router) break;
		}
	}
	if (router) {
		result.found = true;
		try {
			result.routes = router.getRoutes().map(r => ({ path: r.path, name: r.name || '(unnamed)', children: r.children?.length || 0 }));
			const cur = router.currentRoute?.value;
			if (cur) result.current_route = { path: cur.path, name: cur.name, params: cur.params || {}, query: cur.query || {} };
		} catch(e) { result.error = e.message; }
	} else {
		result.note = 'Router instance not accessible';
	}
	return JSON.stringify(result);
}`

const scriptPerformance = `() => {
	const nav = performance.getEntriesByType('navigation')[0] || {};
	const resources = performance.getEntriesByType('resource');
	const apiCalls = resources.filter(e => e.initiatorType === 'xmlhttprequest' || e.initiatorType === 'fetch' || e.name.includes('/api/'));
	return JSON.stringify({
		dom_content_loaded_ms: Math.round(nav.domContentLoadedEventEnd || 0),
		load_complete_ms: Math.round(nav.loadEventEnd || 0),
		dom_interactive_ms: Math.round(nav.domInteractive || 0),
		redirect_count: nav.redirectCount || 0,
		total_resources: resources.length,
		api_calls: apiCalls.length,
		last_api_calls: apiCalls.slice(-5).map(e => ({ url: e.name.split('?')[0], duration_ms: Math.round(e.duration), size_bytes: e.transferSize }))
	});
}`

// ─── Helpers ─────────────────────────────────────────────────────────────────

func runEval(ctx context.Context, js string) (string, error) {
	// Wrap arrow function call: "() => {...}" -> "(() => {...})()"
	wrapped := "(" + js + ")()"
	var result interface{}
	err := chromedp.Run(ctx, chromedp.Evaluate(wrapped, &result))
	if err != nil {
		return "", err
	}
	// JS returns JSON.stringify -> Go string containing JSON; return directly
	if s, ok := result.(string); ok {
		return s, nil
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

func printJSON(data string) {
	var pretty interface{}
	json.Unmarshal([]byte(data), &pretty)
	out, _ := json.MarshalIndent(pretty, "", "  ")
	fmt.Println(string(out))
}

// ─── Commands ────────────────────────────────────────────────────────────────

func cmdInfo(ctx context.Context) {
	fmt.Println("ℹ️  App Info")
	fmt.Println(strings.Repeat("═", 52))
	result, err := runEval(ctx, scriptAppInfo)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	printJSON(result)
}

func cmdComponents(ctx context.Context) {
	fmt.Println("📦 Vue Components")
	fmt.Println(strings.Repeat("═", 52))
	result, err := runEval(ctx, scriptComponentTree)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	printJSON(result)
}

func cmdState(ctx context.Context) {
	fmt.Println("🗂️  Pinia State")
	fmt.Println(strings.Repeat("═", 52))
	result, err := runEval(ctx, scriptPiniaState)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	printJSON(result)
}

func cmdRouter(ctx context.Context) {
	fmt.Println("🧭 Router")
	fmt.Println(strings.Repeat("═", 52))
	result, err := runEval(ctx, scriptRouterInfo)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	printJSON(result)
}

func cmdPerf(ctx context.Context) {
	fmt.Println("⚡ Performance")
	fmt.Println(strings.Repeat("═", 52))
	result, err := runEval(ctx, scriptPerformance)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	printJSON(result)
}

func cmdScreenshot(ctx context.Context) {
	fmt.Println("📸 Screenshot")
	fmt.Println(strings.Repeat("═", 52))
	var data []byte
	err := chromedp.Run(ctx, chromedp.FullScreenshot(&data, 90))
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	path := "screenshot.png"
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Fatalf("Write error: %v", err)
	}
	fmt.Printf("✅ Saved to: %s\n", path)
}

func cmdAudit(ctx context.Context) {
	fmt.Println("🔍 Full Vue Audit")
	fmt.Println(strings.Repeat("═", 60))
	cmdInfo(ctx)
	fmt.Println()
	cmdComponents(ctx)
	fmt.Println()
	cmdState(ctx)
	fmt.Println()
	cmdRouter(ctx)
	fmt.Println()
	cmdPerf(ctx)
	fmt.Println()
}

// ─── Main ────────────────────────────────────────────────────────────────────

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run ./cmd/vue-inspect <command> [url]")
		fmt.Println("Commands: info, components, state, router, perf, screenshot, audit")
		fmt.Printf("Default URL: %s\n", defaultURL)
		os.Exit(1)
	}
	command := os.Args[1]
	url := defaultURL
	if len(os.Args) > 2 && !strings.HasPrefix(os.Args[2], "-") {
		url = os.Args[2]
	} else if len(os.Args) > 3 {
		url = os.Args[3]
	}

	fmt.Printf("🚀 Vue Inspector — %s\n\n", url)

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Headless,
		chromedp.NoSandbox,
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	execCtx, execCancel := chromedp.NewContext(allocCtx)
	defer execCancel()

	if err := chromedp.Run(execCtx, chromedp.Navigate(url)); err != nil {
		log.Fatalf("Navigate error: %v", err)
	}

	// Wait for Vue app to boot
	if err := chromedp.Run(execCtx, chromedp.WaitReady("[data-v-app]", chromedp.ByQuery)); err != nil {
		fmt.Println("⚠️  [data-v-app] not found, continuing anyway")
	} else {
		fmt.Println("✅ Vue app detected")
	}

	switch command {
	case "components":
		cmdComponents(execCtx)
	case "state":
		cmdState(execCtx)
	case "router":
		cmdRouter(execCtx)
	case "perf":
		cmdPerf(execCtx)
	case "screenshot":
		cmdScreenshot(execCtx)
	case "info":
		cmdInfo(execCtx)
	case "audit":
		cmdAudit(execCtx)
	default:
		fmt.Fprintf(os.Stderr, "❌ Unknown command: %s\n", command)
		fmt.Println("Available: components, state, router, perf, screenshot, info, audit")
		os.Exit(1)
	}
}
