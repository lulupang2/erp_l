<script lang="ts">
  import { untrack } from 'svelte';
  import { resultOperation } from '$lib/api';
  import { Mutation } from '$lib/state.svelte';
  import { consumption, formatQuantity, quantity, resultInput } from '$lib/quantity';
  import type { Inventory, Item, OrderDetail, ProductionResult } from '$lib/types';
  import Feedback from './Feedback.svelte';
  import MutationActions from './MutationActions.svelte';
  import Icon from './Icon.svelte';

  let { order, balances, loading = false, onsaved }: {
    order: OrderDetail; balances: Inventory[]; loading?: boolean; onsaved: (result: ProductionResult) => Promise<void>;
  } = $props();
  const mutation = new Mutation(untrack(() => resultOperation(order.id)));
  let good = $state(String(mutation.draft?.good_quantity ?? '0'));
  let defective = $state(String(mutation.draft?.defective_quantity ?? '0'));
  let note = $state(mutation.draft?.note ?? ''); let refreshing = $state(false);
  const stock = $derived(new Map(balances.map(row => [row.item_id, row.quantity])));
  const itemMap = $derived(new Map(order.materials.map(row => [row.item_id, { id: row.item_id, code: row.code, name: row.name, unit: row.unit, kind: 'component', created_at: '' } as Item])));
  const preview = $derived.by(() => {
    try { return consumption(order.materials, quantity(good, 0, '양품'), quantity(defective, 0, '불량')); }
    catch { return []; }
  });
  async function submit(event: SubmitEvent) {
    event.preventDefault(); if (mutation.busy || refreshing) return;
    try {
      // A committed-but-unconfirmed result can be replayed on a completed order.
      const body = mutation.draft ?? resultInput(good, defective, order.remaining_quantity, note);
      const saved = await mutation.submit(body);
      if (saved) {
        good = '0'; defective = '0'; note = ''; refreshing = true;
        await onsaved(saved);
      }
    } catch (error) { mutation.error = error; }
    finally { refreshing = false; }
  }
  function reset() { mutation.reset(); if (!mutation.pending) { good = '0'; defective = '0'; note = ''; } }
</script>

<section class="panel" aria-label="생산 실적 등록">
  <div class="panel-header"><div><h2 class="panel-label"><Icon name="production" size={18} />생산 실적 등록</h2><p>양품과 불량 모두 자재를 소비합니다.</p></div></div>
  <form class="panel-body" onsubmit={submit}>
    {#if order.status === 'completed'}<div class="notice notice-info">완료된 지시입니다. 새로운 실적은 등록할 수 없습니다. 저장 결과가 불확실한 기존 요청은 다시 확인할 수 있습니다.</div>{/if}
    <fieldset disabled={mutation.locked || refreshing || loading || order.status === 'completed'}><legend class="sr-only">생산 실적 수량</legend>
      <div class="form-grid"><div class="field"><label for="result-good">양품 수량<span class="required">*</span></label><input id="result-good" inputmode="numeric" maxlength="7" bind:value={good} required /></div><div class="field"><label for="result-defective">불량 수량<span class="required">*</span></label><input id="result-defective" inputmode="numeric" maxlength="7" bind:value={defective} required /></div></div>
      <div class="field"><label for="result-note">실적 메모</label><textarea id="result-note" bind:value={note} maxlength="500" placeholder="작업 내용 또는 불량 사유 (선택)"></textarea></div>
    </fieldset>
    <div class="consumption-preview" aria-label="예상 자재 소비"><h3>예상 자재 소비</h3><p>BOM 소요량 × (양품 + 불량)</p>
      {#each preview as row (row.item_id)}<div class="summary-line"><span>{row.name}<small class="subtext">현재 {stock.has(row.item_id) ? formatQuantity(stock.get(row.item_id)!) : '확인 필요'} {row.unit}</small></span><strong class:text-negative={stock.has(row.item_id) && stock.get(row.item_id)! < row.required_quantity}>{formatQuantity(row.required_quantity)} {row.unit}</strong></div>{/each}
      {#if !preview.length}<p>정수 수량을 입력하면 소비량이 표시됩니다.</p>{/if}
      <p>표시 재고는 참고용이며, 최종 부족 여부는 서버가 잠금 후 판단합니다.</p>
    </div>
    <Feedback error={mutation.error} items={itemMap} />
    <MutationActions busy={mutation.busy || refreshing || loading} pending={mutation.pending} disabled={order.status === 'completed'} label="실적 등록" onreset={reset} />
  </form>
</section>
