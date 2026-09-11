<script lang="ts">
  import type { ItemKind, OrderStatus, MovementType } from '$lib/types';
  let { value }: { value: ItemKind | OrderStatus | MovementType } = $props();
  const labels = {
    component: '부품', finished_good: '완제품', pending: '대기', in_progress: '진행 중', completed: '완료',
    manual_receipt: '부품 입고', production_consumption: '자재 소비', production_receipt: '생산 입고'
  };
</script>

<span class={`badge ${value}`}><span class="badge-dot" aria-hidden="true"></span>{labels[value] ?? value}</span>
