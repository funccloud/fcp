import {
  mergeApplicationConfig,
  ApplicationConfig,
  inject,
  REQUEST_CONTEXT,
} from '@angular/core';
import { provideServerRendering, withRoutes } from '@angular/ssr';
import { appConfig } from './app.config';
import { serverRoutes } from './app.routes.server';
import { provideFirebaseApp, initializeServerApp } from '@angular/fire/app';
import { getAuth, provideAuth } from '@angular/fire/auth';
import { environment } from '../environments/environment';

const serverConfig: ApplicationConfig = {
  providers: [
    provideServerRendering(withRoutes(serverRoutes)),
    provideFirebaseApp(() => {
      const requestContext = inject(REQUEST_CONTEXT, { optional: true }) as
        | {
            authIdToken: string;
          }
        | undefined;
      const authIdToken = requestContext?.authIdToken;
      return initializeServerApp(environment.firebase, { authIdToken });
    }),
    provideAuth(() => getAuth()),
  ],
};

export const config = mergeApplicationConfig(appConfig, serverConfig);
