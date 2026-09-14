<script lang="ts">
  import { can, name, quantity, short, type Row, type References } from './client.svelte';
  import { allowsWork, workHref } from './workflow';
  let { order, references, compact = false }: { order: Row; references?: References | null; compact?: boolean } = $props();
</script>

<section class="order-flow" aria-label="선택한 작업 지시">
  <div class="heading"><div><p class="eyebrow">선택한 작업 지시 · {short(order.id)}</p><h2>{name(references?.items, order.finished_item_id)}</h2><p>입고 목표 {quantity(order.target_quantity)} · 예정일 {String(order.planned_date ?? '—')}</p></div><a class="button secondary compact" href={workHref('orders', order.id)}>지시 상세</a></div>
  <dl class="flow-quantities">
    {#each [{key:'new_output_quantity',label:'생산 완료'}, {key:'pending_quantity',label:'검사 대기'}, {key:'accepted_quantity',label:'입고 대기'}, {key:'received_quantity',label:'입고 완료'}, {key:'remaining_quantity',label:'목표 잔여'}] as metric}
      <div><dt>{metric.label}</dt><dd>{quantity(order[metric.key])}</dd></div>
    {/each}
  </dl>
  {#if !compact}
    <div class="actions" aria-label="이 지시의 작업">
      {#if allowsWork(order, 'issue') && can('materials')}<a class="button secondary" href={workHref('documents', order.id, 'issue')}>자재 불출</a>{/if}
      {#if ['issued','in_progress'].includes(String(order.status)) && can('operator')}<a class="button secondary" href={workHref('sessions', order.id)}>작업 시작 · 생산 보고</a>{/if}
      {#if allowsWork(order, 'inspection') && Number(order.pending_quantity) > 0 && can('quality')}<a class="button" href={workHref('documents', order.id, 'inspection')}>검사 등록</a>{/if}
      {#if allowsWork(order, 'goods_receipt') && Number(order.accepted_quantity) > 0 && can('materials')}<a class="button" href={workHref('documents', order.id, 'goods_receipt')}>완제품 입고</a>{/if}
      {#if allowsWork(order, 'disposition') && Number(order.rejected_quantity) > 0 && can('quality')}<a class="button secondary" href={workHref('documents', order.id, 'disposition')}>불량 처리</a>{/if}
      {#if allowsWork(order, 'return') && Number(order.floor_quantity) > 0 && can('materials')}<a class="button secondary" href={workHref('documents', order.id, 'return')}>남은 자재 반납</a>{/if}
      <a class="button quiet" href={workHref('documents', order.id)}>처리 이력</a>
    </div>
  {/if}
  {#if order.status === 'held'}<p class="notice notice-info">보류 중입니다. 검사·불량 처리·자재 반납을 할 수 있습니다. 생산과 입고는 지시 재개 후 진행하세요.</p>{/if}
</section>
