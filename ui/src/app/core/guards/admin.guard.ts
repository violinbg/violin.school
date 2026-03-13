import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { AuthService } from '../services/auth.service';

/** Prevents access to admin routes when user is not an admin. */
export const adminGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  const router = inject(Router);
  if (!auth.isLoggedIn()) {
    return router.createUrlTree(['/']);
  }
  if (!auth.isAdmin()) {
    return router.createUrlTree(['/dashboard']);
  }
  return true;
};
