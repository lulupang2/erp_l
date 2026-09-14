<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { workHref } from '$lib/v2/workflow';
  import OrderFlow from '$lib/v2/OrderFlow.svelte';
  import { Resource, Command, request, can, session, label, name, time, short } from '$lib/v2/client.svelte';
  import type { Row, References } from '$lib/v2/client.svelte';
  import Feedback from '$lib/v2/Feedback.svelte';

  let orders = $state<Row[]>([]);
  let sessions = $state<Row[]>([]);
  let resource = new Resource<Row[]>();
  let sessionResource = new Resource<Row[]>();
  let command = new Command();
  let selectedOrder = $state('');
  let sessionFilter = $state('');
  let refs = new Resource<References>();
  const selected = $derived(orders.find(row => String(row.id) === selectedOrder));

  onMount(() => { selectedOrder = page.url.searchParams.get('order') ?? ''; sessionFilter = selectedOrder; if (can('admin', 'planner', 'operator')) void load(); });
  async function load() {
    await resource.load(() => request<Row[]>('/orders'));
    orders = resource.value ?? [];
    await sessionResource.load(() => request<Row[]>('/work-sessions'));
    sessions = sessionResource.value ?? [];
    await refs.load(() => request<References>('/reference'));
  }
  function startableOrders() {
    return orders.filter(row => ['issued', 'in_progress'].includes(String(row.status)) && (can('admin') || String(row.assigned_user_id) === session.user?.id));
  }
  function visibleSessions() {
    return sessionFilter ? sessions.filter(row => String(row.order_id) === sessionFilter) : sessions;
  }
  async function start(event: SubmitEvent) {
    event.preventDefault();
    const saved = await command.run('/work-sessions', { order_id: selectedOrder });
    if (saved) { command.success = '작업을 시작했습니다. 아래 진행 중인 작업에서 생산 수량을 보고하세요.'; sessionFilter = selectedOrder; await load(); }
  }
  async function end(id: string) {
    const saved = await command.run(`/work-sessions/${id}/end`, {});
    if (saved) { command.success = '작업을 종료했습니다. 검사와 자재 반납은 계속 진행할 수 있습니다.'; await load(); }
  }
</script>

<svelte:head><title>현장 작업 · 조립 제조 ERP</title></svelte:head>
<div class="heading"><div><p class="eyebrow">SHOP FLOOR</p><h1>현장 작업</h1><p>작업 시작 → 생산 수량 보고 → 작업 종료. 작업 중 생산 수량을 여러 번 보고할 수 있습니다.</p></div><a class="button secondary" href="/v2/work">업무 대기 목록</a></div>
{#if selected}<OrderFlow order={selected} references={refs.value} compact />{/if}
{#if resource.error || refs.error}<p class="notice notice-error" role="alert">{resource.error || refs.error}</p><button class="button secondary" onclick={load}>다시 불러오기</button>{/if}
{#if !can('admin', 'planner', 'operator')}
  <div class="notice notice-error"><strong>권한이 없습니다</strong><p>작업 세션 조회는 관리자·생산관리·작업자 역할에서 사용할 수 있습니다.</p></div>
{:else}
  <div class="split">
    <section class="card">
      <h2>작업 시작</h2>
      {#if can('operator', 'admin')}
        <form onsubmit={start}><div class="field"><label for="v2-session-order">작업 지시</label><select id="v2-session-order" bind:value={selectedOrder} onchange={() => sessionFilter = selectedOrder} required disabled={command.busy || command.locked}><option value="" disabled>시작할 작업 지시를 선택하세요</option>{#each startableOrders() as row}<option value={String(row.id)}>{name(refs.value?.items, row.finished_item_id)} · {short(row.id)} · {label(row.status)}</option>{/each}</select><span class="subtext">제품과 자재 준비 상태를 확인한 뒤 작업을 시작하세요.</span></div><div class="actions"><button class="button" disabled={command.busy || command.locked || !selectedOrder || !startableOrders().some(row => String(row.id) === selectedOrder) || sessions.some(row => String(row.order_id) === selectedOrder && row.status === 'active' && row.user_id === session.user.id)}>작업 시작</button></div></form>
        {#if sessions.some(row => String(row.order_id) === selectedOrder && row.status === 'active' && row.user_id === session.user.id)}<p class="notice notice-info">이미 시작한 작업입니다. 아래에서 생산 수량을 보고하세요.</p>{/if}
      {:else}<p class="muted">생산관리자는 세션을 조회할 수 있지만 작업을 대신 시작하지 않습니다.</p>{/if}
      <Feedback {command} onRecovered={async () => { sessionFilter = selectedOrder; await load(); }} />
    </section>
    <section class="card">
      <h2>진행 중인 작업 · 작업 이력</h2>
      <p>작업 종료 전에 생산 수량을 모두 보고하세요. 작업 종료만으로 수량이 등록되거나 지시가 마감되지는 않습니다.</p>
      <div class="field"><label for="v2-session-filter">작업 지시</label><select id="v2-session-filter" bind:value={sessionFilter}><option value="">전체</option>{#each orders as row}<option value={String(row.id)}>{name(refs.value?.items, row.finished_item_id)} · {short(row.id)}</option>{/each}</select></div>
      {#if sessionResource.loading}<p>세션을 불러오는 중…</p>
      {:else if sessionResource.error}<p class="notice notice-error">{sessionResource.error}</p>
      {:else if visibleSessions().length}<div class="work-list">{#each visibleSessions() as row}
        {@const order = orders.find(order => order.id === row.order_id)}
        <article class="shop-job"><div class="heading"><div><a class="record-link" href={workHref('orders', row.order_id)}>{name(refs.value?.items, order?.finished_item_id)} · {short(row.order_id)}</a><p class="subtext">{row.user_id === session.user.id ? '내 작업' : name(undefined, row.user_id)} · {label(row.status)} · 시작 {time(row.started_at)}</p></div></div>
        {#if row.status === 'active' && row.user_id === session.user.id}<div class="actions">
          {#if order?.status === 'in_progress'}<a class="button" href={workHref('documents', row.order_id, 'production', undefined, row.id)}>생산 수량 보고</a>{#if Number(order.rework_quantity) > 0}<a class="button secondary" href={workHref('documents', row.order_id, 'rework', undefined, row.id)}>재작업 보고</a>{/if}{:else}<span class="subtext">지시 재개 후 생산을 보고할 수 있습니다.</span>{/if}
          <button class="button secondary" disabled={command.busy || command.locked} onclick={() => end(String(row.id))}>작업 종료</button>
        </div>{/if}</article>
      {/each}</div>
      {:else}<p class="empty">해당 작업 세션이 없습니다.</p>{/if}
    </section>
  </div>
{/if}
