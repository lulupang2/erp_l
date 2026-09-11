<script lang="ts">import { page } from '$app/state';</script>
<section class="panel error-page"><p class="eyebrow">페이지 확인</p><h1>{page.status === 404 ? '페이지를 찾을 수 없습니다.' : '화면을 불러오지 못했습니다.'}</h1><p>주소를 확인하거나 품목 관리 화면으로 돌아가 주세요.</p><a class="button" href="/items">품목 관리로 이동</a></section>
