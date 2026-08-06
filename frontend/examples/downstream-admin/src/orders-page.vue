<script setup lang="ts">
import { ref } from 'vue';
import { ordersService } from './orders-service';

const result = ref('');
async function load() {
  result.value = JSON.stringify(await ordersService.list());
}
</script>

<template>
  <main><h1>Orders</h1><button type="button" @click="load">Load</button><pre>{{ result }}</pre></main>
</template>
