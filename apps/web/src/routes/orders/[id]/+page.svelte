<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { Resource } from '$lib/state.svelte';
  import { formatQuantity, seoulTime } from '$lib/quantity';
  import type { Inventory, Item, OrderDetail, Page, ProductionResult } from '$lib/types';
  import Badge from '$lib/components/Badge.svelte';
  import LoadState from '$lib/components/LoadState.svelte';
  import Pagination from '$lib/components/Pagination.svelte';
  import ResultForm from '$lib/components/ResultForm.svelte';
  import Icon from '$lib/components/Icon.svelte';

  const resource = new Resource<{ order: OrderDetail; item: Item }>();
  const balances = new Resource<Inventory[]>(); const results = new Resource<Page<ProductionResult>>();
  let pageNo = $state(1); let pageSize = $state(20); let saved = $state<ProductionResult | null>(null);
  function loadResults(next = pageNo, size = pageSize) { pageNo = next; pageSize = size; return results.load(() => api.results(page.params.id!, { page: pageNo, page_size: pageSize })); }
  async function load() {
    await resource.load(async () => { const order = await api.order(page.params.id!); return { order, item: await api.item(order.finished_item_id) }; });
    if (!resource.error) await Promise.all([balances.load(() => api.allInventory()), loadResults()]);
  }
  async function onSaved(result: ProductionResult) { saved = result; pageNo = 1; await load(); }
  onMount(() => { void load(); });
</script>

<svelte:head><title>생산 지시 상세 · 조립 제조 ERP</title></svelte:head>
<a class="back-link" href="/orders">← 생산 지시 목록</a>
<LoadState loading={resource.loading} error={resource.error} retry={() => void load()} />
{#if saved}<div class="notice notice-success" role="status">실적이 저장되었습니다. <a href={`/results/${saved.id}`}>실적 상세 보기 ↗</a>{#if resource.error} 최신 집계 조회에 실패했습니다. 저장 요청을 반복하지 말고 다시 조회해 주세요.{/if}</div>{/if}
{#if resource.value && !resource.error}
  {@const data = resource.value}
  <div class="page-heading"><div><p class="eyebrow">PRODUCTION ORDER</p><h1>생산 지시 상세</h1><p>{data.item.name} · <span class="mono">{data.item.code}</span></p></div><div class="inline-actions"><Badge value={data.order.status} /><button class="button secondary" disabled={resource.loading} onclick={() => void load()}>새로고침</button></div></div>
  <div class="stat-grid" aria-label="생산 집계"><div class="stat"><span>계획 수량</span><strong data-testid="planned-quantity">{formatQuantity(data.order.planned_quantity)}</strong><small>{data.item.unit}</small></div><div class="stat"><span>양품 누계</span><strong class="text-good" data-testid="good-quantity">{formatQuantity(data.order.good_quantity)}</strong><small>{data.item.unit}</small></div><div class="stat"><span>불량 누계</span><strong class="text-defect" data-testid="defective-quantity">{formatQuantity(data.order.defective_quantity)}</strong><small>{data.item.unit}</small></div><div class="stat remaining"><span>잔여 수량</span><strong data-testid="remaining-quantity">{formatQuantity(data.order.remaining_quantity)}</strong><small>{data.item.unit}</small></div></div>
  <div class="progress-panel" aria-label="생산 진행 현황"><div class="progress-label"><span class="panel-label"><Icon name="production" size={16} />처리 진행률 · 양품 + 불량</span><strong>{Math.round((data.order.planned_quantity - data.order.remaining_quantity) / data.order.planned_quantity * 100)}%</strong></div><div class="progress-track" role="progressbar" aria-label="계획 대비 처리 진행률" aria-valuemin="0" aria-valuemax="100" aria-valuenow={Math.round((data.order.planned_quantity - data.order.remaining_quantity) / data.order.planned_quantity * 100)}><span style:width={`${(data.order.planned_quantity - data.order.remaining_quantity) / data.order.planned_quantity * 100}%`}></span></div></div>
  <div class="split"><div class="stack">
    <section class="panel"><div class="panel-header"><div><h2>생산 기준 · BOM 복사본</h2><p>지시 생성 시 확정된 완제품 1개 기준입니다.</p></div></div><div class="table-scroll"><table><caption class="sr-only">생산 지시 BOM 복사본</caption><thead><tr><th scope="col">부품</th><th scope="col" class="numeric">단위 소요량</th><th scope="col">재고 이력</th></tr></thead><tbody>{#each data.order.materials as material (material.item_id)}<tr><td><a class="item-name" href={`/items/${material.item_id}`}>{material.name}</a><span class="subtext mono">{material.code}</span></td><td class="numeric">{formatQuantity(material.quantity_per_unit)} {material.unit}</td><td><a href={`/movements?item_id=${material.item_id}`}>이력 보기 ↗</a></td></tr>{/each}</tbody></table></div><div class="panel-body"><p class="help">지시 ID <span class="mono id-text">{data.order.id}</span><br />생성 {seoulTime(data.order.created_at)} · 재고 예약 없음</p></div></section>
    <section class="panel" aria-label="생산 실적 이력"><div class="panel-header"><div><h2>생산 실적 이력</h2><p>저장한 실적은 수정하거나 삭제하지 않습니다.</p></div></div><LoadState loading={results.loading} error={results.error} empty={results.value?.data.length === 0} emptyText="아직 등록된 생산 실적이 없습니다." retry={() => void loadResults()} />
      {#if results.value && !results.loading && !results.error}{#if results.value.data.length}<div class="table-scroll"><table><caption class="sr-only">생산 실적 목록</caption><thead><tr><th scope="col">등록 일시 · 서울</th><th scope="col" class="numeric">양품</th><th scope="col" class="numeric">불량</th><th scope="col">상세</th></tr></thead><tbody>{#each results.value.data as result (result.id)}<tr><td>{seoulTime(result.created_at)}</td><td class="numeric text-good">{formatQuantity(result.good_quantity)}</td><td class="numeric text-defect">{formatQuantity(result.defective_quantity)}</td><td><a href={`/results/${result.id}`}>실적 상세 ↗</a></td></tr>{/each}</tbody></table></div>{/if}<Pagination pagination={results.value.pagination} onchange={(next,size) => void loadResults(next,size)} />{/if}
    </section>
  </div><div class="stack">
    {#if balances.error}<div class="notice notice-warning">현재고를 불러오지 못했습니다. 소비량 계산은 가능하며, 서버가 최종 재고를 확인합니다.</div>{/if}
    {#key data.order.id}<ResultForm order={data.order} balances={balances.value ?? []} loading={resource.loading} onsaved={onSaved} />{/key}
    <div class="notice notice-info"><strong>불량도 처리 수량에 포함합니다.</strong><p>양품은 완제품 재고로 입고되지만 불량은 입고하지 않습니다. 추가 생산은 <a href={`/orders/new?item_id=${data.item.id}`}>새로운 지시</a>로 시작하세요.</p></div>
  </div></div>
{/if}
