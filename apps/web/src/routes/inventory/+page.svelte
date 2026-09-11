<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { inventorySummary } from '$lib/summaries';
  import { Resource } from '$lib/state.svelte';
  import { formatQuantity, seoulTime } from '$lib/quantity';
  import type { Inventory, Page } from '$lib/types';
  import Badge from '$lib/components/Badge.svelte';
  import LoadState from '$lib/components/LoadState.svelte';
  import Pagination from '$lib/components/Pagination.svelte';
  import ReceiptForm from '$lib/components/ReceiptForm.svelte';
  import Icon from '$lib/components/Icon.svelte';
  import MetricCard from '$lib/components/MetricCard.svelte';

  const resource = new Resource<Page<Inventory>>();
  const summary = new Resource<Awaited<ReturnType<typeof inventorySummary>>>();
  const totals = $derived(summary.error || summary.loading ? null : summary.value);
  function loadSummary() { return summary.load(() => inventorySummary()); }
  async function refresh() { await Promise.all([load(), loadSummary()]); }
  let q = $state(''); let kind = $state(''); let pageNo = $state(1); let pageSize = $state(20);
  let filters = $state({ q: '', kind: '' });
  function load(next = pageNo, size = pageSize) {
    pageNo = next; pageSize = size;
    return resource.load(() => api.inventory({ ...filters, page: pageNo, page_size: pageSize }));
  }
  function search(event: SubmitEvent) { event.preventDefault(); filters = { q: q.trim(), kind }; void load(1); }
  onMount(() => { void refresh(); });
</script>

<svelte:head><title>재고 · 입고 · 조립 제조 ERP</title></svelte:head>
<div class="page-heading"><div><p class="eyebrow">INVENTORY CONTROL</p><h1>재고 · 입고</h1><p>현재고를 확인하고 부품을 입고합니다. 완제품 재고는 양품 생산으로만 증가합니다.</p></div><button type="button" class="button secondary" disabled={resource.loading || summary.loading} onclick={() => void refresh()}><Icon name="refresh" size={16} />새로고침</button></div>
<div class="metric-grid" aria-label="전체 재고 품목 요약">
  <MetricCard label="관리 품목" value={totals?.total ?? null} loading={summary.loading} icon="inventory" note="전체 재고 관리 품목" testId="stock-summary-total" />
  <MetricCard label="재고 보유 품목" value={totals?.stocked ?? null} loading={summary.loading} icon="box" tone="green" note="현재고가 1 이상인 품목" testId="stock-summary-stocked" />
  <MetricCard label="재고 0 품목" value={totals?.empty ?? null} loading={summary.loading} icon="alert" tone="red" note="미입고 품목도 포함됩니다" testId="stock-summary-empty" />
  <MetricCard label="부품 품목" value={totals?.components ?? null} loading={summary.loading} icon="layers" tone="blue" note="생산 자재로 사용하는 품목" testId="stock-summary-components" />
</div>
<p class="summary-caption" class:summary-error={Boolean(summary.error)}>{summary.error ? '재고 요약을 불러오지 못했습니다.' : '전체 품목 수 기준 · 서로 다른 단위의 재고 수량은 합산하지 않습니다'}{#if summary.error}<button class="summary-retry" onclick={() => void loadSummary()}>요약 다시 조회</button>{/if}</p>
<div class="split">
  <section class="panel" aria-label="현재고 목록">
    <div class="panel-header"><h2 class="panel-label"><Icon name="inventory" size={18} />현재고 목록</h2><span class="panel-count">품목별 재고 현황</span></div>
    <form class="toolbar" onsubmit={search}><div class="field"><label for="inventory-kind">품목 종류</label><select id="inventory-kind" bind:value={kind}><option value="">전체 종류</option><option value="component">부품</option><option value="finished_good">완제품</option></select></div><div class="field search"><label for="inventory-q">코드 · 이름 검색</label><input id="inventory-q" bind:value={q} maxlength="100" placeholder="품목 코드 또는 이름" /></div><button type="submit" class="button secondary" disabled={resource.loading}>조회</button></form>
    <LoadState loading={resource.loading} error={resource.error} empty={resource.value?.data.length === 0} emptyText="조회된 재고가 없습니다. 품목이나 검색 조건을 확인해 주세요." retry={() => void load()} />
    {#if resource.value && !resource.loading && !resource.error}
      {#if resource.value.data.length}<div class="table-scroll"><table><caption class="sr-only">현재고 목록</caption><thead><tr><th scope="col">품목</th><th scope="col">종류</th><th scope="col" class="numeric">현재고</th><th scope="col">단위</th><th scope="col">재고 이력</th></tr></thead><tbody>{#each resource.value.data as row (row.item_id)}<tr><td><a href={`/items/${row.item_id}`} class="item-name">{row.name}</a><span class="subtext mono">{row.code}</span></td><td><Badge value={row.kind} /></td><td class="numeric" title={`갱신: ${seoulTime(row.updated_at)}`}><strong>{formatQuantity(row.quantity)}</strong></td><td>{row.unit}</td><td><a href={`/movements?item_id=${row.item_id}`} aria-label={`${row.name} 재고 이력`}>이력 보기 ↗</a></td></tr>{/each}</tbody></table></div>{/if}
      <Pagination pagination={resource.value.pagination} onchange={(next, size) => void load(next, size)} />
    {/if}
  </section>
  <ReceiptForm initialItemId={page.url.searchParams.get('item_id') ?? ''} onsaved={refresh} />
</div>
<div class="notice notice-info"><strong>재고를 미리 예약하지 않습니다.</strong><p>생산 지시를 만들 때 재고는 변하지 않습니다. 실적 등록 시 양품과 불량을 합친 수량만큼 자재를 소비하며, 최종 가용 재고는 서버에서 확인합니다.</p></div>
