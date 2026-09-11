<script lang="ts">
  import { onMount, type Snippet } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import Icon, { type IconName } from '$lib/components/Icon.svelte';
  import '../app.css';

  let { children }: { children: Snippet } = $props();
  const navigation: { href: string; label: string; icon: IconName; group?: string }[] = [
    { href: '/items', label: '품목 관리', icon: 'box', group: '기준 정보' },
    { href: '/bom', label: 'BOM 구성', icon: 'layers' },
    { href: '/inventory', label: '재고 · 입고', icon: 'inventory', group: '제조 운영' },
    { href: '/orders', label: '생산 지시', icon: 'production' },
    { href: '/movements', label: '재고 이력', icon: 'history' }
  ];
  const current = $derived(navigation.find(item => page.url.pathname.startsWith(item.href)));
  const sectionLabel = $derived(current?.label ?? (page.url.pathname.startsWith('/receipts') ? '입고 상세' : page.url.pathname.startsWith('/results') ? '실적 상세' : '워크스페이스'));
  let search = $state('');
  let searchError = $state('');
  let dark = $state(false);
  let collapsed = $state(false);

  onMount(() => {
    dark = document.documentElement.dataset.theme === 'dark';
    try { collapsed = localStorage.getItem('erp.ui.sidebar-collapsed') === 'true'; } catch { /* Preferences are optional. */ }
  });
  function toggleTheme() {
    dark = !dark;
    document.documentElement.dataset.theme = dark ? 'dark' : 'light';
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', dark ? '#141414' : '#f8f9fb');
    try { localStorage.setItem('erp.ui.theme', dark ? 'dark' : 'light'); } catch { /* Works in memory without storage. */ }
  }
  function toggleSidebar() {
    collapsed = !collapsed;
    try { localStorage.setItem('erp.ui.sidebar-collapsed', String(collapsed)); } catch { /* Optional preference only. */ }
  }
  async function submitSearch(event: SubmitEvent) {
    event.preventDefault(); searchError = '';
    try { await goto(`/items${search.trim() ? `?q=${encodeURIComponent(search.trim())}` : ''}`); }
    catch { searchError = '검색 화면으로 이동하지 못했습니다. 다시 시도해 주세요.'; }
  }
</script>

<svelte:head><title>조립 제조 ERP</title></svelte:head>
<a class="skip-link" href="#main">본문으로 이동</a>
<div class="app-shell" class:sidebar-collapsed={collapsed}>
  <aside class="sidebar">
    <a href="/items" class="brand" aria-label="조립 제조 ERP 홈">
      <span class="brand-mark"><Icon name="box" size={27} /></span>
      <span class="brand-wordmark"><strong>assembly<span>erp</span></strong><small>조립 제조 워크스페이스</small></span>
    </a>
    <div class="workspace-switch"><span class="workspace-avatar">A</span><span class="workspace-switch-label"><strong>조립 제조 ERP</strong><small>단일 조직 · 데모 환경</small></span></div>
    <nav id="workspace-navigation" aria-label="주 메뉴">
      {#each navigation as item}
        {#if item.group}<div class="workspace-label">{item.group}</div>{/if}
        <a href={item.href} aria-label={item.label} title={collapsed ? item.label : undefined} class:active={page.url.pathname.startsWith(item.href)} aria-current={page.url.pathname.startsWith(item.href) ? 'page' : undefined}>
          <Icon name={item.icon} size={19} /><span class="nav-label">{item.label}</span><span class="nav-arrow"><Icon name="chevron" size={14} /></span>
        </a>
      {/each}
    </nav>
    <div class="sidebar-bottom">
      <div class="workflow-note"><span class="workflow-note-icon"><Icon name="layers" /></span><strong>하나로 연결되는 제조</strong><p>품목부터 생산 실적까지,<br />작업의 흐름을 이어보세요.</p><a href="/orders/new">생산 시작하기 <Icon name="arrow" size={15} /></a></div>
      <details class="sidebar-guide"><summary><Icon name="help" size={18} /><span>데모 사용 안내</span></summary><p>품목 등록 → BOM 구성 → 부품 입고 → 생산 지시 → 실적 등록 순서로 사용하세요. 양품과 불량 모두 자재를 소비합니다.</p></details>
      <div class="sidebar-footer"><Icon name="shield" size={17} /><span>로그인 없는 로컬 데모</span><span class="version">v0.1</span></div>
    </div>
  </aside>
  <div class="workspace">
    <header class="topbar">
      <div class="topbar-start"><button type="button" class="icon-button sidebar-toggle" aria-label={collapsed ? '메뉴 펼치기' : '메뉴 접기'} aria-expanded={!collapsed} aria-controls="workspace-navigation" onclick={toggleSidebar}><Icon name="panel" size={20} /></button><nav class="breadcrumb" aria-label="현재 위치"><span>워크스페이스</span><Icon name="chevron" size={13} /><strong>{sectionLabel}</strong></nav></div>
      <form class="global-search" role="search" onsubmit={submitSearch}><Icon name="search" size={17} /><label for="global-item-search" class="sr-only">전역 품목 검색</label><input id="global-item-search" bind:value={search} maxlength="100" placeholder="품목 코드 또는 이름으로 검색" autocomplete="off" /><button type="submit" aria-label="품목 검색 실행"><Icon name="arrow" size={16} /></button></form>
      <div class="topbar-actions"><span class="demo-chip"><span></span>데모 모드</span><button type="button" class="theme-toggle icon-button" aria-label={dark ? '라이트 모드로 전환' : '다크 모드로 전환'} aria-pressed={dark} onclick={toggleTheme}><Icon name={dark ? 'sun' : 'moon'} size={18} /></button><span class="locale-chip">KO</span></div>
    </header>
    {#if searchError}<p class="notice notice-error" role="alert">{searchError}</p>{/if}
    <main id="main" tabindex="-1">
      {#key page.url.pathname + page.url.search}{@render children()}{/key}
    </main>
    <footer class="workspace-footer"><span>ASSEMBLY ERP <span class="footer-separator">·</span> 연결된 기록, 일치하는 수량</span><span>한국어 <span class="footer-separator">/</span> Asia/Seoul</span></footer>
  </div>
</div>
