<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { ApiError, api } from '$lib/api';
  import { Resource } from '$lib/state.svelte';
  import { formatQuantity, seoulTime } from '$lib/quantity';
  import type { Bom, Inventory, Item } from '$lib/types';
  import Badge from '$lib/components/Badge.svelte';
  import LoadState from '$lib/components/LoadState.svelte';
  import MaterialTable from '$lib/components/MaterialTable.svelte';

  const resource = new Resource<{ item: Item; balance: Inventory | undefined; bom: Bom | null }>();
  function load() {
    return resource.load(async () => {
      const item = await api.item(page.params.id!);
      const balances = await api.all<Inventory>('/inventory', { q: item.code });
      let bom: Bom | null = null;
      if (item.kind === 'finished_good') {
        try { bom = await api.bom(item.id); }
        catch (error) { if (!(error instanceof ApiError && error.status === 404)) throw error; }
      }
      return { item, balance: balances.find((row) => row.item_id === item.id), bom };
    });
  }
  onMount(() => { void load(); });
</script>

<svelte:head><title>품목 상세 · 조립 제조 ERP</title></svelte:head>
<a class="back-link" href="/items">← 품목 목록</a>
<LoadState loading={resource.loading} error={resource.error} retry={() => void load()} />
{#if resource.value && !resource.loading && !resource.error}
  {@const data = resource.value}
  <div class="page-heading"><div><p class="eyebrow">ITEM DETAIL</p><h1>{data.item.name}</h1><p><span class="mono">{data.item.code}</span> · <Badge value={data.item.kind} /></p></div><a class="button secondary" href={`/movements?item_id=${data.item.id}`}>재고 이력 보기 ↗</a></div>
  <div class="split">
    <section class="panel"><div class="panel-header"><h2>품목 정보</h2></div><div class="panel-body"><dl class="detail-list"><dt>품목 코드</dt><dd class="mono">{data.item.code}</dd><dt>품목명</dt><dd>{data.item.name}</dd><dt>종류</dt><dd><Badge value={data.item.kind} /></dd><dt>단위</dt><dd>{data.item.unit}</dd><dt>등록 일시</dt><dd>{seoulTime(data.item.created_at)}</dd><dt>품목 ID</dt><dd class="mono id-text">{data.item.id}</dd></dl></div></section>
    <section class="panel"><div class="panel-header"><h2>현재 재고</h2></div><div class="panel-body"><div class="stat remaining"><span>서버에서 조회한 현재고</span><strong>{data.balance ? formatQuantity(data.balance.quantity) : '—'}</strong><small>{data.item.unit}</small></div>{#if data.balance}<p class="help" style="margin-top: 15px">갱신 {seoulTime(data.balance.updated_at)}</p>{:else}<p class="notice notice-warning">재고 행을 확인하지 못했습니다. 다시 조회해 주세요.</p>{/if}<div class="form-actions">{#if data.item.kind === 'component'}<a class="button" href={`/inventory?item_id=${data.item.id}`}>부품 입고</a>{:else}<a class="button" href={`/bom?item_id=${data.item.id}`}>BOM 구성</a>{/if}<button type="button" class="button quiet" onclick={() => void load()}>새로고침</button></div></div></section>
  </div>
  {#if data.item.kind === 'finished_good'}
    <section class="panel" style="margin-top: 25px"><div class="panel-header"><div><h2>현재 BOM</h2><p>이 구성은 이후 생성되는 생산 지시에 적용됩니다.</p></div><a class="button secondary compact" href={`/bom?item_id=${data.item.id}`}>BOM 편집</a></div>{#if data.bom}<MaterialTable materials={data.bom.components} />{:else}<div class="empty-state">등록된 BOM이 없습니다. 생산 지시를 만들기 전에 부품 구성을 등록해 주세요.</div>{/if}</section>
  {/if}
{/if}
