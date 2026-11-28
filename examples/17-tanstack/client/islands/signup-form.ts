// The signup-form island: TanStack Form checks each field while typing, then
// submits through the generated typed command. The server's Validate and
// handler still decide: a refusal's field errors land on the same fields.

import { html, nothing } from "lit";
import { TanStackFormController, type AnyFieldApi } from "@tanstack/lit-form";
import { CommandError } from "templatic";
import { send, type components } from "templatic/commands";
import { SignupFormBase } from "templatic/islands/signup-form.base";

type SignupIn = components.SignupIn;

// The same shapes as SignupIn.Validate; the reserved and taken names are the server's to know.
const checks = {
	username: (v: string) => (/^[a-z0-9-]{3,20}$/.test(v) ? undefined : "3 to 20 of a-z, 0-9 and -"),
	email: (v: string) => (v.indexOf("@") > 0 && !v.endsWith("@") ? undefined : "enter an email address"),
	plan: (v: string) => (v ? undefined : "pick a plan"),
};

// error is what a field shows: the first error once the field was touched, or a server error.
function error(f: AnyFieldApi): string {
	const m = f.state.meta;
	if (!m.isTouched && !m.errorMap.onSubmit) return "";
	return m.errors.map(String)[0] ?? "";
}

customElements.define(
	"signup-form",
	class extends SignupFormBase {
		welcome = "";
		form = new TanStackFormController(this, {
			defaultValues: { username: "", email: "", plan: "" } as SignupIn,
			onSubmit: async ({ value, formApi }) => {
				this.welcome = "";
				try {
					await send("signup.register", value);
				} catch (err) {
					if (!(err instanceof CommandError) || err.status !== 400) throw err;
					// A 400 carries live.InputErrors by field name; they become submit errors,
					// which TanStack Form clears when the field next passes its change check.
					formApi.setErrorMap({ onSubmit: { form: err.formError || undefined, fields: err.fields } });
					return;
				}
				this.welcome = `Welcome, ${value.username}.`;
				formApi.reset();
			},
		});

		protected render(): unknown {
			const api = this.form.api;
			const formError = api.state.errorMap.onSubmit;
			return html`
				<form novalidate @submit=${(e: Event) => {
					e.preventDefault();
					void api.handleSubmit();
				}}>
					${formError ? html`<p class="form-error" role="alert">${String(formError)}</p>` : nothing}
					${this.form.field({ name: "username", validators: { onChange: ({ value }) => checks.username(value) } }, (f) =>
						this.input(f, "Username", "text", "username"),
					)}
					${this.form.field({ name: "email", validators: { onChange: ({ value }) => checks.email(value) } }, (f) =>
						this.input(f, "Email", "email", "email"),
					)}
					${this.form.field({ name: "plan", validators: { onChange: ({ value }) => checks.plan(value) } }, (f) => {
						const msg = error(f);
						return html`<label>
							Plan
							<select name="plan" aria-invalid=${msg ? "true" : "false"} .value=${f.state.value}
								@change=${(e: Event) => f.handleChange((e.target as HTMLSelectElement).value)} @blur=${() => f.handleBlur()}>
								<option value="">choose…</option>
								${(this.props?.plans ?? []).map((p) => html`<option value=${p} ?selected=${f.state.value === p}>${p}</option>`)}
							</select>
							<span class="field-error">${msg}</span>
						</label>`;
					})}
					<button type="submit" ?disabled=${api.state.isSubmitting}>Sign up</button>
					${this.welcome ? html`<p class="ok" role="status">${this.welcome}</p>` : nothing}
				</form>
			`;
		}

		input(f: AnyFieldApi, label: string, type: string, autocomplete: string): unknown {
			const msg = error(f);
			return html`<label>
				${label}
				<input name=${f.name} type=${type} autocomplete=${autocomplete} aria-invalid=${msg ? "true" : "false"}
					.value=${f.state.value as string}
					@input=${(e: Event) => f.handleChange((e.target as HTMLInputElement).value)} @blur=${() => f.handleBlur()} />
				<span class="field-error">${msg}</span>
			</label>`;
		}
	},
);
