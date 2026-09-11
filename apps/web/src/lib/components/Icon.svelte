<script module lang="ts">
  export type IconName = 'box' | 'layers' | 'inventory' | 'history' | 'production' | 'search'
    | 'sun' | 'moon' | 'panel' | 'chevron' | 'plus' | 'arrow' | 'check' | 'clock' | 'shield'
    | 'help' | 'grid' | 'refresh' | 'alert';
</script>

<script lang="ts">
  let { name, size = 20 }: { name: IconName; size?: number } = $props();
  const paths: Record<IconName, string[]> = {
    box: ['M12 3 3 7.5v9L12 21l9-4.5v-9L12 3Z', 'm3 7.5 9 4.5 9-4.5M12 12v9M7.5 5.25l9 4.5'],
    layers: ['m12 3 9 5-9 5-9-5 9-5Z', 'm3 12 9 5 9-5M3 16l9 5 9-5'],
    inventory: ['M3 21V8l9-5 9 5v13H3Z', 'M7 21V11h10v10M7 15h10M7 18h10'],
    history: ['M3 11a9 9 0 1 1 2.7 7M3 4v7h7', 'M12 7v5l3 2'],
    production: ['M3 21V9l6 3V6l6 4V3h4l2 18H3Z', 'M7 16v1m5-1v1m5-1v1'],
    search: ['M20 20l-4.5-4.5', 'M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0Z'],
    sun: ['M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0Z', 'M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5'],
    moon: ['M20.5 13A8.6 8.6 0 0 1 11 3.5 9 9 0 1 0 20.5 13Z'],
    panel: ['M4 4h16v16H4V4Zm4 0v16', 'm14 9-3 3 3 3'],
    chevron: ['m9 5 7 7-7 7'],
    plus: ['M12 5v14M5 12h14'],
    arrow: ['M5 12h14m-6-6 6 6-6 6'],
    check: ['m5 12 4 4L19 6'],
    clock: ['M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z', 'M12 7v5l3 2'],
    shield: ['m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6l8-3Z', 'm8 12 3 3 5-6'],
    help: ['M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z', 'M9 9a3 3 0 0 1 6 0c0 2-3 2-3 4m0 3h.01'],
    grid: ['M3 3h7v7H3V3Zm11 0h7v7h-7V3ZM3 14h7v7H3v-7Zm11 0h7v7h-7v-7Z'],
    refresh: ['M20 7a9 9 0 0 0-15-2L3 7m0-5v5h5M4 17a9 9 0 0 0 15 2l2-2m0 5v-5h-5'],
    alert: ['m12 3 10 18H2L12 3Z', 'M12 9v5m0 3h.01']
  };
</script>

<svg class="ui-icon" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
  {#each paths[name] as d}<path {d} />{/each}
</svg>
