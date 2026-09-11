<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { Resource } from '$lib/state.svelte';
  import { formatQuantity, seoulTime } from '$lib/quantity';
  import type { Item, OrderDetail, ProductionResult } from '$lib/types';
  import LoadState from '$lib/components/LoadState.svelte';

  const resource = new Resource<{ result: ProductionResult; order: OrderDetail; item: Item }>();
  function load() { return resource.load(async () => {
    const result = await api.result(page.params.id!); const order = await api.order(result.order_id);
    return { result, order, item: await api.item(order.finished_item_id) };
  }); }
  onMount(() => { void load(); });
</script>

<svelte:head><title>생산 실적 상세 · 조립 제조 ERP</title></svelte:head>
<a class="back-link" href="/orders">← 생산 지시 목록</a>
<LoadState loading={resource.loading} error={resource.error} retry={() => void load()} />
{#if resource.value && !resource.loading && !resource.error}
  {@const data = resource.value}
  <div class="page-heading"><div><p class="eyebrow">PRODUCTION RESULT</p><h1>생산 실적 상세</h1><p>수정·삭제 없이 보존하는 실적과 실제 자재 소비 기준입니다.</p></div><a href={`/orders/${data.order.id}`} class="button secondary">생산 지시 보기</a></div>
  <div class="stat-grid"><div class="stat"><span>처리 수량</span><strong>{formatQuantity(data.result.good_quantity + data.result.defective_quantity)}</strong><small>{data.item.unit}</small></div><div class="stat"><span>양품</span><strong class="text-good">{formatQuantity(data.result.good_quantity)}</strong><small>{data.item.unit}</small></div><div class="stat"><span>불량</span><strong class="text-defect">{formatQuantity(data.result.defective_quantity)}</strong><small>{data.item.unit}</small></div><div class="stat remaining"><span>완제품 입고</span><strong>{formatQuantity(data.result.good_quantity)}</strong><small>{data.item.unit}</small></div></div>
  <section class="panel"><div class="panel-header"><h2>{data.item.name}</h2><span class="mono">{data.item.code}</span></div><div class="panel-body"><dl class="detail-list"><dt>실적 ID</dt><dd class="mono">{data.result.id}</dd><dt>생산 지시</dt><dd><a class="mono" href={`/orders/${data.order.id}`}>{data.order.id}</a></dd><dt>등록 일시</dt><dd>{seoulTime(data.result.created_at)}</dd><dt>메모</dt><dd class="note-text">{data.result.note || '등록된 메모가 없습니다.'}</dd></dl></div></section>
  <section class="panel"><div class="panel-header"><div><h2>이번 실적의 자재 소비</h2><p>지시의 BOM 복사본 × 이번 양품·불량 합계</p></div></div><div class="table-scroll"><table><caption class="sr-only">실적별 자재 소비</caption><thead><tr><th scope="col">부품</th><th scope="col" class="numeric">단위 소요량</th><th scope="col" class="numeric">소비 수량</th><th scope="col">이력</th></tr></thead><tbody>{#each data.order.materials as material (material.item_id)}<tr><td>{material.name}<span class="subtext mono">{material.code}</span></td><td class="numeric">{formatQuantity(material.quantity_per_unit)}</td><td class="numeric text-negative">−{formatQuantity(material.quantity_per_unit * (data.result.good_quantity + data.result.defective_quantity))} {material.unit}</td><td><a href={`/movements?item_id=${material.item_id}`}>재고 이력 ↗</a></td></tr>{/each}</tbody></table></div></section>
{/if}
