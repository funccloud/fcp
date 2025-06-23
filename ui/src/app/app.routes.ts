import { Routes } from '@angular/router';
import { NotFound } from './shared/not-found/not-found';

export const routes: Routes = [
  {
    path: '**',
    component: NotFound,
  },
];
