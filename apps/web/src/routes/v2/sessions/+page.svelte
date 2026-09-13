<script lang="ts">
  import { onMount } from 'svelte';
  import { Resource, Command, request, can, session, label, name, time, short } from '$lib/v2/client.svelte';
  import type { Row } from '$lib/v2/client.svelte';
  import Feedback from '$lib/v2/Feedback.svelte';

  let orders = $state<Row[]>([]);
  let sessions = $state<Row[]>([]);
  let resource = new Resource<Row[]>();
  let sessionResource = new Resource<Row[]>();
  let command = new Command();
  let selectedOrder = $state('');
  let sessionFilter = $state('');

  onMount(() => { if (can('admin', 'planner', 'operator')) void load(); });
  async function load() {
    await resource.load(() => request<Row[]>('/orders'));
    orders = resource.value ?? [];
    await sessionResource.load(() => request<Row[]>('/work-sessions'));
    sessions = sessionResource.value ?? [];
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
    if (saved) { command.success = '작업 세션을 시작했습니다.'; selectedOrder = ''; await load(); }
  }
  async function end(id: string) {
    const saved = await command.run(`/work-sessions/${id}/end`, {});
    if (saved) { command.success = '작업 세션을 종료했습니다.'; await load(); }
  }
</script>

<svelte:head><title>작업 세션 · 공장 v2</title></svelte:head>
<div class="heading"><div><p class="eyebrow">F2-05 · WORK SESSIONS</p><h1>작업 세션</h1><p>배정된 지시에서 조립을 시작하고 부분 산출을 보고한 뒤 본인의 작업 세션을 종료합니다.</p></div></div>
{#if !can('admin', 'planner', 'operator')}
  <div class="notice notice-error"><strong>권한이 없습니다</strong><p>작업 세션 조회는 관리자·생산관리·작업자 역할에서 사용할 수 있습니다.</p></div>
{:else}
  <div class="split">
    <section class="card">
      <h2>세션 시작</h2>
      {#if can('operator', 'admin')}
        <form onsubmit={start}><div class="field"><label for="v2-session-order">작업 지시</label><select id="v2-session-order" bind:value={selectedOrder} required><option value="" disabled>시작할 작업 지시를 선택하세요</option>{#each startableOrders() as row}<option value={String(row.id)}>{name(undefined, row.finished_item_id)} · {short(row.id)} · {label(row.status)}</option>{/each}</select><span class="subtext">작업자는 자신에게 배정된 발행/작업 중 지시만 표시합니다. 서버가 시작 시 상태와 권한을 다시 검사합니다.</span></div><div class="actions"><button class="button" disabled={command.busy || command.locked || !selectedOrder}>세션 시작</button></div></form>
      {:else}<p class="muted">생산관리자는 세션을 조회할 수 있지만 작업을 대신 시작하지 않습니다.</p>{/if}
      <Feedback {command} />
    </section>
    <section class="card">
      <h2>세션 조회</h2>
      <div class="field"><label for="v2-session-filter">지시 필터</label><select id="v2-session-filter" bind:value={sessionFilter}><option value="">전체</option>{#each orders as row}<option value={String(row.id)}>{short(row.id)} · {label(row.status)}</option>{/each}</select></div>
      {#if sessionResource.loading}<p>세션을 불러오는 중…</p>
      {:else if sessionResource.error}<p class="notice notice-error">{sessionResource.error}</p>
      {:else if visibleSessions().length}<div class="table-scroll"><table><thead><tr><th>지시</th><th>작업자</th><th>상태</th><th>시작</th><th></th></tr></thead><tbody>{#each visibleSessions() as row}<tr><td>{short(row.order_id)}</td><td>{name(undefined, row.user_id)}</td><td><span class="status">{label(row.status)}</span></td><td>{time(row.started_at)}</td><td>{#if row.status === 'active' && String(row.user_id) === session.user?.id}<button class="button secondary compact" disabled={command.busy || command.locked} onclick={() => end(String(row.id))}>내 세션 종료</button>{:else}<span class="subtext">{row.status === 'ended' ? '종료' : '소유 작업자만 종료 가능'}</span>{/if}</td></tr>{/each}</tbody></table></div>
      {:else}<p class="empty">해당 작업 세션이 없습니다.</p>{/if}
    </section>
  </div>
{/if}
