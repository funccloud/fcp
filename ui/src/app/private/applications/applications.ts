import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormBuilder, FormControl, FormsModule, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatSlideToggleModule } from '@angular/material/slide-toggle';

import { merge } from 'rxjs';

@Component({
  selector: 'app-applications',
  templateUrl: './applications.html',
  styleUrl: './applications.scss',
  imports: [MatFormFieldModule, MatInputModule, FormsModule, ReactiveFormsModule, MatSlideToggleModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class Applications {
  private _formBuilder = inject(FormBuilder);

  readonly email = new FormControl('', [Validators.required, Validators.email]);
  readonly image = new FormControl('', [Validators.required]);
  readonly https = new FormControl('', [Validators.requiredTrue]);

  errorMessage = signal('');

  formGroup = this._formBuilder.group({
    email: this.email,
    image: this.image,
    https: this.https,
  });

  constructor() {
    merge(this.email.statusChanges, this.email.valueChanges,
      this.image.statusChanges, this.image.valueChanges,
      this.https.statusChanges, this.https.valueChanges)
      .pipe(takeUntilDestroyed())
      .subscribe(() => this.updateErrorMessage());
  }

  updateErrorMessage() {
    if (this.email.hasError('required')) {
      this.errorMessage.set('You must enter a value');
    } else if (this.email.hasError('email')) {
      this.errorMessage.set('Not a valid email');
    } else {
      this.errorMessage.set('');
    }
  }
}
