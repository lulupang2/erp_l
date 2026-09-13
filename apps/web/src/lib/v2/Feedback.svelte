<script lang="ts">
  import type { Command } from './client.svelte';
  let { command }: { command: Command } = $props();
</script>
{#if command.error}<div class="notice notice-error" role="alert"><strong>{command.uncertain ? '처리 결과 확인 필요' : '처리하지 못했습니다'}</strong><p>{command.error}</p>{#if command.uncertain}<p>입력은 잠겨 있으며 변경하지 않습니다. 아래 같은 요청 다시 확인 버튼은 최초 요청 키와 입력으로 재전송합니다. 새 문서를 중복 생성하지 마세요.</p><button class="button secondary" type="button" disabled={command.busy} onclick={() => void command.retry()}>같은 요청 다시 확인</button>{/if}</div>{/if}
{#if command.success}<p class="notice notice-info" role="status">{command.success}</p>{/if}
