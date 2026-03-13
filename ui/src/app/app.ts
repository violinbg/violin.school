import { Component, OnInit } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { updateSurfacePalette } from '@primeuix/themes';
import { sohoSurface } from './theme';

@Component({
  selector: 'vs-root',
  imports: [RouterOutlet],
  templateUrl: './app.html',
  styleUrl: './app.scss'
})
export class App implements OnInit {
  ngOnInit(): void {
    updateSurfacePalette(sohoSurface);
  }
}
