<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { itemSummary } from '$lib/summaries';
  import { Resource } from '$lib/state.svelte';
  import { seoulTime } from '$lib/quantity';
  import type { Item, Page } from '$lib/types';
  import Badge from '$lib/components/Badge.svelte';
  import LoadState from '$lib/components/LoadState.svelte';
  import Pagination from '$lib/components/Pagination.svelte';
  import Icon from '$lib/components/Icon.svelte';
  import MetricCard from '$lib/components/MetricCard.svelte';

  const resource = new Resource<Page<Item>>();
  const summary = new Resource<Awaited<ReturnType<typeof itemSummary>>>();
  let q = $state(page.url.searchParams.get('q') ?? '');
  let kind = $state('');
  let pageNo = $state(1);
  let pageSize = $state(20);
  let filters = $state({ q: page.url.searchParams.get('q') ?? '', kind: '' });
  const totals = $derived(summary.error || summary.loading ? null : summary.value);
  function loadSummary() { return summary.load(() => itemSummary()); }

  function load(next = pageNo, size = pageSize) {
    pageNo = next; pageSize = size;
    return resource.load(() => api.items({ ...filters, page: pageNo, page_size: pageSize }));
  }
  function search(event: SubmitEvent) {
    event.preventDefault(); filters = { q: q.trim(), kind }; void load(1);
  }
  function selectKind(value: string) { kind = value; filters = { q: q.trim(), kind }; void load(1); }
  onMount(() => { void load(); void loadSummary(); });
</script>

<svelte:head><title>품목 관리 · 조립 제조 ERP</title></svelte:head>
<div class="page-heading"><div><p class="eyebrow">MASTER DATA</p><h1>품목 관리</h1><p>생산에 쓰이는 부품과 완제품의 기준 정보를 관리합니다.</p></div><a href="/items/new" class="button"><Icon name="plus" size={17} />품목 등록</a></div>
<div class="metric-grid three" aria-label="전체 품목 요약">
  <MetricCard label="전체 품목" value={totals?.total ?? null} loading={summary.loading} icon="grid" note="등록된 모든 부품과 완제품" testId="item-summary-total" />
  <MetricCard label="부품" value={totals?.components ?? null} loading={summary.loading} icon="layers" tone="blue" note="생산에 투입하는 자재" testId="item-summary-components" />
  <MetricCard label="완제품" value={totals?.finished ?? null} loading={summary.loading} icon="box" tone="green" note="BOM을 구성해 생산하는 품목" testId="item-summary-finished" />
</div>
<p class="summary-caption" class:summary-error={Boolean(summary.error)}>{summary.error ? '품목 요약을 불러오지 못했습니다.' : '전체 등록 품목 기준 · 아래 검색 조건과 별도 집계'}{#if summary.error}<button class="summary-retry" onclick={() => void loadSummary()}>요약 다시 조회</button>{/if}</p>
<section class="panel" aria-label="품목 목록">
  <div class="panel-header"><h2 class="panel-label"><Icon name="box" size={18} />품목 목록</h2><span class="panel-count">코드 · 종류 · 단위</span></div>
  <div class="view-tabs" role="group" aria-label="품목 종류 빠른 필터"><button type="button" aria-pressed={filters.kind === ''} onclick={() => selectKind('')} disabled={resource.loading}>전체 품목</button><button type="button" aria-pressed={filters.kind === 'component'} onclick={() => selectKind('component')} disabled={resource.loading}>부품</button><button type="button" aria-pressed={filters.kind === 'finished_good'} onclick={() => selectKind('finished_good')} disabled={resource.loading}>완제품</button></div>
  <form class="toolbar" onsubmit={search}>
    <div class="field"><label for="kind-filter">품목 종류</label><select id="kind-filter" bind:value={kind}><option value="">전체 종류</option><option value="component">부품</option><option value="finished_good">완제품</option></select></div>
    <div class="field search"><label for="item-search">코드 · 이름 검색</label><input id="item-search" bind:value={q} placeholder="품목 코드 또는 이름" maxlength="100" /></div>
    <button type="submit" class="button secondary" disabled={resource.loading}><Icon name="search" size={16} />조회</button>
  </form>
  <LoadState loading={resource.loading} error={resource.error} empty={resource.value?.data.length === 0} emptyText="등록된 품목이 없습니다. 부품 또는 완제품을 등록해 주세요." retry={() => void load()} />
  {#if resource.value && !resource.loading && !resource.error}
    {#if resource.value.data.length}
      <div class="table-scroll"><table><caption class="sr-only">품목 목록</caption><thead><tr><th scope="col">품목 코드</th><th scope="col">품목명</th><th scope="col">종류</th><th scope="col">단위</th><th scope="col">등록 일시 · 서울</th><th scope="col">상세</th></tr></thead>
        <tbody>{#each resource.value.data as item (item.id)}<tr><td class="mono">{item.code}</td><td><a class="item-name" href={`/items/${item.id}`}>{item.name}</a></td><td><Badge value={item.kind} /></td><td>{item.unit}</td><td class="text-muted">{seoulTime(item.created_at)}</td><td><a href={`/items/${item.id}`} aria-label={`${item.name} 상세 보기`}>상세 보기 ↗</a></td></tr>{/each}</tbody>
      </table></div>
    {/if}
    <Pagination pagination={resource.value.pagination} loading={resource.loading} onchange={(next, size) => void load(next, size)} />
  {/if}
</section>
<div class="notice notice-info"><strong>등록 후 업무 연결</strong><p>부품은 입고 후 생산 자재로 사용할 수 있습니다. 완제품은 BOM을 구성한 뒤 생산 지시를 생성합니다. 신규 품목의 현재고는 0입니다.</p></div>
