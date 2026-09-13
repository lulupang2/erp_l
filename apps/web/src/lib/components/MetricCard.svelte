<script lang="ts">
  import { localeTag, t } from '$lib/i18n.svelte';
  import Icon, { type IconName } from './Icon.svelte';

  let { label, value, note, icon, tone = 'orange', loading = false, testId }: {
    label: string; value: number | null; note: string; icon: IconName;
    tone?: 'orange' | 'blue' | 'green' | 'red'; loading?: boolean; testId?: string;
  } = $props();
</script>

<article class={`metric-card tone-${tone}`} aria-label={t('metricSummary', { label })} aria-busy={loading}>
  <div class="metric-heading"><span>{label}</span><span class="metric-icon"><Icon name={icon} size={20} /></span></div>
  <div class="metric-value"><strong data-testid={testId}>{loading ? '…' : value === null ? t('dash') : value.toLocaleString(localeTag())}</strong><span>{t('countUnit')}</span></div>
  <p class="metric-note">{loading ? t('metricLoading') : value === null ? t('metricUnavailable') : note}</p>
</article>
