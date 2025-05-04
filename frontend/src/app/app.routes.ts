import { Routes } from '@angular/router';
import { NotFoundComponent } from './public/not-found/not-found.component';
import { LandingPageComponent } from './public/landing-page/landing-page.component';

export const routes: Routes = [
    { path: '', component: LandingPageComponent},
    { path: '**', component: NotFoundComponent },
];
