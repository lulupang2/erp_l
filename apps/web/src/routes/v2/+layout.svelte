<script lang="ts">
  import type { Snippet } from 'svelte';
  import '$lib/v2/factory.css';
  let { children }: { children: Snippet } = $props();
</script>
<div class="factory">{@render children()}</div>
