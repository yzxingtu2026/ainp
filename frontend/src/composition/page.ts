import type { PageRegistration } from './contract';

export const platformPage = (id: string, component: PageRegistration['component']): PageRegistration => ({
  id,
  owner: 'ainp',
  component
});
