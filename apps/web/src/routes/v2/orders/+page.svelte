<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { page } from '$app/state';
  import OrderFlow from '$lib/v2/OrderFlow.svelte';
  import { Resource, Command, request, can, label, name, quantity, short, publicActor } from '$lib/v2/client.svelte';
  import type { Row, References, Role } from '$lib/v2/client.svelte';
  import Feedback from '$lib/v2/Feedback.svelte';

  import Icon from '$lib/components/Icon.svelte';
  import MetricCard from '$lib/components/MetricCard.svelte';
  import { t } from '$lib/i18n.svelte';

  type ActionName = 'issue' | 'hold' | 'resume' | 'target' | 'allowance' | 'close' | 'early-close' | 'cancel';
  type Action = { action: ActionName; labelKey: Parameters<typeof t>[0]; roles: Role[] };
  const actions: Action[] = [
    { action: 'issue', labelKey: 'action_issue', roles: ['planner'] },
    { action: 'hold', labelKey: 'action_hold', roles: ['planner'] },
    { action: 'resume', labelKey: 'action_resume', roles: ['planner'] },
    { action: 'target', labelKey: 'action_target', roles: ['planner'] },
    { action: 'allowance', labelKey: 'action_allowance', roles: ['planner'] },
    { action: 'close', labelKey: 'action_close', roles: ['planner'] },
    { action: 'early-close', labelKey: 'action_early_close', roles: ['planner'] },
    { action: 'cancel', labelKey: 'action_cancel', roles: ['planner'] }
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

  onMount(() => { void load(); void loadRefs(); const id = page.url.searchParams.get('order'); if (id) void select(id); });
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
      command.success = t('orderDraftSaved');
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
      const actionLabel = actions.find(item => item.action === action)?.labelKey;
      command.success = t('actionProcessed', { action: actionLabel ? t(actionLabel) : action });
      await load();
      await select(selected);
    }
  }
</script>

<svelte:head><title>{t('orderPageTitle')} · {t('appTitle')}</title></svelte:head>
<div class="page-heading"><div><p class="eyebrow">PRODUCTION CONTROL</p><h1 id="order-view-title" tabindex="-1">{view === 'create' ? t('orderCreateTitle') : view === 'detail' ? t('orderDetailTitle') : t('orderPageTitle')}</h1><p>{view === 'create' ? t('orderCreateLead') : t('orderListLead')}</p></div>
{#if view === 'list'}<button class="button" onclick={() => show('create')}><Icon name="plus" size={17} />{t('createOrder')}</button>{:else}<button class="button secondary" disabled={command.busy || command.locked} onclick={() => show('list')}>{t('backToList')}</button>{/if}</div>
{#if view === 'list'}
<div class="metric-grid" aria-label={t('metricSummary', { label: t('orderPageTitle') })}>
{#each [{label:t('metricAllOrders'), count: orders.length, note:t('metricAllOrdersNote')}, {label:t('metricWaiting'),count:orders.filter(r => r.status === 'issued').length,note:t('metricWaitingNote')}, {label:t('metricInProgress'),count:orders.filter(r => r.status === 'in_progress').length,note:t('metricInProgressNote')}, {label:t('metricHeld'),count:orders.filter(r => r.status === 'held').length,note:t('metricHeldNote')}] as metric}
<MetricCard label={metric.label} value={resource.error ? null : metric.count} loading={resource.loading} icon="production" note={metric.note} />
{/each}</div>
{/if}
{#if view === 'create'}
  <section class="card">
    <h2>{t('orderDraft')}</h2>
    {#if !can('planner', 'admin')}
      <p class="notice notice-info">{t('plannerNeeded')}</p>
    {:else if references}
      <form onsubmit={create}><div class="form-grid">
        <div class="field"><label for="v2-order-item">{t('finishedItem')}</label><select id="v2-order-item" bind:value={createForm.finished_item_id} onchange={productChanged} required><option value="" disabled>{t('selectFinishedItem')}</option>{#each references.items.filter(item => item.kind === 'finished' && item.active !== false) as item}<option value={String(item.id)}>{item.name ?? item.code}</option>{/each}</select></div>
        <div class="field"><label for="v2-order-target">{t('targetQuantity')}</label><input id="v2-order-target" type="number" min="1" max="1000000" step="1" bind:value={createForm.target_quantity} required /></div>
        <div class="field"><label for="v2-order-bom">{t('approvedBom')}</label><select id="v2-order-bom" bind:value={createForm.bom_revision_id} disabled={!createForm.finished_item_id} required><option value="" disabled>{!createForm.finished_item_id ? t('selectFinishedFirst') : t('selectApprovedBom')}</option>{#each references.bom_revisions.filter(row => row.status === 'approved' && revisionMatches(row)) as row}<option value={String(row.id)}>{row.revision}</option>{/each}</select></div>
        <div class="field"><label for="v2-order-inspection">{t('inspectionRevision')}</label><select id="v2-order-inspection" bind:value={createForm.inspection_revision_id} disabled={!createForm.finished_item_id} required><option value="" disabled>{!createForm.finished_item_id ? t('selectFinishedFirst') : t('selectInspection')}</option>{#each references.inspection_revisions.filter(row => row.status === 'approved' && revisionMatches(row)) as row}<option value={String(row.id)}>{row.revision}</option>{/each}</select></div>
        <div class="field"><label for="v2-order-date">{t('plannedDate')}</label><input id="v2-order-date" type="date" bind:value={createForm.planned_date} required /></div>
      </div><div class="actions"><button class="button" disabled={command.busy || command.locked}>{t('saveDraft')}</button></div></form>
    {:else if refs.error}<p class="notice notice-error" role="alert">{refs.error}</p><button class="button secondary" onclick={loadRefs}>{t('reload')}</button>{:else}<p role="status">{t('loadingRefs')}</p>{/if}
    {#if references && createForm.finished_item_id && (!references.bom_revisions.some(r => r.status === 'approved' && revisionMatches(r)) || !references.inspection_revisions.some(r => r.status === 'approved' && revisionMatches(r)))}<p class="notice notice-info">{t('missingApprovedRefs')} <a href="/v2/reference">{t('registerRefs')}</a></p>{/if}
    <Feedback {command} />
  </section>
{/if}
{#if view === 'list'}
  <section class="card order-list">
    <div class="panel-header"><h2 class="panel-label"><Icon name="production" size={18} />{t('orderList')}</h2><span class="panel-count">{orders.length}{t('countUnit')}</span></div>
    <div class="toolbar"><div class="field search"><label for="order-search">{t('finishedSearch')}</label><input id="order-search" bind:value={search} placeholder={t('itemCodeOrName')} /></div>
    <div class="field"><label for="v2-order-filter">{t('status')}</label><select id="v2-order-filter" bind:value={filter}><option value="">{t('all')}</option><option value="draft">{label('draft')}</option><option value="issued">{label('issued')}</option><option value="in_progress">{label('in_progress')}</option><option value="held">{label('held')}</option><option value="closed">{label('closed')}</option><option value="early_closed">{label('early_closed')}</option><option value="cancelled">{label('cancelled')}</option></select></div>
    </div>
    {#if resource.loading}<p>{t('loading')}</p>{:else if resource.error}<p class="notice notice-error">{resource.error}</p>{:else}<div class="table-scroll"><table><thead><tr><th>{t('finishedItem')}</th><th>{t('target')}</th><th>{t('startAllowance')}</th><th>{t('status')}</th><th>{t('plannedDate')}</th></tr></thead><tbody>{#each orders.filter(row => (!filter || row.status === filter) && name(references?.items, row.finished_item_id).toLowerCase().includes(search.toLowerCase())) as row}<tr><td><button class="record-link" onclick={() => void select(String(row.id))}>{name(references?.items, row.finished_item_id)}</button><span class="subtext">{t('orderIdPrefix')} {short(row.id)}</span></td><td class="number">{quantity(row.target_quantity)}</td><td class="number">{quantity(row.start_allowance)}</td><td><span class="status" class:held={row.status === 'held'}>{label(row.status)}</span></td><td>{String(row.planned_date ?? t('dash'))}</td></tr>{:else}<tr><td colspan="5" class="empty">{t('noMatchingOrders')}</td></tr>{/each}</tbody></table></div>{/if}
  </section>
{/if}

{#if view === 'detail' && detail.loading}<p role="status">{t('loadingOrder')}</p>{:else if view === 'detail' && detail.error}<p class="notice notice-error" role="alert">{detail.error}</p><button class="button secondary" onclick={() => select(selected)}>{t('reload')}</button>{/if}
{#if view === 'detail' && order && !detail.loading && !detail.error}<section class="card">
  <div class="heading"><div><p class="eyebrow">{t('detailEyebrow')}</p><h2>{name(references?.items, order.finished_item_id)}</h2><p>{t('targetSummary', { target: quantity(order.target_quantity), allowance: quantity(order.start_allowance), date: String(order.planned_date) })}</p></div><span class="status" class:held={order.status === 'held'}>{label(order.status)}</span></div>
  <div class="notice notice-info"><strong>{t('nextWork')}</strong><p>{order.status === 'draft' ? t('nextDraft') : order.status === 'held' ? t('nextHeld') : ['closed','early_closed','cancelled'].includes(String(order.status)) ? t('nextClosed') : Number(order.pending_quantity) > 0 ? t('nextPending') : Number(order.accepted_quantity) > 0 ? t('nextAccepted') : t('nextDefault')}</p><div class="actions"><a class="button secondary" href="/v2/work">업무 대기 목록</a><a class="button quiet" href="/v2/inventory">{t('inventoryTrace')}</a></div></div>
  <OrderFlow {order} {references} />
  {#if order.status === 'held'}<div class="notice notice-info"><strong>{t('heldTitle')}</strong><p>{t('heldGuide')}</p></div>{/if}
  {#if availableActions().length}
    <form onsubmit={(event) => { event.preventDefault(); void run(); }}>
      <div class="form-grid">
        <div class="field"><label for="v2-order-action">{t('action')}</label><select id="v2-order-action" bind:value={selectedAction} required><option value="" disabled>{t('selectAction')}</option>{#each availableActions() as action}<option value={action.action}>{t(action.labelKey)}</option>{/each}</select></div>
        {#if selectedAction && actionNeedsQuantity(selectedAction)}<div class="field"><label for="v2-order-action-quantity">{selectedAction === 'target' ? t('newTargetQuantity') : t('extraStartQuantity')}</label><input id="v2-order-action-quantity" type="number" min={selectedAction === 'target' ? Number(order.target_quantity) : 1} max="1000000" step="1" bind:value={actionForm.quantity} required /></div>{/if}
        {#if selectedAction && (actionNeedsReason(selectedAction) || ['hold', 'cancel'].includes(selectedAction))}<div class="field wide"><label for="v2-order-reason">{t('reason')}{actionNeedsReason(selectedAction) ? t('requiredSuffix') : t('optionalSuffix')}</label><textarea id="v2-order-reason" bind:value={actionForm.reason} required={actionNeedsReason(selectedAction)}></textarea></div>{/if}
        {#if selectedAction === 'close' && order.has_variances}<label class="check wide"><input type="checkbox" bind:checked={acknowledgeVariances} required />{t('acknowledgeVariance')}</label>{/if}
      </div>
      <div class="actions"><button class="button" disabled={!selectedAction || command.busy || command.locked}>{t('runSelectedAction')}</button></div>
    </form>
  {:else if can('planner')}<p class="muted">{t('noPlannerActions')}</p>{/if}
  <Feedback {command} />
  <div class="flow" aria-label={t('orderFlow')}><div><strong>{quantity(order.new_output_quantity)}</strong><span>{t('newOutput')}</span></div><div><strong>{quantity(order.pending_quantity)}</strong><span>{t('pendingInspection')}</span></div><div><strong>{quantity(order.accepted_quantity)}</strong><span>{t('acceptedWaiting')}</span></div><div><strong>{quantity(order.rejected_quantity)}</strong><span>{t('rejected')}</span></div><div><strong>{quantity(order.rework_quantity)}</strong><span>{t('rework')}</span></div><div><strong>{quantity(order.received_quantity)}</strong><span>{t('netReceipt')}</span></div></div>
  <p class="muted">{t('remainingSummary', { remaining: quantity(order.remaining_quantity), floor: quantity(order.floor_quantity), sessions: quantity(order.active_sessions) })}{order.has_variances ? t('bomVariance') : ''}</p>
</section>{/if}
