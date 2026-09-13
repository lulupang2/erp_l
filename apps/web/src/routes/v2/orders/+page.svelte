<script lang="ts">
  import { onMount } from 'svelte';
  import { Resource, Command, request, can, label, name, quantity, short, publicActor } from '$lib/v2/client.svelte';
  import type { Row, References, Role } from '$lib/v2/client.svelte';
  import Feedback from '$lib/v2/Feedback.svelte';

  type ActionName = 'issue' | 'hold' | 'resume' | 'target' | 'allowance' | 'close' | 'early-close' | 'cancel';
  type Action = { action: ActionName; label: string; roles: Role[] };
  const actions: Action[] = [
    { action: 'issue', label: '발행', roles: ['planner'] },
    { action: 'hold', label: '보류', roles: ['planner'] },
    { action: 'resume', label: '재개', roles: ['planner'] },
    { action: 'target', label: '목표 변경', roles: ['planner'] },
    { action: 'allowance', label: '추가 착수 승인', roles: ['planner'] },
    { action: 'close', label: '정상 마감', roles: ['planner'] },
    { action: 'early-close', label: '조기종결', roles: ['planner'] },
    { action: 'cancel', label: '취소', roles: ['planner'] }
  ];
  const allowedByState: Record<string, ActionName[]> = {
    draft: ['issue', 'target', 'cancel'],
    issued: ['hold', 'target', 'allowance', 'close', 'early-close', 'cancel'],
    in_progress: ['hold', 'target', 'allowance', 'close', 'early-close', 'cancel'],
    held: ['resume', 'target', 'allowance', 'early-close', 'cancel']
  };

  let orders = $state<Row[]>([]);
  let references = $state<References | null>(null);
  let order = $state<Row | null>(null);
  let resource = new Resource<Row[]>();
  let refs = new Resource<References>();
  let detail = new Resource<Row>();
  let command = new Command();
  let filter = $state('');
  let selected = $state('');
  let createForm = $state<Record<string, string>>({});
  let actionForm = $state<Record<string, string>>({ reason: '', quantity: '' });
  let acknowledgeVariances = $state(false);
  let selectedAction = $state<ActionName | ''>('');

  onMount(() => { void load(); void loadRefs(); });
  async function load() { await resource.load(() => request<Row[]>('/orders')); orders = resource.value ?? []; }
  async function loadRefs() { await refs.load(() => request<References>('/reference')); references = refs.value; }
  async function select(id: string) {
    selected = id;
    order = null;
    await detail.load(() => request<Row>(`/orders/${id}`));
    order = detail.value;
    actionForm = { reason: '', quantity: '' };
    acknowledgeVariances = false;
    selectedAction = '';
  }
  function revisionMatches(row: Row) { return !createForm.finished_item_id || String(row.finished_item_id) === createForm.finished_item_id; }
  function availableActions(): Action[] {
    if (!order) return [];
    const allowed = allowedByState[String(order.status)] ?? [];
    return actions.filter(action => allowed.includes(action.action) && can(...action.roles));
  }
  function actionNeedsReason(action: ActionName) { return ['target', 'allowance', 'early-close'].includes(action); }
  function actionNeedsQuantity(action: ActionName) { return ['target', 'allowance'].includes(action); }

  async function create(event: SubmitEvent) {
    event.preventDefault();
    const saved = await command.run<Row>('/orders', {
      finished_item_id: createForm.finished_item_id,
      target_quantity: Number(createForm.target_quantity),
      bom_revision_id: createForm.bom_revision_id,
      inspection_revision_id: createForm.inspection_revision_id,
      assigned_user_id: publicActor.id,
      planned_date: createForm.planned_date
    });
    if (saved) {
      command.success = '작업 지시 초안을 저장했습니다.';
      createForm = {};
      await load();
      if (saved.id) await select(String(saved.id));
    }
  }

  async function run() {
    if (!selected || !order || !selectedAction) return;
    const action = selectedAction;
    const body: Record<string, unknown> = {};
    if (actionNeedsReason(action)) body.reason = actionForm.reason?.trim() ?? '';
    if (actionNeedsQuantity(action)) body.quantity = Number(actionForm.quantity);
    if (action === 'close') body.acknowledge_variances = acknowledgeVariances;
    if (action === 'hold' || action === 'cancel') {
      const reason = actionForm.reason?.trim();
      if (reason) body.reason = reason;
    }
    const saved = await command.run(`/orders/${selected}/${action}`, body);
    if (saved) {
      const actionLabel = actions.find(item => item.action === action)?.label ?? action;
      command.success = `${actionLabel} 요청을 처리했습니다.`;
      await load();
      await select(selected);
    }
  }
</script>

<svelte:head><title>작업 지시 · 공장 v2</title></svelte:head>
<div class="heading"><div><p class="eyebrow">F2-03 · PRODUCTION ORDERS</p><h1>작업 지시</h1><p>발행, 보류, 재개, 목표·착수 허용량 변경, 정상·조기 마감과 취소를 현재 상태에 맞춰 처리합니다.</p></div><span class="status">{orders.length}개 지시</span></div>
<div class="split">
  <section class="card">
    <h2>작업 지시 초안</h2>
    {#if !can('planner', 'admin')}
      <p class="notice notice-info">초안 생성은 생산관리 또는 관리자 역할이 필요합니다. 이후 발행·보류·마감 명령은 현재 서버 계약상 생산관리 역할이 필요합니다.</p>
    {:else if references}
      <form onsubmit={create}><div class="form-grid">
        <div class="field"><label for="v2-order-item">완제품</label><select id="v2-order-item" bind:value={createForm.finished_item_id} required><option value="" disabled>완제품을 선택하세요</option>{#each references.items.filter(item => item.kind === 'finished' && item.active !== false) as item}<option value={String(item.id)}>{item.name ?? item.code}</option>{/each}</select></div>
        <div class="field"><label for="v2-order-target">합격 입고 목표</label><input id="v2-order-target" type="number" min="1" max="1000000" step="1" bind:value={createForm.target_quantity} required /></div>
        <div class="field"><label for="v2-order-bom">승인 BOM</label><select id="v2-order-bom" bind:value={createForm.bom_revision_id} required><option value="" disabled>승인된 BOM을 선택하세요</option>{#each references.bom_revisions.filter(row => row.status === 'approved' && revisionMatches(row)) as row}<option value={String(row.id)}>{row.revision}</option>{/each}</select></div>
        <div class="field"><label for="v2-order-inspection">검사 기준</label><select id="v2-order-inspection" bind:value={createForm.inspection_revision_id} required><option value="" disabled>검사 기준을 선택하세요</option>{#each references.inspection_revisions.filter(row => row.status === 'approved' && revisionMatches(row)) as row}<option value={String(row.id)}>{row.revision}</option>{/each}</select></div>
        <div class="field"><label for="v2-order-date">예정일</label><input id="v2-order-date" type="date" bind:value={createForm.planned_date} required /></div>
      </div><div class="actions"><button class="button" disabled={command.busy || command.locked}>초안 저장</button></div></form>
    {:else}<p class="notice notice-info">기준정보를 불러오는 중입니다.</p>{/if}
    <Feedback {command} />
  </section>
  <section class="card">
    <h2>지시 목록</h2>
    <div class="field"><label for="v2-order-filter">상태</label><select id="v2-order-filter" bind:value={filter}><option value="">전체</option><option value="draft">초안</option><option value="issued">발행</option><option value="in_progress">작업 중</option><option value="held">보류</option><option value="closed">마감</option><option value="early_closed">조기종결</option><option value="cancelled">취소</option></select></div>
    {#if resource.loading}<p>불러오는 중…</p>{:else if resource.error}<p class="notice notice-error">{resource.error}</p>{:else}<div class="table-scroll"><table><thead><tr><th>완제품</th><th>목표</th><th>착수 허용</th><th>상태</th><th>예정일</th></tr></thead><tbody>{#each orders.filter(row => !filter || row.status === filter) as row}<tr><td><button class="button text compact" onclick={() => void select(String(row.id))}>{name(references?.items, row.finished_item_id)}</button><span class="subtext">{short(row.id)}</span></td><td class="number">{quantity(row.target_quantity)}</td><td class="number">{quantity(row.start_allowance)}</td><td><span class="status" class:held={row.status === 'held'}>{label(row.status)}</span></td><td>{String(row.planned_date ?? '—')}</td></tr>{/each}</tbody></table></div>{/if}
  </section>
</div>

{#if order}<section class="card">
  <div class="heading"><div><p class="eyebrow">지시 상세</p><h2>{name(references?.items, order.finished_item_id)}</h2><p>목표 {quantity(order.target_quantity)} · 시작 허용 {quantity(order.start_allowance)} · BOM {short(order.bom_revision_id)} · 담당 {short(order.assigned_user_id)}</p></div><span class="status" class:held={order.status === 'held'}>{label(order.status)}</span></div>
  {#if order.status === 'held'}<div class="notice notice-info"><strong>보류 중</strong><p>새 자재 불출·생산·소비성 재작업·합격품 입고는 차단됩니다. 반납·검사·부적합 처분·정정·진행 세션 종료는 계속 처리할 수 있습니다.</p></div>{/if}
  {#if availableActions().length}
    <form onsubmit={(event) => { event.preventDefault(); void run(); }}>
      <div class="form-grid">
        <div class="field"><label for="v2-order-action">처리</label><select id="v2-order-action" bind:value={selectedAction} required><option value="" disabled>처리할 작업을 선택하세요</option>{#each availableActions() as action}<option value={action.action}>{action.label}</option>{/each}</select></div>
        {#if selectedAction && actionNeedsQuantity(selectedAction)}<div class="field"><label for="v2-order-action-quantity">{selectedAction === 'target' ? '새 목표 수량' : '추가 착수 수량'}</label><input id="v2-order-action-quantity" type="number" min={selectedAction === 'target' ? Number(order.target_quantity) : 1} max="1000000" step="1" bind:value={actionForm.quantity} required /></div>{/if}
        {#if selectedAction && (actionNeedsReason(selectedAction) || ['hold', 'cancel'].includes(selectedAction))}<div class="field wide"><label for="v2-order-reason">사유{actionNeedsReason(selectedAction) ? ' (필수)' : ' (선택)'}</label><textarea id="v2-order-reason" bind:value={actionForm.reason} required={actionNeedsReason(selectedAction)}></textarea></div>{/if}
        {#if selectedAction === 'close' && order.has_variances}<label class="check wide"><input type="checkbox" bind:checked={acknowledgeVariances} required />BOM 대비 실제 소비 차이를 확인했습니다</label>{/if}
      </div>
      <div class="actions"><button class="button" disabled={!selectedAction || command.busy || command.locked}>선택한 처리 실행</button></div>
    </form>
  {:else if can('planner')}<p class="muted">현재 상태에서 실행 가능한 생산관리 명령이 없습니다.</p>{/if}
  <Feedback {command} />
  <div class="flow" aria-label="지시 진행 흐름"><div><strong>{quantity(order.new_output_quantity)}</strong><span>신규 산출</span></div><div><strong>{quantity(order.pending_quantity)}</strong><span>검사 대기</span></div><div><strong>{quantity(order.accepted_quantity)}</strong><span>합격 대기</span></div><div><strong>{quantity(order.rejected_quantity)}</strong><span>부적합</span></div><div><strong>{quantity(order.rework_quantity)}</strong><span>재작업</span></div><div><strong>{quantity(order.received_quantity)}</strong><span>순입고</span></div></div>
  <p class="muted">잔여 목표 {quantity(order.remaining_quantity)} · 현장 잔여 {quantity(order.floor_quantity)} · 활성 세션 {quantity(order.active_sessions)}{order.has_variances ? ' · BOM 소비 차이 있음' : ''}</p>
</section>{/if}
