import { Injectable, REQUEST, inject } from '@angular/core';
import { isPlatformServer } from '@angular/common';
import { PLATFORM_ID } from '@angular/core';

@Injectable({ providedIn: 'root' })
export class AuthService {
  private token: string | null = null;

  constructor() {
    const platformId = inject(PLATFORM_ID);

    if (isPlatformServer(platformId)) {
      const req = inject(REQUEST) as Request;
      const access_token = req.headers.get('access_token') ?? '';
      this.token = this.extractTokenFromCookieHeader(access_token);
    } else {
      const cookieHeader = document.cookie;
      this.token = this.extractTokenFromCookieHeader(cookieHeader);
    }
  }

  private extractTokenFromCookieHeader(cookieHeader: string): string | null {
    const match = cookieHeader.match(/(?:^|;\s*)token=([^;]+)/);
    return match ? decodeURIComponent(match[1]) : null;
  }

  isLoggedIn(): boolean {
    return !!this.token;
  }

  getToken(): string | null {
    return this.token;
  }
}
