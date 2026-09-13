<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { Resource, Command, request, can, label, name, quantity, short, publicActor } from '$lib/v2/client.svelte';
  import type { Row, References, Role } from '$lib/v2/client.svelte';
  import Feedback from '$lib/v2/Feedback.svelte';

  import Icon from '$lib/components/Icon.svelte';
  import MetricCard from '$lib/components/MetricCard.svelte';

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

  let view = $state<'list' | 'create' | 'detail'>('list');
  let search = $state('');
  async function show(next: typeof view) { view = next; await tick(); document.getElementById('order-view-title')?.focus(); }
  function productChanged() { createForm.bom_revision_id = ''; createForm.inspection_revision_id = ''; }
  let orders = $state<Row[]>([]);
  let references = $state<References | null>(null);
  let order = $state<Row | null>(null);
  let resource = new Resource<Row[]>();
  let refs = new Resource<References>();
  let detail = new Resource<Row>();
  let command = new Command();
  let filter = $state('');
  let selected = $state('');
  let createForm = $state<Record<string, string>>({ finished_item_id: '', bom_revision_id: '', inspection_revision_id: '' });
  let actionForm = $state<Record<string, string>>({ reason: '', quantity: '' });
  let acknowledgeVariances = $state(false);
  let selectedAction = $state<ActionName | ''>('');

  onMount(() => { void load(); void loadRefs(); });
  async function load() { await resource.load(() => request<Row[]>('/orders')); orders = resource.value ?? []; }
  async function loadRefs() { await refs.load(() => request<References>('/reference')); references = refs.value; }
  async function select(id: string) {
    void show('detail');
    selected = id;
    order = null;
    await detail.load(() => request<Row>(`/orders/${id}`));
    order = detail.value;
    actionForm = { reason: '', quantity: '' };
    acknowledgeVariances = false;
    selectedAction = '';
  }
  function revisionMatches(row: Row) { return !!createForm.finished_item_id && String(row.finished_item_id) === createForm.finished_item_id; }
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
      createForm = { finished_item_id: '', bom_revision_id: '', inspection_revision_id: '' };
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

<svelte:head><title>작업 지시 · 조립 제조 ERP</title></svelte:head>
<div class="page-heading"><div><p class="eyebrow">PRODUCTION CONTROL</p><h1 id="order-view-title" tabindex="-1">{view === 'create' ? '작업 지시 만들기' : view === 'detail' ? '작업 지시 상세' : '작업 지시'}</h1><p>{view === 'create' ? '완제품을 선택하고 생산 목표와 예정일을 입력하세요.' : '자재 준비부터 검사와 입고까지, 생산 진행 상황을 확인하세요.'}</p></div>
{#if view === 'list'}<button class="button" onclick={() => show('create')}><Icon name="plus" size={17} />작업 지시 만들기</button>{:else}<button class="button secondary" disabled={command.busy || command.locked} onclick={() => show('list')}>목록으로</button>{/if}</div>
{#if view === 'list'}
<div class="metric-grid" aria-label="작업 지시 요약">
{#each [{label:'전체 지시', count: orders.length, note:'등록된 생산 계획'}, {label:'작업 대기',count:orders.filter(r => r.status === 'issued').length,note:'발행 후 착수 대기'}, {label:'작업 중',count:orders.filter(r => r.status === 'in_progress').length,note:'현장에서 생산 진행'}, {label:'보류',count:orders.filter(r => r.status === 'held').length,note:'후속 처리 확인 필요'}] as metric}
<MetricCard label={metric.label} value={resource.error ? null : metric.count} loading={resource.loading} icon="production" note={metric.note} />
{/each}</div>
{/if}
{#if view === 'create'}
  <section class="card">
    <h2>작업 지시 초안</h2>
    {#if !can('planner', 'admin')}
      <p class="notice notice-info">초안 생성은 생산관리 또는 관리자 역할이 필요합니다. 이후 발행·보류·마감 명령은 현재 서버 계약상 생산관리 역할이 필요합니다.</p>
    {:else if references}
      <form onsubmit={create}><div class="form-grid">
        <div class="field"><label for="v2-order-item">완제품</label><select id="v2-order-item" bind:value={createForm.finished_item_id} onchange={productChanged} required><option value="" disabled>완제품을 선택하세요</option>{#each references.items.filter(item => item.kind === 'finished' && item.active !== false) as item}<option value={String(item.id)}>{item.name ?? item.code}</option>{/each}</select></div>
        <div class="field"><label for="v2-order-target">합격 입고 목표</label><input id="v2-order-target" type="number" min="1" max="1000000" step="1" bind:value={createForm.target_quantity} required /></div>
        <div class="field"><label for="v2-order-bom">승인 BOM</label><select id="v2-order-bom" bind:value={createForm.bom_revision_id} disabled={!createForm.finished_item_id} required><option value="" disabled>{!createForm.finished_item_id ? '완제품을 먼저 선택하세요' : '승인된 BOM을 선택하세요'}</option>{#each references.bom_revisions.filter(row => row.status === 'approved' && revisionMatches(row)) as row}<option value={String(row.id)}>{row.revision}</option>{/each}</select></div>
        <div class="field"><label for="v2-order-inspection">검사 기준</label><select id="v2-order-inspection" bind:value={createForm.inspection_revision_id} disabled={!createForm.finished_item_id} required><option value="" disabled>{!createForm.finished_item_id ? '완제품을 먼저 선택하세요' : '검사 기준을 선택하세요'}</option>{#each references.inspection_revisions.filter(row => row.status === 'approved' && revisionMatches(row)) as row}<option value={String(row.id)}>{row.revision}</option>{/each}</select></div>
        <div class="field"><label for="v2-order-date">예정일</label><input id="v2-order-date" type="date" bind:value={createForm.planned_date} required /></div>
      </div><div class="actions"><button class="button" disabled={command.busy || command.locked}>초안 저장</button></div></form>
    {:else if refs.error}<p class="notice notice-error" role="alert">{refs.error}</p><button class="button secondary" onclick={loadRefs}>다시 불러오기</button>{:else}<p role="status">기준정보를 불러오는 중입니다.</p>{/if}
    {#if references && createForm.finished_item_id && (!references.bom_revisions.some(r => r.status === 'approved' && revisionMatches(r)) || !references.inspection_revisions.some(r => r.status === 'approved' && revisionMatches(r)))}<p class="notice notice-info">선택한 완제품의 승인된 BOM과 검사 기준이 필요합니다. <a href="/v2/reference">기준정보 등록하기</a></p>{/if}
    <Feedback {command} />
  </section>
{/if}
{#if view === 'list'}
  <section class="card order-list">
    <div class="panel-header"><h2 class="panel-label"><Icon name="production" size={18} />작업 지시 목록</h2><span class="panel-count">{orders.length}건</span></div>
    <div class="toolbar"><div class="field search"><label for="order-search">완제품 검색</label><input id="order-search" bind:value={search} placeholder="품목 코드 또는 이름" /></div>
    <div class="field"><label for="v2-order-filter">상태</label><select id="v2-order-filter" bind:value={filter}><option value="">전체</option><option value="draft">초안</option><option value="issued">발행</option><option value="in_progress">작업 중</option><option value="held">보류</option><option value="closed">마감</option><option value="early_closed">조기종결</option><option value="cancelled">취소</option></select></div>
    </div>
    {#if resource.loading}<p>불러오는 중…</p>{:else if resource.error}<p class="notice notice-error">{resource.error}</p>{:else}<div class="table-scroll"><table><thead><tr><th>완제품</th><th>목표</th><th>착수 허용</th><th>상태</th><th>예정일</th></tr></thead><tbody>{#each orders.filter(row => (!filter || row.status === filter) && name(references?.items, row.finished_item_id).toLowerCase().includes(search.toLowerCase())) as row}<tr><td><button class="record-link" onclick={() => void select(String(row.id))}>{name(references?.items, row.finished_item_id)}</button><span class="subtext">지시 {short(row.id)}</span></td><td class="number">{quantity(row.target_quantity)}</td><td class="number">{quantity(row.start_allowance)}</td><td><span class="status" class:held={row.status === 'held'}>{label(row.status)}</span></td><td>{String(row.planned_date ?? '—')}</td></tr>{:else}<tr><td colspan="5" class="empty">조건에 맞는 작업 지시가 없습니다.</td></tr>{/each}</tbody></table></div>{/if}
  </section>
{/if}

{#if view === 'detail' && detail.loading}<p role="status">작업 지시를 불러오는 중입니다…</p>{:else if view === 'detail' && detail.error}<p class="notice notice-error" role="alert">{detail.error}</p><button class="button secondary" onclick={() => select(selected)}>다시 불러오기</button>{/if}
{#if view === 'detail' && order && !detail.loading && !detail.error}<section class="card">
  <div class="heading"><div><p class="eyebrow">지시 상세</p><h2>{name(references?.items, order.finished_item_id)}</h2><p>목표 {quantity(order.target_quantity)} · 시작 허용 {quantity(order.start_allowance)} · 예정일 {String(order.planned_date)}</p></div><span class="status" class:held={order.status === 'held'}>{label(order.status)}</span></div>
  <div class="notice notice-info"><strong>다음 작업</strong><p>{order.status === 'draft' ? '지시를 발행하면 자재 불출과 현장 작업을 시작할 수 있습니다.' : order.status === 'held' ? '보류 사유를 확인하고 작업을 재개하세요.' : ['closed','early_closed','cancelled'].includes(String(order.status)) ? '종료된 지시입니다. 재고와 전표 이력을 확인하세요.' : Number(order.pending_quantity) > 0 ? '검사를 기다리는 산출물이 있습니다. 품질 전표에서 검사를 진행하세요.' : Number(order.accepted_quantity) > 0 ? '합격한 수량을 완제품 창고에 입고하세요.' : '자재 불출과 현장 작업을 확인한 뒤 생산 실적을 기록하세요.'}</p><div class="actions"><a class="button secondary" href="/v2/documents">자재 · 생산 · 품질</a><a class="button secondary" href="/v2/sessions">현장 작업</a><a class="button quiet" href="/v2/inventory">재고 · 추적</a></div></div>
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
