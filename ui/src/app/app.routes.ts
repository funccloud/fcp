import { Routes } from '@angular/router';
import { NotFound } from './shared/not-found/not-found';
import { ApplicationModule } from '@angular/core';
import { Applications } from './private/applications/applications';

export const routes: Routes = [

  {
    path: 'application',
    component: Applications,
  },
  {
    path: '**',
    component: NotFound,
  },
];
