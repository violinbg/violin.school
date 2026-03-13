import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { AuthService } from '../services/auth.service';

/** Prevents access to /setup once the app is already initialized. */
export const setupGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  const router = inject(Router);
  if (auth.isInitialized()) {
    return router.createUrlTree(['/']);
  }
  return true;
};

/** Prevents access to protected routes when not logged in. */
export const authGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  const router = inject(Router);
  if (!auth.isLoggedIn()) {
    return router.createUrlTree(['/']);
  }
  return true;
};

/** Redirects to /setup when the app has never been initialized. */
export const initializedGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  const router = inject(Router);
  if (!auth.isInitialized()) {
    return router.createUrlTree(['/setup']);
  }
  return true;
};
