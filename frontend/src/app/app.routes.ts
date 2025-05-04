import { Routes } from '@angular/router';
import { NotFoundComponent } from './public/not-found/not-found.component';
import { LandingPageComponent } from './public/landing-page/landing-page.component';
import { SignUpComponent } from './public/sign-up/sign-up/sign-up.component';
import { SignInComponent } from './public/auth/sign-in/sign-in.component';

export const routes: Routes = [
    { path: '', component: LandingPageComponent},
    { path: 'sign-in', component: SignInComponent},
    { path: 'sign-up', component: SignUpComponent},
    { path: '**', component: NotFoundComponent },
];
