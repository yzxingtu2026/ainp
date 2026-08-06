export const ordersService = {
  async list() {
    const response = await fetch('/api/v1/example/orders');
    const envelope = await response.json();
    if (!response.ok || envelope.code !== 0) throw new Error(envelope.message || 'request failed');
    return envelope.data;
  }
};
