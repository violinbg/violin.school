import { Routes } from '@angular/router';
import { HomeComponent } from './home/home.component';
import { SetupComponent } from './setup/setup.component';
import { DashboardComponent } from './dashboard/dashboard.component';
import { UsersComponent } from './users/users.component';
import { AiSettingsComponent } from './ai-settings/ai-settings.component';
import { setupGuard, authGuard, initializedGuard } from './core/guards/auth.guard';
import { adminGuard } from './core/guards/admin.guard';

export const routes: Routes = [
  { path: '', component: HomeComponent, canActivate: [initializedGuard] },
  { path: 'setup', component: SetupComponent, canActivate: [setupGuard] },
  { path: 'dashboard', component: DashboardComponent, canActivate: [authGuard] },
  { path: 'users', component: UsersComponent, canActivate: [adminGuard] },
  { path: 'settings/ai', component: AiSettingsComponent, canActivate: [adminGuard] },
  { path: '**', redirectTo: '' }
];
