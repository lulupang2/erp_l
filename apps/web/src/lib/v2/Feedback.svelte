<script lang="ts">
  import type { Command } from './client.svelte';
  import { t } from '$lib/i18n.svelte';
  let { command }: { command: Command } = $props();
</script>
{#if command.error}<div class="notice notice-error" role="alert"><strong>{command.uncertain ? t('commandNeedsCheck') : t('commandFailed')}</strong><p>{command.error}</p>{#if command.uncertain}<p>{t('commandUncertainGuide')}</p><button class="button secondary" type="button" disabled={command.busy} onclick={() => void command.retry()}>{t('retrySameRequest')}</button>{/if}</div>{/if}
{#if command.success}<p class="notice notice-info" role="status">{command.success}</p>{/if}
