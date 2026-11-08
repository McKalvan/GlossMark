// @gloss implements gmi:BackstagePlugin scope=file
// @gloss presents gm:View scope=file
import {
  configApiRef,
  createApiFactory,
  createComponentExtension,
  createPlugin,
  createRoutableExtension,
  fetchApiRef,
} from '@backstage/core-plugin-api';
import { GlossMarkClient, glossMarkApiRef } from './api';
import { rootRouteRef } from './routes';

export const glossMarkPlugin = createPlugin({
  id: 'glossmark',
  apis: [
    createApiFactory({
      api: glossMarkApiRef,
      deps: { configApi: configApiRef, fetchApi: fetchApiRef },
      factory: ({ configApi, fetchApi }) => new GlossMarkClient({ configApi, fetchApi }),
    }),
  ],
  routes: {
    root: rootRouteRef,
  },
});

/** The GlossMark tab of a Component or System: `<EntityLayout.Route path="/glossmark" title="GlossMark">`. */
export const EntityGlossMarkContent = glossMarkPlugin.provide(
  createComponentExtension({
    name: 'EntityGlossMarkContent',
    component: {
      lazy: () => import('./components/EntityGlossMarkContent').then(m => m.EntityGlossMarkContent),
    },
  }),
);

/** The landscape page: every boundary across repositories with its drift summary. */
export const GlossMarkLandscapePage = glossMarkPlugin.provide(
  createRoutableExtension({
    name: 'GlossMarkLandscapePage',
    component: () => import('./components/GlossMarkLandscapePage').then(m => m.GlossMarkLandscapePage),
    mountPoint: rootRouteRef,
  }),
);
