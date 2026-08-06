import type { AdminInitializerContext } from '../../../src/composition';

export function installOrdersStore({ app }: AdminInitializerContext) {
  app.provide('example.orders.store', { selectedOrderId: null });
}
