import { definePreset } from '@primeuix/themes';
import Aura from '@primeuix/themes/aura';

export const sohoSurface = {
  0: '#ffffff',
  50: '#ececec',
  100: '#dedfdf',
  200: '#c4c4c6',
  300: '#adaeb0',
  400: '#97979b',
  500: '#7f8084',
  600: '#6a6b70',
  700: '#55565b',
  800: '#3f4046',
  900: '#2c2c34',
  950: '#16161d'
};

export const ViolinPreset = definePreset(Aura, {
  semantic: {
    primary: {
      50: '{surface.50}',
      100: '{surface.100}',
      200: '{surface.200}',
      300: '{surface.300}',
      400: '{surface.400}',
      500: '{surface.500}',
      600: '{surface.600}',
      700: '{surface.700}',
      800: '{surface.800}',
      900: '{surface.900}',
      950: '{surface.950}'
    },
    colorScheme: {
      light: {
        primary: {
          color: '{primary.950}',
          contrastColor: '#ffffff',
          hoverColor: '{primary.800}',
          activeColor: '{primary.700}'
        },
        highlight: {
          background: '{primary.950}',
          focusBackground: '{primary.700}',
          color: '#ffffff',
          focusColor: '#ffffff'
        }
      },
      dark: {
        primary: {
          color: '{primary.50}',
          contrastColor: '{primary.950}',
          hoverColor: '{primary.200}',
          activeColor: '{primary.300}'
        },
        highlight: {
          background: '{primary.50}',
          focusBackground: '{primary.300}',
          color: '{primary.950}',
          focusColor: '{primary.950}'
        }
      }
    }
  }
});
