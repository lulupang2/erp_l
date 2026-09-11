<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { orderSummary } from '$lib/summaries';
  import { Resource } from '$lib/state.svelte';
  import { formatQuantity, seoulTime } from '$lib/quantity';
  import type { Item, Order, Page } from '$lib/types';
  import Badge from '$lib/components/Badge.svelte';
  import LoadState from '$lib/components/LoadState.svelte';
  import Pagination from '$lib/components/Pagination.svelte';
  import Icon from '$lib/components/Icon.svelte';
  import MetricCard from '$lib/components/MetricCard.svelte';

  const resource = new Resource<{ rows: Page<Order>; items: Map<string, Item> }>();
  const summary = new Resource<Awaited<ReturnType<typeof orderSummary>>>();
  const totals = $derived(summary.error || summary.loading ? null : summary.value);
  function loadSummary() { return summary.load(() => orderSummary()); }
  let status = $state(''); let filter = $state(''); let pageNo = $state(1); let pageSize = $state(20);
  function load(next = pageNo, size = pageSize) {
    pageNo = next; pageSize = size;
    return resource.load(async () => {
      const rows = await api.orders({ status: filter, page: pageNo, page_size: pageSize });
      return { rows, items: await api.itemMap(rows.data.map(row => row.finished_item_id)) };
    });
  }
  function search(event: SubmitEvent) { event.preventDefault(); filter = status; void load(1); }
  function selectStatus(value: string) { status = value; filter = value; void load(1); }
  onMount(() => { void load(); void loadSummary(); });
</script>

<svelte:head><title>생산 지시 · 조립 제조 ERP</title></svelte:head>
<div class="page-heading"><div><p class="eyebrow">PRODUCTION CONTROL</p><h1>생산 지시</h1><p>계획한 처리 수량과 양품·불량 실적을 연결해 관리합니다.</p></div><a href="/orders/new" class="button"><Icon name="plus" size={17} />생산 지시 생성</a></div>
<div class="metric-grid" aria-label="전체 생산 지시 요약">
  <MetricCard label="전체 지시" value={totals?.total ?? null} loading={summary.loading} icon="production" note="등록된 전체 생산 계획" testId="order-summary-total" />
  <MetricCard label="대기" value={totals?.pending ?? null} loading={summary.loading} icon="clock" tone="blue" note="아직 생산을 시작하지 않은 지시" testId="order-summary-pending" />
  <MetricCard label="진행 중" value={totals?.progress ?? null} loading={summary.loading} icon="layers" note="부분 생산이 진행된 지시" testId="order-summary-progress" />
  <MetricCard label="완료" value={totals?.completed ?? null} loading={summary.loading} icon="check" tone="green" note="양품과 불량의 처리가 완료된 지시" testId="order-summary-completed" />
</div>
<p class="summary-caption" class:summary-error={Boolean(summary.error)}>{summary.error ? '생산 지시 요약을 불러오지 못했습니다.' : '전체 생산 지시 기준 · 아래 상태 필터와 별도 집계'}{#if summary.error}<button class="summary-retry" onclick={() => void loadSummary()}>요약 다시 조회</button>{/if}</p>
<section class="panel" aria-label="생산 지시 목록">
  <div class="panel-header"><h2 class="panel-label"><Icon name="production" size={18} />생산 지시 목록</h2><span class="panel-count">계획과 실적을 한눈에</span></div>
  <div class="view-tabs" role="group" aria-label="생산 상태 빠른 필터">{#each [{ value: '', label: '전체 지시' }, { value: 'pending', label: '대기' }, { value: 'in_progress', label: '진행 중' }, { value: 'completed', label: '완료' }] as option}<button type="button" aria-pressed={filter === option.value} disabled={resource.loading} onclick={() => selectStatus(option.value)}>{option.label}</button>{/each}</div>
  <form class="toolbar" onsubmit={search}><div class="field"><label for="order-status">생산 상태</label><select id="order-status" bind:value={status}><option value="">전체 상태</option><option value="pending">대기</option><option value="in_progress">진행 중</option><option value="completed">완료</option></select></div><button class="button secondary" type="submit" disabled={resource.loading}>조회</button></form>
  <LoadState loading={resource.loading} error={resource.error} empty={resource.value?.rows.data.length === 0} emptyText="조회된 생산 지시가 없습니다. BOM을 구성한 완제품으로 새 지시를 생성해 주세요." retry={() => void load()} />
  {#if resource.value && !resource.loading && !resource.error}
    {@const data = resource.value}
    {#if data.rows.data.length}<div class="table-scroll"><table><caption class="sr-only">생산 지시 목록</caption><thead><tr><th scope="col">완제품 · 지시</th><th scope="col">상태</th><th scope="col" class="numeric">계획</th><th scope="col" class="numeric">양품</th><th scope="col" class="numeric">불량</th><th scope="col" class="numeric">잔여</th><th scope="col">생성 일시 · 서울</th></tr></thead><tbody>
      {#each data.rows.data as order (order.id)}<tr><td><a class="item-name" href={`/orders/${order.id}`}>{data.items.get(order.finished_item_id)?.name ?? order.finished_item_id}</a><span class="subtext mono">{order.id.slice(0, 8)}</span></td><td><Badge value={order.status} /></td><td class="numeric">{formatQuantity(order.planned_quantity)}</td><td class="numeric text-good">{formatQuantity(order.good_quantity)}</td><td class="numeric text-defect">{formatQuantity(order.defective_quantity)}</td><td class="numeric"><strong>{formatQuantity(order.remaining_quantity)}</strong></td><td class="text-muted">{seoulTime(order.created_at)}</td></tr>{/each}
    </tbody></table></div>{/if}
    <Pagination pagination={data.rows.pagination} onchange={(next, size) => void load(next, size)} />
  {/if}
</section>
<div class="notice notice-info"><strong>계획 = 양품 + 불량 처리 목표</strong><p>불량을 포함해 계획 수량을 모두 처리하면 완료됩니다. 재고는 지시 생성 시 예약하지 않고, 실적 등록 시 확정합니다.</p></div>
