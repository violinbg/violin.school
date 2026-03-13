import { HttpInterceptorFn, HttpRequest } from '@angular/common/http';
import { inject } from '@angular/core';
import { catchError, from, switchMap, throwError } from 'rxjs';
import { AuthService } from '../services/auth.service';

const TOKEN_KEY = 'vr_token';

/** Endpoints that should never trigger a token refresh on 401. */
function isAuthEndpoint(req: HttpRequest<unknown>): boolean {
  const noRefresh = ['/auth/login', '/auth/refresh', '/auth/register', '/auth/logout'];
  return noRefresh.some(path => req.url.includes(path));
}

export const authInterceptor: HttpInterceptorFn = (req, next) => {
  // inject() must be called here (synchronous interceptor body) — not inside
  // a deferred RxJS callback where the injection context is no longer active.
  const authService = inject(AuthService);

  if (!req.url.includes('/api/v1/')) {
    return next(req);
  }

  let token: string | null = null;
  try {
    token = localStorage.getItem(TOKEN_KEY);
  } catch {
    return next(req);
  }

  const authedReq = token
    ? req.clone({ setHeaders: { Authorization: `Bearer ${token}` } })
    : req;

  return next(authedReq).pipe(
    catchError(error => {
      if (error.status !== 401 || isAuthEndpoint(req)) {
        return throwError(() => error);
      }

      return from(authService.refreshToken()).pipe(
        switchMap(newToken => {
          const retryReq = req.clone({ setHeaders: { Authorization: `Bearer ${newToken}` } });
          return next(retryReq);
        }),
        catchError(refreshError => {
          authService.logout();
          return throwError(() => refreshError);
        })
      );
    })
  );
};
