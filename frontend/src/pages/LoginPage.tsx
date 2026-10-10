import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { z } from "zod";
import { Button } from "../components/atoms/Button";
import { Input } from "../components/atoms/Input";
import { FormField } from "../components/molecules/FormField";
import { useSession } from "../features/auth/session";
import { describeError } from "../lib/api/errors";
import { applyApiError } from "../lib/forms";

const schema = z.object({
  email: z.string().trim().min(1, "Escribe tu correo.").pipe(z.email("Escribe un correo válido.")),
  password: z.string().min(1, "Escribe tu contraseña."),
});
type LoginValues = z.infer<typeof schema>;

export function LoginPage() {
  const { state, signIn } = useSession();
  const navigate = useNavigate();
  const location = useLocation();
  const [formError, setFormError] = useState<string | null>(null);
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<LoginValues>({ resolver: zodResolver(schema), defaultValues: { email: "", password: "" } });

  const from = (location.state as { from?: string } | null)?.from ?? "/inicio";
  if (state.status === "authenticated") return <Navigate to={from} replace />;

  const submit = handleSubmit(async ({ email, password }) => {
    setFormError(null);
    try {
      await signIn(email, password);
      navigate(from, { replace: true });
    } catch (error) {
      setFormError(applyApiError(error, setError, ["email", "password"]) ?? describeError(error));
    }
  });

  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-sm flex-col justify-center gap-6 px-5 py-10">
      <header>
        <h1 className="text-3xl font-bold">Inventario</h1>
        <p className="text-ink-soft">Inicia sesión para continuar.</p>
      </header>
      <form onSubmit={submit} noValidate className="flex flex-col gap-4">
        <FormField label="Correo" error={errors.email?.message}>
          <Input type="email" autoComplete="username" inputMode="email" {...register("email")} />
        </FormField>
        <FormField label="Contraseña" error={errors.password?.message}>
          <Input type="password" autoComplete="current-password" {...register("password")} />
        </FormField>
        {formError && (
          <p role="alert" className="rounded-xl bg-bad-soft px-4 py-3 text-sm font-medium text-bad">
            {formError}
          </p>
        )}
        <Button type="submit" variant="primary" block disabled={isSubmitting}>
          {isSubmitting ? "Entrando…" : "Entrar"}
        </Button>
      </form>
    </main>
  );
}
