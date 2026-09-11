<script lang="ts">
  import { formatQuantity } from '$lib/quantity';
  import Icon, { type IconName } from './Icon.svelte';

  let { label, value, note, icon, tone = 'orange', loading = false, testId }: {
    label: string; value: number | null; note: string; icon: IconName;
    tone?: 'orange' | 'blue' | 'green' | 'red'; loading?: boolean; testId?: string;
  } = $props();
</script>

<article class={`metric-card tone-${tone}`} aria-label={`${label} 요약`} aria-busy={loading}>
  <div class="metric-heading"><span>{label}</span><span class="metric-icon"><Icon name={icon} size={20} /></span></div>
  <div class="metric-value"><strong data-testid={testId}>{loading ? '…' : value === null ? '—' : formatQuantity(value)}</strong><span>건</span></div>
  <p class="metric-note">{loading ? '서버에서 확인 중' : value === null ? '현재 확인할 수 없습니다' : note}</p>
</article>
