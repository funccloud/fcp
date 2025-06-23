import { RenderMode, ServerRoute } from '@angular/ssr';
import { routes } from './app.routes';

const serverRoutes: ServerRoute[] = routes
  .filter((route) => route.path !== '**')
  .map((route) => ({
    path: route.path!,
    renderMode: RenderMode.Server,
    status: 200,
  }));

serverRoutes.push({
  path: '**',
  renderMode: RenderMode.Server,
  status: 404,
});

export { serverRoutes };
