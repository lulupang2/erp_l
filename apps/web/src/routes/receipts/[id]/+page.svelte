<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { Resource } from '$lib/state.svelte';
  import { formatQuantity, seoulTime } from '$lib/quantity';
  import type { Item, Receipt } from '$lib/types';
  import LoadState from '$lib/components/LoadState.svelte';
  import Badge from '$lib/components/Badge.svelte';

  const resource = new Resource<{ receipt: Receipt; item: Item }>();
  function load() { return resource.load(async () => { const receipt = await api.receipt(page.params.id!); return { receipt, item: await api.item(receipt.item_id) }; }); }
  onMount(() => { void load(); });
</script>

<svelte:head><title>입고 상세 · 조립 제조 ERP</title></svelte:head>
<a href="/movements" class="back-link">← 재고 이력</a>
<LoadState loading={resource.loading} error={resource.error} retry={() => void load()} />
{#if resource.value && !resource.loading && !resource.error}
  {@const data = resource.value}
  <div class="page-heading"><div><p class="eyebrow">RECEIPT RECORD</p><h1>입고 상세</h1><p>수정하거나 삭제하지 않는 입고 원인 기록입니다.</p></div><Badge value="manual_receipt" /></div>
  <section class="panel form-panel"><div class="panel-header"><h2>{data.item.name}</h2><span class="mono">{data.item.code}</span></div><div class="panel-body"><dl class="detail-list"><dt>입고 ID</dt><dd class="mono">{data.receipt.id}</dd><dt>부품</dt><dd><a href={`/items/${data.item.id}`}>{data.item.name}</a></dd><dt>입고 수량</dt><dd class="text-positive"><strong>+{formatQuantity(data.receipt.quantity)} {data.item.unit}</strong></dd><dt>입고 일시</dt><dd>{seoulTime(data.receipt.created_at)}</dd><dt>메모</dt><dd class="note-text">{data.receipt.note || '등록된 메모가 없습니다.'}</dd></dl><div class="form-actions"><a class="button" href={`/movements?item_id=${data.item.id}`}>이 부품의 재고 이력</a><a class="button secondary" href="/inventory">현재고 확인</a></div></div></section>
{/if}
