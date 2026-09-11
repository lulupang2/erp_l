<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { Resource } from '$lib/state.svelte';
  import { formatQuantity, seoulTime } from '$lib/quantity';
  import type { Item, Movement, Page } from '$lib/types';
  import Badge from '$lib/components/Badge.svelte';
  import LoadState from '$lib/components/LoadState.svelte';
  import Pagination from '$lib/components/Pagination.svelte';

  const resource = new Resource<{ rows: Page<Movement>; items: Map<string, Item> }>();
  let selected = $state(page.url.searchParams.get('item_id') ?? '');
  let filter = $state(untrack(() => selected)); let pageNo = $state(1); let pageSize = $state(20);
  function load(next = pageNo, size = pageSize) {
    pageNo = next; pageSize = size;
    return resource.load(async () => {
      const rows = await api.movements({ item_id: filter, page: pageNo, page_size: pageSize });
      const items = await api.itemMap([...rows.data.map((row) => row.item_id), ...(filter ? [filter] : [])]);
      return { rows, items };
    });
  }
  function search(event: SubmitEvent) { event.preventDefault(); filter = selected; void load(1); }
  onMount(() => { void load(); });
</script>

<svelte:head><title>재고 이력 · 조립 제조 ERP</title></svelte:head>
<div class="page-heading"><div><p class="eyebrow">STOCK MOVEMENTS</p><h1>재고 이력</h1><p>변동 수량과 원인 문서를 연결해 재고가 바뀐 이유를 확인합니다.</p></div><a href="/inventory" class="button secondary">현재고 보기</a></div>
<section class="panel">
  <form class="toolbar" onsubmit={search}><div class="field search"><label for="movement-item">조회 품목</label><select id="movement-item" bind:value={selected}><option value="">전체 품목</option>{#each [...(resource.value?.items.values() ?? [])] as item (item.id)}<option value={item.id}>{item.code} · {item.name}</option>{/each}</select></div><button type="submit" class="button secondary" disabled={resource.loading}>조회</button></form>
  <LoadState loading={resource.loading} error={resource.error} empty={resource.value?.rows.data.length === 0} emptyText="조회된 재고 변동이 없습니다. 입고 또는 생산 실적을 등록하면 기록됩니다." retry={() => void load()} />
  {#if resource.value && !resource.loading && !resource.error}
    {#if resource.value.rows.data.length}<div class="table-scroll"><table><caption class="sr-only">재고 변동 이력</caption><thead><tr><th scope="col">변동 일시 · 서울</th><th scope="col">품목</th><th scope="col">변동 종류</th><th scope="col" class="numeric">증감 수량</th><th scope="col">단위</th><th scope="col">원인 문서</th></tr></thead><tbody>{#each resource.value.rows.data as row (row.id)}{@const item = resource.value.items.get(row.item_id)}<tr><td class="text-muted">{seoulTime(row.created_at)}</td><td><a href={`/items/${row.item_id}`} class="item-name">{item?.name ?? row.item_id}</a><span class="subtext mono">{item?.code ?? '품목 상세 확인'}</span></td><td><Badge value={row.movement_type} /></td><td class:text-positive={row.delta > 0} class:text-negative={row.delta < 0} class="numeric"><strong>{row.delta > 0 ? '+' : ''}{formatQuantity(row.delta)}</strong></td><td>{item?.unit ?? '—'}</td><td>{#if row.receipt_id}<a href={`/receipts/${row.receipt_id}`}>입고 상세 ↗</a>{:else if row.result_id}<a href={`/results/${row.result_id}`}>실적 상세 ↗</a>{:else}<span class="text-muted">연결 없음</span>{/if}</td></tr>{/each}</tbody></table></div>{/if}
    <Pagination pagination={resource.value.rows.pagination} onchange={(next, size) => void load(next, size)} />
  {/if}
</section>
