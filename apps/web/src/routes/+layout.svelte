<script lang="ts">
  import { onMount, type Snippet } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import Icon, { type IconName } from '$lib/components/Icon.svelte';
  import { initLocale, t, toggleLocale, i18n } from '$lib/i18n.svelte';
  import '../app.css';

  let { children }: { children: Snippet } = $props();
  const legacyNavigation: { href: string; labelKey: Parameters<typeof t>[0]; icon: IconName; groupKey?: Parameters<typeof t>[0] }[] = [
    { href: '/items', labelKey: 'navItems', icon: 'box', groupKey: 'legacyGroupMaster' },
    { href: '/bom', labelKey: 'navBom', icon: 'layers' },
    { href: '/inventory', labelKey: 'navInventoryReceipt', icon: 'inventory', groupKey: 'legacyGroupOperations' },
    { href: '/orders', labelKey: 'navProductionOrders', icon: 'production' },
    { href: '/movements', labelKey: 'navInventoryHistory', icon: 'history' }
  ];
  const factoryNavigation: typeof legacyNavigation = [
    { href: '/v2/reference', labelKey: 'navReference', icon: 'box', groupKey: 'legacyGroupMaster' },
    { href: '/v2/orders', labelKey: 'navOrders', icon: 'production', groupKey: 'legacyGroupOperations' },
    { href: '/v2/sessions', labelKey: 'navSessions', icon: 'clock' },
    { href: '/v2/documents', labelKey: 'navDocuments', icon: 'layers' },
    { href: '/v2/inventory', labelKey: 'navFactoryInventory', icon: 'inventory' }
  ];
  const isFactory = $derived(page.url.pathname.startsWith('/v2'));
  const navigation = $derived(isFactory ? factoryNavigation : legacyNavigation);
  const current = $derived(navigation.find(item => page.url.pathname.startsWith(item.href)));
  const sectionLabel = $derived(current ? t(current.labelKey) : page.url.pathname.startsWith('/receipts') ? t('receiptDetail') : page.url.pathname.startsWith('/results') ? t('resultDetail') : t('workspace'));
  let search = $state('');
  let searchError = $state('');
  let dark = $state(false);
  let collapsed = $state(false);

  onMount(() => {
    initLocale();
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
    catch { searchError = t('searchNavigationError'); }
  }
</script>

<svelte:head><title>{t('appTitle')}</title></svelte:head>
  <a class="skip-link" href="#main">{t('skipMain')}</a>
<div class="app-shell" class:sidebar-collapsed={collapsed}>
  <aside class="sidebar">
    <a href={isFactory ? "/v2/orders" : "/items"} class="brand" aria-label={t('appHome')}>
      <span class="brand-mark"><Icon name="box" size={27} /></span>
      <span class="brand-wordmark"><strong>assembly<span>erp</span></strong><small>{t('appWorkspace')}</small></span>
    </a>
    <div class="workspace-switch"><span class="workspace-avatar">A</span><span class="workspace-switch-label"><strong>{t('workspaceName')}</strong><small>{t('workspaceScope')}</small></span></div>
    <nav id="workspace-navigation" aria-label={t('mainMenu')}>
      {#each navigation as item}
        {#if item.groupKey}<div class="workspace-label">{t(item.groupKey)}</div>{/if}
        <a href={item.href} aria-label={t(item.labelKey)} title={collapsed ? t(item.labelKey) : undefined} class:active={page.url.pathname.startsWith(item.href)} aria-current={page.url.pathname.startsWith(item.href) ? 'page' : undefined}>
          <Icon name={item.icon} size={19} /><span class="nav-label">{t(item.labelKey)}</span><span class="nav-arrow"><Icon name="chevron" size={14} /></span>
        </a>
      {/each}
    </nav>
    <div class="sidebar-bottom">
      <div class="workflow-note"><span class="workflow-note-icon"><Icon name="layers" /></span><strong>{t('connectedManufacturing')}</strong><p>{#each t('connectedManufacturingBody').split('\n') as line}{line}<br />{/each}</p><a href={isFactory ? "/v2/orders" : "/orders/new"}>{t('startProduction')} <Icon name="arrow" size={15} /></a></div>
      <details class="sidebar-guide"><summary><Icon name="help" size={18} /><span>{t('demoGuide')}</span></summary><p>{isFactory ? t('factoryGuide') : t('legacyGuide')}</p></details>
      <div class="sidebar-footer"><Icon name="shield" size={17} /><span>{t('portfolioDemo')}</span>{#if !isFactory}<a class="version" href="/">{t('factoryWork')}</a>{/if}</div>
    </div>
  </aside>
  <div class="workspace">
    <header class="topbar">
      <div class="topbar-start"><button type="button" class="icon-button sidebar-toggle" aria-label={collapsed ? t('expandMenu') : t('collapseMenu')} aria-expanded={!collapsed} aria-controls="workspace-navigation" onclick={toggleSidebar}><Icon name="panel" size={20} /></button><nav class="breadcrumb" aria-label={t('currentLocation')}><span>{t('workspace')}</span><Icon name="chevron" size={13} /><strong>{sectionLabel}</strong></nav></div>
      {#if !isFactory}<form class="global-search" role="search" onsubmit={submitSearch}><Icon name="search" size={17} /><label for="global-item-search" class="sr-only">{t('globalItemSearch')}</label><input id="global-item-search" bind:value={search} maxlength="100" placeholder={t('itemSearchPlaceholder')} autocomplete="off" /><button type="submit" aria-label={t('runItemSearch')}><Icon name="arrow" size={16} /></button></form>{/if}
      <div class="topbar-actions"><span class="demo-chip"><span></span>{t('demoMode')}</span><button type="button" class="theme-toggle icon-button" aria-label={dark ? t('switchLight') : t('switchDark')} aria-pressed={dark} onclick={toggleTheme}><Icon name={dark ? 'sun' : 'moon'} size={18} /></button><button type="button" class="locale-chip" aria-label={t('toggleLocale')} onclick={toggleLocale}>{i18n.locale.toUpperCase()}</button></div>
    </header>
    {#if searchError}<p class="notice notice-error" role="alert">{searchError}</p>{/if}
    <main id="main" tabindex="-1">
      {#key page.url.pathname + page.url.search}{@render children()}{/key}
    </main>
    <footer class="workspace-footer"><span>ASSEMBLY ERP <span class="footer-separator">·</span> {t('footerSlogan')}</span><span>{t('localeTimezone')}</span></footer>
  </div>
</div>
