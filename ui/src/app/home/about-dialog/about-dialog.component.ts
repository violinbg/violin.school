import { Component, EventEmitter, Output } from '@angular/core';
import { DialogModule } from 'primeng/dialog';
import { ButtonModule } from 'primeng/button';
import { TranslatePipe } from '@ngx-translate/core';

@Component({
  selector: 'vs-about-dialog',
  standalone: true,
  imports: [DialogModule, ButtonModule, TranslatePipe],
  templateUrl: './about-dialog.component.html',
  styleUrl: './about-dialog.component.scss'
})
export class AboutDialogComponent {
  @Output() closed = new EventEmitter<void>();

  visible = true;

  close(): void {
    this.visible = false;
    this.closed.emit();
  }
}
