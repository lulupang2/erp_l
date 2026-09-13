<script lang="ts">
  import { type Snippet } from 'svelte';
  import { page } from '$app/state';
  import { can } from '$lib/v2/client.svelte';
  import '$lib/v2/factory.css';
  let { children }: { children: Snippet } = $props();
  let dark = $state(false);
  const navigation = [{ href: '/v2/orders', label: '작업 지시 · 현장' }, { href: '/v2/documents', label: '수불 · 생산 · 품질' }, { href: '/v2/inventory', label: '재고 · 추적' }, { href: '/v2/reference', label: '기준정보' }];
  function theme() { dark = !dark; document.documentElement.dataset.theme = dark ? 'dark' : 'light'; try { localStorage.setItem('erp.ui.theme', dark ? 'dark' : 'light'); } catch { /* Optional visual preference. */ } }
</script>
<div class="factory">
  <a class="skip-link" href="#factory-main">공장 업무 본문으로 이동</a>
  <header class="factory-header"><a class="factory-brand" href="/v2/orders">assembly<span>erp</span> / 공장 v2</a><a href="/items">v1 기록</a><div class="identity"><span><strong>포트폴리오 데모</strong><span class="subtext">단일 작업 공간</span></span><button class="button secondary compact" onclick={theme} aria-pressed={dark}>{dark ? '라이트 모드' : '다크 모드'}</button></div></header>
  <nav class="factory-nav" aria-label="공장 업무">{#each navigation as item}{#if item.href !== '/v2/reference' || can('admin', 'planner', 'materials', 'quality')}<a href={item.href} aria-current={page.url.pathname.startsWith(item.href) ? 'page' : undefined}>{item.label}</a>{/if}{/each}<a href="/v2/sessions" aria-current={page.url.pathname.startsWith('/v2/sessions') ? 'page' : undefined}>작업 세션</a></nav>
  <main id="factory-main" class="factory-content" tabindex="-1">
    {@render children()}
  </main>
</div>
